package engine

import (
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

// SpawnerStat identifies a parent process frequently creating child processes.
type SpawnerStat struct {
	PID        uint32 `json:"pid"`
	Name       string `json:"name"`
	ChildCount int    `json:"child_count"`
	Throttled  bool   `json:"throttled"`
}

// ManagedProcessInfo describes one background process currently under MEMORA QoS control.
type ManagedProcessInfo struct {
	PID              uint32  `json:"pid"`
	Name             string  `json:"name"`
	WorkingSetMB     float64 `json:"working_set_mb"`
	PrivateMB        float64 `json:"private_mb"`
	MemoryPriority   string  `json:"memory_priority"`
	EcoQoS           bool    `json:"eco_qos"`
	BelowNormalCPU   bool    `json:"below_normal_cpu"`
	StormThrottled   bool    `json:"storm_throttled"`
	AdaptiveCooldown bool    `json:"adaptive_cooldown"`
	ManagedSeconds   int64   `json:"managed_seconds"`
}

// OptimizeTarget reports how much working set one process released during an optimize pass.
type OptimizeTarget struct {
	PID         uint32  `json:"pid"`
	Name        string  `json:"name"`
	ReclaimedMB float64 `json:"reclaimed_mb"`
}

// OptimizeResult summarizes an on-demand smart optimization sweep.
type OptimizeResult struct {
	TrimmedCount  int              `json:"trimmed_count"`
	WSShrunkMB    float64          `json:"ws_shrunk_mb"`
	AvailGainMB   float64          `json:"avail_gain_mb"`
	StandbyPurged bool             `json:"standby_purged"`
	ModifiedFlush bool             `json:"modified_flushed"`
	DurationMS    int64            `json:"duration_ms"`
	Targets       []OptimizeTarget `json:"targets,omitempty"`
}

// MemoryBreakdown decomposes TotalPhys into non-overlapping buckets summing to TotalPhys.
type MemoryBreakdown struct {
	TotalBytes        uint64   `json:"total_bytes"`
	InUseBytes        uint64   `json:"in_use_bytes"`
	AvailBytes        uint64   `json:"avail_bytes"`
	AppPrivateWSBytes uint64   `json:"app_private_ws_bytes"`
	ActiveAppWSBytes  uint64   `json:"active_app_ws_bytes"`
	IdleAppWSBytes    uint64   `json:"idle_app_ws_bytes"`
	VidBytes          uint64   `json:"vid_bytes"`
	ZombieLeakBytes   uint64   `json:"zombie_leak_bytes"`
	KernelDriverBytes uint64   `json:"kernel_driver_bytes"`
	Slot2Mode         string   `json:"slot2_mode"` // "vid" | "idle_bg"
	TopIdleApps       []string `json:"top_idle_apps,omitempty"`
}

// ComputeBreakdown partitions physical RAM into mutually exclusive buckets.
func ComputeBreakdown(snap winmem.Snapshot, privWS uint64, deep DeepDiagnosis) MemoryBreakdown {
	total := snap.TotalPhys
	avail := snap.AvailPhys
	if avail > total {
		avail = total
	}
	inUse := total - avail

	apps := privWS
	if apps > inUse {
		apps = inUse
	}
	rem := inUse - apps

	vid := deep.Extras.VidBytes
	if vid > rem {
		vid = rem
	}
	rem -= vid

	kernelTotal := snap.KernelTotal
	var zombie, kernel uint64
	if rem > kernelTotal {
		// Note: rem - kernelTotal includes shared working set (mapped DLLs/code) plus any
		// leaked PFN/page-table stubs. Attribute up to 4 KB per excess leaked Process object
		// (an exited EPROCESS has already torn down its user VADs/working set, so only kernel
		// pool + ~1 page of residual PFN/handle overhead remains) so normal shared memory
		// is never miscounted as zombie leak.
		unaccountedInUse := rem - kernelTotal
		if deep.Leak.Suspected {
			if deep.Leak.ProcessObjects > 0 {
				excessObjs := int64(deep.Leak.ProcessObjects) - int64(deep.Leak.ActiveProcesses)
				if excessObjs < 0 {
					excessObjs = 0
				}
				estUnaccounted := uint64(excessObjs) * 4 * 1024
				if unaccountedInUse > estUnaccounted {
					unaccountedInUse = estUnaccounted
				}
			}
			leakPool := deep.Leak.RelatedPoolByte
			if leakPool > kernelTotal {
				leakPool = kernelTotal
			}
			zombie = leakPool + unaccountedInUse
			if zombie > rem {
				zombie = rem
			}
			kernel = rem - zombie
		} else {
			kernel = rem
		}
	} else {
		if deep.Leak.Suspected {
			leakPool := deep.Leak.RelatedPoolByte
			if leakPool > rem {
				leakPool = rem
			}
			zombie = leakPool
			kernel = rem - zombie
		} else {
			kernel = rem
		}
	}

	slot2Mode := "idle_bg"
	if vid >= 64<<20 {
		slot2Mode = "vid"
	}

	return MemoryBreakdown{
		TotalBytes:        total,
		InUseBytes:        inUse,
		AvailBytes:        avail,
		AppPrivateWSBytes: apps,
		ActiveAppWSBytes:  apps,
		IdleAppWSBytes:    0,
		VidBytes:          vid,
		ZombieLeakBytes:   zombie,
		KernelDriverBytes: kernel,
		Slot2Mode:         slot2Mode,
	}
}

// ComputeAdaptiveBreakdown enriches ComputeBreakdown with workload-aware Slot 2 partitioning:
// when WSL2/Hyper-V Vid locks >= 64 MB, Slot 2 surfaces virtualization lock pages; otherwise
// Slot 2 automatically adapts to show idle non-foreground background PrivateWS so all 5 cards
// remain high-signal on standard gaming/office/creative Windows machines.
func (e *Engine) ComputeAdaptiveBreakdown(snap winmem.Snapshot, privWS uint64, procs []proc.Info, deep DeepDiagnosis) MemoryBreakdown {
	bd := ComputeBreakdown(snap, privWS, deep)
	if bd.Slot2Mode == "vid" || len(procs) == 0 || bd.AppPrivateWSBytes == 0 {
		return bd
	}

	fgPID := proc.ForegroundPID()
	family := proc.FamilyPIDs(procs, fgPID)
	selfPID := uint32(os.Getpid())

	var idleSum uint64
	idleByName := make(map[string]uint64, 32)
	for _, p := range procs {
		if p.SessionID == 0 || p.PID <= 4 || p.PID == selfPID || family[p.PID] || e.isProtected(p.Name) {
			continue
		}
		if e.lastCPUPct[p.PID] >= 2.5 {
			continue
		}
		idleSum += p.PrivateWS
		if p.PrivateWS >= 16<<20 && p.Name != "" {
			idleByName[p.Name] += p.PrivateWS
		}
	}
	if idleSum > bd.AppPrivateWSBytes {
		idleSum = bd.AppPrivateWSBytes
	}
	bd.IdleAppWSBytes = idleSum
	bd.ActiveAppWSBytes = bd.AppPrivateWSBytes - idleSum

	if len(idleByName) > 0 {
		type kv struct {
			name string
			ws   uint64
		}
		ranked := make([]kv, 0, len(idleByName))
		for k, v := range idleByName {
			ranked = append(ranked, kv{name: k, ws: v})
		}
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].ws > ranked[j].ws })
		limit := 3
		if len(ranked) < limit {
			limit = len(ranked)
		}
		top := make([]string, 0, limit)
		for i := 0; i < limit; i++ {
			gb := float64(ranked[i].ws) / (1 << 30)
			if gb >= 0.1 {
				top = append(top, fmt.Sprintf("%s (%.1f GB)", ranked[i].name, gb))
			} else {
				top = append(top, fmt.Sprintf("%s (%.0f MB)", ranked[i].name, float64(ranked[i].ws)/(1<<20)))
			}
		}
		bd.TopIdleApps = top
	}
	return bd
}

