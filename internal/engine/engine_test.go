package engine

import (
	"testing"
	"time"

	"memora/internal/config"
	"memora/internal/ledger"
	"memora/internal/proc"
	"memora/internal/winmem"
)

func TestGroupProcessesAndLeakDetection(t *testing.T) {
	procs := []proc.Info{
		{PID: 10, Name: "ZCode.exe", WorkingSet: 100 << 20, Private: 200 << 20, PrivateWS: 80 << 20, Handles: 50},
		{PID: 11, Name: "zcode.exe", WorkingSet: 50 << 20, Private: 300 << 20, PrivateWS: 40 << 20, Handles: 30},
		{PID: 20, Name: "One.exe", WorkingSet: 20 << 20, Private: 100 << 20, PrivateWS: 15 << 20, Handles: 10},
	}
	groups := GroupProcesses(procs)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Count != 2 || groups[0].Private != 500<<20 || groups[0].Handles != 80 {
		t.Fatalf("unexpected first group: %+v", groups[0])
	}

	if !isProcessObjectLeakSuspected(KernelLeakSummary{
		ProcessObjects:  180000,
		ProcessHandles:  4000,
		ActiveProcesses: 390,
		ProcTagAllocs:   180000,
		ProcTagFrees:    2,
	}) {
		t.Fatalf("expected leak suspicion to trigger for 180k zombie processes")
	}

	if isProcessObjectLeakSuspected(KernelLeakSummary{
		ProcessObjects:  420,
		ProcessHandles:  900,
		ActiveProcesses: 390,
		ProcTagAllocs:   5000,
		ProcTagFrees:    4990,
	}) {
		t.Fatalf("false positive on healthy process count")
	}
}

func TestNUMADisparity(t *testing.T) {
	tight, ok := NUMADisparity([]winmem.NUMANode{
		{ID: "0", TotalMB: 15843, AvailableMB: 44},
		{ID: "1", TotalMB: 24287, AvailableMB: 14215},
	})
	if !ok || tight.ID != "0" {
		t.Fatalf("expected node 0 disparity, got ok=%v tight=%+v", ok, tight)
	}

	if _, ok := NUMADisparity([]winmem.NUMANode{
		{ID: "0", TotalMB: 16000, AvailableMB: 6000},
		{ID: "1", TotalMB: 16000, AvailableMB: 7000},
	}); ok {
		t.Fatalf("unexpected disparity on balanced NUMA nodes")
	}
}

func TestEngineSingleStepSmoke(t *testing.T) {
	eng := New(config.Default(), nil)
	a, err := eng.Step(Options{Act: false})
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if !a.ProcessComplete || a.ProcessCount == 0 || a.Private == 0 {
		t.Fatalf("expected complete process reading, got complete=%v count=%d priv=%d",
			a.ProcessComplete, a.ProcessCount, a.Private)
	}
}

func TestPlanPrioritizesLargestIdleNonProtected(t *testing.T) {
	cfg := config.Default()
	eng := New(cfg, nil)
	now := time.Now()
	idle := now.Add(-30 * time.Minute)
	eng.idleSince = map[uint32]time.Time{
		100: idle,
		101: idle,
		102: idle,
		103: idle,
		104: idle,
		105: now.Add(-2 * time.Minute), // not idle long enough
	}
	procs := []proc.Info{
		{PID: 100, Name: "vmmemWSL", WorkingSet: 2 << 30},      // protected
		{PID: 101, Name: "small.exe", WorkingSet: 220 << 20},    // 4th largest eligible -> excluded
		{PID: 102, Name: "medium.exe", WorkingSet: 400 << 20},   // 3rd
		{PID: 103, Name: "huge.exe", WorkingSet: 1200 << 20},    // 1st
		{PID: 104, Name: "large.exe", WorkingSet: 800 << 20},    // 2nd
		{PID: 105, Name: "recent.exe", WorkingSet: 1500 << 20},  // still active recently
	}
	a := Assessment{At: now, AvailPct: 8.5, Severity: 2}
	plan, ok := eng.plan(a, procs, map[uint32]bool{103: false})
	if !ok {
		t.Fatalf("expected action plan under pressure")
	}
	if len(plan.Trim) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(plan.Trim))
	}
	want := []uint32{103, 104, 102}
	for i, p := range plan.Trim {
		if p.PID != want[i] {
			t.Fatalf("target[%d] = PID %d (%s), want PID %d", i, p.PID, p.Name, want[i])
		}
	}
}

