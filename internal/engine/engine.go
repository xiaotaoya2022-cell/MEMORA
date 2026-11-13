// Package engine is MEMORA's brain: sample, assess, decide, act, record.
//
// The order matters. Assessment never triggers action by itself; a plan is
// only built when the system is actually under pressure, the action is
// enabled, the cooldown is over and the hourly budget is not spent.
package engine

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"memora/internal/config"
	"memora/internal/journal"
	"memora/internal/proc"
	"memora/internal/winmem"
)

// Sample is one reading: system state plus process totals.
type Sample struct {
	At        time.Time
	Snap      winmem.Snapshot
	Procs     []proc.Info
	Complete  bool // true when all processes were read via SystemProcessInformation
	WS        uint64
	PrivateWS uint64
	Private   uint64
}

// Grower is a process that grew the most inside the current window.
type Grower struct {
	PID   uint32
	Name  string
	Delta uint64
}

// Assessment is the current reading plus the trend behind it.
type Assessment struct {
	At              time.Time
	Snap            winmem.Snapshot
	AvailPct        float64
	WS              uint64
	PrivateWS       uint64
	Private         uint64
	ProcessCount    int
	ProcessComplete bool
	Residual        int64   // commit not owned by processes or the pools
	InUseSlope      float64 // MB per hour
	CommitSlope     float64
	StandbySlope    float64
	WindowMinutes   float64
	Kind            string // none | commit | workset | standby
	Severity        int    // 0 quiet, 1 watching, 2 pressure, 3 critical
	TopGrowers      []Grower
}

// Plan is what the policy decided to do, and why.
type Plan struct {
	Trim   []proc.Info
	Purge  bool
	Reason string
}

// Options control one run of the engine.
type Options struct {
	Interval  time.Duration // overrides config when non-zero
	Act       bool          // execute plans; false turns guard into observe
	Verbose   bool
	OnSample  func(Assessment)
	OnReading func(Sample)
	TargetPID uint32 // when non-zero, only this process is considered for actions
}

type cpuMark struct {
	seconds float64
	at      time.Time
}

type baselineProc struct {
	name string
	ws   uint64
}

type reclaim struct {
	at time.Time
	mb float64
}

type histPoint struct {
	at   time.Time
	snap winmem.Snapshot
}

// Engine holds the run state. It is not safe for concurrent use.
type Engine struct {
	cfg config.Config
	jr  *journal.Journal

	hist       []histPoint
	baseline   map[uint32]baselineProc
	baselineAt time.Time

	lastCPU   map[uint32]cpuMark
	idleSince map[uint32]time.Time
	lastAt    time.Time

	lastAction  time.Time
	actionTimes []time.Time
	reclaims    []reclaim
	lastPurge   time.Time
	trimmedPIDs map[uint32]time.Time
}

// New builds an engine around already-loaded settings.
func New(cfg config.Config, jr *journal.Journal) *Engine {
	return &Engine{
		cfg:         cfg,
		jr:          jr,
		lastCPU:     map[uint32]cpuMark{},
		idleSince:   map[uint32]time.Time{},
		trimmedPIDs: map[uint32]time.Time{},
		baseline:    map[uint32]baselineProc{},
	}
}

func (e *Engine) interval(opts Options) time.Duration {
	if opts.Interval > 0 {
		return opts.Interval
	}
	if e.cfg.IntervalSeconds > 0 {
		return time.Duration(e.cfg.IntervalSeconds) * time.Second
	}
	return 10 * time.Second
}

// Run samples until the context is cancelled. It never panics on a failed
// reading: a transient API error skips a tick instead of killing the daemon.
func (e *Engine) Run(ctx context.Context, opts Options) error {
	interval := e.interval(opts)
	e.record(journal.Entry{
		Kind:   "start",
		Detail: fmt.Sprintf("mode=%s interval=%s", e.cfg.Mode, interval),
	})

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.record(journal.Entry{Kind: "stop", Detail: "context cancelled"})
			return nil
		case <-ticker.C:
			if _, err := e.Step(opts); err != nil {
				e.record(journal.Entry{Kind: "error", Detail: err.Error()})
			}
		}
	}
}

