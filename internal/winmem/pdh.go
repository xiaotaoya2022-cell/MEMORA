package winmem

import (
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

const (
	pdhFmtLarge = 0x00000400
	pdhMoreData = 0x800007D2
)

var (
	pdh = syscall.NewLazyDLL("pdh.dll")

	procPdhOpenQueryW               = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounterW       = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData         = pdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterArray = pdh.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery               = pdh.NewProc("PdhCloseQuery")
)

type pdhFmtCounterValueItem struct {
	Name       *uint16
	CStatus    uint32
	LargeValue int64
}

// NUMANode reports physical memory availability for one NUMA node.
type NUMANode struct {
	ID          string `json:"id"`
	TotalMB     uint64 `json:"total_mb"`
	AvailableMB uint64 `json:"available_mb"`
}

// SystemExtras collects supplemental Windows telemetry not exposed by
// GlobalMemoryStatusEx / GetPerformanceInfo: Hyper-V Vid locked guest pages
// (used by WSL2/VMs) and per-NUMA-node availability.
type SystemExtras struct {
	VidBytes  uint64     `json:"vid_bytes,omitempty"`
	HasVid    bool       `json:"has_vid"`
	NUMANodes []NUMANode `json:"numa_nodes,omitempty"`
	HasNUMA   bool       `json:"has_numa"`
}

// ReadSystemExtras reads Hyper-V Vid partition pages and NUMA node memory via pdh.dll
// without spawning external processes. Unavailable counters degrade cleanly.
func ReadSystemExtras(pageSize uint64) SystemExtras {
	if pageSize == 0 {
		pageSize = 4096
	}
	var query uintptr
	st, _, _ := procPdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&query)))
	if st != 0 || query == 0 {
		return SystemExtras{}
	}
	defer procPdhCloseQuery.Call(query)

	cVid, okVid := addEnglishCounter(query, `\Hyper-V VM Vid Partition(_total)\Physical Pages Allocated`)
	cNumaAvail, okAvail := addEnglishCounter(query, `\NUMA Node Memory(*)\Available MBytes`)
	cNumaTotal, okTotal := addEnglishCounter(query, `\NUMA Node Memory(*)\Total MBytes`)
	if !okVid && !okAvail && !okTotal {
		return SystemExtras{}
	}

	if st, _, _ := procPdhCollectQueryData.Call(query); st != 0 {
		return SystemExtras{}
	}

	var out SystemExtras
	if okVid {
		if items, ok := readCounterLargeArray(cVid); ok {
			for _, it := range items {
				if strings.EqualFold(it.name, "_total") && it.val >= 0 {
					out.VidBytes = uint64(it.val) * pageSize
					out.HasVid = true
					break
				}
			}
		}
	}

	if okAvail {
		availItems, okA := readCounterLargeArray(cNumaAvail)
		totalMap := map[string]uint64{}
		if okTotal {
			if totalItems, okT := readCounterLargeArray(cNumaTotal); okT {
				for _, it := range totalItems {
					if it.val >= 0 {
						totalMap[it.name] = uint64(it.val)
					}
				}
			}
		}
		if okA {
			for _, it := range availItems {
				if strings.EqualFold(it.name, "_Total") || it.val < 0 {
					continue
				}
				out.NUMANodes = append(out.NUMANodes, NUMANode{
					ID:          it.name,
					TotalMB:     totalMap[it.name],
					AvailableMB: uint64(it.val),
				})
			}
			sort.Slice(out.NUMANodes, func(i, j int) bool {
				return out.NUMANodes[i].ID < out.NUMANodes[j].ID
			})
			out.HasNUMA = len(out.NUMANodes) > 0
		}
	}

	return out
}

func addEnglishCounter(query uintptr, path string) (uintptr, bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var counter uintptr
	st, _, _ := procPdhAddEnglishCounterW.Call(query, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&counter)))
	return counter, st == 0 && counter != 0
}

type pdhItem struct {
	name string
	val  int64
}

func readCounterLargeArray(counter uintptr) ([]pdhItem, bool) {
	var bufSize, itemCount uint32
	st, _, _ := procPdhGetFormattedCounterArray.Call(
		counter,
		pdhFmtLarge,
		uintptr(unsafe.Pointer(&bufSize)),
		uintptr(unsafe.Pointer(&itemCount)),
		0,
	)
	if uint32(st) != pdhMoreData || bufSize == 0 {
		return nil, false
	}

	buf := make([]byte, bufSize)
	st, _, _ = procPdhGetFormattedCounterArray.Call(
		counter,
		pdhFmtLarge,
		uintptr(unsafe.Pointer(&bufSize)),
		uintptr(unsafe.Pointer(&itemCount)),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if st != 0 || itemCount == 0 {
		return nil, false
	}

	itemSize := unsafe.Sizeof(pdhFmtCounterValueItem{})
	if int(itemCount)*int(itemSize) > len(buf) {
		return nil, false
	}
	items := unsafe.Slice((*pdhFmtCounterValueItem)(unsafe.Pointer(&buf[0])), itemCount)
	out := make([]pdhItem, 0, itemCount)
	for _, it := range items {
		if it.Name == nil || (it.CStatus != 0 && it.CStatus != 1) {
			continue
		}
		name := utf16PtrToString(it.Name)
		out = append(out, pdhItem{name: name, val: it.LargeValue})
	}
	return out, len(out) > 0
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	const maxChars = 512
	s := unsafe.Slice(p, maxChars)
	for i, c := range s {
		if c == 0 {
			return syscall.UTF16ToString(s[:i])
		}
	}
	return syscall.UTF16ToString(s)
}