// UpdateConfig mutates configuration atomically inside the engine loop, restores
// controls for any disabled actions, and saves config.json.
func (e *Engine) UpdateConfig(fn func(*config.Config)) error {
	if fn == nil {
		return nil
	}
	prev := e.cfg
	fn(&e.cfg)

	if e.cfg.Mode != "guard" {
		e.RestoreAll()
	} else {
		if prev.Actions.SoftMemoryPriority && !e.cfg.Actions.SoftMemoryPriority {
			e.restoreFeature(true, false, false, false)
		}
		if prev.Actions.EcoQoSBackground && !e.cfg.Actions.EcoQoSBackground {
			e.restoreFeature(false, true, true, false)
		}
		if prev.Actions.SuppressSpawnStorm && !e.cfg.Actions.SuppressSpawnStorm {
			e.restoreFeature(false, false, false, true)
		}
	}
	return config.Save(e.cfg)
}

// ConfigSnapshot returns a copy of the engine's active settings.
func (e *Engine) ConfigSnapshot() config.Config {
	return e.cfg
}

// ManagedProcesses returns a sorted snapshot of processes currently throttled by MEMORA.
func (e *Engine) ManagedProcesses() []ManagedProcessInfo {
	now := time.Now()
	out := make([]ManagedProcessInfo, 0, len(e.softState))
	for pid, st := range e.softState {
		prio := "Normal"
		if st.MemoryPriority == proc.MemoryPriorityVeryLow {
			prio = "VeryLow"
		} else if st.MemoryPriority == proc.MemoryPriorityLow {
			prio = "Low"
		}
		sec := int64(0)
		if !st.ManagedAt.IsZero() && now.After(st.ManagedAt) {
			sec = int64(now.Sub(st.ManagedAt).Seconds())
		}
		out = append(out, ManagedProcessInfo{
			PID:              pid,
			Name:             st.Name,
			WorkingSetMB:     float64(st.WorkingSet) / (1 << 20),
			PrivateMB:        float64(st.Private) / (1 << 20),
			MemoryPriority:   prio,
			EcoQoS:           st.EcoQoS,
			BelowNormalCPU:   st.BelowNormal,
			StormThrottled:   st.StormThrottled,
			AdaptiveCooldown: e.isReboundProne(st.Name),
			ManagedSeconds:   sec,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StormThrottled != out[j].StormThrottled {
			return out[i].StormThrottled
		}
		return out[i].WorkingSetMB > out[j].WorkingSetMB
	})
	return out
}

// RecentJournal returns up to n most recent journal entries cleanly from disk.
func (e *Engine) RecentJournal(n int) []journal.Entry {
	if e.jr == nil {
		return nil
	}
	entries, err := journal.Tail(e.jr.Path(), n)
	if err != nil {
		return nil
	}
	// Return newest first for UI readability.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries
}

// computeSmartTier selects Tier 1 (cruise), Tier 2 (active balance), or Tier 3 (urgent).
func computeSmartTier(availPct float64, tightNUMA bool) (int, string) {
	inUsePct := 100 - availPct
	switch {
	case inUsePct > 86 || tightNUMA:
		return 3, "三级 · 高压急救"
	case inUsePct >= 78:
		return 2, "二级 · 主动平衡"
	default:
		return 1, "一级 · 轻度巡航"
	}
}

func (e *Engine) trackSpawns(s Sample, family map[uint32]bool, act bool) (float64, []SpawnerStat) {
	now := s.At
	// Measure kernel-wide creation rate via Proc pool alloc deltas.
	var kernelRate float64
	if tags, err := winmem.ReadPoolTags(); err == nil {
		for _, pt := range tags {
			if pt.Tag == "Proc" {
				if !e.lastProcAllocsAt.IsZero() && pt.NonPagedAllocs >= e.lastProcAllocs {
					dt := now.Sub(e.lastProcAllocsAt).Minutes()
					if dt > 0 {
						kernelRate = float64(pt.NonPagedAllocs-e.lastProcAllocs) / dt
					}
				}
				e.lastProcAllocs = pt.NonPagedAllocs
				e.lastProcAllocsAt = now
				break
			}
		}
	}

	// Count live child spawns visible between ticks grouped by ParentPID.
	byPID := make(map[uint32]proc.Info, len(s.Procs))
	for _, p := range s.Procs {
		byPID[p.PID] = p
	}
	if !e.lastSpawnPoll.IsZero() {
		deltaByParent := map[uint32]int{}
		for _, p := range s.Procs {
			if _, seen := e.seenPIDs[p.PID]; !seen && p.ParentPID != 0 && p.ParentPID != 4 {
				deltaByParent[p.ParentPID]++
			}
		}
		for ppid, cnt := range deltaByParent {
			name := ""
			if parent, ok := byPID[ppid]; ok {
				name = parent.Name
			} else if prev, ok := e.seenParents[ppid]; ok {
				name = prev.Name
			}
			e.spawnEvents = append(e.spawnEvents, spawnEvent{
				At:    now,
				PPID:  ppid,
				Name:  name,
				Count: cnt,
			})
		}
	}
	e.lastSpawnPoll = now
	e.seenPIDs = make(map[uint32]bool, len(s.Procs))
	for _, p := range s.Procs {
		e.seenPIDs[p.PID] = true
	}

	top := e.aggregateSpawnersLocked(byPID, now)
	if len(top) > 5 {
		top = top[:5]
	}
	if kernelRate == 0 && len(e.spawnEvents) > 0 {
		totalVisible := 0
		for _, ev := range e.spawnEvents {
			totalVisible += ev.Count
		}
		if totalVisible > 0 {
			kernelRate = float64(totalVisible) / 5.0
		}
	}

	if act && e.cfg.Actions.SuppressSpawnStorm && len(family) > 0 {
		familyNames := make(map[string]bool)
		for _, p := range s.Procs {
			if family[p.PID] && p.Name != "" {
				familyNames[strings.ToLower(p.Name)] = true
			}
		}
		throttledCount := 0
		for i := range top {
			sp := &top[i]
			if sp.Throttled {
				continue
			}
			// Require sustained spawning (>=12 children in 5m, or >=6 during global storm >=40/min)
			// so normal user app activity never gets misclassified as a spawn storm.
			if !((kernelRate >= 40 && sp.ChildCount >= 6) || sp.ChildCount >= 12) {
				continue
			}
			p, ok := byPID[sp.PID]
			if !ok || familyNames[strings.ToLower(p.Name)] {
				continue
			}
			reason := fmt.Sprintf("检测到子进程派生异常 (%.0f 个/分)，5分钟内 %s(%d) 派生 %d 次，已施加 EcoQoS + BelowNormal 压制",
				kernelRate, p.Name, p.PID, sp.ChildCount)
			if e.applyStormThrottle(p, family, reason, now) {
				sp.Throttled = true
				throttledCount++
				if e.cfg.Actions.AutoLearnStormList {
					e.learnSuppressedSpawner(p.Name)
				}
			}
		}
		for _, p := range s.Procs {
			if familyNames[strings.ToLower(p.Name)] || !e.isUserSuppressedSpawner(p.Name) {
				continue
			}
			reason := fmt.Sprintf("自动压制名单命中后台派生宿主 %s(%d)", p.Name, p.PID)
			if e.applyStormThrottle(p, family, reason, now) {
				throttledCount++
			}
		}
		if e.lg != nil && throttledCount > 0 {
			e.lg.RecordStormSuppression(throttledCount)
		}
	}

	return kernelRate, top
}

// SuppressActiveSpawnersNow immediately enables spawn-storm suppression, releases any
// user-facing browsers/IDEs that are now storm-protected, and throttles active
// multi-child spawners (ChildCount >= 2), known storm hosts, or handle-leaking daemons.
func (e *Engine) SuppressActiveSpawnersNow() (int, int) {
	now := time.Now()
	procs, _, err := proc.ListAll()
	if err != nil {
		return 0, 0
	}
	family := proc.FamilyPIDs(procs, proc.ForegroundPID())
	aliveMap := make(map[uint32]proc.Info, len(procs))
	for _, p := range procs {
		aliveMap[p.PID] = p
	}

	// Unthrottle any process that now falls under storm protection (e.g. browsers / IDEs).
	for pid, st := range e.softState {
		if !st.StormThrottled {
			continue
		}
		p, ok := aliveMap[pid]
		if !ok || family[pid] || proc.IsShellOrSystemHost(p.Name) || e.isProtected(p.Name) || e.isStormProtected(p.Name) {
			restoreProcessControls(pid, st)
			delete(e.softState, pid)
			if e.lg != nil {
				e.lg.RecordSoft(0, 1)
			}
		}
	}

	spawners := e.aggregateSpawnersLocked(aliveMap, now)

	throttledCount := 0
	handled := map[uint32]bool{}
	for _, sp := range spawners {
		if sp.ChildCount < 2 {
			continue
		}
		p, ok := aliveMap[sp.PID]
		if !ok {
			continue
		}
		reason := fmt.Sprintf("手动压制高频派生源：5分钟内 %s(%d) 派生 %d 次子进程", p.Name, p.PID, sp.ChildCount)
		if e.applyStormThrottle(p, family, reason, now) {
			throttledCount++
			if e.cfg.Actions.AutoLearnStormList {
				e.learnSuppressedSpawner(p.Name)
			}
		}
		handled[p.PID] = true
	}
	for _, p := range procs {
		if handled[p.PID] {
			continue
		}
		var reason string
		switch {
		case e.isUserSuppressedSpawner(p.Name) || isKnownStormHost(p.Name):
			reason = fmt.Sprintf("手动压制已知/名单内高频后台派生组件 %s(%d)", p.Name, p.PID)
		case p.Handles >= 10000:
			reason = fmt.Sprintf("手动压制高句柄泄漏后台宿主 %s(%d)（%d 句柄）", p.Name, p.PID, p.Handles)
			if e.cfg.Actions.AutoLearnStormList {
				e.learnSuppressedSpawner(p.Name)
			}
		default:
			continue
		}
		if e.applyStormThrottle(p, family, reason, now) {
			throttledCount++
		}
	}
	if e.lg != nil && throttledCount > 0 {
		e.lg.RecordStormSuppression(throttledCount)
	}

	totalThrottled := 0
	for pid, st := range e.softState {
		if st.StormThrottled {
			if _, ok := aliveMap[pid]; ok {
				totalThrottled++
			}
		}
	}
	return throttledCount, totalThrottled
}

func (e *Engine) aggregateSpawnersLocked(aliveMap map[uint32]proc.Info, now time.Time) []SpawnerStat {
	cutoff := now.Add(-5 * time.Minute)
	kept := e.spawnEvents[:0]
	agg := map[uint32]*SpawnerStat{}
	for _, ev := range e.spawnEvents {
		if !ev.At.After(cutoff) {
			continue
		}
		kept = append(kept, ev)
		st, ok := agg[ev.PPID]
		if !ok {
			st = &SpawnerStat{PID: ev.PPID, Name: ev.Name}
			agg[ev.PPID] = st
		}
		st.ChildCount += ev.Count
		if st.Name == "" {
			st.Name = ev.Name
		}
	}
	e.spawnEvents = kept

	out := make([]SpawnerStat, 0, len(agg))
	for _, st := range agg {
		if _, alive := aliveMap[st.PID]; !alive {
			continue
		}
		if cur, ok := e.softState[st.PID]; ok && cur.StormThrottled {
			st.Throttled = true
		}
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ChildCount > out[j].ChildCount })
	return out
}