// Step takes one sample, assesses it, and acts if the policy allows.
func (e *Engine) Step(opts Options) (Assessment, error) {
	s, err := e.collect(opts)
	if err != nil {
		return Assessment{}, err
	}
	if opts.OnReading != nil {
		opts.OnReading(s)
	}

	a := e.assess(s)

	if opts.Act && e.cfg.Mode == "guard" {
		candidates := s.Procs
		if opts.TargetPID != 0 {
			candidates = candidates[:0:0]
			for _, p := range s.Procs {
				if p.PID == opts.TargetPID {
					candidates = append(candidates, p)
				}
			}
		}
		if plan, ok := e.plan(a, candidates); ok {
			if after, ok := e.execute(opts, plan, a); ok {
				a = after
			}
		}
	}

	if opts.Verbose {
		fmt.Println(formatLine(a))
	}
	if opts.OnSample != nil {
		opts.OnSample(a)
	}
	return a, nil
}

func (e *Engine) collect(opts Options) (Sample, error) {
	snap, err := winmem.Collect()
	if err != nil {
		return Sample{}, err
	}
	procs, complete, _ := proc.ListAll()

	var ws, privWS, private uint64
	for _, p := range procs {
		ws += p.WorkingSet
		privWS += p.PrivateWS
		private += p.Private
	}
	s := Sample{
		At:        time.Now(),
		Snap:      snap,
		Procs:     procs,
		Complete:  complete,
		WS:        ws,
		PrivateWS: privWS,
		Private:   private,
	}

	// Roll the baseline window so per-process growth stays meaningful.
	if e.baselineAt.IsZero() ||
		s.At.Sub(e.baselineAt) > time.Duration(e.cfg.Thresholds.GrowthWindowMinutes)*time.Minute {
		e.baseline = make(map[uint32]baselineProc, len(procs))
		for _, p := range procs {
			e.baseline[p.PID] = baselineProc{name: p.Name, ws: p.WorkingSet}
		}
		e.baselineAt = s.At
	}

	e.hist = append(e.hist, histPoint{at: s.At, snap: snap})
	maxSamples := int(e.cfg.Thresholds.GrowthWindowMinutes*60/float64(e.interval(opts))) + 8
	if maxSamples < 12 {
		maxSamples = 12
	}
	if len(e.hist) > maxSamples {
		e.hist = e.hist[len(e.hist)-maxSamples:]
	}

	e.trackIdle(procs, s.At)
	e.lastAt = s.At
	return s, nil
}

func commitResidual(s winmem.Snapshot, private uint64) int64 {
	return int64(s.Commit) - int64(private) - int64(s.KernelPaged) - int64(s.KernelNonpaged)
}

// trackIdle keeps a per-process "quiet since" timestamp. CPU is measured as
// a delta against the previous sample, so a process that is merely open is
// not mistaken for one that is busy.
func (e *Engine) trackIdle(procs []proc.Info, now time.Time) {
	wall := now.Sub(e.lastAt)
	if wall <= 0 {
		wall = time.Second
	}
	nextCPU := make(map[uint32]cpuMark, len(procs))
	nextIdle := make(map[uint32]time.Time, len(e.idleSince))

	for _, p := range procs {
		prev, seen := e.lastCPU[p.PID]
		pct := 0.0
		if seen && p.CPUSeconds > prev.seconds {
			pct = (p.CPUSeconds - prev.seconds) / wall.Seconds() * 100
		}
		nextCPU[p.PID] = cpuMark{seconds: p.CPUSeconds, at: now}

		if pct < 1.0 {
			if since, ok := e.idleSince[p.PID]; ok {
				nextIdle[p.PID] = since
			} else {
				nextIdle[p.PID] = now
			}
		}
	}

	e.lastCPU = nextCPU
	e.idleSince = nextIdle
}

