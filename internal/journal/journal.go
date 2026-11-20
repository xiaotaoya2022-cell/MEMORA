// Package journal records what MEMORA saw and did, in append-only JSONL.
//
// The journal is the product's answer to "你凭什么动我的内存": every action
// has a reason, a before, an after and a measured result.
package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Stats is the compact memory state attached to an entry.
type Stats struct {
	AvailableMB  float64 `json:"available_mb"`
	InUseMB      float64 `json:"in_use_mb"`
	CommitMB     float64 `json:"commit_mb"`
	StandbyMB    float64 `json:"standby_mb"`
	AvailablePct float64 `json:"available_pct"`
}

// Target is one process an action touched.
type Target struct {
	PID          uint32  `json:"pid"`
	Name         string  `json:"name"`
	WorkingSetMB float64 `json:"working_set_mb"`
}

// Entry is one journal line.
type Entry struct {
	Time        time.Time `json:"time"`
	Kind        string    `json:"kind"` // start | stop | sample | action | error
	Action      string    `json:"action,omitempty"`
	Targets     []Target  `json:"targets,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Before      *Stats    `json:"before,omitempty"`
	After       *Stats    `json:"after,omitempty"`
	ReclaimedMB float64   `json:"reclaimed_mb,omitempty"`
	Result      string    `json:"result,omitempty"`
	Detail      string    `json:"detail,omitempty"`
}

// Journal is a rotating JSONL writer.
type Journal struct {
	mu             sync.Mutex
	path           string
	file           *os.File
	limit          int64
	lastStormByPID map[uint32]time.Time
}

const defaultLimit = 2 << 20 // 2 MiB

// Open prepares the journal under dir, creating it if needed and compacting oversized legacy logs.
func Open(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	j := &Journal{
		path:           filepath.Join(dir, "journal.jsonl"),
		limit:          defaultLimit,
		lastStormByPID: make(map[uint32]time.Time),
	}
	if info, err := os.Stat(j.path); err == nil && info.Size() > defaultLimit {
		compactJournal(j.path, 1200)
	}
	return j, nil
}

func compactJournal(path string, keep int) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	entries := scanLastEntries(f, keep*3)
	_ = f.Close()
	if len(entries) == 0 {
		return
	}
	filtered := make([]Entry, 0, len(entries))
	lastStorm := make(map[uint32]time.Time)
	for _, e := range entries {
		if e.Kind == "storm_throttle" && len(e.Targets) == 1 && e.Targets[0].PID != 0 {
			pid := e.Targets[0].PID
			if prev, ok := lastStorm[pid]; ok && e.Time.Sub(prev) < 5*time.Minute {
				continue
			}
			lastStorm[pid] = e.Time
		}
		filtered = append(filtered, e)
	}
	if len(filtered) > keep {
		filtered = filtered[len(filtered)-keep:]
	}
	var out []byte
	for _, e := range filtered {
		if b, err := json.Marshal(e); err == nil {
			out = append(out, b...)
			out = append(out, '\n')
		}
	}
	if len(out) > 0 {
		_ = os.WriteFile(path, out, 0o644)
	}
}

// Path returns the active journal file.
func (j *Journal) Path() string { return j.path }

// Write appends one entry, deduplicating repeated storm_throttle emissions for the same PID within 5m.
func (j *Journal) Write(e Entry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	if e.Kind == "storm_throttle" && len(e.Targets) == 1 && e.Targets[0].PID != 0 {
		pid := e.Targets[0].PID
		if prev, ok := j.lastStormByPID[pid]; ok && e.Time.Sub(prev) < 5*time.Minute {
			return nil
		}
		if len(j.lastStormByPID) > 512 {
			j.lastStormByPID = make(map[uint32]time.Time)
		}
		j.lastStormByPID[pid] = e.Time
	}

	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	if j.file == nil {
		f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		j.file = f
	}
	if info, err := j.file.Stat(); err == nil && info.Size() >= j.limit {
		j.rotate()
	}

	_, err = fmt.Fprintf(j.file, "%s\n", data)
	return err
}

func (j *Journal) rotate() {
	j.file.Close()
	j.file = nil
	_ = os.Remove(j.path + ".1")
	_ = os.Rename(j.path, j.path+".1")
	f, err := os.OpenFile(j.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		j.file = f
	}
}

// Close releases the handle.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.file == nil {
		return nil
	}
	err := j.file.Close()
	j.file = nil
	return err
}

// Tail reads up to n most recent valid entries from the active journal file in chronological order.
func Tail(path string, n int) ([]Entry, error) {
	if n <= 0 || path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	const tailWindow = 512 << 10
	if info, err := f.Stat(); err == nil && info.Size() > tailWindow {
		if _, err := f.Seek(-tailWindow, io.SeekEnd); err == nil {
			// Skip the partial first line after seeking mid-file.
			r := bufio.NewReader(f)
			_, _ = r.ReadBytes('\n')
			return scanLastEntries(r, n), nil
		}
	}
	return scanLastEntries(f, n), nil
}

func scanLastEntries(r io.Reader, n int) []Entry {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	ring := make([]Entry, 0, n)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(line, &e) == nil {
			if len(ring) < n {
				ring = append(ring, e)
			} else {
				copy(ring, ring[1:])
				ring[n-1] = e
			}
		}
	}
	return ring
}