func (e *Engine) applyStormThrottle(p proc.Info, family map[uint32]bool, reason string, now time.Time) bool {
	selfPID := uint32(os.Getpid())
	if p.PID == selfPID || p.PID <= 4 || family[p.PID] ||
		proc.IsShellOrSystemHost(p.Name) || e.isProtected(p.Name) || e.isStormProtected(p.Name) ||
		e.qosDenied[p.PID] {
		return false
	}
	st := e.softState[p.PID]
	if st.StormThrottled {
		return false
	}
	st.Name = p.Name
	st.WorkingSet = p.WorkingSet
	st.Private = p.Private
	if st.ManagedAt.IsZero() {
		st.ManagedAt = now
	}
	changed := false
	if st.MemoryPriority != proc.MemoryPriorityLow && st.MemoryPriority != proc.MemoryPriorityVeryLow {
		if err := proc.SetMemoryPriority(p.PID, proc.MemoryPriorityLow); err == nil {
			st.MemoryPriority = proc.MemoryPriorityLow
			changed = true
		}
	}
	if !st.EcoQoS {
		if err := proc.SetEcoQoS(p.PID, true); err == nil {
			st.EcoQoS = true
			changed = true
		}
	}
	if !st.BelowNormal {
		if err := proc.SetBelowNormalPriority(p.PID, true); err == nil {
			st.BelowNormal = true
			changed = true
		}
	}
	if st.EcoQoS || st.BelowNormal || st.MemoryPriority != 0 {
		st.StormThrottled = true
		e.softState[p.PID] = st
		if changed {
			e.record(journal.Entry{
				Kind:   "storm_throttle",
				Action: "suppress_spawn_storm",
				Reason: reason,
				Targets: []journal.Target{{
					PID:          p.PID,
					Name:         p.Name,
					WorkingSetMB: float64(p.WorkingSet) / (1 << 20),
				}},
			})
		}
		return changed
	}
	e.qosDenied[p.PID] = true
	return false
}

