package proc

import (
	"os"
	"strings"
	"testing"
)

func TestListAllFindsSelfAndSystem(t *testing.T) {
	procs, complete, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if !complete {
		t.Fatalf("expected SystemProcessInformation complete=true on Windows")
	}
	if len(procs) < 10 {
		t.Fatalf("expected at least 10 processes, got %d", len(procs))
	}

	selfPID := uint32(os.Getpid())
	self, ok := Find(procs, selfPID)
	if !ok {
		t.Fatalf("current process PID %d not found in ListAll", selfPID)
	}
	if self.WorkingSet == 0 || self.Private == 0 || self.Handles == 0 {
		t.Fatalf("unexpected zero counters for self: %+v", self)
	}
	if self.PrivateWS > self.WorkingSet {
		t.Fatalf("PrivateWS (%d) should not exceed WorkingSet (%d)", self.PrivateWS, self.WorkingSet)
	}

	// Cross-check against OpenProcess + GetProcessMemoryInfo for self.
	direct, ok := read(selfPID)
	if !ok {
		t.Fatalf("direct read(%d) failed", selfPID)
	}
	if !strings.EqualFold(self.Name, direct.Name) {
		t.Fatalf("process name mismatch: %q vs %q", self.Name, direct.Name)
	}

	// System (PID 4) should also be visible via SystemProcessInformation.
	sys, ok := Find(procs, 4)
	if !ok {
		t.Fatalf("System (PID 4) not returned by ListAll")
	}
	if sys.Handles == 0 {
		t.Fatalf("expected System (PID 4) to have handles > 0")
	}
}

func TestQoSAndFamilyHelpers(t *testing.T) {
	pid := uint32(os.Getpid())
	orig, err := GetMemoryPriority(pid)
	if err != nil {
		t.Fatalf("GetMemoryPriority failed: %v", err)
	}
	if orig == 0 {
		t.Fatalf("expected non-zero original memory priority")
	}
	if err := SetMemoryPriority(pid, MemoryPriorityLow); err != nil {
		t.Fatalf("SetMemoryPriority(Low) failed: %v", err)
	}
	got, err := GetMemoryPriority(pid)
	if err != nil || got != MemoryPriorityLow {
		t.Fatalf("expected MemoryPriorityLow (%d), got %d (err=%v)", MemoryPriorityLow, got, err)
	}
	if err := SetMemoryPriority(pid, orig); err != nil {
		t.Fatalf("restore MemoryPriority failed: %v", err)
	}

	if err := SetEcoQoS(pid, true); err != nil {
		t.Fatalf("SetEcoQoS(true) failed: %v", err)
	}
	if err := SetEcoQoS(pid, false); err != nil {
		t.Fatalf("SetEcoQoS(false) failed: %v", err)
	}
	if err := SetBelowNormalPriority(pid, true); err != nil {
		t.Fatalf("SetBelowNormalPriority(true) failed: %v", err)
	}
	if err := SetBelowNormalPriority(pid, false); err != nil {
		t.Fatalf("SetBelowNormalPriority(false) failed: %v", err)
	}

	if _, err := TrimWorkingSetMeasured(pid); err != nil {
		t.Fatalf("TrimWorkingSetMeasured(self) failed: %v", err)
	}
	if !isFullscreenBusyExempt(0, "") || !isFullscreenBusyExempt(123, "wallpaper64.exe") || !isFullscreenBusyExempt(124, "explorer.exe") {
		t.Fatalf("expected desktop/wallpaper hosts to be exempt from QUNS_BUSY lock")
	}
	if isFullscreenBusyExempt(456, "cs2.exe") {
		t.Fatalf("did not expect game executable to be exempt from QUNS_BUSY")
	}

	state, _ := IsFullscreenOrBusy()
	if state == 0 {
		t.Log("SHQueryUserNotificationState returned 0 (desktop session not active or unavailable)")
	}
	_ = VisibleWindowPIDs()

	procs := []Info{
		{PID: 10, ParentPID: 4, Name: "explorer.exe"},
		{PID: 20, ParentPID: 10, Name: "steam.exe"},
		{PID: 30, ParentPID: 20, Name: "game.exe"},
		{PID: 40, ParentPID: 30, Name: "anticheat.exe"},
		{PID: 50, ParentPID: 40, Name: "crashhandler.exe"},
		{PID: 60, ParentPID: 10, Name: "background.exe"},
	}
	fam := FamilyPIDs(procs, 30)
	for _, want := range []uint32{20, 30, 40, 50} {
		if !fam[want] {
			t.Fatalf("expected PID %d inside foreground family: %+v", want, fam)
		}
	}
	if fam[10] || fam[60] {
		t.Fatalf("shell host or unrelated child should not be protected: %+v", fam)
	}
}
