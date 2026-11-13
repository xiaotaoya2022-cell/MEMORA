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
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	psapi    = syscall.NewLazyDLL("psapi.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

	procEnumProcesses             = psapi.NewProc("EnumProcesses")
	procGetProcessMemoryInfo      = psapi.NewProc("GetProcessMemoryInfo")
	procEmptyWorkingSet           = psapi.NewProc("EmptyWorkingSet")
	procOpenProcess               = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetProcessTimes           = kernel32.NewProc("GetProcessTimes")
	procGetProcessHandleCount     = kernel32.NewProc("GetProcessHandleCount")
	procGetForegroundWindow       = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessID  = user32.NewProc("GetWindowThreadProcessId")
	procCloseHandle               = kernel32.NewProc("CloseHandle")
)

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
// process, so summing it over all processes over-counts system-wide.
type Info struct {
	PID        uint32
	Name       string
	WorkingSet uint64
	Private    uint64 // committed private bytes, does not double count
	Handles    uint32
	CPUSeconds float64
}

// List returns every process the current user can open. Processes that
// cannot be opened are skipped silently; that is a fact of life for an
// unelevated tool and should be visible in the total, not raised as errors.
func List() ([]Info, error) {
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