func isKnownStormHost(name string) bool {
	switch strings.ToLower(name) {
	case "lenovopcmanagerservice.exe", "lnvsvcfdn.exe", "slbagent.exe", "appvant.exe",
		"esurfingsvr.exe", "wetype_update.exe":
		return true
	default:
		return false
	}
}

func (e *Engine) isStormProtected(name string) bool {
	// Never throttle core OS/VM/AV daemons, active browsers, or IDEs/coding toolchains during a spawn storm,
	// while still permitting third-party background updaters/agents such as LenovoPcManagerService.
	switch strings.ToLower(name) {
	case "system", "registry", "memory compression", "secure system",
		"vmmem", "vmmem.exe", "vmmemwsl", "vmmemwsl.exe", "vmcompute.exe", "vmwp.exe",
		"wsl.exe", "wslservice.exe", "docker desktop.exe", "com.docker.backend.exe", "com.docker.build.exe",
		"smss.exe", "csrss.exe", "wininit.exe", "winlogon.exe",
		"services.exe", "lsass.exe", "svchost.exe", "dwm.exe",
		"explorer.exe", "audiodg.exe", "msmpeng.exe", "securityhealthservice.exe",
		"nissrv.exe", "memora.exe", "hipsdaemon.exe",
		"msedge.exe", "msedgewebview2.exe", "chrome.exe", "firefox.exe", "electron.exe",
		"zcode.exe", "antigravity.exe", "code.exe", "devenv.exe", "omnicode.exe",
		"language_server.exe", "language_server_windows_x64.exe",
		"gopls.exe", "rust-analyzer.exe", "pyright.exe", "claude.exe", "codex.exe",
		"go.exe", "node.exe", "npm.exe", "pnpm.exe", "python.exe", "git.exe", "bash.exe", "sh.exe",
		"cargo.exe", "rustc.exe", "dotnet.exe", "msbuild.exe", "java.exe", "javaw.exe",
		"wezterm-gui.exe", "windowsterminal.exe":
		return true
	default:
		return false
	}
}

