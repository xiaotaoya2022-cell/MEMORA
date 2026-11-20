package engine

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"memora/internal/config"
	"memora/internal/proc"
	"memora/internal/winmem"
)

const (
	probeFileName   = "telemetry_probes.jsonl"
	maxProbeEntries = 1000
)

// ProbeMetrics captures fine-grained system memory, cache, kernel object, and pool-tag state.
type ProbeMetrics struct {
	At               time.Time             `json:"at"`
	InUseBytes       uint64                `json:"in_use_bytes"`
	AvailBytes       uint64                `json:"avail_bytes"`
	TotalWSBytes     uint64                `json:"total_ws_bytes"`
	PrivateWSBytes   uint64                `json:"private_ws_bytes"`
	SharedWSBytes    uint64                `json:"shared_ws_bytes"`
	PrivateBytes     uint64                `json:"private_bytes"`
	CommitBytes      uint64                `json:"commit_bytes"`
	SystemCacheBytes uint64                `json:"system_cache_bytes"`
	StandbyBytes     uint64                `json:"standby_bytes"`
	ModifiedBytes    uint64                `json:"modified_bytes"`
	FreeBytes        uint64                `json:"free_bytes"`
	KernelTotalBytes uint64                `json:"kernel_total_bytes"`
	KernelPagedBytes uint64                `json:"kernel_paged_bytes"`
	KernelNonpaged   uint64                `json:"kernel_nonpaged_bytes"`
	VidBytes         uint64                `json:"vid_bytes"`
	ProcessCount     int                   `json:"process_count"`
	SystemHandles    uint32                `json:"system_handles"`
	ProcessObjects   uint32                `json:"process_objects"`
	ProcessHandles   uint32                `json:"process_handles"`
	TokenObjects     uint32                `json:"token_objects"`
	EventObjects     uint32                `json:"event_objects"`
	ProcTagAllocs    uint32                `json:"proc_tag_allocs"`
	ProcTagFrees     uint32                `json:"proc_tag_frees"`
	ProcTagBytes     uint64                `json:"proc_tag_bytes"`
	RelatedPoolBytes uint64                `json:"related_pool_bytes"`
	CckTActive       int64                 `json:"cckt_active,omitempty"`
	CckTBytes        uint64                `json:"cckt_bytes,omitempty"`
	TopTags          []PoolFinding         `json:"top_tags,omitempty"`
	HandleHolders    []ProcessHandleLeaker `json:"handle_holders,omitempty"`
	Services         []winmem.ServiceState `json:"services,omitempty"`
}

// ProcWSDelta records a process whose Working Set dropped sharply during an external boost.
type ProcWSDelta struct {
	PID        uint32  `json:"pid"`
	Name       string  `json:"name"`
	SessionID  uint32  `json:"session_id"`
	BeforeMB   float64 `json:"before_mb"`
	AfterMB    float64 `json:"after_mb"`
	ShrunkMB   float64 `json:"shrunk_mb"`
}

