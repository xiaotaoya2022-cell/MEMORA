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

// LeakForecast estimates ongoing Ring-0 zombie object bleed rate and how many
// hours remain before physical memory approaches critical saturation (90% used).
type LeakForecast struct {
	Active             bool    `json:"active"`
	ProcObjRatePerHour float64 `json:"proc_obj_rate_per_hour"`
	LeakMBPerHour      float64 `json:"leak_mb_per_hour"`
	BytesPerZombieKB   float64 `json:"bytes_per_zombie_kb,omitempty"`
	AvailMB            float64 `json:"avail_mb"`
	ETAHours           float64 `json:"eta_hours"` // hours until avail < 10% total RAM (0 if > 72h or already < 10%)
	AlreadyCritical    bool    `json:"already_critical"`
	SampleHours        float64 `json:"sample_hours"`
	Urgent             bool    `json:"urgent"` // true when AlreadyCritical or (ETAHours > 0 && ETAHours <= 12)
}

type trendPoint struct {
	At         time.Time
	ProcObjs   uint32
	InUseBytes uint64
}

var (
	probeMu           sync.Mutex
	lastProbeSample   *Sample
	lastProbeMetrics  *ProbeMetrics
	lastHourlyProbeAt time.Time
	lastHourlyObjs    uint32
	lastSelfActionAt  time.Time
	trendLoaded       bool
	trendHistory      []trendPoint
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
		recordTrendPointLocked(cur.At, cur.ProcessObjects, cur.InUseBytes)
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
		inheritKernelProbeMetrics(&pm, lastProbeMetrics)
		lastProbeMetrics = &pm
	}

	copySample := s
	copySample.Procs = append([]proc.Info(nil), s.Procs...)
	lastProbeSample = &copySample
}

func inheritKernelProbeMetrics(dst, src *ProbeMetrics) {
	if dst == nil || src == nil {
		return
	}
	if dst.ProcessObjects == 0 && src.ProcessObjects > 0 {
		dst.ProcessObjects = src.ProcessObjects
		dst.ProcessHandles = src.ProcessHandles
		dst.TokenObjects = src.TokenObjects
		dst.EventObjects = src.EventObjects
		dst.ProcTagAllocs = src.ProcTagAllocs
		dst.ProcTagFrees = src.ProcTagFrees
		dst.ProcTagBytes = src.ProcTagBytes
		dst.RelatedPoolBytes = src.RelatedPoolBytes
		dst.CckTActive = src.CckTActive
		dst.CckTBytes = src.CckTBytes
	}
}

func recordTrendPointLocked(at time.Time, procObjs uint32, inUse uint64) {
	if procObjs == 0 {
		return
	}
	// Detect reboot: if ProcessObjects dropped by >50%, reset current-boot trend window.
	if n := len(trendHistory); n > 0 && procObjs*2 < trendHistory[n-1].ProcObjs {
		trendHistory = trendHistory[:0]
	}
	trendHistory = append(trendHistory, trendPoint{
		At:         at,
		ProcObjs:   procObjs,
		InUseBytes: inUse,
	})
	if len(trendHistory) > 36 {
		trendHistory = trendHistory[len(trendHistory)-36:]
	}
}

func ensureTrendLoadedLocked() {
	if trendLoaded {
		return
	}
	trendLoaded = true
	recs := LoadTelemetryProbes(80)
	for _, r := range recs {
		if r.After == nil || r.After.ProcessObjects == 0 {
			continue
		}
		if r.Kind != "hourly_trend" && r.Kind != "external_boost" && r.Kind != "conflict_mitigation" {
			continue
		}
		recordTrendPointLocked(r.After.At, r.After.ProcessObjects, r.After.InUseBytes)
	}
}

// RecordProbeEvent writes an explicit action probe (memora_optimize / conflict_mitigation / leaker_reset).
// If Before/After omitted expensive kernel pool scans (ProcessObjects==0), it inherits
// the latest cached kernel object counters so telemetry logs stay continuous.
func RecordProbeEvent(rec ProbeRecord) {
	probeMu.Lock()
	defer probeMu.Unlock()
	if rec.At.IsZero() {
		rec.At = time.Now()
	}
	if lastProbeMetrics != nil {
		inheritKernelProbeMetrics(rec.Before, lastProbeMetrics)
		inheritKernelProbeMetrics(rec.After, lastProbeMetrics)
	}
	if rec.After != nil && rec.After.ProcessObjects > 0 {
		recordTrendPointLocked(rec.After.At, rec.After.ProcessObjects, rec.After.InUseBytes)
	}
	appendProbeRecordLocked(rec)
}