func (e *Engine) isUserSuppressedSpawner(name string) bool {
	if name == "" || e.isProtected(name) || e.isStormProtected(name) {
		return false
	}
	target := strings.ToLower(strings.TrimSpace(name))
	for _, item := range e.cfg.SuppressedSpawners {
		if strings.EqualFold(strings.TrimSpace(item), target) {
			return true
		}
	}
	return false
}

func (e *Engine) learnSuppressedSpawner(name string) bool {
	clean := strings.ToLower(strings.TrimSpace(name))
	if clean == "" || proc.IsShellOrSystemHost(clean) || e.isProtected(clean) || e.isStormProtected(clean) {
		return false
	}
	if e.isUserSuppressedSpawner(clean) {
		return false
	}
	e.cfg.SuppressedSpawners = append(e.cfg.SuppressedSpawners, clean)
	_ = config.Save(e.cfg)
	e.record(journal.Entry{
		Kind:   "storm_list",
		Action: "auto_learn_spawner",
		Reason: fmt.Sprintf("自动将高频派生/泄漏宿主 %s 加入长期压制名单", clean),
		Targets: []journal.Target{{
			Name: clean,
		}},
	})
	return true
}

// AddSuppressedSpawner adds a process name to the persistent storm suppression list.
func (e *Engine) AddSuppressedSpawner(name string) bool {
	clean := strings.ToLower(strings.TrimSpace(name))
	if clean == "" || proc.IsShellOrSystemHost(clean) || e.isProtected(clean) || e.isStormProtected(clean) {
		return false
	}
	if !e.isUserSuppressedSpawner(clean) {
		e.cfg.SuppressedSpawners = append(e.cfg.SuppressedSpawners, clean)
		_ = config.Save(e.cfg)
	}
	if e.cfg.Mode == "guard" && e.cfg.Actions.SuppressSpawnStorm {
		if procs, _, err := proc.ListAll(); err == nil {
			now := time.Now()
			family := proc.FamilyPIDs(procs, proc.ForegroundPID())
			for _, p := range procs {
				if strings.EqualFold(p.Name, clean) {
					e.applyStormThrottle(p, family, fmt.Sprintf("手动将 %s(%d) 加入长期压制名单", p.Name, p.PID), now)
				}
			}
		}
	}
	return true
}

// RemoveSuppressedSpawner removes a process name from the persistent storm suppression list
// and releases any matching active storm throttle immediately.
func (e *Engine) RemoveSuppressedSpawner(name string) bool {
	clean := strings.ToLower(strings.TrimSpace(name))
	if clean == "" {
		return false
	}
	removed := false
	next := make([]string, 0, len(e.cfg.SuppressedSpawners))
	for _, item := range e.cfg.SuppressedSpawners {
		if strings.EqualFold(strings.TrimSpace(item), clean) {
			removed = true
			continue
		}
		next = append(next, item)
	}
	if !removed {
		return false
	}
	e.cfg.SuppressedSpawners = next
	_ = config.Save(e.cfg)
	restores := 0
	for pid, st := range e.softState {
		if st.StormThrottled && strings.EqualFold(st.Name, clean) {
			restoreProcessControls(pid, st)
			delete(e.softState, pid)
			restores++
		}
	}
	if e.lg != nil && restores > 0 {
		e.lg.RecordSoft(0, restores)
	}
	return true
}

// SuppressedSpawners returns a copy of the configured persistent storm suppression names.
func (e *Engine) SuppressedSpawners() []string {
	out := make([]string, len(e.cfg.SuppressedSpawners))
	copy(out, e.cfg.SuppressedSpawners)
	return out
}

