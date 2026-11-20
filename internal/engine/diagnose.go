package engine

import (
	"sort"
	"strings"
	"sync"
	"time"

	"memora/internal/proc"
	"memora/internal/winmem"
)

// ProcGroup aggregates multi-process applications (e.g. browsers, IDEs, WebView2)
// so total footprint is visible instead of scattered across dozens of PIDs.
type ProcGroup struct {
	Name       string `json:"name"`
	Count      int    `json:"count"`
	WorkingSet uint64 `json:"working_set_bytes"`
	PrivateWS  uint64 `json:"private_ws_bytes"`
	Private    uint64 `json:"private_bytes"`
	Handles    uint32 `json:"handles"`
}

// KernelLeakSummary summarizes whether dead kernel process/token/job objects
// are accumulating without being dereferenced.
type KernelLeakSummary struct {
	ProcessObjects  uint32 `json:"process_objects"`
	ProcessHandles  uint32 `json:"process_handles"`
	TokenObjects    uint32 `json:"token_objects"`
	TokenHandles    uint32 `json:"token_handles"`
	JobObjects      uint32 `json:"job_objects"`
	JobHandles      uint32 `json:"job_handles"`
	EventObjects    uint32 `json:"event_objects"`
	EventHandles    uint32 `json:"event_handles"`
	ActiveProcesses int    `json:"active_processes"`
	ProcTagAllocs   uint32 `json:"proc_tag_allocs"`
	ProcTagFrees    uint32 `json:"proc_tag_frees"`
	ProcTagBytes    uint64 `json:"proc_tag_bytes"`
	RelatedPoolByte uint64 `json:"related_pool_bytes"` // Proc + MiP2 + PsIn + Toke + SeTl/Td/Dt
	Suspected       bool   `json:"suspected"`
}

// PoolFinding highlights one notable kernel pool tag and optional driver match.
type PoolFinding struct {
	Tag     string   `json:"tag"`
	Pool    string   `json:"pool"` // nonpaged | paged
	Bytes   uint64   `json:"bytes"`
	Allocs  uint32   `json:"allocs"`
	Frees   uint32   `json:"frees"`
	Active  int64    `json:"active"`
	Desc    string   `json:"desc,omitempty"`
	Drivers []string `json:"drivers,omitempty"`
}

// ProcessHandleLeaker identifies a specific PID holding excessive Process-type handles.
type ProcessHandleLeaker struct {
	PID            uint32 `json:"pid"`
	Name           string `json:"name"`
	ProcessHandles uint32 `json:"process_handles"`
}

// DeepDiagnosis holds kernel, virtualization, NUMA, and aggregated process findings.
type DeepDiagnosis struct {
	Extras               winmem.SystemExtras     `json:"extras"`
	HasObjectTypes       bool                    `json:"has_object_types"`
	ObjectTypes          []winmem.ObjectTypeStat `json:"object_types,omitempty"`
	HasPoolTags          bool                    `json:"has_pool_tags"`
	PoolTags             []winmem.PoolTag        `json:"-"`
	Leak                 KernelLeakSummary       `json:"kernel_leak"`
	ProcessHandleHolders []ProcessHandleLeaker   `json:"process_handle_holders,omitempty"`
	TopNonPagedTags      []PoolFinding           `json:"top_nonpaged_tags,omitempty"`
	TopPagedTags         []PoolFinding           `json:"top_paged_tags,omitempty"`
	ProcGroups           []ProcGroup             `json:"proc_groups,omitempty"`
	HandleLeakers        []ProcGroup             `json:"handle_leakers,omitempty"`
	DriversScanned        bool                    `json:"drivers_scanned"`
	TotalWSBytes          uint64                  `json:"total_ws_bytes,omitempty"`
	SharedWSBytes         uint64                  `json:"shared_ws_bytes,omitempty"`
	ConflictServices      []winmem.ServiceState   `json:"conflict_services,omitempty"`
	ConflictMitigated     bool                    `json:"conflict_mitigated"`
	ConflictPendingReboot bool                    `json:"conflict_pending_reboot"`
}