func TestPlanRespectsForegroundFamily(t *testing.T) {
	cfg := config.Default()
	eng := New(cfg, nil)
	now := time.Now()
	idle := now.Add(-40 * time.Minute)
	eng.idleSince = map[uint32]time.Time{201: idle, 202: idle}
	procs := []proc.Info{
		{PID: 201, Name: "anticheat.exe", WorkingSet: 900 << 20},
		{PID: 202, Name: "other_idle.exe", WorkingSet: 300 << 20},
	}
	family := map[uint32]bool{201: true}
	plan, ok := eng.plan(Assessment{At: now, AvailPct: 7.0, Severity: 2}, procs, family)
	if !ok || len(plan.Trim) != 1 || plan.Trim[0].PID != 202 {
		t.Fatalf("expected family PID 201 skipped and only PID 202 targeted, got ok=%v plan=%+v", ok, plan)
	}
}

func TestDetectOrphansTracksExitedParent(t *testing.T) {
	dir := t.TempDir()
	lg, err := ledger.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	eng := New(config.Default(), nil)
	eng.SetLedger(lg)

	// First tick: parent 5000 (MyApp.exe) and child 5001 (MyHelper.exe) and shell 6000 (explorer.exe) -> child 6001
	t1 := []proc.Info{
		{PID: 5000, ParentPID: 6000, SessionID: 1, CreateTime: 100, Name: "MyApp.exe", Private: 200 << 20},
		{PID: 5001, ParentPID: 5000, SessionID: 1, CreateTime: 110, Name: "MyHelper.exe", Private: 150 << 20},
		{PID: 6000, ParentPID: 4, SessionID: 1, CreateTime: 50, Name: "explorer.exe", Private: 100 << 20},
		{PID: 6001, ParentPID: 6000, SessionID: 1, CreateTime: 120, Name: "NormalApp.exe", Private: 200 << 20},
	}
	if got := eng.detectOrphans(t1, nil); len(got) != 0 {
		t.Fatalf("expected 0 orphans initially, got %+v", got)
	}

	// Second tick: MyApp (5000) exited, explorer (6000) also gone, MyHelper (5001) & NormalApp (6001) remain.
	t2 := []proc.Info{
		{PID: 5001, ParentPID: 5000, SessionID: 1, CreateTime: 110, Name: "MyHelper.exe", Private: 150 << 20},
		{PID: 6001, ParentPID: 6000, SessionID: 1, CreateTime: 120, Name: "NormalApp.exe", Private: 200 << 20},
	}
	orphans := eng.detectOrphans(t2, nil)
	eng.recordNewOrphans(orphans)
	if len(orphans) != 1 || orphans[0].PID != 5001 {
		t.Fatalf("expected only PID 5001 flagged as orphan (shell host child 6001 excluded), got %+v", orphans)
	}
	if snap := lg.Snapshot(); snap.OrphansDetected != 1 {
		t.Fatalf("expected OrphansDetected=1 in ledger, got %d", snap.OrphansDetected)
	}
}

func TestComputeBreakdownSumsToTotal(t *testing.T) {
	snap := winmem.Snapshot{
		TotalPhys:   40 << 30,
		AvailPhys:   4 << 30,
		KernelTotal: 7 << 30,
	}
	deep := DeepDiagnosis{
		Extras: winmem.SystemExtras{VidBytes: 6 << 30},
		Leak: KernelLeakSummary{
			Suspected:       true,
			RelatedPoolByte: 3 << 30,
		},
	}
	bd := ComputeBreakdown(snap, 4<<30, deep)
	if bd.InUseBytes != 36<<30 {
		t.Fatalf("InUseBytes = %d, want %d", bd.InUseBytes, uint64(36)<<30)
	}
	sumBuckets := bd.AppPrivateWSBytes + bd.VidBytes + bd.ZombieLeakBytes + bd.KernelDriverBytes
	if sumBuckets != bd.InUseBytes {
		t.Fatalf("buckets sum = %d, want InUseBytes = %d (bd=%+v)", sumBuckets, bd.InUseBytes, bd)
	}
	if bd.ZombieLeakBytes <= 3<<30 {
		t.Fatalf("expected ZombieLeakBytes to capture leaked pool + unaccounted PFN/page-tables, got %+v", bd)
	}
}

