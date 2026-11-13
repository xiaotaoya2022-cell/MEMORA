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
