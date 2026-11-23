package engine

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"memora/internal/journal"
	"memora/internal/proc"
)

type parentRecord struct {
	Name       string
	CreateTime int64
}

type softControl struct {
	MemoryPriority uint32
	EcoQoS         bool
	BelowNormal    bool
	StormThrottled bool
	SoftWSCapped   bool
	Name           string
	WorkingSet     uint64
	Private        uint64
	ManagedAt      time.Time
}

const (
	orphanMinBytes        = 64 << 20
	softQoSMinBytes       = 32 << 20
	maxSoftChangesPerTick = 16
)

// detectOrphans finds non-system Session > 0 processes holding significant RAM
// whose previously observed non-shell parent exited while they remain headless
// (no visible top-level window) and outside the active foreground family.
func (e *Engine) detectOrphans(procs []proc.Info, family map[uint32]bool) []OrphanProcess {
	alive := make(map[uint32]proc.Info, len(procs))
	for _, p := range procs {
		alive[p.PID] = p
	}

	type candidate struct {
		p          proc.Info
		parentName string
	}
	var cands []candidate
	selfPID := uint32(os.Getpid())

	for _, p := range procs {
		if p.SessionID == 0 || p.PID == selfPID || family[p.PID] || e.isProtected(p.Name) {
			continue
		}
		if p.Private < orphanMinBytes && p.WorkingSet < orphanMinBytes {
			continue
		}
		if p.ParentPID == 0 || p.ParentPID == 4 {
			continue
		}
		if _, parentAlive := alive[p.ParentPID]; parentAlive {
			continue
		}
		parent, seenBefore := e.seenParents[p.ParentPID]
		if !seenBefore || proc.IsShellOrSystemHost(parent.Name) {
			continue
		}
		// Guard against PID reuse: original parent must not be newer than child.
		if parent.CreateTime > 0 && p.CreateTime > 0 && parent.CreateTime > p.CreateTime {
			continue
		}
		if e.lastCPUPct[p.PID] >= 1.0 {
			continue
		}
		cands = append(cands, candidate{p: p, parentName: parent.Name})
	}

	var orphans []OrphanProcess
	if len(cands) > 0 {
		visible := proc.VisibleWindowPIDs()
		for _, c := range cands {
			if visible[c.p.PID] {
				continue
			}
			orphans = append(orphans, OrphanProcess{
				PID:        c.p.PID,
				ParentPID:  c.p.ParentPID,
				ParentName: c.parentName,
				Name:       c.p.Name,
				WorkingSet: c.p.WorkingSet,
				Private:    c.p.Private,
			})
		}
		sort.Slice(orphans, func(i, j int) bool { return orphans[i].Private > orphans[j].Private })
	}

	for _, p := range procs {
		e.seenParents[p.PID] = parentRecord{
			Name:       p.Name,
			CreateTime: p.CreateTime,
		}
	}
	for pid := range e.announcedOrphans {
		if _, ok := alive[pid]; !ok {
			delete(e.announcedOrphans, pid)
		}
	}
	return orphans
}

func (e *Engine) recordNewOrphans(orphans []OrphanProcess) {
	if len(orphans) == 0 {
		return
	}
	newCount := 0
	for _, o := range orphans {
		if e.announcedOrphans[o.PID] {
			continue
		}
		e.announcedOrphans[o.PID] = true
		newCount++
		e.record(journal.Entry{
			Kind:   "orphan",
			Reason: fmt.Sprintf("父进程 %s(%d) 已退出但无窗子进程滞留", o.ParentName, o.ParentPID),
			Targets: []journal.Target{{
				PID:          o.PID,
				Name:         o.Name,
				WorkingSetMB: float64(o.WorkingSet) / (1 << 20),
			}},
		})
	}
	if e.lg != nil && newCount > 0 {
		e.lg.RecordOrphans(newCount)
	}
}

func restoreProcessControls(pid uint32, st softControl) {
	if st.SoftWSCapped {
		_ = proc.ClearSoftWorkingSetCap(pid)
	}
	if st.MemoryPriority != 0 && st.MemoryPriority != proc.MemoryPriorityNormal {
		_ = proc.SetMemoryPriority(pid, proc.MemoryPriorityNormal)
	}
	if st.EcoQoS {
		_ = proc.SetEcoQoS(pid, false)
	}
	if st.BelowNormal {
		_ = proc.SetBelowNormalPriority(pid, false)
	}
}