func TestComputeSmartTierBoundaries(t *testing.T) {
	cases := []struct {
		avail float64
		numa  bool
		tier  int
	}{
		{35, false, 1},
		{22, false, 2},
		{13.9, false, 3},
		{35, true, 3},
	}
	for _, tc := range cases {
		got, _ := computeSmartTier(tc.avail, tc.numa)
		if got != tc.tier {
			t.Fatalf("computeSmartTier(avail=%.1f, numa=%v) = %d, want %d", tc.avail, tc.numa, got, tc.tier)
		}
	}
}

func TestUpdateConfigRestoresDisabledControls(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	eng := New(config.Default(), nil)
	now := time.Now()
	eng.softState[999991] = softControl{
		MemoryPriority: proc.MemoryPriorityVeryLow,
		EcoQoS:         true,
		BelowNormal:    true,
		StormThrottled: false,
		Name:           "bg_test.exe",
		WorkingSet:     120 << 20,
		Private:        150 << 20,
		ManagedAt:      now.Add(-90 * time.Second),
	}
	eng.softState[999992] = softControl{
		MemoryPriority: proc.MemoryPriorityLow,
		EcoQoS:         true,
		BelowNormal:    true,
		StormThrottled: true,
		Name:           "spawner_test.exe",
		WorkingSet:     80 << 20,
		Private:        90 << 20,
		ManagedAt:      now.Add(-30 * time.Second),
	}

	listed := eng.ManagedProcesses()
	if len(listed) != 2 || !listed[0].StormThrottled || listed[0].PID != 999992 {
		t.Fatalf("expected StormThrottled process first, got %+v", listed)
	}
	if listed[1].MemoryPriority != "VeryLow" {
		t.Fatalf("expected VeryLow label on PID 999991, got %+v", listed[1])
	}

	if err := eng.UpdateConfig(func(c *config.Config) {
		c.Actions.SoftMemoryPriority = false
	}); err != nil {
		t.Fatalf("UpdateConfig(SoftMemoryPriority=false): %v", err)
	}
	if eng.softState[999991].MemoryPriority != 0 {
		t.Fatalf("expected MemoryPriority cleared when SoftMemoryPriority disabled")
	}

	if err := eng.UpdateConfig(func(c *config.Config) {
		c.Actions.EcoQoSBackground = false
		c.Actions.SuppressSpawnStorm = false
	}); err != nil {
		t.Fatalf("UpdateConfig(all soft off): %v", err)
	}
	if len(eng.softState) != 0 {
		t.Fatalf("expected all softState entries cleared after disabling all soft controls, got %+v", eng.softState)
	}
}

func TestTrimCandidatesOrderingAndCooldown(t *testing.T) {
	eng := New(config.Default(), nil)
	now := time.Now()
	idle := now.Add(-10 * time.Minute)
	eng.idleSince = map[uint32]time.Time{
		301: idle,
		302: idle,
		303: idle,
		304: idle,
		305: now.Add(-30 * time.Second), // too recently active
	}
	eng.trimmedPIDs[304] = now.Add(-30 * time.Second) // inside cooldown

	procs := []proc.Info{
		{PID: 301, SessionID: 1, Name: "alpha.exe", WorkingSet: 100 << 20},
		{PID: 302, SessionID: 1, Name: "beta.exe", WorkingSet: 250 << 20},
		{PID: 303, SessionID: 1, Name: "fg.exe", WorkingSet: 500 << 20},
		{PID: 304, SessionID: 1, Name: "cooled.exe", WorkingSet: 400 << 20},
		{PID: 305, SessionID: 1, Name: "busy_idle.exe", WorkingSet: 300 << 20},
	}
	cands := eng.trimCandidates(procs, 0, map[uint32]bool{303: true}, now, 35, 4*time.Minute, 2*time.Minute, 5)
	if len(cands) != 2 || cands[0].PID != 302 || cands[1].PID != 301 {
		t.Fatalf("expected [302, 301] sorted by descending WS, got %+v", cands)
	}
}

