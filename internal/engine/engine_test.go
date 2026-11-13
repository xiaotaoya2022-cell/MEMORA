package engine

import (
	"testing"
	"time"

	"memora/internal/config"
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
		105: now.Add(-5 * time.Minute), // not idle long enough
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
	plan, ok := eng.plan(a, procs)
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
