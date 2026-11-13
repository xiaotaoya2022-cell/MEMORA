package winmem

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	// systemMemoryListInformation is SystemMemoryListInformation from the
	// SYSTEM_INFORMATION_CLASS enum. It is not in the public SDK.
	systemMemoryListInformation = 0x50
	// memoryPurgeStandbyList is the command accepted by
	// NtSetSystemInformation for the class above.
	memoryPurgeStandbyList = 4

	// pageListEntries is how many SIZE_T we read back. The structure has
	// grown over Windows versions; the first 22 fields are stable enough
	// (free/zero/modified/bad + eight standby priority lists).
	pageListEntries = 22
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	psapi    = syscall.NewLazyDLL("psapi.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")

	procGlobalMemoryStatusEx     = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetPerformanceInfo       = psapi.NewProc("GetPerformanceInfo")
	procNtQuerySystemInformation = ntdll.NewProc("NtQuerySystemInformation")
	procNtSetSystemInformation   = ntdll.NewProc("NtSetSystemInformation")
)

// memoryStatusEx mirrors MEMORYSTATUSEX.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// performanceInfo mirrors PERFORMANCE_INFORMATION as shipped on current
// Windows builds, where the structure starts with a cb length field.
type performanceInfo struct {
	Cb                uint32
	_                 uint32
	CommitTotal       uintptr
	CommitLimit       uintptr
	CommitPeak        uintptr
	PhysicalTotal     uintptr
	PhysicalAvailable uintptr
	SystemCache       uintptr
	KernelTotal       uintptr
	KernelPaged       uintptr
	KernelNonpaged    uintptr
	PageSize          uintptr
	HandleCount       uint32
	ProcessCount      uint32
	ThreadCount       uint32
}

// legacyPerformanceInfo is the historical layout without the leading cb.
// Older builds reject the cb form as a bad length, so both are attempted.
type legacyPerformanceInfo struct {
	CommitTotal       uintptr
	CommitLimit       uintptr
	CommitPeak        uintptr
	PhysicalTotal     uintptr
	PhysicalAvailable uintptr
	SystemCache       uintptr
	KernelTotal       uintptr
	KernelPaged       uintptr
	KernelNonpaged    uintptr
	PageSize          uintptr
	HandleCount       uint32
	ProcessCount      uint32
	ThreadCount       uint32
}

// Pages is the physical page breakdown reported by the memory manager.
type Pages struct {
	Free         uint64 // pages on the free list
	Zeroed       uint64 // free pages already zeroed
	Modified     uint64 // dirty pages waiting to be written
	ModifiedNoIO uint64 // dirty pages the writer has not picked up
	Bad          uint64
	Standby      uint64 // sum of all standby priority lists
}

// Snapshot is one reading of system-wide memory state. Every field is bytes
// except the counters, so callers never have to remember units.
type Snapshot struct {
	TotalPhys      uint64
	AvailPhys      uint64
	TotalCommit    uint64 // commit limit
	Commit         uint64 // committed bytes
	CommitPeak     uint64
	KernelTotal    uint64
	KernelPaged    uint64
	KernelNonpaged uint64
	SystemCache    uint64
	PageSize       uint64
	Handles        uint32
	Processes      uint32
	Threads        uint32

	Pages    Pages
	HasPages bool // false when SystemMemoryListInformation was unavailable
}

// InUse is physical memory the system considers occupied, matching the
// "in use" figure in Task Manager.
func (s Snapshot) InUse() uint64 {
	if s.TotalPhys <= s.AvailPhys {
		return 0
	}
	return s.TotalPhys - s.AvailPhys
}

// AvailablePercent is the share of physical memory the system can hand out
// without trimming.
func (s Snapshot) AvailablePercent() float64 {
	if s.TotalPhys == 0 {
		return 0
	}
	return float64(s.AvailPhys) / float64(s.TotalPhys) * 100
}