func TestStormProtectionAndHandleLeakers(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	eng := New(config.Default(), nil)
	for _, protected := range []string{"msedge.exe", "ZCode.exe", "Antigravity.exe", "chrome.exe", "language_server.exe", "OmniCode.exe"} {
		if !eng.isStormProtected(protected) {
			t.Fatalf("expected %s to be storm-protected", protected)
		}
	}
	if eng.isStormProtected("LnvSvcFdn.exe") {
		t.Fatalf("did not expect LnvSvcFdn.exe to be storm-protected")
	}
	for _, host := range []string{"LnvSvcFdn.exe", "ESurfingSvr.exe", "wetype_update.exe"} {
		if !isKnownStormHost(host) {
			t.Fatalf("expected %s to be recognized as known storm host", host)
		}
	}

	if !eng.AddSuppressedSpawner("bad_spawner.exe") || !eng.isUserSuppressedSpawner("bad_spawner.exe") {
		t.Fatalf("expected bad_spawner.exe added to persistent storm list")
	}
	if !eng.RemoveSuppressedSpawner("bad_spawner.exe") || eng.isUserSuppressedSpawner("bad_spawner.exe") {
		t.Fatalf("expected bad_spawner.exe removed from persistent storm list")
	}
	if eng.learnSuppressedSpawner("language_server.exe") {
		t.Fatalf("must not learn storm-protected process language_server.exe")
	}

	newly, total := eng.SuppressActiveSpawnersNow()
	if newly < 0 || total < 0 {
		t.Fatalf("unexpected negative suppression counts: newly=%d total=%d", newly, total)
	}

	leakers := findHandleLeakers([]ProcGroup{
		{Name: "System", Count: 1, Handles: 50000},
		{Name: "msedge.exe", Count: 18, Handles: 15000},
		{Name: "ESurfingSvr.exe", Count: 1, Handles: 19515},
		{Name: "small.exe", Count: 1, Handles: 1200},
	})
	if len(leakers) != 1 || leakers[0].Name != "ESurfingSvr.exe" {
		t.Fatalf("expected only ESurfingSvr.exe as handle leaker, got %+v", leakers)
	}
}

func TestPostRebootSharedWSNotCountedAsZombieLeak(t *testing.T) {
	if isProcessObjectLeakSuspected(KernelLeakSummary{
		ProcessObjects:  1946,
		ProcessHandles:  3772,
		ActiveProcesses: 327,
		ProcTagAllocs:   1947,
		ProcTagFrees:    1,
		RelatedPoolByte: 26 << 20,
	}) {
		t.Fatalf("expected moderate post-boot handle turnover (1946 objs, 26MB pool) not to flag severe zombie leak")
	}
	if isProcessObjectLeakSuspected(KernelLeakSummary{
		ProcessObjects:  4936,
		ProcessHandles:  1400,
		ActiveProcesses: 295,
		ProcTagAllocs:   4936,
		ProcTagFrees:    100,
		RelatedPoolByte: 90 << 20,
	}) {
		t.Fatalf("expected ~4936 post-build cached process objects (<10000 excess, <256MB pool) not to trigger false alarm")
	}

	snap := winmem.Snapshot{
		TotalPhys:   40 << 30,
		AvailPhys:   30 << 30,
		KernelTotal: 1500 << 20,
	}
	deep := DeepDiagnosis{
		Leak: KernelLeakSummary{
			Suspected:       true,
			ProcessObjects:  1946,
			ActiveProcesses: 327,
			RelatedPoolByte: 26 << 20,
		},
	}
	bd := ComputeBreakdown(snap, 4<<30, deep)
	if bd.ZombieLeakBytes > 256<<20 {
		t.Fatalf("expected bounded zombie bytes for 1619 excess objects, got %.2f GB", float64(bd.ZombieLeakBytes)/(1<<30))
	}
}

