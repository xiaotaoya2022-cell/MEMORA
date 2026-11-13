package winmem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectAndKernelDiagnostics(t *testing.T) {
	snap, err := Collect()
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if snap.TotalPhys == 0 || snap.PageSize == 0 || snap.Commit == 0 {
		t.Fatalf("unexpected zero fields in snapshot: %+v", snap)
	}

	tags, err := ReadPoolTags()
	if err != nil {
		t.Fatalf("ReadPoolTags failed: %v", err)
	}
	if len(tags) < 50 {
		t.Fatalf("expected many pool tags, got %d", len(tags))
	}
	foundProc := false
	for _, pt := range tags {
		if pt.Tag == "Proc" {
			foundProc = true
			break
		}
	}
	if !foundProc {
		t.Fatalf("expected 'Proc' tag in ReadPoolTags")
	}

	objs, err := ReadObjectTypes()
	if err != nil {
		t.Fatalf("ReadObjectTypes failed: %v", err)
	}
	foundProcObj := false
	for _, o := range objs {
		if o.Name == "Process" && o.Objects > 0 {
			foundProcObj = true
			break
		}
	}
	if !foundProcObj {
		t.Fatalf("expected 'Process' object type with Objects > 0")
	}

	drivers, err := ListDrivers()
	if err != nil {
		t.Fatalf("ListDrivers failed: %v", err)
	}
	if len(drivers) < 10 {
		t.Fatalf("expected loaded drivers, got %d", len(drivers))
	}

	extras := ReadSystemExtras(snap.PageSize)
	if extras.HasNUMA && len(extras.NUMANodes) == 0 {
		t.Fatalf("HasNUMA is true but NUMANodes empty")
	}
}

func TestMatchPoolTagsToDrivers(t *testing.T) {
	tmp := t.TempDir()
	fwdPath := filepath.Join(tmp, "drv_fwd.sys")
	revPath := filepath.Join(tmp, "drv_rev.sys")
	// Forward ASCII "AbcD", little-endian immediate of 'XyzW' -> "WzyX"
	if err := os.WriteFile(fwdPath, []byte("xxAbcDyy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(revPath, []byte("12WzyX34"), 0o644); err != nil {
		t.Fatal(err)
	}

	hits := MatchPoolTagsToDrivers(
		[]string{"AbcD", "XyzW", "None"},
		[]DriverModule{
			{Name: "drv_fwd.sys", Path: fwdPath},
			{Name: "drv_rev.sys", Path: revPath},
			{Name: "ntoskrnl.exe", Path: fwdPath}, // core image skipped
		},
	)
	if len(hits["AbcD"]) != 1 || hits["AbcD"][0] != "drv_fwd.sys" {
		t.Fatalf("unexpected hit for AbcD: %+v", hits["AbcD"])
	}
	if len(hits["XyzW"]) != 1 || hits["XyzW"][0] != "drv_rev.sys" {
		t.Fatalf("unexpected hit for XyzW: %+v", hits["XyzW"])
	}
	if len(hits["None"]) != 0 {
		t.Fatalf("expected no hits for None, got %+v", hits["None"])
	}
}
