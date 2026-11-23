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
	"sync"
	"time"

	"memora/internal/config"
	"memora/internal/journal"
	"memora/internal/ledger"
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

// OrphanProcess represents a detached non-system background process holding significant
// memory after its tracked parent process exited without leaving a visible window.
type OrphanProcess struct {
	PID        uint32 `json:"pid"`
	ParentPID  uint32 `json:"parent_pid"`
	ParentName string `json:"parent_name"`
	Name       string `json:"name"`
	WorkingSet uint64 `json:"working_set_bytes"`
	Private    uint64 `json:"private_bytes"`
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
	FullscreenState uint32
	FullscreenQuiet       bool
	FullscreenLabel       string
	FamilyProtected       int
	SoftManaged           int
	SmartTier             int
	SmartLabel            string
	SpawnRatePerMin       float64
	TopSpawners           []SpawnerStat
	BurstSpawners         []proc.BurstSpawnerStat
	Orphans               []OrphanProcess
	AdaptiveCooldownProcs []string `json:"adaptive_cooldown_procs,omitempty"`
	KernelLeakBackoff     bool     `json:"kernel_leak_backoff,omitempty"`
	AFKFrozenCount        int      `json:"afk_frozen_count,omitempty"`
}

// Plan is what the policy decided to do, and why.
type Plan struct {
	Trim             []proc.Info
	Purge            bool
	PurgeLowPriority bool
	Reason           string
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

type spawnEvent struct {
	At    time.Time
	PPID  uint32
	Name  string
	Count int
}

type trimObservation struct {
	At          time.Time
	Name        string
	PostTrimWS  uint64
	ShrunkBytes uint64
	Checked     bool
}

// Engine holds the run state. It is not safe for concurrent use across Step calls,
// though mode updates from control UIs may use SetMode/ConfigSnapshot safely.
type Engine struct {
	cfg config.Config
	jr  *journal.Journal
	lg  *ledger.Ledger

	hist       []histPoint
	baseline   map[uint32]baselineProc
	baselineAt time.Time

	lastCPU    map[uint32]cpuMark
	lastCPUPct map[uint32]float64
	idleSince  map[uint32]time.Time
	lastAt     time.Time

	lastAction  time.Time
	actionTimes []time.Time
	reclaims    []reclaim
	lastPurge   time.Time
	trimmedPIDs map[uint32]time.Time

	lastTrimObs   map[uint32]trimObservation
	reboundCounts map[string]int // lower(name) -> rebound count after TrimWorkingSet

	seenParents      map[uint32]parentRecord
	announcedOrphans map[uint32]bool
	softState        map[uint32]softControl
	qosDenied        map[uint32]bool
	lastExtrasAt     time.Time

	lastProcAllocs   uint32
	lastProcAllocsAt time.Time
	lastSpawnPoll    time.Time
	seenPIDs         map[uint32]bool
	spawnEvents      []spawnEvent
	lastSmartTrim    time.Time

	lastLeakSuspected   bool
	lastZombieLeakBytes uint64

	burstMu           sync.Mutex
	burstInFlight     bool
	lastBurstAt       time.Time
	lastBurstSpawners []proc.BurstSpawnerStat

	afkMu              sync.Mutex
	afkFrozenPIDs      map[uint32]string
	afkParentPIDs      map[uint32]uint32
	afkLastPulseAt     time.Time
	afkWatchdogRunning bool
	lastSystemWSFlush  time.Time
}

// New builds an engine around already-loaded settings.
func New(cfg config.Config, jr *journal.Journal) *Engine {
	return &Engine{
		cfg:              cfg,
		jr:               jr,
		lastCPU:          map[uint32]cpuMark{},
		lastCPUPct:       map[uint32]float64{},
		idleSince:        map[uint32]time.Time{},
		trimmedPIDs:      map[uint32]time.Time{},
		lastTrimObs:      map[uint32]trimObservation{},
		reboundCounts:    map[string]int{},
		baseline:         map[uint32]baselineProc{},
		seenParents:      map[uint32]parentRecord{},
		announcedOrphans: map[uint32]bool{},
		softState:        map[uint32]softControl{},
		qosDenied:        map[uint32]bool{},
		seenPIDs:         map[uint32]bool{},
		afkFrozenPIDs:    map[uint32]string{},
		afkParentPIDs:    map[uint32]uint32{},
	}
}

// SetLedger attaches an optional long-term Quiet Ledger to the engine.
func (e *Engine) SetLedger(lg *ledger.Ledger) {
	e.lg = lg
	if lg != nil {
		if snap := lg.ReboundProcsSnapshot(); len(snap) > 0 {
			for k, v := range snap {
				e.reboundCounts[k] = v
			}
		}
	}
}

// SetMode switches between "guard" and "observe"; switching away from guard
// restores any active soft priority / EcoQoS hints immediately.
func (e *Engine) SetMode(mode string) {
	if mode != "guard" && mode != "observe" {
		return
	}
	e.cfg.Mode = mode
	if mode != "guard" {
		e.RestoreAll()
	}
}

// Mode returns the current policy mode ("guard" or "observe").
func (e *Engine) Mode() string {
	return e.cfg.Mode
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
	if e.lg != nil {
		_ = e.lg.StartSession()
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.RestoreAll()
			if e.lg != nil {
				_ = e.lg.Flush()
			}
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
	if opts.TargetPID == 0 {
		ObserveProbeSample(s)
	}

	a := e.assess(s)
	fgPID := proc.ForegroundPID()
	family := proc.FamilyPIDs(s.Procs, fgPID)
	qState, qBusy := proc.IsFullscreenOrBusy()
	a.FullscreenState = qState
	a.FullscreenQuiet = e.cfg.Thresholds.QuietOnFullscreen && qBusy
	a.FullscreenLabel = proc.NotificationStateLabel(qState)
	a.FamilyProtected = len(family)

	a.Orphans = e.detectOrphans(s.Procs, family)
	e.recordNewOrphans(a.Orphans)
	e.evaluateTrimRebounds(s)

	extras := winmem.ReadSystemExtras(s.Snap.PageSize)
	_, tightNUMA := NUMADisparity(extras.NUMANodes)
	a.SmartTier, a.SmartLabel = computeSmartTier(a.AvailPct, tightNUMA)

	canAct := opts.Act && e.cfg.Mode == "guard" && !a.FullscreenQuiet
	if opts.Act && e.cfg.Mode == "guard" {
		e.restoreEligibleQoS(s.Procs, family, a.SpawnRatePerMin)
	}
	a.SpawnRatePerMin, a.TopSpawners = e.trackSpawns(s, family, canAct)

	acted := false
	if opts.Act && e.cfg.Mode == "guard" {
		if !a.FullscreenQuiet {
			if e.cfg.Actions.SmartMemorySchedule {
				if e.stepSmartSchedule(opts, s, &a, family) {
					acted = true
				}
			} else {
				e.applySoftQoS(s, opts.TargetPID, family, a.Orphans, a.SmartTier)
				candidates := s.Procs
				if opts.TargetPID != 0 {
					candidates = candidates[:0:0]
					for _, p := range s.Procs {
						if p.PID == opts.TargetPID {
							candidates = append(candidates, p)
						}
					}
				}
				if plan, ok := e.plan(a, candidates, family); ok {
					if after, ok := e.execute(opts, plan, a); ok {
						reclaimed := float64(after.Snap.AvailPhys)/(1<<20) - float64(a.Snap.AvailPhys)/(1<<20)
						if reclaimed < 0 {
							reclaimed = 0
						}
						if e.lg != nil {
							e.lg.RecordTrim(reclaimed, a.Severity >= 2)
						}
						after.FullscreenState = a.FullscreenState
						after.FullscreenQuiet = a.FullscreenQuiet
						after.FullscreenLabel = a.FullscreenLabel
						after.FamilyProtected = a.FamilyProtected
						after.Orphans = a.Orphans
						after.SmartTier = a.SmartTier
						after.SmartLabel = a.SmartLabel
						after.SpawnRatePerMin = a.SpawnRatePerMin
						after.TopSpawners = a.TopSpawners
						a = after
						acted = true
					}
				}
			}
		}
	}
	a.SoftManaged = len(e.softState)
	a.AdaptiveCooldownProcs = e.AdaptiveCooldownNames()
	e.maybeTriggerBurstCapture(a.SpawnRatePerMin, a.FullscreenQuiet)
	a.BurstSpawners = e.BurstSpawners()
	e.stepAFKSpawnFreeze(s, &a, family, canAct)

	if e.lg != nil && opts.Act {
		e.lg.RecordSample(s.At, e.interval(opts), !acted, a.FullscreenQuiet,
			float64(s.Snap.InUse())/(1<<20), float64(s.Private)/(1<<20))
		e.maybeRecordExtras(s)
	}

	if opts.Verbose {
		fmt.Println(formatLine(a))
	}
	if opts.OnSample != nil {
		opts.OnSample(a)
	}
	return a, nil
}

// SetKernelLeakState updates the engine's awareness of active Ring-0 zombie leaks so
// Tier 3 user-space trimming can automatically back off instead of thrashing.
func (e *Engine) SetKernelLeakState(suspected bool, zombieBytes uint64) {
	e.lastLeakSuspected = suspected
	e.lastZombieLeakBytes = zombieBytes
}

// BurstSpawners returns the latest high-frequency short-lived spawner capture results.
func (e *Engine) BurstSpawners() []proc.BurstSpawnerStat {
	e.burstMu.Lock()
	defer e.burstMu.Unlock()
	if len(e.lastBurstSpawners) == 0 {
		return nil
	}
	out := make([]proc.BurstSpawnerStat, len(e.lastBurstSpawners))
	copy(out, e.lastBurstSpawners)
	return out
}

// RunBurstSpawnCapture performs an on-demand 2.5s high-frequency Toolhelp32 capture
// to uncover short-lived (<100ms) child process spawners missed by 5s-10s polling.
func (e *Engine) RunBurstSpawnCapture(ctx context.Context) []proc.BurstSpawnerStat {
	leakKB := 0.0
	if e.lastLeakSuspected {
		leakKB = 150.0
	}
	res := proc.CaptureBurstSpawners(ctx, 2500*time.Millisecond, 30*time.Millisecond, leakKB)
	e.burstMu.Lock()
	e.lastBurstSpawners = append([]proc.BurstSpawnerStat(nil), res...)
	e.lastBurstAt = time.Now()
	e.burstMu.Unlock()
	return res
}

func (e *Engine) maybeTriggerBurstCapture(spawnRatePerMin float64, fullscreenQuiet bool) {
	if fullscreenQuiet {
		return
	}
	if spawnRatePerMin < 12 && !e.lastLeakSuspected {
		return
	}
	e.burstMu.Lock()
	if e.burstInFlight || (!e.lastBurstAt.IsZero() && time.Since(e.lastBurstAt) < 3*time.Minute) {
		e.burstMu.Unlock()
		return
	}
	e.burstInFlight = true
	leakKB := 0.0
	if e.lastLeakSuspected {
		leakKB = 150.0
	}
	e.burstMu.Unlock()

	go func() {
		defer func() {
			e.burstMu.Lock()
			e.burstInFlight = false
			e.burstMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		res := proc.CaptureBurstSpawners(ctx, 2500*time.Millisecond, 15*time.Millisecond, leakKB)
		e.burstMu.Lock()
		e.lastBurstSpawners = res
		e.lastBurstAt = time.Now()
		e.burstMu.Unlock()
	}()
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
	nextPct := make(map[uint32]float64, len(procs))
	nextIdle := make(map[uint32]time.Time, len(e.idleSince))

	for _, p := range procs {
		prev, seen := e.lastCPU[p.PID]
		pct := 0.0
		if seen && p.CPUSeconds > prev.seconds {
			pct = (p.CPUSeconds - prev.seconds) / wall.Seconds() * 100
		}
		nextCPU[p.PID] = cpuMark{seconds: p.CPUSeconds, at: now}
		nextPct[p.PID] = pct

		if pct < 1.0 {
			if since, ok := e.idleSince[p.PID]; ok {
				nextIdle[p.PID] = since
			} else {
				nextIdle[p.PID] = now
			}
		}
	}

	e.lastCPU = nextCPU
	e.lastCPUPct = nextPct
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

func (e *Engine) plan(a Assessment, procs []proc.Info, family map[uint32]bool) (Plan, bool) {
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
			if p.PID == foreground || p.PID == self || family[p.PID] {
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
		plan.PurgeLowPriority = a.Severity < 3
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
	purgeAction := "purge_standby"
	if plan.PurgeLowPriority {
		purgeAction = "purge_low_priority_standby"
	}
	if plan.Purge && len(plan.Trim) > 0 {
		action = "trim_idle_working_sets+" + purgeAction
	} else if plan.Purge {
		action = purgeAction
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

	if len(plan.Trim) > 0 || plan.Purge {
		MarkSelfMemoryAction()
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
		var err error
		if plan.PurgeLowPriority {
			err = winmem.PurgeLowPriorityStandby()
		} else {
			err = winmem.PurgeStandby()
		}
		if err != nil {
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

func isEssentialInteractive(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "wetype_server.exe", "wetype_service.exe", "wetype.exe",
		"ctfmon.exe", "textinputhost.exe", "chstime.exe",
		"sogoucloud.exe", "sogouimebroker.exe", "sgtool.exe",
		"qqpinyin.exe", "baiduime.exe", "weaselserver.exe",
		"shellexperiencehost.exe", "startmenuexperiencehost.exe",
		"searchhost.exe", "lockapp.exe":
		return true
	default:
		return false
	}
}

func (e *Engine) isProtected(name string) bool {
	if isEssentialInteractive(name) {
		return true
	}
	for _, p := range e.cfg.Protected {
		if strings.EqualFold(p, name) {
			return true
		}
	}
	for _, w := range e.cfg.Whitelist {
		if strings.EqualFold(strings.TrimSpace(w), name) {
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
	extra := ""
	if a.SmartLabel != "" {
		extra = "  [" + a.SmartLabel + "]"
	}
	if a.FullscreenQuiet {
		extra += "  [" + a.FullscreenLabel + "免打扰]"
	} else if a.SoftManaged > 0 {
		extra += fmt.Sprintf("  [软管理:%d]", a.SoftManaged)
	}
	if a.SpawnRatePerMin >= 20 {
		extra += fmt.Sprintf("  [派生:%.0f/分]", a.SpawnRatePerMin)
	}
	if len(a.Orphans) > 0 {
		extra += fmt.Sprintf("  [孤儿残留:%d]", len(a.Orphans))
	}
	return fmt.Sprintf("%s  可用 %.1f GB (%.0f%%)  使用 %.1f GB  提交 %.1f GB  待机 %.1f GB  趋势 %+.0f MB/h  %s%s",
		a.At.Format("15:04:05"),
		float64(a.Snap.AvailPhys)/(1<<30),
		a.AvailPct,
		float64(a.Snap.InUse())/(1<<30),
		float64(a.Snap.Commit)/(1<<30),
		float64(a.Snap.Pages.Standby)/(1<<30),
		a.CommitSlope,
		kind,
		extra,
	)
}