// Collect reads the whole system state in one pass.
//
// The page list query is optional on purpose: a failure there must not fail
// the whole snapshot, because the rest of the numbers are still valid and
// more trustworthy than a fabricated zero.
func Collect() (Snapshot, error) {
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	if r, _, err := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))); r == 0 {
		return Snapshot{}, fmt.Errorf("GlobalMemoryStatusEx: %w", err)
	}

	var pi performanceInfo
	pi.Cb = uint32(unsafe.Sizeof(pi))
	if r, _, _ := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&pi)), unsafe.Sizeof(pi)); r == 0 || !sanePageSize(uint64(pi.PageSize)) {
		var legacy legacyPerformanceInfo
		if r2, _, err2 := procGetPerformanceInfo.Call(uintptr(unsafe.Pointer(&legacy)), unsafe.Sizeof(legacy)); r2 == 0 || !sanePageSize(uint64(legacy.PageSize)) {
			return Snapshot{}, fmt.Errorf("GetPerformanceInfo: unsupported layout (%v)", err2)
		}
		pi = performanceInfo{
			CommitTotal:       legacy.CommitTotal,
			CommitLimit:       legacy.CommitLimit,
			CommitPeak:        legacy.CommitPeak,
			PhysicalTotal:     legacy.PhysicalTotal,
			PhysicalAvailable: legacy.PhysicalAvailable,
			SystemCache:       legacy.SystemCache,
			KernelTotal:       legacy.KernelTotal,
			KernelPaged:       legacy.KernelPaged,
			KernelNonpaged:    legacy.KernelNonpaged,
			PageSize:          legacy.PageSize,
			HandleCount:       legacy.HandleCount,
			ProcessCount:      legacy.ProcessCount,
			ThreadCount:       legacy.ThreadCount,
		}
	}

	s := Snapshot{
		TotalPhys:      ms.TotalPhys,
		AvailPhys:      ms.AvailPhys,
		TotalCommit:    ms.TotalPageFile,
		Commit:         uint64(pi.CommitTotal) * uint64(pi.PageSize),
		CommitPeak:     uint64(pi.CommitPeak) * uint64(pi.PageSize),
		KernelTotal:    uint64(pi.KernelTotal) * uint64(pi.PageSize),
		KernelPaged:    uint64(pi.KernelPaged) * uint64(pi.PageSize),
		KernelNonpaged: uint64(pi.KernelNonpaged) * uint64(pi.PageSize),
		SystemCache:    uint64(pi.SystemCache) * uint64(pi.PageSize),
		PageSize:       uint64(pi.PageSize),
		Handles:        pi.HandleCount,
		Processes:      pi.ProcessCount,
		Threads:        pi.ThreadCount,
	}

	if pages, ok := readPages(); ok {
		pageSize := uint64(pi.PageSize)
		pages.Free *= pageSize
		pages.Zeroed *= pageSize
		pages.Modified *= pageSize
		pages.ModifiedNoIO *= pageSize
		pages.Bad *= pageSize
		pages.Standby *= pageSize
		s.Pages = pages
		s.HasPages = true
	}

	return s, nil
}

func sanePageSize(v uint64) bool {
	return v >= 4096 && v <= 65536 && v&(v-1) == 0
}

// readPages asks the memory manager for the page list lengths. Returns false
// when the class is not supported or the result does not look like pages.
func readPages() (Pages, bool) {
	buf := make([]uintptr, 64)
	var returned uintptr
	status, _, _ := procNtQuerySystemInformation.Call(
		uintptr(systemMemoryListInformation),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf))*unsafe.Sizeof(buf[0]),
		uintptr(unsafe.Pointer(&returned)),
	)
	if int32(status) < 0 {
		return Pages{}, false
	}
	if returned == 0 || returned > uintptr(len(buf))*unsafe.Sizeof(buf[0]) {
		return Pages{}, false
	}

	p := Pages{
		Zeroed:       uint64(buf[0]),
		Free:         uint64(buf[1]),
		Modified:     uint64(buf[2]),
		ModifiedNoIO: uint64(buf[3]),
		Bad:          uint64(buf[4]),
	}
	for i := 5; i <= 12; i++ {
		p.Standby += uint64(buf[i])
	}

	// A page list made of numbers this large is a parsing artifact, not a
	// system state. 2^48 pages is more memory than any Windows box has.
	const sanePages = 1 << 48
	total := p.Zeroed + p.Free + p.Modified + p.ModifiedNoIO + p.Bad + p.Standby
	if total == 0 || total > sanePages ||
		p.Standby > sanePages || p.Free > sanePages {
		return Pages{}, false
	}
	return p, true
}

// PurgeStandby drops clean cached pages from the standby list. It needs
// SeProfileSingleProcessPrivilege and does nothing without it.
//
// This is the one action that measurably lowers "in use" without touching
// any process, and it costs file cache, so callers must gate it hard.
func PurgeStandby() error {
	if err := EnablePrivilege("SeProfileSingleProcessPrivilege"); err != nil {
		return fmt.Errorf("enable SeProfileSingleProcessPrivilege: %w", err)
	}
	command := uint32(memoryPurgeStandbyList)
	status, _, _ := procNtSetSystemInformation.Call(
		uintptr(systemMemoryListInformation),
		uintptr(unsafe.Pointer(&command)),
		unsafe.Sizeof(command),
	)
	if int32(status) < 0 {
		return fmt.Errorf("NtSetSystemInformation(SystemMemoryListInformation): NTSTATUS 0x%08X", uint32(int32(status)))
	}
	return nil
}
