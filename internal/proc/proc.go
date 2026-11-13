// Package proc enumerates processes and reads their memory footprint.
package proc

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	processQueryInformation        = 0x0400
	processVMRead                  = 0x0010
	processSetQuota                = 0x0100

	systemProcessInformation = 5
	statusInfoLengthMismatch = 0xC0000004
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	psapi    = syscall.NewLazyDLL("psapi.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")

	procEnumProcesses             = psapi.NewProc("EnumProcesses")
	procGetProcessMemoryInfo      = psapi.NewProc("GetProcessMemoryInfo")
	procEmptyWorkingSet           = psapi.NewProc("EmptyWorkingSet")
	procNtQuerySystemInformation  = ntdll.NewProc("NtQuerySystemInformation")
	procOpenProcess               = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetProcessTimes           = kernel32.NewProc("GetProcessTimes")
	procGetProcessHandleCount     = kernel32.NewProc("GetProcessHandleCount")
	procGetForegroundWindow       = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessID  = user32.NewProc("GetWindowThreadProcessId")
	procCloseHandle               = kernel32.NewProc("CloseHandle")
)

type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        uintptr
}

// systemProcessInfo mirrors SYSTEM_PROCESS_INFORMATION up through PrivatePageCount.
type systemProcessInfo struct {
	NextEntryOffset              uint32
	NumberOfThreads              uint32
	WorkingSetPrivateSize        int64
	HardFaultCount               uint32
	NumberOfThreadsHighWatermark uint32
	CycleTime                    uint64
	CreateTime                   int64
	UserTime                     int64
	KernelTime                   int64
	ImageName                    unicodeString
	BasePriority                 int32
	UniqueProcessId              uintptr
	InheritedFromUniqueProcessId uintptr
	HandleCount                  uint32
	SessionId                    uint32
	UniqueProcessKey             uintptr
	PeakVirtualSize              uintptr
	VirtualSize                  uintptr
	PageFaultCount               uint32
	PeakWorkingSetSize           uintptr
	WorkingSetSize               uintptr
	QuotaPeakPagedPoolUsage      uintptr
	QuotaPagedPoolUsage          uintptr
	QuotaPeakNonPagedPoolUsage   uintptr
	QuotaNonPagedPoolUsage       uintptr
	PagefileUsage                uintptr
	PeakPagefileUsage            uintptr
	PrivatePageCount             uintptr
}

// processMemoryCountersEx mirrors PROCESS_MEMORY_COUNTERS_EX.
type processMemoryCountersEx struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

// Info is one process reading. WorkingSet counts shared pages once per
// process, so summing it over all processes over-counts system-wide;
// PrivateWS and Private avoid cross-process double counting.
type Info struct {
	PID        uint32
	ParentPID  uint32
	SessionID  uint32
	Name       string
	WorkingSet uint64
	PrivateWS  uint64
	Private    uint64 // committed private bytes, does not double count
	Handles    uint32
	Threads    uint32
	CPUSeconds float64
}

// List returns all readable processes.
func List() ([]Info, error) {
	procs, _, err := ListAll()
	return procs, err
}

// ListAll enumerates processes using NtQuerySystemInformation(SystemProcessInformation)
// first, which reads every active process without opening per-process handles
// (eliminating privilege blind spots for services, WSL, and protected apps).
// Falls back to EnumProcesses + OpenProcess only if the kernel query fails.
func ListAll() ([]Info, bool, error) {
	if procs, err := listViaSystemInfo(); err == nil {
		return procs, true, nil
	}
	procs, err := listViaEnumProcesses()
	return procs, false, err
}

func listViaSystemInfo() ([]Info, error) {
	size := 1 << 20 // 1 MiB initial buffer
	for attempt := 0; attempt < 8; attempt++ {
		buf := make([]byte, size)
		var ret uint32
		status, _, _ := procNtQuerySystemInformation.Call(
			uintptr(systemProcessInformation),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			uintptr(unsafe.Pointer(&ret)),
		)
		if uint32(int32(status)) == statusInfoLengthMismatch {
			if int(ret) > size {
				size = int(ret) + (128 << 10)
			} else {
				size *= 2
			}
			continue
		}
		if int32(status) < 0 {
			return nil, fmt.Errorf("NtQuerySystemInformation(SystemProcessInformation): NTSTATUS 0x%08X", uint32(int32(status)))
		}
		return parseSystemProcesses(buf), nil
	}
	return nil, fmt.Errorf("NtQuerySystemInformation(SystemProcessInformation): buffer exhausted")
}

func parseSystemProcesses(buf []byte) []Info {
	out := make([]Info, 0, 256)
	if len(buf) == 0 {
		return out
	}
	base := uintptr(unsafe.Pointer(&buf[0]))
	off := 0
	minSize := int(unsafe.Sizeof(systemProcessInfo{}))
	for off+minSize <= len(buf) {
		entry := (*systemProcessInfo)(unsafe.Pointer(&buf[off]))
		pid := uint32(entry.UniqueProcessId)
		if pid != 0 {
			name := readUnicodeString(entry.ImageName, base, buf)
			if name == "" && pid == 4 {
				name = "System"
			}
			var privWS uint64
			if entry.WorkingSetPrivateSize > 0 {
				privWS = uint64(entry.WorkingSetPrivateSize)
			}
			out = append(out, Info{
				PID:        pid,
				ParentPID:  uint32(entry.InheritedFromUniqueProcessId),
				SessionID:  entry.SessionId,
				Name:       name,
				WorkingSet: uint64(entry.WorkingSetSize),
				PrivateWS:  privWS,
				Private:    uint64(entry.PrivatePageCount),
				Handles:    entry.HandleCount,
				Threads:    entry.NumberOfThreads,
				CPUSeconds: float64(entry.KernelTime+entry.UserTime) / 1e7,
			})
		}
		if entry.NextEntryOffset == 0 {
			break
		}
		off += int(entry.NextEntryOffset)
	}
	return out
}

