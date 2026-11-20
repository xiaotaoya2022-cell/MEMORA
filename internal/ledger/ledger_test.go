package ledger

import (
	"testing"
	"time"
)

func TestLedgerPersistenceAndRates(t *testing.T) {
	dir := t.TempDir()
	lg, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := lg.StartSession(); err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	now := time.Now()
	lg.RecordSample(now, 10*time.Second, true, true, 12000, 8500)
	lg.RecordSample(now.Add(10*time.Second), 10*time.Second, true, false, 11400, 8100)
	lg.RecordSample(now.Add(20*time.Second), 10*time.Second, false, false, 13000, 9000)
	lg.RecordSoft(2, 1)
	lg.RecordOrphans(3)
	lg.RecordDynamicReclaim(512)
	lg.RecordStormSuppression(4)
	lg.RecordTrim(420, true)
	lg.RecordBaselineExtras(now, 6144, 180000, 1200, "180k zombies")
	lg.RecordReboundProc("chrome.exe", 3)
	if err := lg.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("Re-open failed: %v", err)
	}
	s := reopened.Snapshot()
	if s.Sessions != 1 || s.TotalSamples != 3 || s.RestrainedSamples != 2 || s.FullscreenSkips != 1 {
		t.Fatalf("unexpected sample metrics: %+v", s)
	}
	if s.SoftDowngrades != 2 || s.SoftRestores != 1 || s.OrphansDetected != 3 || s.DynamicReclaimedMB != 512 || s.StormSuppressions != 4 {
		t.Fatalf("unexpected soft/orphan/dynamic counters: %+v", s)
	}
	if s.TrimActions != 1 || s.PeakSaves != 1 || s.AvgReclaimMB() != 420 {
		t.Fatalf("unexpected trim metrics: %+v", s)
	}
	if len(s.DailyBaselines) != 1 || s.DailyBaselines[0].MinInUseMB != 11400 || s.DailyBaselines[0].MinPrivateMB != 8100 {
		t.Fatalf("unexpected daily baseline waterline: %+v", s.DailyBaselines)
	}
	if got := s.RestraintRate(); got < 66.0 || got > 67.0 {
		t.Fatalf("unexpected restraint rate: %.2f", got)
	}

	if err := reopened.Reset(); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}
	if after := reopened.Snapshot(); after.TotalSamples != 0 || len(after.DailyBaselines) != 0 {
		t.Fatalf("expected clean reset, got %+v", after)
	}
}