// GroupProcesses aggregates processes by case-insensitive image name, sorted by Private bytes desc.
func GroupProcesses(procs []proc.Info) []ProcGroup {
	byName := make(map[string]*ProcGroup, len(procs)/2+1)
	for _, p := range procs {
		name := p.Name
		if name == "" {
			name = "(unknown)"
		}
		key := strings.ToLower(name)
		g, ok := byName[key]
		if !ok {
			g = &ProcGroup{Name: name}
			byName[key] = g
		}
		g.Count++
		g.WorkingSet += p.WorkingSet
		g.PrivateWS += p.PrivateWS
		g.Private += p.Private
		g.Handles += p.Handles
	}
	out := make([]ProcGroup, 0, len(byName))
	for _, g := range byName {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Private != out[j].Private {
			return out[i].Private > out[j].Private
		}
		return out[i].WorkingSet > out[j].WorkingSet
	})
	return out
}

var (
	lastHoldersMu sync.Mutex
	lastHoldersAt time.Time
	lastHolders   []ProcessHandleLeaker
)

// InspectDeep gathers SystemExtras (Vid/NUMA), ObjectTypes, PoolTags, and optionally
// maps non-core high-usage or zero-free pool tags back to loaded driver files.
func InspectDeep(snap winmem.Snapshot, procs []proc.Info, matchDrivers bool) DeepDiagnosis {
	d := DeepDiagnosis{
		Extras:     winmem.ReadSystemExtras(snap.PageSize),
		ProcGroups: GroupProcesses(procs),
	}
	d.Leak.ActiveProcesses = len(procs)

	var totalWS, privWS uint64
	for _, p := range procs {
		totalWS += p.WorkingSet
		privWS += p.PrivateWS
	}
	d.TotalWSBytes = totalWS
	if totalWS > privWS {
		d.SharedWSBytes = totalWS - privWS
	}

	if objs, err := winmem.ReadObjectTypes(); err == nil {
		d.HasObjectTypes = true
		d.ObjectTypes = objs
		for _, o := range objs {
			switch o.Name {
			case "Process":
				d.Leak.ProcessObjects = o.Objects
				d.Leak.ProcessHandles = o.Handles
			case "Token":
				d.Leak.TokenObjects = o.Objects
				d.Leak.TokenHandles = o.Handles
			case "Job":
				d.Leak.JobObjects = o.Objects
				d.Leak.JobHandles = o.Handles
			case "Event":
				d.Leak.EventObjects = o.Objects
				d.Leak.EventHandles = o.Handles
			}
		}
	}

	if tags, err := winmem.ReadPoolTags(); err == nil {
		d.HasPoolTags = true
		d.PoolTags = tags
		d.TopNonPagedTags, d.TopPagedTags = buildPoolFindings(tags)
		populateLeakTags(&d.Leak, tags)
	}

	d.HandleLeakers = findHandleLeakers(d.ProcGroups)

	d.Leak.Suspected = isProcessObjectLeakSuspected(d.Leak)

	if d.Leak.Suspected {
		lastHoldersMu.Lock()
		cached := lastHolders
		fresh := !lastHoldersAt.IsZero() && time.Since(lastHoldersAt) < 60*time.Second
		lastHoldersMu.Unlock()

		if !matchDrivers && fresh {
			if len(cached) > 0 {
				d.ProcessHandleHolders = append([]ProcessHandleLeaker(nil), cached...)
			}
		} else if holders, err := winmem.ReadProcessHandleHolders(200); err == nil && len(holders) > 0 {
			nameByPID := make(map[uint32]string, len(procs))
			for _, p := range procs {
				nameByPID[p.PID] = p.Name
			}
			for _, h := range holders {
				nm := nameByPID[h.PID]
				if nm == "" && h.PID == 4 {
					nm = "System"
				}
				d.ProcessHandleHolders = append(d.ProcessHandleHolders, ProcessHandleLeaker{
					PID:            h.PID,
					Name:           nm,
					ProcessHandles: h.ProcessHandle,
				})
			}
			sort.Slice(d.ProcessHandleHolders, func(i, j int) bool {
				return d.ProcessHandleHolders[i].ProcessHandles > d.ProcessHandleHolders[j].ProcessHandles
			})
			if len(d.ProcessHandleHolders) > 10 {
				d.ProcessHandleHolders = d.ProcessHandleHolders[:10]
			}
			lastHoldersMu.Lock()
			lastHolders = append([]ProcessHandleLeaker(nil), d.ProcessHandleHolders...)
			lastHoldersAt = time.Now()
			lastHoldersMu.Unlock()
		}
	}

	if d.HasPoolTags {
		attachDriverMatches(&d, matchDrivers)
	}

	d.ConflictServices = winmem.ReadServiceStates([]string{
		"lnvscenter", "lrtp", "netrtp",
		"LenovoPcManagerService", "LnvSvcFdn", "LenovoSmartService", "LenovoUtilityService",
		"sysdiag", "ESurfingSvr",
	})
	d.ConflictMitigated = computeConflictMitigated(d.ConflictServices)
	d.ConflictPendingReboot = computeConflictPendingReboot(d.ConflictServices)

	return d
}