// AddWhitelist adds a process name to the user whitelist (never trimmed, downgraded, or
// storm-suppressed), immediately restores any matching managed processes, and removes it
// from suppressed_spawners if present.
func (e *Engine) AddWhitelist(name string) bool {
	clean := strings.ToLower(strings.TrimSpace(name))
	if clean == "" {
		return false
	}
	for _, w := range e.cfg.Whitelist {
		if strings.EqualFold(strings.TrimSpace(w), clean) {
			return false
		}
	}
	e.cfg.Whitelist = append(e.cfg.Whitelist, clean)

	// Remove from suppressed_spawners if present so whitelist takes precedence cleanly.
	filtered := make([]string, 0, len(e.cfg.SuppressedSpawners))
	for _, s := range e.cfg.SuppressedSpawners {
		if !strings.EqualFold(strings.TrimSpace(s), clean) {
			filtered = append(filtered, s)
		}
	}
	e.cfg.SuppressedSpawners = filtered
	_ = config.Save(e.cfg)

	restores := 0
	for pid, st := range e.softState {
		if strings.EqualFold(st.Name, clean) {
			restoreProcessControls(pid, st)
			delete(e.softState, pid)
			restores++
		}
	}
	if e.lg != nil && restores > 0 {
		e.lg.RecordSoft(0, restores)
	}
	return true
}

// RemoveWhitelist removes a process name from the user whitelist.
func (e *Engine) RemoveWhitelist(name string) bool {
	clean := strings.ToLower(strings.TrimSpace(name))
	if clean == "" {
		return false
	}
	removed := false
	next := make([]string, 0, len(e.cfg.Whitelist))
	for _, item := range e.cfg.Whitelist {
		if strings.EqualFold(strings.TrimSpace(item), clean) {
			removed = true
			continue
		}
		next = append(next, item)
	}
	if !removed {
		return false
	}
	e.cfg.Whitelist = next
	_ = config.Save(e.cfg)
	return true
}

// Whitelist returns a copy of the configured user whitelist process names.
func (e *Engine) Whitelist() []string {
	out := make([]string, len(e.cfg.Whitelist))
	copy(out, e.cfg.Whitelist)
	return out
}

func (e *Engine) stepSmartSchedule(opts Options, s Sample, a *Assessment, family map[uint32]bool) bool {
	extras := winmem.ReadSystemExtras(s.Snap.PageSize)
	_, tightNUMA := NUMADisparity(extras.NUMANodes)
	tier, label := computeSmartTier(a.AvailPct, tightNUMA)
	a.SmartTier = tier
	a.SmartLabel = label

	// Tier 1+: Soft memory priority & EcoQoS & BelowNormal CPU on idle background processes.
	e.applySoftQoS(s, opts.TargetPID, family, a.Orphans, tier)

	// Tier 1 (cruise): gently reclaim heavily bloated idle hogs (>=160 MB idle >=5 min) every 75s
	// so normal baseline usage doesn't drift upward; Tier 2/3 accelerate reclamation.
	t := e.cfg.Thresholds
	minWS := t.TrimMinWorkingSetMB
	if minWS <= 0 {
		minWS = 35
	}
	idleReq := time.Duration(t.IdleMinutes * float64(time.Minute))
	if idleReq <= 0 {
		idleReq = 4 * time.Minute
	}
	procCooldown := time.Duration(t.CooldownMinutes * float64(time.Minute))
	if procCooldown <= 0 {
		procCooldown = 2 * time.Minute
	}
	globalInterval := 45 * time.Second
	maxBatch := 6

	if tier < 2 {
		if minWS < 160 {
			minWS = 160
		}
		if idleReq < 5*time.Minute {
			idleReq = 5 * time.Minute
		}
		if procCooldown < 8*time.Minute {
			procCooldown = 8 * time.Minute
		}
		globalInterval = 75 * time.Second
		maxBatch = 3
	} else if tier >= 3 {
		if minWS > 20 {
			minWS = 20
		}
		if idleReq > 90*time.Second {
			idleReq = 90 * time.Second
		}
		if procCooldown > 90*time.Second {
			procCooldown = 90 * time.Second
		}
		globalInterval = 30 * time.Second
		maxBatch = 12
	}

	if !e.lastSmartTrim.IsZero() && s.At.Sub(e.lastSmartTrim) < globalInterval {
		return false
	}

	var trimmedTargets []journal.Target
	var shrunkBytes uint64
	if e.cfg.Actions.TrimIdleWorkingSets {
		cands := e.trimCandidates(s.Procs, opts.TargetPID, family, s.At, minWS, idleReq, procCooldown, maxBatch)
		for _, p := range cands {
			if delta, err := proc.TrimWorkingSetMeasured(p.PID); err == nil {
				e.trimmedPIDs[p.PID] = s.At
				e.recordTrimObservation(p, delta, s.At)
				shrunkBytes += delta
				trimmedTargets = append(trimmedTargets, journal.Target{
					PID:          p.PID,
					Name:         p.Name,
					WorkingSetMB: float64(p.WorkingSet) / (1 << 20),
				})
			}
		}
	}

	purged := false
	flushed := false
	if tier >= 3 && e.cfg.Actions.PurgeStandby && s.Snap.HasPages &&
		float64(s.Snap.Pages.Standby)/(1<<20) >= t.StandbyPurgeMinMB &&
		s.At.Sub(e.lastPurge) >= 5*time.Minute {
		if err := winmem.FlushModified(); err == nil {
			flushed = true
		}
		if err := winmem.PurgeLowPriorityStandby(); err == nil {
			purged = true
			e.lastPurge = s.At
		}
	}

	if len(trimmedTargets) == 0 && !purged && !flushed {
		return false
	}
	e.lastSmartTrim = s.At

	shrunkMB := float64(shrunkBytes) / (1 << 20)
	// Also sample immediate physical availability gain cleanly without blocking ticks for 3s.
	availGainMB := 0.0
	if afterSnap, err := winmem.Collect(); err == nil && afterSnap.AvailPhys > s.Snap.AvailPhys {
		availGainMB = float64(afterSnap.AvailPhys-s.Snap.AvailPhys) / (1 << 20)
	}
	totalGainMB := availGainMB
	if shrunkMB > totalGainMB {
		totalGainMB = shrunkMB
	}

	b := statsOf(*a)
	action := "smart_schedule"
	if purged {
		action += "+purge_low_priority_standby"
	}
	e.record(journal.Entry{
		Kind:        "action",
		Action:      action,
		Reason:      fmt.Sprintf("%s（可用 %.1f%%），动态收敛 %d 个后台进程工作集", label, a.AvailPct, len(trimmedTargets)),
		Before:      &b,
		Targets:     trimmedTargets,
		ReclaimedMB: totalGainMB,
		Result:      "ok",
	})
	if e.lg != nil {
		e.lg.RecordDynamicReclaim(totalGainMB)
		if purged || tier >= 3 {
			e.lg.RecordTrim(availGainMB, true)
		}
	}
	return true
}

