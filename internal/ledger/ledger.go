// Package ledger persists MEMORA's long-term "Quiet Ledger" (长期静默守护账本),
// focusing on restraint rate, peak-pressure intervention, orphan convergence,
// kernel anomaly sightings and daily low-water baseline drift rather than
// vanity total-reclaimed counters.
package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DailyBaseline tracks the lowest observed memory waterline per calendar day
// so users can verify across weeks whether idle RAM actually returns to baseline.
type DailyBaseline struct {
	Date           string  `json:"date"` // YYYY-MM-DD
	MinInUseMB     float64 `json:"min_in_use_mb"`
	MinPrivateMB   float64 `json:"min_private_mb"`
	ZombieLeakMB   float64 `json:"zombie_leak_mb,omitempty"`
	NetMinInUseMB  float64 `json:"net_min_in_use_mb,omitempty"`
	VidMB          float64 `json:"vid_mb,omitempty"`
	ProcessObjects uint32  `json:"process_objects,omitempty"`
}

// Data holds persisted cumulative metrics.
type Data struct {
	StartedAt              time.Time       `json:"started_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
	Sessions               int             `json:"sessions"`
	TotalGuardSeconds      float64         `json:"total_guard_seconds"`
	TotalSamples           int64           `json:"total_samples"`
	RestrainedSamples      int64           `json:"restrained_samples"`
	FullscreenSkips        int64           `json:"fullscreen_skips"`
	SoftDowngrades         int64           `json:"soft_downgrades"`
	SoftRestores           int64           `json:"soft_restores"`
	TrimActions            int64           `json:"trim_actions"`
	PeakSaves              int64           `json:"peak_saves"`
	TotalReclaimedMB       float64         `json:"total_reclaimed_mb"`
	DynamicReclaimedMB     float64         `json:"dynamic_reclaimed_mb"`
	StormSuppressions      int64           `json:"storm_suppressions"`
	OrphansDetected        int64           `json:"orphans_detected"`
	KernelLeakObservations int64           `json:"kernel_leak_observations"`
	LastKernelLeakDesc     string          `json:"last_kernel_leak_desc,omitempty"`
	DailyBaselines         []DailyBaseline `json:"daily_baselines,omitempty"`
	ReboundProcs           map[string]int  `json:"rebound_procs,omitempty"`
}

// RestraintRate returns the percentage of samples where MEMORA chose not to
// aggressively trim or purge.
func (d Data) RestraintRate() float64 {
	if d.TotalSamples <= 0 {
		return 100
	}
	return float64(d.RestrainedSamples) / float64(d.TotalSamples) * 100
}

// AvgReclaimMB returns the average measured gain per trim/purge intervention.
func (d Data) AvgReclaimMB() float64 {
	if d.TrimActions <= 0 {
		return 0
	}
	return d.TotalReclaimedMB / float64(d.TrimActions)
}

// Ledger manages safe thread-confined updates and periodic atomic flushes.
type Ledger struct {
	mu       sync.Mutex
	path     string
	data     Data
	dirty    bool
	lastSave time.Time
}

// Open loads or initializes ledger.json inside dir.
func Open(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "ledger.json")
	now := time.Now()
	d := Data{StartedAt: now, UpdatedAt: now}
	if raw, err := os.ReadFile(p); err == nil {
		var parsed Data
		if json.Unmarshal(raw, &parsed) == nil && !parsed.StartedAt.IsZero() {
			d = parsed
		}
	}
	return &Ledger{
		path:     p,
		data:     d,
		lastSave: now,
	}, nil
}

// Path returns the on-disk JSON file location.
func (l *Ledger) Path() string { return l.path }

// Snapshot returns a value copy of the current cumulative metrics.
func (l *Ledger) Snapshot() Data {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := l.data
	if len(l.data.DailyBaselines) > 0 {
		cp.DailyBaselines = make([]DailyBaseline, len(l.data.DailyBaselines))
		copy(cp.DailyBaselines, l.data.DailyBaselines)
	}
	if len(l.data.ReboundProcs) > 0 {
		cp.ReboundProcs = make(map[string]int, len(l.data.ReboundProcs))
		for k, v := range l.data.ReboundProcs {
			cp.ReboundProcs[k] = v
		}
	}
	return cp
}

// StartSession increments session count and flushes right away.
func (l *Ledger) StartSession() error {
	l.mu.Lock()
	l.data.Sessions++
	l.data.UpdatedAt = time.Now()
	l.dirty = true
	l.mu.Unlock()
	return l.Flush()
}

// RecordSample updates elapsed watch time, restraint stats, and today's lowest waterline.
func (l *Ledger) RecordSample(now time.Time, step time.Duration, restrained bool, fullscreenSkipped bool, inUseMB, privMB float64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if step > 0 && step <= 5*time.Minute {
		l.data.TotalGuardSeconds += step.Seconds()
	}
	l.data.TotalSamples++
	if restrained {
		l.data.RestrainedSamples++
	}
	if fullscreenSkipped {
		l.data.FullscreenSkips++
	}
	l.data.UpdatedAt = now

	day := now.Format("2006-01-02")
	n := len(l.data.DailyBaselines)
	if n == 0 || l.data.DailyBaselines[n-1].Date != day {
		l.data.DailyBaselines = append(l.data.DailyBaselines, DailyBaseline{
			Date:         day,
			MinInUseMB:   inUseMB,
			MinPrivateMB: privMB,
		})
		if len(l.data.DailyBaselines) > 30 {
			l.data.DailyBaselines = l.data.DailyBaselines[len(l.data.DailyBaselines)-30:]
		}
	} else {
		cur := &l.data.DailyBaselines[n-1]
		if inUseMB > 0 && (cur.MinInUseMB == 0 || inUseMB < cur.MinInUseMB) {
			cur.MinInUseMB = inUseMB
		}
		if privMB > 0 && (cur.MinPrivateMB == 0 || privMB < cur.MinPrivateMB) {
			cur.MinPrivateMB = privMB
		}
	}

	l.dirty = true
	if now.Sub(l.lastSave) >= 2*time.Minute {
		_ = l.saveLocked()
	}
}

// RecordBaselineExtras enriches today's baseline record with Vid/kernel object/zombie leak readings.
func (l *Ledger) RecordBaselineExtras(now time.Time, vidMB float64, procObjects uint32, zombieLeakMB float64, leakNote string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	day := now.Format("2006-01-02")
	if n := len(l.data.DailyBaselines); n > 0 && l.data.DailyBaselines[n-1].Date == day {
		cur := &l.data.DailyBaselines[n-1]
		if vidMB > cur.VidMB {
			cur.VidMB = vidMB
		}
		if procObjects > cur.ProcessObjects {
			cur.ProcessObjects = procObjects
		}
		cur.ZombieLeakMB = zombieLeakMB
		net := cur.MinInUseMB - zombieLeakMB
		if net < 0 {
			net = 0
		}
		cur.NetMinInUseMB = net
	}
	if leakNote != "" {
		l.data.KernelLeakObservations++
		l.data.LastKernelLeakDesc = leakNote
	}
	l.dirty = true
}

// RecordReboundProc persists adaptive trim rebound counts across restarts.
func (l *Ledger) RecordReboundProc(name string, count int) {
	if name == "" || count <= 0 {
		return
	}
	l.mu.Lock()
	if l.data.ReboundProcs == nil {
		l.data.ReboundProcs = make(map[string]int)
	}
	l.data.ReboundProcs[name] = count
	l.dirty = true
	l.mu.Unlock()
}

// ReboundProcsSnapshot returns a copy of persisted trim rebound counts.
func (l *Ledger) ReboundProcsSnapshot() map[string]int {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make(map[string]int, len(l.data.ReboundProcs))
	for k, v := range l.data.ReboundProcs {
		out[k] = v
	}
	return out
}

// RecordSoft counts soft priority / EcoQoS changes.
func (l *Ledger) RecordSoft(downgrades, restores int) {
	if downgrades == 0 && restores == 0 {
		return
	}
	l.mu.Lock()
	l.data.SoftDowngrades += int64(downgrades)
	l.data.SoftRestores += int64(restores)
	l.dirty = true
	l.mu.Unlock()
}

// RecordTrim records an explicit working-set/standby intervention.
func (l *Ledger) RecordTrim(reclaimedMB float64, underPressure bool) {
	l.mu.Lock()
	l.data.TrimActions++
	if underPressure {
		l.data.PeakSaves++
	}
	if reclaimedMB > 0 {
		l.data.TotalReclaimedMB += reclaimedMB
	}
	l.data.UpdatedAt = time.Now()
	l.dirty = true
	_ = l.saveLocked()
	l.mu.Unlock()
}

// RecordOrphans records newly observed detached child processes.
func (l *Ledger) RecordOrphans(count int) {
	if count <= 0 {
		return
	}
	l.mu.Lock()
	l.data.OrphansDetected += int64(count)
	l.dirty = true
	l.mu.Unlock()
}

// RecordDynamicReclaim adds measured working-set and standby cache bytes reclaimed
// by continuous smart scheduling and manual optimization.
func (l *Ledger) RecordDynamicReclaim(mb float64) {
	if mb <= 0 {
		return
	}
	l.mu.Lock()
	l.data.DynamicReclaimedMB += mb
	l.data.UpdatedAt = time.Now()
	l.dirty = true
	l.mu.Unlock()
}

// RecordStormSuppression increments the count of spawn-storm throttle interventions.
func (l *Ledger) RecordStormSuppression(n int) {
	if n <= 0 {
		return
	}
	l.mu.Lock()
	l.data.StormSuppressions += int64(n)
	l.data.UpdatedAt = time.Now()
	l.dirty = true
	l.mu.Unlock()
}

// Flush writes pending changes to disk atomically.
func (l *Ledger) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.dirty {
		return nil
	}
	return l.saveLocked()
}

// Reset clears cumulative counters while keeping the file path active.
func (l *Ledger) Reset() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.data = Data{StartedAt: now, UpdatedAt: now}
	l.dirty = true
	return l.saveLocked()
}

func (l *Ledger) saveLocked() error {
	raw, err := json.MarshalIndent(l.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, l.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	l.dirty = false
	l.lastSave = time.Now()
	return nil
}