func computeConflictPendingReboot(svcs []winmem.ServiceState) bool {
	for _, s := range svcs {
		switch strings.ToLower(s.Name) {
		case "lnvscenter", "lrtp", "netrtp":
			if s.Exists && s.Running && strings.EqualFold(s.StartType, "disabled") {
				return true
			}
		}
	}
	return false
}

func computeConflictMitigated(svcs []winmem.ServiceState) bool {
	if len(svcs) == 0 {
		return false
	}
	hasConflictDrv := false
	anyActiveAuto := false
	for _, s := range svcs {
		switch strings.ToLower(s.Name) {
		case "lnvscenter", "lrtp", "netrtp", "lenovopcmanagerservice", "lnvsvcfdn":
			if s.Exists {
				hasConflictDrv = true
				if s.Running && !strings.EqualFold(s.StartType, "disabled") {
					anyActiveAuto = true
				}
			}
		}
	}
	return hasConflictDrv && !anyActiveAuto
}

func findHandleLeakers(groups []ProcGroup) []ProcGroup {
	var out []ProcGroup
	for _, g := range groups {
		if strings.EqualFold(g.Name, "System") {
			continue
		}
		minHandles := uint32(8000)
		if strings.EqualFold(g.Name, "explorer.exe") {
			minHandles = 16000
		}
		if g.Count <= 3 && g.Handles >= minHandles {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Handles > out[j].Handles
	})
	return out
}

func isProcessObjectLeakSuspected(l KernelLeakSummary) bool {
	if l.ProcessObjects >= 500 && l.ActiveProcesses > 0 {
		excess := int(l.ProcessObjects) - l.ActiveProcesses
		if int(l.ProcessObjects) > l.ActiveProcesses*3 &&
			l.ProcessObjects > l.ProcessHandles*3 &&
			(excess >= 10000 || l.RelatedPoolByte >= 256<<20) {
			return true
		}
	}
	if l.ProcTagAllocs >= 500 && l.ProcTagAllocs >= l.ProcTagFrees && l.ProcTagAllocs > l.ProcTagFrees*10 && int(l.ProcTagAllocs-l.ProcTagFrees) >= 10000 && int(l.ProcTagAllocs-l.ProcTagFrees) > l.ActiveProcesses*3 {
		return true
	}
	return false
}

func populateLeakTags(l *KernelLeakSummary, tags []winmem.PoolTag) {
	for _, pt := range tags {
		switch pt.Tag {
		case "Proc":
			l.ProcTagAllocs = pt.NonPagedAllocs
			l.ProcTagFrees = pt.NonPagedFrees
			l.ProcTagBytes = pt.NonPagedUsed
			l.RelatedPoolByte += pt.TotalUsed()
		case "MiP2", "PsIn", "Toke", "SeTl", "SeTd", "SeDt":
			l.RelatedPoolByte += pt.TotalUsed()
		}
	}
}

func buildPoolFindings(tags []winmem.PoolTag) (nonpaged []PoolFinding, paged []PoolFinding) {
	npSorted := make([]winmem.PoolTag, len(tags))
	copy(npSorted, tags)
	sort.Slice(npSorted, func(i, j int) bool { return npSorted[i].NonPagedUsed > npSorted[j].NonPagedUsed })
	seenNP := map[string]bool{}
	for i := 0; i < len(npSorted); i++ {
		pt := npSorted[i]
		if pt.NonPagedUsed < 8<<20 {
			break
		}
		if len(nonpaged) >= 12 && pt.NonPagedActive() < 100000 {
			continue
		}
		if seenNP[pt.Tag] {
			continue
		}
		seenNP[pt.Tag] = true
		nonpaged = append(nonpaged, PoolFinding{
			Tag:    pt.Tag,
			Pool:   "nonpaged",
			Bytes:  pt.NonPagedUsed,
			Allocs: pt.NonPagedAllocs,
			Frees:  pt.NonPagedFrees,
			Active: pt.NonPagedActive(),
			Desc:   winmem.DescribePoolTag(pt.Tag),
		})
	}

	pgSorted := make([]winmem.PoolTag, len(tags))
	copy(pgSorted, tags)
	sort.Slice(pgSorted, func(i, j int) bool { return pgSorted[i].PagedUsed > pgSorted[j].PagedUsed })
	for i := 0; i < len(pgSorted) && i < 10; i++ {
		pt := pgSorted[i]
		if pt.PagedUsed < 8<<20 {
			break
		}
		paged = append(paged, PoolFinding{
			Tag:    pt.Tag,
			Pool:   "paged",
			Bytes:  pt.PagedUsed,
			Allocs: pt.PagedAllocs,
			Frees:  pt.PagedFrees,
			Active: pt.PagedActive(),
			Desc:   winmem.DescribePoolTag(pt.Tag),
		})
	}
	return nonpaged, paged
}