func readUnicodeString(us unicodeString, base uintptr, buf []byte) string {
	if us.Length == 0 || us.Buffer < base {
		return ""
	}
	rel := int(us.Buffer - base)
	byteLen := int(us.Length)
	if rel < 0 || byteLen < 2 || rel+byteLen > len(buf) {
		return ""
	}
	n := byteLen / 2
	u16 := make([]uint16, n)
	for i := 0; i < n; i++ {
		off := rel + i*2
		u16[i] = uint16(buf[off]) | uint16(buf[off+1])<<8
	}
	return syscall.UTF16ToString(u16)
}

func listViaEnumProcesses() ([]Info, error) {
	pids := make([]uint32, 2048)
	for {
		var needed uint32
		r, _, err := procEnumProcesses.Call(
			uintptr(unsafe.Pointer(&pids[0])),
			uintptr(len(pids))*unsafe.Sizeof(pids[0]),
			uintptr(unsafe.Pointer(&needed)),
		)
		if r == 0 {
			return nil, fmt.Errorf("EnumProcesses: %w", err)
		}
		count := needed / uint32(unsafe.Sizeof(pids[0]))
		if int(count) >= len(pids) {
			pids = make([]uint32, len(pids)*2)
			continue
		}
		out := make([]Info, 0, count)
		for i := uint32(0); i < count; i++ {
			pid := pids[i]
			if pid == 0 || pid == 4 {
				continue
			}
			if info, ok := read(pid); ok {
				out = append(out, info)
			}
		}
		return out, nil
	}
}

func read(pid uint32) (Info, bool) {
	handle, ok := open(pid)
	if !ok {
		return Info{}, false
	}
	defer procCloseHandle.Call(uintptr(handle))

	var mc processMemoryCountersEx
	mc.Cb = uint32(unsafe.Sizeof(mc))
	r, _, _ := procGetProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&mc)),
		unsafe.Sizeof(mc),
	)
	if r == 0 {
		return Info{}, false
	}

	info := Info{
		PID:        pid,
		Name:       imageName(handle),
		WorkingSet: uint64(mc.WorkingSetSize),
		Private:    uint64(mc.PrivateUsage),
	}
	var handles uint32
	if r, _, _ := procGetProcessHandleCount.Call(uintptr(handle), uintptr(unsafe.Pointer(&handles))); r != 0 {
		info.Handles = handles
	}

	var created, exited, kernel, user syscall.Filetime
	if r, _, _ := procGetProcessTimes.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&created)),
		uintptr(unsafe.Pointer(&exited)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	); r != 0 {
		info.CPUSeconds = float64(kernel.Nanoseconds()+user.Nanoseconds()) / 1e9
	}

	return info, true
}

func open(pid uint32) (syscall.Handle, bool) {
	// QUERY_LIMITED is enough for memory counters and is granted for many
	// processes that reject VM_READ outright (session 0 services, protected
	// processes). Try the narrowest access first.
	for _, access := range []uintptr{
		processQueryLimitedInformation,
		processQueryInformation,
		processQueryInformation | processVMRead,
	} {
		r, _, _ := procOpenProcess.Call(access, 0, uintptr(pid))
		if r != 0 {
			return syscall.Handle(r), true
		}
	}
	return 0, false
}

func imageName(handle syscall.Handle) string {
	buf := make([]uint16, 512)
	size := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageName.Call(
		uintptr(handle),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r == 0 || size == 0 {
		return ""
	}
	name := syscall.UTF16ToString(buf[:size])
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '\\' || name[i] == '/' {
			return name[i+1:]
		}
	}
	return name
}

// Find returns the process entry for pid if present in procs.
func Find(procs []Info, pid uint32) (Info, bool) {
	for _, p := range procs {
		if p.PID == pid {
			return p, true
		}
	}
	return Info{}, false
}

// ForegroundPID returns the process that owns the foreground window, or 0.
func ForegroundPID() uint32 {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return 0
	}
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// TrimWorkingSet asks Windows to move a process's pages to the standby list.
// Requires PROCESS_SET_QUOTA plus query access; returns the call's error.
func TrimWorkingSet(pid uint32) error {
	r, _, err := procOpenProcess.Call(
		processSetQuota|processQueryLimitedInformation,
		0,
		uintptr(pid),
	)
	if r == 0 {
		r, _, err = procOpenProcess.Call(
			processSetQuota|processQueryInformation,
			0,
			uintptr(pid),
		)
	}
	if r == 0 {
		return fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	handle := syscall.Handle(r)
	defer procCloseHandle.Call(uintptr(handle))

	ok, _, callErr := procEmptyWorkingSet.Call(uintptr(handle))
	if ok == 0 {
		return fmt.Errorf("EmptyWorkingSet(%d): %w", pid, callErr)
	}
	return nil
}
