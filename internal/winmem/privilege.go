package winmem

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	procOpenProcessToken      = advapi32.NewProc("OpenProcessToken")
	procLookupPrivilegeValue  = advapi32.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPrivileges = advapi32.NewProc("AdjustTokenPrivileges")
	procGetCurrentProcess     = kernel32.NewProc("GetCurrentProcess")
)

const (
	tokenAdjustPrivileges = 0x0020
	tokenQuery            = 0x0008
	sePrivilegeEnabled    = 0x0002
	errorNotAllAssigned   = 1300
)

type luid struct {
	LowPart  uint32
	HighPart int32
}

type tokenPrivileges struct {
	PrivilegeCount uint32
	Privileges     [1]luid
	Attributes     uint32
}

// EnablePrivilege turns on a single token privilege for the current process.
// Used only by actions that actually require it, never at startup.
func EnablePrivilege(name string) error {
	var token syscall.Handle
	r, _, err := procGetCurrentProcess.Call()
	if r == 0 {
		return fmt.Errorf("GetCurrentProcess: %w", err)
	}
	ok, _, callErr := procOpenProcessToken.Call(
		r,
		uintptr(tokenAdjustPrivileges|tokenQuery),
		uintptr(unsafe.Pointer(&token)),
	)
	if ok == 0 {
		return fmt.Errorf("OpenProcessToken: %w", callErr)
	}
	defer syscall.CloseHandle(token)

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	var l luid
	ok, _, callErr = procLookupPrivilegeValue.Call(
		0,
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&l)),
	)
	if ok == 0 {
		return fmt.Errorf("LookupPrivilegeValue(%s): %w", name, callErr)
	}

	tp := tokenPrivileges{
		PrivilegeCount: 1,
		Privileges:     [1]luid{l},
		Attributes:     sePrivilegeEnabled,
	}
	var previous tokenPrivileges
	var returnLen uint32
	var previousLen uint32 = uint32(unsafe.Sizeof(previous))
	ok, _, callErr = procAdjustTokenPrivileges.Call(
		uintptr(token),
		0,
		uintptr(unsafe.Pointer(&tp)),
		uintptr(previousLen),
		uintptr(unsafe.Pointer(&returnLen)),
		uintptr(unsafe.Pointer(&previous)),
	)
	if ok == 0 {
		return fmt.Errorf("AdjustTokenPrivileges: %w", callErr)
	}

	lastErr := syscall.GetLastError()
	if errno, _ := lastErr.(syscall.Errno); uintptr(errno) == errorNotAllAssigned {
		return fmt.Errorf("token does not hold %s (run elevated)", name)
	}
	return nil
}