func TestAdaptiveReboundCooldownCalibration(t *testing.T) {
	eng := New(config.Default(), nil)
	base := time.Now()
	pBefore := proc.Info{
		PID:        4242,
		SessionID:  1,
		Name:       "ElectronCacheApp.exe",
		WorkingSet: 240 << 20,
	}
	eng.idleSince[pBefore.PID] = base.Add(-20 * time.Minute)
	eng.lastCPUPct[pBefore.PID] = 0.2

	// Simulate two trim+rebound cycles within 2 minutes while still idle.
	for i := 0; i < 2; i++ {
		tTrim := base.Add(time.Duration(i*3) * time.Minute)
		eng.trimmedPIDs[pBefore.PID] = tTrim
		eng.recordTrimObservation(pBefore, 80<<20, tTrim)
		eng.evaluateTrimRebounds(Sample{
			At: tTrim.Add(45 * time.Second),
			Procs: []proc.Info{{
				PID:        pBefore.PID,
				SessionID:  1,
				Name:       pBefore.Name,
				WorkingSet: 230 << 20, // rebounded >65% of the 80MB shrunk from 160MB post-trim
			}},
		})
	}

	if !eng.isReboundProne(pBefore.Name) {
		t.Fatalf("expected %s to be marked rebound-prone after 2 rebounds", pBefore.Name)
	}
	names := eng.AdaptiveCooldownNames()
	if len(names) != 1 || names[0] != "electroncacheapp.exe" {
		t.Fatalf("expected adaptive cooldown name recorded, got %+v", names)
	}

	// Normal 8-minute cooldown would allow re-trimming after 10 minutes, but rebound-prone should block until >=30m.
	checkAt := base.Add(3*time.Minute + 15*time.Minute)
	cands := eng.trimCandidates([]proc.Info{pBefore}, 0, map[uint32]bool{}, checkAt, 35, 4*time.Minute, 8*time.Minute, 5)
	if len(cands) != 0 {
		t.Fatalf("expected rebound-prone process skipped during extended 32m cooldown, got %+v", cands)
	}
	laterAt := base.Add(3*time.Minute + 35*time.Minute)
	candsAfter := eng.trimCandidates([]proc.Info{pBefore}, 0, map[uint32]bool{}, laterAt, 35, 4*time.Minute, 8*time.Minute, 5)
	if len(candsAfter) != 1 || candsAfter[0].PID != pBefore.PID {
		t.Fatalf("expected rebound-prone process eligible again after >=32m cooldown, got %+v", candsAfter)
	}
}

func TestAdaptiveBreakdownSlot2SwitchesByWorkload(t *testing.T) {
	eng := New(config.Default(), nil)
	snap := winmem.Snapshot{
		TotalPhys:   16 << 30,
		AvailPhys:   8 << 30,
		KernelTotal: 2 << 30,
	}
	procs := []proc.Info{
		{PID: 4, SessionID: 0, Name: "System", PrivateWS: 1 << 30},
		{PID: 500, SessionID: 1, Name: "IdleApp.exe", PrivateWS: 2 << 30},
		{PID: 501, SessionID: 1, Name: "BusyGame.exe", PrivateWS: 1 << 30},
	}
	eng.lastCPUPct[501] = 18.5 // busy active app should stay in Slot 1 ActiveAppWSBytes

	bdIdleMode := eng.ComputeAdaptiveBreakdown(snap, 4<<30, procs, DeepDiagnosis{})
	if bdIdleMode.Slot2Mode != "idle_bg" {
		t.Fatalf("expected Slot2Mode idle_bg when VidBytes=0, got %q", bdIdleMode.Slot2Mode)
	}
	if bdIdleMode.IdleAppWSBytes != 2<<30 || bdIdleMode.ActiveAppWSBytes != 2<<30 {
		t.Fatalf("expected 2GB idle / 2GB active, got idle=%d active=%d", bdIdleMode.IdleAppWSBytes, bdIdleMode.ActiveAppWSBytes)
	}
	if len(bdIdleMode.TopIdleApps) == 0 || bdIdleMode.TopIdleApps[0] != "IdleApp.exe (2.0 GB)" {
		t.Fatalf("unexpected TopIdleApps: %+v", bdIdleMode.TopIdleApps)
	}

	bdVidMode := eng.ComputeAdaptiveBreakdown(snap, 4<<30, procs, DeepDiagnosis{
		Extras: winmem.SystemExtras{VidBytes: 2 << 30},
	})
	if bdVidMode.Slot2Mode != "vid" {
		t.Fatalf("expected Slot2Mode vid when VidBytes >= 64MB, got %q", bdVidMode.Slot2Mode)
	}
	if bdVidMode.VidBytes != 2<<30 {
		t.Fatalf("expected VidBytes=2GB, got %d", bdVidMode.VidBytes)
	}
}