func (e *Engine) restoreEligibleQoS(procs []proc.Info, family map[uint32]bool, spawnRate float64) {
	if len(e.softState) == 0 {
		return
	}
	alive := make(map[uint32]proc.Info, len(procs))
	familyNames := make(map[string]bool)
	for _, p := range procs {
		alive[p.PID] = p
		if family[p.PID] && p.Name != "" {
			familyNames[strings.ToLower(p.Name)] = true
		}
	}
	restores := 0
	for pid, st := range e.softState {
		p, ok := alive[pid]
		if !ok {
			delete(e.softState, pid)
			delete(e.qosDenied, pid)
			continue
		}
		st.Name = p.Name
		st.WorkingSet = p.WorkingSet
		st.Private = p.Private

		activeBurst := e.lastCPUPct[pid] >= 2.0
		sameAppAsFG := !st.StormThrottled && familyNames[strings.ToLower(p.Name)]
		// Keep StormThrottled processes suppressed while in background; only restore when
		// entering foreground family, or explicitly whitelisted / protected.
		var shouldRestore bool
		if st.StormThrottled {
			shouldRestore = family[pid] || familyNames[strings.ToLower(p.Name)] || e.isProtected(p.Name) || e.isStormProtected(p.Name)
		} else {
			shouldRestore = family[pid] || sameAppAsFG || e.isProtected(p.Name) || activeBurst
		}
		if shouldRestore {
			restoreProcessControls(pid, st)
			delete(e.softState, pid)
			restores++
		} else {
			e.softState[pid] = st
		}
	}
	if e.lg != nil && restores > 0 {
		e.lg.RecordSoft(0, restores)
	}
}

// RestoreAll immediately returns every MEMORA-managed process back to OS defaults.
func (e *Engine) RestoreAll() {
	e.resumeAllAFKFrozen()
	if len(e.softState) == 0 {
		return
	}
	restores := 0
	for pid, st := range e.softState {
		restoreProcessControls(pid, st)
		delete(e.softState, pid)
		restores++
	}
	if e.lg != nil && restores > 0 {
		e.lg.RecordSoft(0, restores)
	}
}

func (e *Engine) restoreFeature(mem, eco, below, storm bool) {
	if storm {
		e.resumeAllAFKFrozen()
	}
	if len(e.softState) == 0 {
		return
	}
	restores := 0
	for pid, st := range e.softState {
		if mem && st.MemoryPriority != 0 && st.MemoryPriority != proc.MemoryPriorityNormal {
			_ = proc.SetMemoryPriority(pid, proc.MemoryPriorityNormal)
			st.MemoryPriority = 0
		}
		if mem && st.SoftWSCapped {
			_ = proc.ClearSoftWorkingSetCap(pid)
			st.SoftWSCapped = false
		}
		if eco && st.EcoQoS {
			_ = proc.SetEcoQoS(pid, false)
			st.EcoQoS = false
		}
		if below && st.BelowNormal {
			_ = proc.SetBelowNormalPriority(pid, false)
			st.BelowNormal = false
		}
		if storm && st.StormThrottled {
			st.StormThrottled = false
			if !e.cfg.Actions.SoftMemoryPriority && !e.cfg.Actions.EcoQoSBackground {
				restoreProcessControls(pid, st)
				st.MemoryPriority = 0
				st.EcoQoS = false
				st.BelowNormal = false
				st.SoftWSCapped = false
			}
		}
		if st.MemoryPriority == 0 && !st.EcoQoS && !st.BelowNormal && !st.StormThrottled && !st.SoftWSCapped {
			delete(e.softState, pid)
			restores++
		} else {
			e.softState[pid] = st
		}
	}
	if e.lg != nil && restores > 0 {
		e.lg.RecordSoft(0, restores)
	}
}