func (e *Engine) trimCandidates(
	procs []proc.Info,
	targetPID uint32,
	family map[uint32]bool,
	now time.Time,
	minWSMB float64,
	idleReq time.Duration,
	procCooldown time.Duration,
	limit int,
) []proc.Info {
	self := uint32(os.Getpid())
	minBytes := uint64(minWSMB * (1 << 20))
	cands := make([]proc.Info, 0, len(procs))
	for _, p := range procs {
		if targetPID != 0 && p.PID != targetPID {
			continue
		}
		if p.SessionID == 0 || p.PID == self || p.PID <= 4 || family[p.PID] || e.isProtected(p.Name) {
			continue
		}
		if p.WorkingSet < minBytes {
			continue
		}
		if e.lastCPUPct[p.PID] >= 2.0 {
			continue
		}
		since, ok := e.idleSince[p.PID]
		if !ok || now.Sub(since) < idleReq {
			continue
		}
		effCooldown := procCooldown
		if e.isReboundProne(p.Name) {
			effCooldown = procCooldown * 4
			if effCooldown < 30*time.Minute {
				effCooldown = 30 * time.Minute
			}
		}
		if last, ok := e.trimmedPIDs[p.PID]; ok && now.Sub(last) < effCooldown {
			continue
		}
		cands = append(cands, p)
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].WorkingSet != cands[j].WorkingSet {
			return cands[i].WorkingSet > cands[j].WorkingSet
		}
		return e.idleSince[cands[i].PID].Before(e.idleSince[cands[j].PID])
	})
	if limit > 0 && len(cands) > limit {
		cands = cands[:limit]
	}
	return cands
}

// OptimizeNow executes an immediate safe smart-scheduling pass on non-foreground idle processes
// and flushes modified/low-priority standby pages, reporting exact measured gains.
func (e *Engine) OptimizeNow() (OptimizeResult, error) {
	started := time.Now()
	s, err := e.collect(Options{})
	if err != nil {
		return OptimizeResult{}, err
	}
	a := e.assess(s)
	fgPID := proc.ForegroundPID()
	family := proc.FamilyPIDs(s.Procs, fgPID)
	selfPID := uint32(os.Getpid())
	const minBytes = 20 << 20

	var targets []proc.Info
	for _, p := range s.Procs {
		if p.SessionID == 0 || p.PID == selfPID || p.PID <= 4 || family[p.PID] || e.isProtected(p.Name) {
			continue
		}
		if p.WorkingSet < minBytes {
			continue
		}
		if e.lastCPUPct[p.PID] >= 2.0 {
			continue
		}
		targets = append(targets, p)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].WorkingSet > targets[j].WorkingSet })

	var res OptimizeResult
	var jTargets []journal.Target
	var totalShrunk uint64

	for _, p := range targets {
		st := e.softState[p.PID]
		st.Name = p.Name
		st.WorkingSet = p.WorkingSet
		st.Private = p.Private
		if st.ManagedAt.IsZero() {
			st.ManagedAt = started
		}
		if e.cfg.Actions.SoftMemoryPriority && st.MemoryPriority != proc.MemoryPriorityVeryLow {
			if err := proc.SetMemoryPriority(p.PID, proc.MemoryPriorityVeryLow); err == nil {
				st.MemoryPriority = proc.MemoryPriorityVeryLow
			}
		}
		if e.cfg.Actions.EcoQoSBackground {
			if !st.EcoQoS {
				if err := proc.SetEcoQoS(p.PID, true); err == nil {
					st.EcoQoS = true
				}
			}
			if !st.BelowNormal {
				if err := proc.SetBelowNormalPriority(p.PID, true); err == nil {
					st.BelowNormal = true
				}
			}
		}
		if st.MemoryPriority != 0 || st.EcoQoS || st.BelowNormal {
			e.softState[p.PID] = st
		}

		if delta, err := proc.TrimWorkingSetMeasured(p.PID); err == nil {
			e.trimmedPIDs[p.PID] = started
			e.recordTrimObservation(p, delta, started)
			res.TrimmedCount++
			totalShrunk += delta
			mb := float64(delta) / (1 << 20)
			if len(res.Targets) < 20 && mb >= 1 {
				res.Targets = append(res.Targets, OptimizeTarget{
					PID:         p.PID,
					Name:        p.Name,
					ReclaimedMB: mb,
				})
			}
			if len(jTargets) < 12 {
				jTargets = append(jTargets, journal.Target{
					PID:          p.PID,
					Name:         p.Name,
					WorkingSetMB: float64(p.WorkingSet) / (1 << 20),
				})
			}
		}
	}

	if err := winmem.FlushModified(); err == nil {
		res.ModifiedFlush = true
	}
	if a.AvailPct < 18 {
		if err := winmem.PurgeStandby(); err == nil {
			res.StandbyPurged = true
			e.lastPurge = started
		} else if err := winmem.PurgeLowPriorityStandby(); err == nil {
			res.StandbyPurged = true
			e.lastPurge = started
		}
	} else if err := winmem.PurgeLowPriorityStandby(); err == nil {
		res.StandbyPurged = true
		e.lastPurge = started
	}

	afterSnap, err := winmem.Collect()
	if err == nil && afterSnap.AvailPhys > s.Snap.AvailPhys {
		res.AvailGainMB = float64(afterSnap.AvailPhys-s.Snap.AvailPhys) / (1 << 20)
	}
	res.WSShrunkMB = float64(totalShrunk) / (1 << 20)
	res.DurationMS = time.Since(started).Milliseconds()

	recordedMB := res.AvailGainMB
	if res.WSShrunkMB > recordedMB {
		recordedMB = res.WSShrunkMB
	}
	b := statsOf(a)
	var af *journal.Stats
	if err == nil {
		afVal := statsOf(Assessment{Snap: afterSnap, AvailPct: afterSnap.AvailablePercent()})
		af = &afVal
	}
	e.record(journal.Entry{
		Kind:        "action",
		Action:      "manual_smart_optimize",
		Reason:      fmt.Sprintf("用户手动触发一键智能调度（压缩 %d 个后台进程工作集 %.0f MB，系统可用净增 %.0f MB）", res.TrimmedCount, res.WSShrunkMB, res.AvailGainMB),
		Before:      &b,
		After:       af,
		Targets:     jTargets,
		ReclaimedMB: recordedMB,
		Result:      "ok",
	})
	if e.lg != nil {
		e.lg.RecordDynamicReclaim(recordedMB)
		e.lg.RecordTrim(res.AvailGainMB, a.AvailPct < e.cfg.Thresholds.AvailableLowPercent)
		_ = e.lg.Flush()
	}
	return res, nil
}