// ProbeRecord is one multi-dimensional telemetry snapshot written to telemetry_probes.jsonl.
type ProbeRecord struct {
	At               time.Time      `json:"at"`
	Kind             string         `json:"kind"` // external_boost | hourly_trend | memora_optimize | conflict_mitigation
	Note             string         `json:"note,omitempty"`
	Before           *ProbeMetrics  `json:"before,omitempty"`
	After            *ProbeMetrics  `json:"after,omitempty"`
	AvailDeltaMB     float64        `json:"avail_delta_mb,omitempty"`
	SharedWSDeltaMB  float64        `json:"shared_ws_delta_mb,omitempty"`
	PrivWSDeltaMB    float64        `json:"priv_ws_delta_mb,omitempty"`
	StandbyDeltaMB   float64        `json:"standby_delta_mb,omitempty"`
	SysCacheDeltaMB  float64        `json:"syscache_delta_mb,omitempty"`
	ProcObjDelta     int64          `json:"proc_obj_delta,omitempty"`
	ProcObjRatePerHr float64        `json:"proc_obj_rate_per_hr,omitempty"`
	TopShrunkProcs   []ProcWSDelta  `json:"top_shrunk_procs,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
}

var (
	probeMu           sync.Mutex
	lastProbeSample   *Sample
	lastProbeMetrics  *ProbeMetrics
	lastHourlyProbeAt time.Time
	lastHourlyObjs    uint32
	lastSelfActionAt  time.Time
)

var trackedConflictServices = []string{
	"lnvscenter", "lrtp", "netrtp",
	"LenovoPcManagerService", "LnvSvcFdn", "LenovoSmartService", "LenovoUtilityService",
	"sysdiag", "ESurfingSvr",
}

// MarkSelfMemoryAction stamps when MEMORA intentionally trimmed/purged RAM so
// external-boost detection won't confuse MEMORA's own actions with third-party optimizers.
func MarkSelfMemoryAction() {
	probeMu.Lock()
	lastSelfActionAt = time.Now()
	probeMu.Unlock()
}

// CaptureProbeMetrics builds a detailed kernel + process + pool + service telemetry snapshot.
func CaptureProbeMetrics(s Sample, includeDeep bool) ProbeMetrics {
	sharedWS := uint64(0)
	if s.WS > s.PrivateWS {
		sharedWS = s.WS - s.PrivateWS
	}
	pm := ProbeMetrics{
		At:               s.At,
		InUseBytes:       s.Snap.InUse(),
		AvailBytes:       s.Snap.AvailPhys,
		TotalWSBytes:     s.WS,
		PrivateWSBytes:   s.PrivateWS,
		SharedWSBytes:    sharedWS,
		PrivateBytes:     s.Private,
		CommitBytes:      s.Snap.Commit,
		SystemCacheBytes: s.Snap.SystemCache,
		StandbyBytes:     s.Snap.Pages.Standby,
		ModifiedBytes:    s.Snap.Pages.Modified,
		FreeBytes:        s.Snap.Pages.Free,
		KernelTotalBytes: s.Snap.KernelTotal,
		KernelPagedBytes: s.Snap.KernelPaged,
		KernelNonpaged:   s.Snap.KernelNonpaged,
		ProcessCount:     len(s.Procs),
		SystemHandles:    s.Snap.Handles,
	}
	extras := winmem.ReadSystemExtras(s.Snap.PageSize)
	pm.VidBytes = extras.VidBytes

	if includeDeep {
		if objs, err := winmem.ReadObjectTypes(); err == nil {
			for _, o := range objs {
				switch o.Name {
				case "Process":
					pm.ProcessObjects = o.Objects
					pm.ProcessHandles = o.Handles
				case "Token":
					pm.TokenObjects = o.Objects
				case "Event":
					pm.EventObjects = o.Objects
				}
			}
		}
		if tags, err := winmem.ReadPoolTags(); err == nil {
			var leak KernelLeakSummary
			populateLeakTags(&leak, tags)
			pm.ProcTagAllocs = leak.ProcTagAllocs
			pm.ProcTagFrees = leak.ProcTagFrees
			pm.ProcTagBytes = leak.ProcTagBytes
			pm.RelatedPoolBytes = leak.RelatedPoolByte
			np, _ := buildPoolFindings(tags)
			if len(np) > 8 {
				np = np[:8]
			}
			pm.TopTags = np
			for _, pt := range tags {
				if pt.Tag == "cckT" {
					pm.CckTActive = pt.NonPagedActive() + pt.PagedActive()
					pm.CckTBytes = pt.TotalUsed()
					break
				}
			}
		}
		if holders, err := winmem.ReadProcessHandleHolders(200); err == nil && len(holders) > 0 {
			nameByPID := make(map[uint32]string, len(s.Procs))
			for _, p := range s.Procs {
				nameByPID[p.PID] = p.Name
			}
			for _, h := range holders {
				nm := nameByPID[h.PID]
				if nm == "" && h.PID == 4 {
					nm = "System"
				}
				pm.HandleHolders = append(pm.HandleHolders, ProcessHandleLeaker{
					PID:            h.PID,
					Name:           nm,
					ProcessHandles: h.ProcessHandle,
				})
			}
			sort.Slice(pm.HandleHolders, func(i, j int) bool {
				return pm.HandleHolders[i].ProcessHandles > pm.HandleHolders[j].ProcessHandles
			})
			if len(pm.HandleHolders) > 8 {
				pm.HandleHolders = pm.HandleHolders[:8]
			}
		}
		pm.Services = winmem.ReadServiceStates(trackedConflictServices)
	}
	return pm
}

// ObserveProbeSample watches samples for two passive dimensions:
// 1) sudden external memory drops (>= 1.5 GB avail jump without MEMORA action)
// 2) hourly kernel/object/driver trend telemetry.
func ObserveProbeSample(s Sample) {
	probeMu.Lock()
	defer probeMu.Unlock()

	const externalJumpThreshold = uint64(1536) << 20 // 1.5 GB jump in one tick
	if lastProbeSample != nil && s.At.Sub(lastProbeSample.At) <= 90*time.Second {
		if s.Snap.AvailPhys > lastProbeSample.Snap.AvailPhys+externalJumpThreshold &&
			(lastSelfActionAt.IsZero() || s.At.Sub(lastSelfActionAt) > 25*time.Second) {
			topShrunk := diffShrunkProcesses(lastProbeSample.Procs, s.Procs, 15)
			prevShared := uint64(0)
			if lastProbeSample.WS > lastProbeSample.PrivateWS {
				prevShared = lastProbeSample.WS - lastProbeSample.PrivateWS
			}
			curShared := uint64(0)
			if s.WS > s.PrivateWS {
				curShared = s.WS - s.PrivateWS
			}
			sharedDelta := int64(curShared) - int64(prevShared)
			standbyDelta := int64(s.Snap.Pages.Standby) - int64(lastProbeSample.Snap.Pages.Standby)
			isGlobalBoost := len(topShrunk) >= 6 ||
				standbyDelta <= -(1536<<20) ||
				(len(topShrunk) >= 3 && sharedDelta <= -(1024<<20))

			if isGlobalBoost {
				beforeM := lastProbeMetrics
				if beforeM == nil {
					bm := CaptureProbeMetrics(*lastProbeSample, true)
					beforeM = &bm
				}
				afterM := CaptureProbeMetrics(s, true)
				rec := ProbeRecord{
					At:              s.At,
					Kind:            "external_boost",
					Note:            "检测到非 MEMORA 触发的全局内存释放（例如联想一键加速/外部清理工具）",
					Before:          beforeM,
					After:           &afterM,
					AvailDeltaMB:    float64(int64(afterM.AvailBytes)-int64(beforeM.AvailBytes)) / (1 << 20),
					SharedWSDeltaMB: float64(int64(afterM.SharedWSBytes)-int64(beforeM.SharedWSBytes)) / (1 << 20),
					PrivWSDeltaMB:   float64(int64(afterM.PrivateWSBytes)-int64(beforeM.PrivateWSBytes)) / (1 << 20),
					StandbyDeltaMB:  float64(int64(afterM.StandbyBytes)-int64(beforeM.StandbyBytes)) / (1 << 20),
					SysCacheDeltaMB: float64(int64(afterM.SystemCacheBytes)-int64(beforeM.SystemCacheBytes)) / (1 << 20),
					ProcObjDelta:    int64(afterM.ProcessObjects) - int64(beforeM.ProcessObjects),
					TopShrunkProcs:  topShrunk,
				}
				appendProbeRecordLocked(rec)
				lastHourlyProbeAt = s.At
				lastHourlyObjs = afterM.ProcessObjects
				lastProbeMetrics = &afterM
			}
		}
	}

	if lastHourlyProbeAt.IsZero() || s.At.Sub(lastHourlyProbeAt) >= time.Hour {
		cur := CaptureProbeMetrics(s, true)
		var delta int64
		var rate float64
		if !lastHourlyProbeAt.IsZero() && s.At.Sub(lastHourlyProbeAt) > time.Minute {
			delta = int64(cur.ProcessObjects) - int64(lastHourlyObjs)
			rate = float64(delta) / s.At.Sub(lastHourlyProbeAt).Hours()
		}
		appendProbeRecordLocked(ProbeRecord{
			At:               s.At,
			Kind:             "hourly_trend",
			Note:             "每小时内核对象、池标签与冲突驱动自愈状态采样",
			After:            &cur,
			ProcObjDelta:     delta,
			ProcObjRatePerHr: rate,
		})
		lastHourlyProbeAt = s.At
		lastHourlyObjs = cur.ProcessObjects
		lastProbeMetrics = &cur
	} else if lastProbeMetrics == nil {
		// Keep a lightweight baseline without expensive pool scans on every 10s tick.
		pm := CaptureProbeMetrics(s, false)
		lastProbeMetrics = &pm
	} else {
		// Refresh cheap metrics every tick so external_boost always has fresh Before stats.
		pm := CaptureProbeMetrics(s, false)
		pm.ProcessObjects = lastProbeMetrics.ProcessObjects
		pm.ProcessHandles = lastProbeMetrics.ProcessHandles
		pm.RelatedPoolBytes = lastProbeMetrics.RelatedPoolBytes
		pm.CckTActive = lastProbeMetrics.CckTActive
		pm.CckTBytes = lastProbeMetrics.CckTBytes
		lastProbeMetrics = &pm
	}

	copySample := s
	copySample.Procs = append([]proc.Info(nil), s.Procs...)
	lastProbeSample = &copySample
}

// RecordProbeEvent writes an explicit action probe (memora_optimize / conflict_mitigation).
func RecordProbeEvent(rec ProbeRecord) {
	probeMu.Lock()
	defer probeMu.Unlock()
	if rec.At.IsZero() {
		rec.At = time.Now()
	}
	appendProbeRecordLocked(rec)
}

// LoadTelemetryProbes reads the latest telemetry probe records from disk.
func LoadTelemetryProbes(limit int) []ProbeRecord {
	path := filepath.Join(config.Dir(), probeFileName)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []ProbeRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 2<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var r ProbeRecord
		if json.Unmarshal([]byte(line), &r) == nil {
			all = append(all, r)
		}
	}
	if limit > 0 && len(all) > limit {
		return all[len(all)-limit:]
	}
	return all
}

func appendProbeRecordLocked(rec ProbeRecord) {
	dir := config.Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, probeFileName)
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		_, _ = f.Write(append(data, '\n'))
		_ = f.Close()
	}
	// Compact periodically if file exceeds ~3MB so disk footprint stays bounded.
	if info, err := os.Stat(path); err == nil && info.Size() > 3<<20 {
		if entries := loadAllRawProbes(path); len(entries) > maxProbeEntries {
			trimmed := entries[len(entries)-maxProbeEntries:]
			var out []byte
			for _, line := range trimmed {
				out = append(out, line...)
				out = append(out, '\n')
			}
			_ = os.WriteFile(path, out, 0o644)
		}
	}
}

func loadAllRawProbes(path string) [][]byte {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(raw), "\n")
	out := make([][]byte, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, []byte(l))
		}
	}
	return out
}

func diffShrunkProcesses(before, after []proc.Info, topN int) []ProcWSDelta {
	afterMap := make(map[uint32]proc.Info, len(after))
	for _, p := range after {
		afterMap[p.PID] = p
	}
	var deltas []ProcWSDelta
	for _, b := range before {
		a, ok := afterMap[b.PID]
		if !ok {
			continue
		}
		if b.WorkingSet > a.WorkingSet+(4<<20) {
			deltas = append(deltas, ProcWSDelta{
				PID:       b.PID,
				Name:      b.Name,
				SessionID: b.SessionID,
				BeforeMB:  float64(b.WorkingSet) / (1 << 20),
				AfterMB:   float64(a.WorkingSet) / (1 << 20),
				ShrunkMB:  float64(b.WorkingSet-a.WorkingSet) / (1 << 20),
			})
		}
	}
	sort.Slice(deltas, func(i, j int) bool {
		return deltas[i].ShrunkMB > deltas[j].ShrunkMB
	})
	if topN > 0 && len(deltas) > topN {
		deltas = deltas[:topN]
	}
	return deltas
}