func (e *Engine) assess(s Sample) Assessment {
	a := Assessment{
		At:              s.At,
		Snap:            s.Snap,
		WS:              s.WS,
		PrivateWS:       s.PrivateWS,
		Private:         s.Private,
		ProcessCount:    len(s.Procs),
		ProcessComplete: s.Complete,
		Residual:        commitResidual(s.Snap, s.Private),
	}

	window := e.cfg.Thresholds.GrowthWindowMinutes
	if window <= 0 {
		window = 30
	}
	a.WindowMinutes = window

	if len(e.hist) >= 6 {
		a.InUseSlope = slopeMBPerHour(e.hist, func(x winmem.Snapshot) float64 {
			return float64(x.InUse()) / (1 << 20)
		})
		a.CommitSlope = slopeMBPerHour(e.hist, func(x winmem.Snapshot) float64 {
			return float64(x.Commit) / (1 << 20)
		})
		a.StandbySlope = slopeMBPerHour(e.hist, func(x winmem.Snapshot) float64 {
			return float64(x.Pages.Standby) / (1 << 20)
		})
	}

	a.AvailPct = s.Snap.AvailablePercent()
	horizon := window / 60
	signal := e.cfg.Thresholds.GrowthSignalMB

	switch {
	case a.CommitSlope*horizon >= signal:
		a.Kind = "commit"
	case a.InUseSlope*horizon >= signal:
		a.Kind = "workset"
	case a.Snap.HasPages && s.Snap.Pages.Standby > 2<<30 &&
		a.AvailPct < e.cfg.Thresholds.AvailableLowPercent:
		a.Kind = "standby"
	default:
		a.Kind = "none"
	}

	switch {
	case a.AvailPct < 6:
		a.Severity = 3
	case a.AvailPct < e.cfg.Thresholds.AvailableLowPercent:
		a.Severity = 2
	case a.Kind != "none":
		a.Severity = 1
	}

	for _, p := range s.Procs {
		if b, ok := e.baseline[p.PID]; ok && p.WorkingSet > b.ws+64<<20 {
			a.TopGrowers = append(a.TopGrowers, Grower{
				PID: p.PID, Name: p.Name, Delta: p.WorkingSet - b.ws,
			})
		}
	}
	sort.Slice(a.TopGrowers, func(i, j int) bool { return a.TopGrowers[i].Delta > a.TopGrowers[j].Delta })
	if len(a.TopGrowers) > 5 {
		a.TopGrowers = a.TopGrowers[:5]
	}

	return a
}

func slopeMBPerHour(hist []histPoint, value func(winmem.Snapshot) float64) float64 {
	n := float64(len(hist))
	if n < 3 {
		return 0
	}
	t0 := hist[0].at
	var sumX, sumY, sumXY, sumXX float64
	for _, h := range hist {
		x := h.at.Sub(t0).Hours()
		y := value(h.snap)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	den := n*sumXX - sumX*sumX
	if den == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / den
}

func (e *Engine) plan(a Assessment, procs []proc.Info) (Plan, bool) {
	t := e.cfg.Thresholds
	if a.AvailPct >= t.AvailableLowPercent || a.Severity < 2 {
		return Plan{}, false
	}
	if time.Since(e.lastAction) < time.Duration(t.CooldownMinutes)*time.Minute {
		return Plan{}, false
	}
	if e.actionsThisHour() >= t.MaxActionsPerHour {
		return Plan{}, false
	}
	if e.reclaimedThisHour() >= t.ReclaimBudgetMBPerHour {
		return Plan{}, false
	}

	foreground := proc.ForegroundPID()
	self := uint32(os.Getpid())
	var plan Plan

	if e.cfg.Actions.TrimIdleWorkingSets {
		candidates := make([]proc.Info, 0, len(procs))
		for _, p := range procs {
			if p.PID == foreground || p.PID == self {
				continue
			}
			if p.WorkingSet < uint64(t.TrimMinWorkingSetMB*(1<<20)) {
				continue
			}
			if e.isProtected(p.Name) {
				continue
			}
			since, ok := e.idleSince[p.PID]
			if !ok || a.At.Sub(since) < time.Duration(t.IdleMinutes)*time.Minute {
				continue
			}
			if last, ok := e.trimmedPIDs[p.PID]; ok &&
				a.At.Sub(last) < time.Duration(t.CooldownMinutes)*time.Minute {
				continue
			}
			candidates = append(candidates, p)
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].WorkingSet != candidates[j].WorkingSet {
				return candidates[i].WorkingSet > candidates[j].WorkingSet
			}
			return e.idleSince[candidates[i].PID].Before(e.idleSince[candidates[j].PID])
		})
		if len(candidates) > 3 {
			candidates = candidates[:3]
		}
		plan.Trim = candidates
		if len(plan.Trim) > 0 {
			plan.Reason = fmt.Sprintf("可用内存 %.1f%% 低于阈值 %.1f%%，回收空闲进程工作集",
				a.AvailPct, t.AvailableLowPercent)
		}
	}

	if e.cfg.Actions.PurgeStandby && a.Snap.HasPages &&
		float64(a.Snap.Pages.Standby)/(1<<20) >= t.StandbyPurgeMinMB &&
		time.Since(e.lastPurge) > 30*time.Minute {
		plan.Purge = true
		if plan.Reason == "" {
			plan.Reason = fmt.Sprintf("待机缓存 %.0f MB，可用内存 %.1f%%",
				float64(a.Snap.Pages.Standby)/(1<<20), a.AvailPct)
		}
	}

	if len(plan.Trim) == 0 && !plan.Purge {
		return Plan{}, false
	}
	return plan, true
}

