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
	mu    sync.Mutex
	path  string
	file  *os.File
	limit int64
}

const defaultLimit = 8 << 20 // 8 MiB

// Open prepares the journal under dir, creating it if needed.
func Open(dir string) (*Journal, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	j := &Journal{
		path:  filepath.Join(dir, "journal.jsonl"),
		limit: defaultLimit,
	}
	return j, nil
}

// Path returns the active journal file.
func (j *Journal) Path() string { return j.path }

// Write appends one entry.
func (j *Journal) Write(e Entry) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	j.mu.Lock()
	defer j.mu.Unlock()

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