func TestWhitelistAndIMEProtection(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	eng := New(config.Default(), nil)

	// Input method servers must always be protected so typing never freezes.
	for _, ime := range []string{"wetype_server.exe", "ctfmon.exe", "TextInputHost.exe"} {
		if !eng.isProtected(ime) {
			t.Fatalf("expected IME %s to be protected", ime)
		}
	}

	if eng.isProtected("OmniCode.exe") {
		t.Fatalf("did not expect OmniCode.exe to be protected before whitelisting")
	}
	if !eng.AddWhitelist("OmniCode.exe") {
		t.Fatalf("expected AddWhitelist to succeed")
	}
	if !eng.isProtected("omnicode.exe") {
		t.Fatalf("expected OmniCode.exe to be protected after whitelisting")
	}
	if !eng.RemoveWhitelist("OmniCode.exe") {
		t.Fatalf("expected RemoveWhitelist to succeed")
	}
	if eng.isProtected("OmniCode.exe") {
		t.Fatalf("expected OmniCode.exe to no longer be protected after removal")
	}
}

func TestComputeBreakdownCapsDeadProcessOverhead(t *testing.T) {
	snap := winmem.Snapshot{
		TotalPhys:   40 << 30,
		AvailPhys:   7 << 30,
		KernelTotal: 5 << 30,
	}
	deep := DeepDiagnosis{
		Extras: winmem.SystemExtras{VidBytes: 6 << 30},
		Leak: KernelLeakSummary{
			Suspected:       true,
			ProcessObjects:  222435,
			ActiveProcesses: 423,
			RelatedPoolByte: 1755805152,
		},
	}
	bd := ComputeBreakdown(snap, 7<<30, deep)
	// With 222k dead EPROCESS stubs, zombie leak should reflect pool (~1.63 GiB) + ~4 KB/obj (~0.85 GiB),
	// not swallow 15+ GiB of Shared Working Set.
	if bd.ZombieLeakBytes > 4<<30 {
		t.Fatalf("expected ZombieLeakBytes <= 4 GiB, got %.2f GiB", float64(bd.ZombieLeakBytes)/(1<<30))
	}
	sumBuckets := bd.AppPrivateWSBytes + bd.VidBytes + bd.ZombieLeakBytes + bd.KernelDriverBytes
	if sumBuckets != bd.InUseBytes {
		t.Fatalf("buckets sum = %d, want InUseBytes = %d", sumBuckets, bd.InUseBytes)
	}
}

func TestConflictMitigatedAndExplorerThreshold(t *testing.T) {
	if computeConflictMitigated(nil) {
		t.Fatalf("expected empty services not to report mitigated")
	}
	if computeConflictMitigated([]winmem.ServiceState{
		{Name: "lnvscenter", Exists: true, Running: true},
		{Name: "lrtp", Exists: true, Running: false},
	}) {
		t.Fatalf("expected running lnvscenter not to report mitigated")
	}
	if !computeConflictMitigated([]winmem.ServiceState{
		{Name: "lnvscenter", Exists: true, Running: false},
		{Name: "LenovoPcManagerService", Exists: true, Running: false},
		{Name: "sysdiag", Exists: true, Running: true},
	}) {
		t.Fatalf("expected stopped conflict drivers/services to report mitigated even with standalone sysdiag running")
	}

	leakers := findHandleLeakers([]ProcGroup{
		{Name: "explorer.exe", Count: 1, Handles: 9200},
		{Name: "small.exe", Count: 1, Handles: 500},
	})
	if len(leakers) != 0 {
		t.Fatalf("expected explorer.exe at 9200 handles not to trigger leaker alert, got %+v", leakers)
	}
	leakersHigh := findHandleLeakers([]ProcGroup{
		{Name: "explorer.exe", Count: 1, Handles: 17500},
	})
	if len(leakersHigh) != 1 || leakersHigh[0].Name != "explorer.exe" {
		t.Fatalf("expected explorer.exe at 17500 handles to trigger leaker alert, got %+v", leakersHigh)
	}
}