// ComputeLeakForecast returns the estimated Ring-0 object/memory bleed rate and
// exhaustion ETA based on current-boot hourly probe history and the live reading.
func ComputeLeakForecast(snap winmem.Snapshot, leak KernelLeakSummary) LeakForecast {
	probeMu.Lock()
	ensureTrendLoadedLocked()
	pts := make([]trendPoint, len(trendHistory))
	copy(pts, trendHistory)
	probeMu.Unlock()

	if leak.ProcessObjects > 0 {
		nowPt := trendPoint{At: time.Now(), ProcObjs: leak.ProcessObjects, InUseBytes: snap.InUse()}
		if len(pts) > 0 && nowPt.ProcObjs*2 < pts[len(pts)-1].ProcObjs {
			pts = nil
		}
		pts = append(pts, nowPt)
	}
	return computeLeakForecastFromPoints(snap, leak, pts)
}

func computeLeakForecastFromPoints(snap winmem.Snapshot, leak KernelLeakSummary, pts []trendPoint) LeakForecast {
	if !leak.Suspected || len(pts) < 2 || snap.TotalPhys == 0 {
		return LeakForecast{}
	}
	// Focus on the last 12 hours of the current boot session for responsive rate estimation.
	latest := pts[len(pts)-1]
	cutoff := latest.At.Add(-12 * time.Hour)
	startIdx := 0
	for i := range pts {
		if !pts[i].At.Before(cutoff) {
			startIdx = i
			break
		}
	}
	first := pts[startIdx]
	hours := latest.At.Sub(first.At).Hours()
	if hours < 0.25 && startIdx > 0 {
		first = pts[0]
		hours = latest.At.Sub(first.At).Hours()
	}
	if hours < 0.15 {
		return LeakForecast{}
	}
	deltaObjs := int64(latest.ProcObjs) - int64(first.ProcObjs)
	if deltaObjs <= 0 {
		return LeakForecast{}
	}
	rateObjsHr := float64(deltaObjs) / hours
	if rateObjsHr < 100 {
		return LeakForecast{}
	}

	// Empirical Ring-0 non-reclaimable footprint per leaked zombie EPROCESS
	// (EPROCESS + MiP2 + Token + Page Directory/PTE/Kernel stack) is ~140-180 KB.
	// Use observed InUse slope when positive and plausible (40..320 KB/obj), else fallback to 150 KB/obj.
	const defaultKBPerObj = 150.0
	kbPerObj := defaultKBPerObj
	deltaInUseMB := float64(int64(latest.InUseBytes)-int64(first.InUseBytes)) / (1 << 20)
	if deltaInUseMB > 0 && hours >= 1.0 {
		obsKB := (deltaInUseMB * 1024.0) / float64(deltaObjs)
		if obsKB >= 40 && obsKB <= 320 {
			kbPerObj = obsKB
		}
	}
	leakMBHr := rateObjsHr * kbPerObj / 1024.0
	if leakMBHr < 10 {
		return LeakForecast{}
	}

	availMB := float64(snap.AvailPhys) / (1 << 20)
	critFloorMB := float64(snap.TotalPhys) * 0.10 / (1 << 20)
	fc := LeakForecast{
		Active:             true,
		ProcObjRatePerHour: rateObjsHr,
		LeakMBPerHour:      leakMBHr,
		BytesPerZombieKB:   kbPerObj,
		AvailMB:            availMB,
		SampleHours:        hours,
	}
	if availMB <= critFloorMB {
		fc.AlreadyCritical = true
		fc.Urgent = true
	} else {
		eta := (availMB - critFloorMB) / leakMBHr
		if eta <= 72 {
			fc.ETAHours = eta
			if eta <= 12 {
				fc.Urgent = true
			}
		}
	}
	return fc
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
