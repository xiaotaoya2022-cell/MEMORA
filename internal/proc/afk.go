package proc

import (
	"fmt"
	"time"
	"unsafe"
)

const processSuspendResume = 0x0800

var (
	procGetLastInputInfo = user32.NewProc("GetLastInputInfo")
	procGetTickCount64   = kernel32.NewProc("GetTickCount64")
	procNtSuspendProcess = ntdll.NewProc("NtSuspendProcess")
	procNtResumeProcess  = ntdll.NewProc("NtResumeProcess")
)

type lastInputInfo struct {
	CbSize uint32
	DwTime uint32
}

// UserIdleDuration returns how long the user has been away from mouse and keyboard input.
func UserIdleDuration() time.Duration {
	if procGetLastInputInfo.Find() != nil || procGetTickCount64.Find() != nil {
		return 0
	}
	var lii lastInputInfo
	lii.CbSize = uint32(unsafe.Sizeof(lii))
	r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii)))
	if r == 0 {
		return 0
	}
	tick64, _, _ := procGetTickCount64.Call()
	now32 := uint32(tick64)
	if now32 < lii.DwTime {
		return 0
	}
	return time.Duration(now32-lii.DwTime) * time.Millisecond
}

// SuspendProcess temporarily suspends a process via NtSuspendProcess (used only for
// runaway background spawners while the user is AFK, with instant input-triggered resume).
func SuspendProcess(pid uint32) error {
	if pid <= 4 {
		return fmt.Errorf("refusing to suspend system pid %d", pid)
	}
	if err := procNtSuspendProcess.Find(); err != nil {
		return err
	}
	h, _, err := procOpenProcess.Call(processSuspendResume, 0, uintptr(pid))
	if h == 0 {
		return err
	}
	defer procCloseHandle.Call(h)

	st, _, _ := procNtSuspendProcess.Call(h)
	if st != 0 {
		return fmt.Errorf("NtSuspendProcess status 0x%x", uint32(st))
	}
	return nil
}

// ResumeProcess resumes a previously suspended process via NtResumeProcess.
func ResumeProcess(pid uint32) error {
	if pid <= 4 {
		return nil
	}
	if err := procNtResumeProcess.Find(); err != nil {
		return err
	}
	h, _, err := procOpenProcess.Call(processSuspendResume, 0, uintptr(pid))
	if h == 0 {
		return err
	}
	defer procCloseHandle.Call(h)

	st, _, _ := procNtResumeProcess.Call(h)
	if st != 0 {
		return fmt.Errorf("NtResumeProcess status 0x%x", uint32(st))
	}
	return nil
}
