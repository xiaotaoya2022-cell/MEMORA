package proc

import (
	"context"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// BurstSpawnerStat summarizes ultra-short-lived child processes caught by a
// high-frequency Toolhelp32 burst capture (typically 2.5s @ 30ms).
type BurstSpawnerStat struct {
	ParentPID        uint32  `json:"parent_pid"`
	ParentName       string  `json:"parent_name"`
	ChildName        string  `json:"child_name"`
	CapturedCount    int     `json:"captured_count"`
	WindowSeconds    float64 `json:"window_seconds"`
	EstRatePerHour   float64 `json:"est_rate_per_hour"`
	EstLeakMBPerHour float64 `json:"est_leak_mb_per_hour,omitempty"`
}

// CaptureBurstSpawners performs a short, low-overhead high-frequency process scan
// using CreateToolhelp32Snapshot to catch short-lived (<100ms) child processes that
// vanish between normal 5s-10s telemetry intervals (e.g. IDE git polling loops).
// If leakKBPerProc > 0, EstLeakMBPerHour estimates the physical RAM bleed rate caused
// by those spawns under an active Ring-0 EPROCESS reference leak.
func CaptureBurstSpawners(ctx context.Context, duration, step time.Duration, leakKBPerProc float64) []BurstSpawnerStat {
	if duration <= 0 {
		duration = 2500 * time.Millisecond
	}
	if duration > 10*time.Second {
		duration = 10 * time.Second
	}
	if step < 15*time.Millisecond {
		step = 30 * time.Millisecond
	}

	seen := make(map[uint32]string, 512)
	type aggKey struct {
		ppid  uint32
		child string
	}
	counts := make(map[aggKey]*BurstSpawnerStat)

	snapshotOnce := func(record bool) {
		h, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
		if err != nil {
			return
		}
		defer syscall.CloseHandle(h)

		var pe syscall.ProcessEntry32
		pe.Size = uint32(unsafe.Sizeof(pe))
		if err := syscall.Process32First(h, &pe); err != nil {
			return
		}
		for {
			pid := pe.ProcessID
			name := syscall.UTF16ToString(pe.ExeFile[:])
			if _, ok := seen[pid]; !ok {
				seen[pid] = name
				if record && pid > 4 {
					ppid := pe.ParentProcessID
					parentName := seen[ppid]
					if parentName == "" && ppid != 0 {
						parentName = "(exited)"
					}
					key := aggKey{ppid: ppid, child: strings.ToLower(name)}
					st, ok := counts[key]
					if !ok {
						st = &BurstSpawnerStat{
							ParentPID:  ppid,
							ParentName: parentName,
							ChildName:  name,
						}
						counts[key] = st
					} else if (st.ParentName == "" || st.ParentName == "(exited)") && parentName != "" {
						st.ParentName = parentName
					}
					st.CapturedCount++
				}
			}
			if err := syscall.Process32Next(h, &pe); err != nil {
				break
			}
		}
	}

	// Baseline snapshot before measuring new process births.
	snapshotOnce(false)
	started := time.Now()
	ticker := time.NewTicker(step)
	defer ticker.Stop()
	deadline := time.NewTimer(duration)
	defer deadline.Stop()

	for {
		select {
		case <-ctx.Done():
			goto done
		case <-deadline.C:
			snapshotOnce(true)
			goto done
		case <-ticker.C:
			snapshotOnce(true)
		}
	}

done:
	elapsed := time.Since(started).Seconds()
	if elapsed < 0.2 {
		elapsed = 0.2
	}
	out := make([]BurstSpawnerStat, 0, len(counts))
	for _, st := range counts {
		st.WindowSeconds = elapsed
		st.EstRatePerHour = float64(st.CapturedCount) * 3600.0 / elapsed
		if leakKBPerProc > 0 {
			st.EstLeakMBPerHour = st.EstRatePerHour * leakKBPerProc / 1024.0
		}
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CapturedCount != out[j].CapturedCount {
			return out[i].CapturedCount > out[j].CapturedCount
		}
		return strings.ToLower(out[i].ParentName) < strings.ToLower(out[j].ParentName)
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}