func (e *Engine) recordTrimObservation(p proc.Info, shrunkBytes uint64, at time.Time) {
	if shrunkBytes < 8<<20 || p.PID == 0 {
		return
	}
	if e.lastTrimObs == nil {
		e.lastTrimObs = map[uint32]trimObservation{}
	}
	postWS := uint64(0)
	if p.WorkingSet > shrunkBytes {
		postWS = p.WorkingSet - shrunkBytes
	}
	e.lastTrimObs[p.PID] = trimObservation{
		At:          at,
		Name:        p.Name,
		PostTrimWS:  postWS,
		ShrunkBytes: shrunkBytes,
	}
}

func (e *Engine) evaluateTrimRebounds(s Sample) {
	if len(e.lastTrimObs) == 0 {
		return
	}
	if e.reboundCounts == nil {
		e.reboundCounts = map[string]int{}
	}
	alive := make(map[uint32]proc.Info, len(s.Procs))
	for _, p := range s.Procs {
		alive[p.PID] = p
	}
	for pid, obs := range e.lastTrimObs {
		p, ok := alive[pid]
		if !ok {
			delete(e.lastTrimObs, pid)
			continue
		}
		dt := s.At.Sub(obs.At)
		if dt > 5*time.Minute {
			delete(e.lastTrimObs, pid)
			continue
		}
		if obs.Checked {
			continue
		}
		if dt > 2*time.Minute {
			obs.Checked = true
			e.lastTrimObs[pid] = obs
			continue
		}
		if dt < 8*time.Second || e.lastCPUPct[pid] >= 2.0 {
			continue
		}
		reboundThreshold := obs.PostTrimWS + uint64(float64(obs.ShrunkBytes)*0.65)
		if p.WorkingSet >= reboundThreshold {
			obs.Checked = true
			e.lastTrimObs[pid] = obs
			key := strings.ToLower(strings.TrimFunc(p.Name, func(r rune) bool {
				return r < 32 || r == ' ' || r == '\t'
			}))
			if key == "" {
				continue
			}
			e.reboundCounts[key]++
			if e.reboundCounts[key] == 2 {
				e.record(journal.Entry{
					Kind:   "adaptive_calib",
					Action: "rebound_cooldown_calibrated",
					Reason: fmt.Sprintf("检测到 %s(%d) 在工作集收紧后 2 分钟内自动反扑回涨，已自动延长其硬 Trim 冷静期 4 倍并转为仅 EcoQoS + VeryLow 软约束，消除无效磁盘换页", p.Name, p.PID),
					Targets: []journal.Target{{
						PID:          p.PID,
						Name:         p.Name,
						WorkingSetMB: float64(p.WorkingSet) / (1 << 20),
					}},
				})
			}
		}
	}
}

func (e *Engine) isReboundProne(name string) bool {
	if len(e.reboundCounts) == 0 || name == "" {
		return false
	}
	return e.reboundCounts[strings.ToLower(strings.TrimSpace(name))] >= 2
}

// AdaptiveCooldownNames returns sorted process names that tripped automatic rebound calibration.
func (e *Engine) AdaptiveCooldownNames() []string {
	if len(e.reboundCounts) == 0 {
		return nil
	}
	var out []string
	for name, cnt := range e.reboundCounts {
		if cnt >= 2 {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