func (e *Engine) applySoftQoS(s Sample, targetPID uint32, family map[uint32]bool, orphans []OrphanProcess, tier int) {
	wantMem := e.cfg.Actions.SoftMemoryPriority
	wantEco := e.cfg.Actions.EcoQoSBackground
	if !wantMem && !wantEco {
		return
	}

	alive := make(map[uint32]proc.Info, len(s.Procs))
	for _, p := range s.Procs {
		alive[p.PID] = p
	}
	orphanSet := make(map[uint32]bool, len(orphans))
	for _, o := range orphans {
		orphanSet[o.PID] = true
	}

	for pid, st := range e.softState {
		p, ok := alive[pid]
		if !ok {
			continue
		}
		if e.isProtected(p.Name) {
			restoreProcessControls(pid, st)
			delete(e.softState, pid)
			continue
		}
		st.Name = p.Name
		st.WorkingSet = p.WorkingSet
		st.Private = p.Private
		if wantMem && st.MemoryPriority == proc.MemoryPriorityLow &&
			(tier >= 3 || e.isReboundProne(p.Name) || (!st.ManagedAt.IsZero() && s.At.Sub(st.ManagedAt) >= 8*time.Minute)) {
			if err := proc.SetMemoryPriority(pid, proc.MemoryPriorityVeryLow); err == nil {
				st.MemoryPriority = proc.MemoryPriorityVeryLow
			}
		}
		// Apply Win32 soft working-set quota ceiling (Flags=0, non-hard) on rebound-prone or
		// storm-throttled idle background hogs so MmWorkingSetManager holds them gently in Ring 0.
		if wantMem && !st.SoftWSCapped && (e.isReboundProne(p.Name) || st.StormThrottled) && p.WorkingSet >= 64<<20 {
			capBytes := p.WorkingSet / 2
			if capBytes < 48<<20 {
				capBytes = 48 << 20
			}
			if capBytes > 160<<20 {
				capBytes = 160 << 20
			}
			if err := proc.ApplySoftWorkingSetCap(pid, 8<<20, capBytes); err == nil {
				st.SoftWSCapped = true
			}
		}
		if wantEco && !st.EcoQoS && (tier >= 3 || orphanSet[pid] || (!st.ManagedAt.IsZero() && s.At.Sub(st.ManagedAt) >= 5*time.Minute)) {
			if err := proc.SetEcoQoS(pid, true); err == nil {
				st.EcoQoS = true
			}
		}
		// Only apply BelowNormalPriority to confirmed detached orphans or storm-throttled spawners;
		// never starve normal user-interactive background processes on CPU scheduling.
		if wantEco && !st.BelowNormal && (orphanSet[pid] || st.StormThrottled) {
			if err := proc.SetBelowNormalPriority(pid, true); err == nil {
				st.BelowNormal = true
			}
		}
		e.softState[pid] = st
	}

	softIdle := time.Duration(e.cfg.Thresholds.SoftIdleMinutes * float64(time.Minute))
	if softIdle <= 0 {
		softIdle = 2 * time.Minute
	}
	orphanIdle := time.Duration(e.cfg.Thresholds.OrphanIdleMinutes * float64(time.Minute))
	if orphanIdle <= 0 {
		orphanIdle = 1 * time.Minute
	}

	var cands []proc.Info
	if len(family) == 0 {
		// Foreground window is transitioning/locked; never downgrade processes blindly.
		return
	}
	familyNames := make(map[string]bool)
	for _, p := range s.Procs {
		if family[p.PID] && p.Name != "" {
			familyNames[strings.ToLower(p.Name)] = true
		}
	}
	selfPID := uint32(os.Getpid())
	selfFamily := proc.FamilyPIDs(s.Procs, selfPID)
	for _, p := range s.Procs {
		if targetPID != 0 && p.PID != targetPID {
			continue
		}
		if p.SessionID == 0 || p.PID == selfPID || selfFamily[p.PID] || family[p.PID] || familyNames[strings.ToLower(p.Name)] || e.isProtected(p.Name) || e.qosDenied[p.PID] {
			continue
		}
		if p.WorkingSet < softQoSMinBytes && p.Private < softQoSMinBytes {
			continue
		}
		if _, managed := e.softState[p.PID]; managed {
			continue
		}
		since, ok := e.idleSince[p.PID]
		if !ok {
			continue
		}
		needIdle := softIdle
		if orphanSet[p.PID] {
			needIdle = orphanIdle
		}
		if s.At.Sub(since) < needIdle {
			continue
		}
		cands = append(cands, p)
	}

	sort.Slice(cands, func(i, j int) bool { return cands[i].Private > cands[j].Private })

	baseMemPrio := uint32(proc.MemoryPriorityLow)
	if tier >= 3 {
		baseMemPrio = proc.MemoryPriorityVeryLow
	}

	applied := 0
	for _, p := range cands {
		if applied >= maxSoftChangesPerTick {
			break
		}
		st := softControl{
			Name:       p.Name,
			WorkingSet: p.WorkingSet,
			Private:    p.Private,
			ManagedAt:  s.At,
		}
		candMemPrio := baseMemPrio
		if e.isReboundProne(p.Name) {
			candMemPrio = proc.MemoryPriorityVeryLow
		}
		deniedCount := 0
		if wantMem {
			if err := proc.SetMemoryPriority(p.PID, candMemPrio); err == nil {
				st.MemoryPriority = candMemPrio
			} else {
				deniedCount++
			}
		}
		if wantEco {
			if err := proc.SetEcoQoS(p.PID, true); err == nil {
				st.EcoQoS = true
			} else {
				deniedCount++
			}
			if orphanSet[p.PID] {
				if err := proc.SetBelowNormalPriority(p.PID, true); err == nil {
					st.BelowNormal = true
				}
			}
		}
		if st.MemoryPriority != 0 || st.EcoQoS || st.BelowNormal {
			e.softState[p.PID] = st
			applied++
		} else if deniedCount > 0 {
			e.qosDenied[p.PID] = true
		}
	}
	if e.lg != nil && applied > 0 {
		e.lg.RecordSoft(applied, 0)
	}
}

func (e *Engine) maybeRecordExtras(s Sample) {
	if e.lg == nil {
		return
	}
	if !e.lastExtrasAt.IsZero() && s.At.Sub(e.lastExtrasAt) < 5*time.Minute {
		return
	}
	e.lastExtrasAt = s.At
	deep := InspectDeep(s.Snap, s.Procs, false)
	bd := ComputeBreakdown(s.Snap, s.PrivateWS, deep)
	e.SetKernelLeakState(deep.Leak.Suspected, bd.ZombieLeakBytes)
	leakNote := ""
	if deep.Leak.Suspected {
		leakNote = fmt.Sprintf("Process对象 %d 个 vs 活跃 %d 个（相关内核池 %.0f MB）",
			deep.Leak.ProcessObjects, deep.Leak.ActiveProcesses, float64(deep.Leak.RelatedPoolByte)/(1<<20))
	}
	e.lg.RecordBaselineExtras(s.At, float64(deep.Extras.VidBytes)/(1<<20), deep.Leak.ProcessObjects, float64(bd.ZombieLeakBytes)/(1<<20), leakNote)
}