func TestCaptureProbeMetricsAndDiffShrunk(t *testing.T) {
	s := Sample{
		At: time.Now(),
		Snap: winmem.Snapshot{
			TotalPhys:   32 << 30,
			AvailPhys:   10 << 30,
			SystemCache: 1500 << 20,
			HasPages:    true,
			Pages: winmem.Pages{
				Standby:  4 << 30,
				Modified: 128 << 20,
				Free:     6 << 30,
			},
		},
		WS:        8 << 30,
		PrivateWS: 5 << 30,
		Private:   5 << 30,
		Procs: []proc.Info{
			{PID: 100, Name: "svchost.exe", SessionID: 0, WorkingSet: 600 << 20},
			{PID: 200, Name: "app.exe", SessionID: 1, WorkingSet: 200 << 20},
		},
	}
	pm := CaptureProbeMetrics(s, false)
	if pm.SharedWSBytes != 3<<30 {
		t.Fatalf("SharedWSBytes = %d, want %d", pm.SharedWSBytes, uint64(3<<30))
	}
	if pm.SystemCacheBytes != 1500<<20 || pm.StandbyBytes != 4<<30 {
		t.Fatalf("unexpected page cache metrics: %+v", pm)
	}

	after := []proc.Info{
		{PID: 100, Name: "svchost.exe", SessionID: 0, WorkingSet: 120 << 20},
		{PID: 200, Name: "app.exe", SessionID: 1, WorkingSet: 198 << 20},
	}
	deltas := diffShrunkProcesses(s.Procs, after, 10)
	if len(deltas) != 1 || deltas[0].PID != 100 || deltas[0].ShrunkMB < 470 {
		t.Fatalf("expected svchost.exe ~480MB shrunk delta, got %+v", deltas)
	}
	if !isProtectedCoreSession0("csrss.exe") || isProtectedCoreSession0("svchost.exe") {
		t.Fatalf("unexpected Session 0 protection classification")
	}
}

func TestComputeBreakdownDarkMemoryWithSharedWSAndStormNoPingPong(t *testing.T) {
	// Simulate 4-day telemetry state: InUse=27.30 GiB, PrivWS=7.19 GiB, SharedWS=5.68 GiB,
	// KernelTotal=3.49 GiB, ProcessObjects=102467 (401 active), RelatedPoolByte=780 MiB.
	snap := winmem.Snapshot{
		TotalPhys:   uint64(39*1024+195) << 20,
		AvailPhys:   uint64(11*1024+911) << 20,
		KernelTotal: uint64(3*1024+502) << 20,
	}
	deep := DeepDiagnosis{
		SharedWSBytes: 5680 << 20,
		Leak: KernelLeakSummary{
			Suspected:       true,
			ProcessObjects:  102467,
			ActiveProcesses: 401,
			RelatedPoolByte: 780 << 20,
		},
	}
	bd := ComputeBreakdown(snap, uint64(7*1024+195)<<20, deep)
	// True dark EPROCESS memory (~10.95 GiB) + pool (~0.76 GiB) ~= 11.7 GiB.
	if bd.ZombieLeakBytes < 10<<30 || bd.ZombieLeakBytes > 13<<30 {
		t.Fatalf("expected ZombieLeakBytes around 11.7 GiB with SharedWS isolated, got %.2f GiB", float64(bd.ZombieLeakBytes)/(1<<30))
	}
	// SharedWS must stay inside KernelDriverBytes (slot 4), not pushed into slot 3.
	if bd.KernelDriverBytes < deep.SharedWSBytes {
		t.Fatalf("expected KernelDriverBytes >= SharedWSBytes, got %.2f GiB", float64(bd.KernelDriverBytes)/(1<<30))
	}

	// Verify restoreEligibleQoS does NOT un-throttle a background StormThrottled process on CPU burst alone.
	eng := New(config.Default(), nil)
	eng.softState[9001] = softControl{
		Name:           "Nexus.exe",
		MemoryPriority: proc.MemoryPriorityLow,
		EcoQoS:         true,
		BelowNormal:    true,
		StormThrottled: true,
	}
	eng.lastCPUPct[9001] = 15.0
	procs := []proc.Info{
		{PID: 100, Name: "Code.exe", SessionID: 1},
		{PID: 9001, Name: "Nexus.exe", SessionID: 1, WorkingSet: 100 << 20},
	}
	eng.restoreEligibleQoS(procs, map[uint32]bool{100: true}, 2.0)
	if _, stillThrottled := eng.softState[9001]; !stillThrottled {
		t.Fatalf("expected background StormThrottled process 9001 to remain throttled even during background CPU burst")
	}
	// Moving PID 9001 into foreground family MUST restore it.
	eng.restoreEligibleQoS(procs, map[uint32]bool{9001: true}, 2.0)
	if _, stillThrottled := eng.softState[9001]; stillThrottled {
		t.Fatalf("expected foreground StormThrottled process 9001 to be restored immediately")
	}
}