func (e *Engine) execute(opts Options, plan Plan, before Assessment) (Assessment, bool) {
	b := statsOf(before)
	action := "trim_idle_working_sets"
	if plan.Purge && len(plan.Trim) > 0 {
		action = "trim_idle_working_sets+purge_standby"
	} else if plan.Purge {
		action = "purge_standby"
	}

	entry := journal.Entry{
		Kind:   "action",
		Action: action,
		Reason: plan.Reason,
		Before: &b,
	}
	for _, p := range plan.Trim {
		entry.Targets = append(entry.Targets, journal.Target{
			PID:          p.PID,
			Name:         p.Name,
			WorkingSetMB: float64(p.WorkingSet) / (1 << 20),
		})
	}

	var failures []string
	for _, p := range plan.Trim {
		if err := proc.TrimWorkingSet(p.PID); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		e.trimmedPIDs[p.PID] = time.Now()
	}
	if plan.Purge {
		if err := winmem.PurgeStandby(); err != nil {
			failures = append(failures, err.Error())
		} else {
			e.lastPurge = time.Now()
		}
	}

	e.lastAction = time.Now()
	e.actionTimes = append(e.actionTimes, e.lastAction)

	// Let the memory manager settle before measuring the result.
	time.Sleep(3 * time.Second)
	s, err := e.collect(opts)
	if err != nil {
		entry.Result = "unverified"
		entry.Detail = err.Error()
		e.record(entry)
		return Assessment{}, false
	}
	after := e.assess(s)
	af := statsOf(after)
	entry.After = &af

	reclaimed := float64(after.Snap.AvailPhys)/(1<<20) - float64(before.Snap.AvailPhys)/(1<<20)
	if reclaimed < 0 {
		reclaimed = 0
	}
	entry.ReclaimedMB = reclaimed
	e.reclaims = append(e.reclaims, reclaim{at: time.Now(), mb: reclaimed})

	if len(failures) > 0 {
		entry.Result = "partial"
		entry.Detail = strings.Join(failures, "; ")
	} else {
		entry.Result = "ok"
	}
	e.record(entry)
	return after, true
}

func (e *Engine) isProtected(name string) bool {
	for _, p := range e.cfg.Protected {
		if strings.EqualFold(p, name) {
			return true
		}
	}
	return false
}

func (e *Engine) actionsThisHour() int {
	cutoff := time.Now().Add(-time.Hour)
	kept := e.actionTimes[:0]
	for _, t := range e.actionTimes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	e.actionTimes = kept
	return len(kept)
}

func (e *Engine) reclaimedThisHour() float64 {
	cutoff := time.Now().Add(-time.Hour)
	kept := e.reclaims[:0]
	for _, r := range e.reclaims {
		if r.at.After(cutoff) {
			kept = append(kept, r)
		}
	}
	e.reclaims = kept
	var sum float64
	for _, r := range kept {
		sum += r.mb
	}
	return sum
}

func statsOf(a Assessment) journal.Stats {
	return journal.Stats{
		AvailableMB:  float64(a.Snap.AvailPhys) / (1 << 20),
		InUseMB:      float64(a.Snap.InUse()) / (1 << 20),
		CommitMB:     float64(a.Snap.Commit) / (1 << 20),
		StandbyMB:    float64(a.Snap.Pages.Standby) / (1 << 20),
		AvailablePct: a.AvailPct,
	}
}

func (e *Engine) record(entry journal.Entry) {
	if e.jr == nil {
		return
	}
	_ = e.jr.Write(entry)
}

// formatLine renders one compact status line for --verbose runs.
func formatLine(a Assessment) string {
	kind := map[string]string{
		"none": "安静", "commit": "提交增长", "workset": "工作集增长", "standby": "待机缓存",
	}[a.Kind]
	return fmt.Sprintf("%s  可用 %.1f GB (%.0f%%)  使用 %.1f GB  提交 %.1f GB  待机 %.1f GB  趋势 %+.0f MB/h  %s",
		a.At.Format("15:04:05"),
		float64(a.Snap.AvailPhys)/(1<<30),
		a.AvailPct,
		float64(a.Snap.InUse())/(1<<30),
		float64(a.Snap.Commit)/(1<<30),
		float64(a.Snap.Pages.Standby)/(1<<30),
		a.CommitSlope,
		kind,
	)
}