var (
	driverCacheMu    sync.Mutex
	cachedDrivers    []winmem.DriverModule
	cachedTagMatches = map[string][]string{}
	cachedDriversAt  time.Time
)

// ResetDriverScanCache clears cached loaded-driver / pool-tag associations so the
// next deep inspection rescans live kernel modules immediately.
func ResetDriverScanCache() {
	driverCacheMu.Lock()
	cachedDrivers = nil
	cachedTagMatches = map[string][]string{}
	cachedDriversAt = time.Time{}
	driverCacheMu.Unlock()
}

func attachDriverMatches(d *DeepDiagnosis, forceRefresh bool) {
	tagSet := map[string]struct{}{}
	var queryTags []string
	addIfUnknown := func(f PoolFinding) {
		if f.Desc != "" {
			return
		}
		if _, exists := tagSet[f.Tag]; !exists {
			tagSet[f.Tag] = struct{}{}
			queryTags = append(queryTags, f.Tag)
		}
	}
	for _, f := range d.TopNonPagedTags {
		addIfUnknown(f)
	}
	for _, f := range d.TopPagedTags {
		addIfUnknown(f)
	}
	if len(queryTags) == 0 {
		return
	}

	driverCacheMu.Lock()
	defer driverCacheMu.Unlock()

	if forceRefresh || len(cachedDrivers) == 0 || time.Since(cachedDriversAt) >= 5*time.Minute {
		drivers, err := winmem.ListDrivers()
		if err != nil || len(drivers) == 0 {
			return
		}
		cachedDrivers = drivers
		cachedTagMatches = map[string][]string{}
		cachedDriversAt = time.Now()
	}

	var missingTags []string
	for _, tag := range queryTags {
		if _, ok := cachedTagMatches[tag]; !ok {
			missingTags = append(missingTags, tag)
		}
	}
	if len(missingTags) > 0 {
		newMatches := winmem.MatchPoolTagsToDrivers(missingTags, cachedDrivers)
		for _, tag := range missingTags {
			cachedTagMatches[tag] = newMatches[tag]
		}
	}

	const maxMatchNoise = 6
	for i := range d.TopNonPagedTags {
		if ds := cachedTagMatches[d.TopNonPagedTags[i].Tag]; len(ds) > 0 && len(ds) <= maxMatchNoise {
			d.TopNonPagedTags[i].Drivers = ds
		}
	}
	for i := range d.TopPagedTags {
		if ds := cachedTagMatches[d.TopPagedTags[i].Tag]; len(ds) > 0 && len(ds) <= maxMatchNoise {
			d.TopPagedTags[i].Drivers = ds
		}
	}
	d.DriversScanned = true
}

// NUMADisparity returns whether a system with multiple NUMA nodes has one node
// tightly constrained while total physical memory still looks ample.
func NUMADisparity(nodes []winmem.NUMANode) (winmem.NUMANode, bool) {
	if len(nodes) < 2 {
		return winmem.NUMANode{}, false
	}
	var tightest winmem.NUMANode
	var maxAvail uint64
	found := false
	for _, n := range nodes {
		if n.TotalMB == 0 {
			continue
		}
		if !found || n.AvailableMB < tightest.AvailableMB {
			tightest = n
			found = true
		}
		if n.AvailableMB > maxAvail {
			maxAvail = n.AvailableMB
		}
	}
	if !found {
		return winmem.NUMANode{}, false
	}
	if tightest.AvailableMB <= 256 && maxAvail >= 2048 {
		return tightest, true
	}
	return winmem.NUMANode{}, false
}
