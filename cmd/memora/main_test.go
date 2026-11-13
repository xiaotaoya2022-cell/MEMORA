package main

import (
	"memora/internal/engine"
	"strings"
	"testing"
)

func TestTruncateName(t *testing.T) {
	if got := truncateName("short.exe", 20); got != "short.exe" {
		t.Fatalf("unexpected short name: %q", got)
	}
	got := truncateName("Flydigi Space Station Plus Ultra.exe", 26)
	if len(got) != 26 || !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated length 26 with ..., got %q (len=%d)", got, len(got))
	}
}

func TestFormatTagNote(t *testing.T) {
	f := engine.PoolFinding{
		Tag:     "ConT",
		Desc:    "",
		Drivers: []string{"mtkwl6ex.sys"},
		Allocs:  100,
		Frees:   0,
	}
	got := formatTagNote(f)
	if !strings.Contains(got, "疑似驱动: mtkwl6ex.sys") || !strings.Contains(got, "0释放") {
		t.Fatalf("unexpected formatTagNote output: %q", got)
	}
}

func TestConclusionDetectsKernelLeakGrowth(t *testing.T) {
	deep := engine.DeepDiagnosis{
		Leak: engine.KernelLeakSummary{
			Suspected:      true,
			ProcessObjects: 180000,
		},
	}
	got := conclusion(0, 0, 0, 0, 0, 0, 10, 5, deep)
	if !strings.Contains(got, "180000") || !strings.Contains(got, "EPROCESS") {
		t.Fatalf("expected kernel leak conclusion, got: %q", got)
	}
}
