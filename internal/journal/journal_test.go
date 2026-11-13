package journal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJournalWriteAndRotate(t *testing.T) {
	dir := t.TempDir()
	jr, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer jr.Close()
	jr.limit = 128

	for i := 0; i < 4; i++ {
		if err := jr.Write(Entry{
			Kind:   "action",
			Action: "trim_idle_working_sets",
			Reason: strings.Repeat("r", 80),
			Result: "ok",
		}); err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "journal.jsonl")); err != nil {
		t.Fatalf("expected active journal file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "journal.jsonl.1")); err != nil {
		t.Fatalf("expected rotated backup journal file: %v", err)
	}
}
