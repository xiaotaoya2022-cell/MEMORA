package proc

import (
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	// PROCESS_INFORMATION_CLASS values from processthreadsapi.h
	processMemoryPriority  = 0
	processPowerThrottling = 4

	MemoryPriorityVeryLow     = 1
	MemoryPriorityLow         = 2
	MemoryPriorityMedium      = 3
	MemoryPriorityBelowNormal = 4
	MemoryPriorityNormal      = 5

	normalPriorityClass      = 0x00000020
	belowNormalPriorityClass = 0x00004000

	powerThrottlingCurrentVersion = 1
	powerThrottlingExecutionSpeed = 0x1

	qunsNotPresent           = 1
	qunsBusy                 = 2
	qunsRunningD3DFullscreen = 3
	qunsPresentationMode     = 4
	qunsAcceptsNotifications = 5
)

type memoryPriorityInfo struct {
	MemoryPriority uint32
}

type powerThrottlingState struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}

var (
	shell32 = syscall.NewLazyDLL("shell32.dll")

	procSetProcessInformation        = kernel32.NewProc("SetProcessInformation")
	procGetProcessInformation        = kernel32.NewProc("GetProcessInformation")
	procGetPriorityClass             = kernel32.NewProc("GetPriorityClass")
	procSetPriorityClass             = kernel32.NewProc("SetPriorityClass")
	procSHQueryUserNotificationState = shell32.NewProc("SHQueryUserNotificationState")
	procEnumWindows                  = user32.NewProc("EnumWindows")
	procIsWindowVisible              = user32.NewProc("IsWindowVisible")

	enumWinMu  sync.Mutex
	enumWinMap map[uint32]bool
	enumWinCB  = syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		v, _, _ := procIsWindowVisible.Call(hwnd)
		if v != 0 && enumWinMap != nil {
			var pid uint32
			_, _, _ = procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if pid != 0 {
				enumWinMap[pid] = true
			}
		}
		return 1
	})
)

// SetMemoryPriority hints Windows Memory Manager to deprioritize or restore
// pages belonging to pid without immediately forcing hard page faults.
func SetMemoryPriority(pid uint32, priority uint32) error {
	h, _, err := procOpenProcess.Call(uintptr(processSetInformation), 0, uintptr(pid))
	if h == 0 {
		return fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer procCloseHandle.Call(h)

	info := memoryPriorityInfo{MemoryPriority: priority}
	r, _, err := procSetProcessInformation.Call(
		h,
		uintptr(processMemoryPriority),
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if r == 0 {
		return fmt.Errorf("SetProcessInformation(MemoryPriority, pid=%d): %w", pid, err)
	}
	return nil
}

// GetMemoryPriority reads the current MEMORY_PRIORITY_INFORMATION of pid.
func GetMemoryPriority(pid uint32) (uint32, error) {
	h, _, err := procOpenProcess.Call(uintptr(processQueryLimitedInformation), 0, uintptr(pid))
	if h == 0 {
		return 0, fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer procCloseHandle.Call(h)

	var info memoryPriorityInfo
	r, _, err := procGetProcessInformation.Call(
		h,
		uintptr(processMemoryPriority),
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if r == 0 {
		return 0, fmt.Errorf("GetProcessInformation(MemoryPriority, pid=%d): %w", pid, err)
	}
	return info.MemoryPriority, nil
}

// SetEcoQoS enables EcoQoS background CPU efficiency throttling on pid,
// or releases throttling control back to the OS when enable is false.
func SetEcoQoS(pid uint32, enable bool) error {
	h, _, err := procOpenProcess.Call(uintptr(processSetInformation), 0, uintptr(pid))
	if h == 0 {
		return fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer procCloseHandle.Call(h)

	state := powerThrottlingState{Version: powerThrottlingCurrentVersion}
	if enable {
		state.ControlMask = powerThrottlingExecutionSpeed
		state.StateMask = powerThrottlingExecutionSpeed
	}
	r, _, err := procSetProcessInformation.Call(
		h,
		uintptr(processPowerThrottling),
		uintptr(unsafe.Pointer(&state)),
		unsafe.Sizeof(state),
	)
	if r == 0 {
		return fmt.Errorf("SetProcessInformation(PowerThrottling, pid=%d): %w", pid, err)
	}
	return nil
}

// SetBelowNormalPriority toggles BELOW_NORMAL_PRIORITY_CLASS without altering
// processes whose user/OS priority was already set outside NORMAL / BELOW_NORMAL.
func SetBelowNormalPriority(pid uint32, enable bool) error {
	h, _, err := procOpenProcess.Call(uintptr(processSetInformation|processQueryLimitedInformation), 0, uintptr(pid))
	if h == 0 {
		return fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer procCloseHandle.Call(uintptr(h))

	cur, _, _ := procGetPriorityClass.Call(h)
	if cur == 0 {
		return fmt.Errorf("GetPriorityClass(%d) returned 0", pid)
	}
	var target uintptr
	if enable {
		if cur != normalPriorityClass {
			return nil
		}
		target = belowNormalPriorityClass
	} else {
		if cur != belowNormalPriorityClass {
			return nil
		}
		target = normalPriorityClass
	}
	r, _, err := procSetPriorityClass.Call(h, target)
	if r == 0 {
		return fmt.Errorf("SetPriorityClass(%d, 0x%X): %w", pid, target, err)
	}
	return nil
}

// IsFullscreenOrBusy queries SHQueryUserNotificationState to detect exclusive D3D
// games, fullscreen apps, or presentation mode so MEMORA goes strictly quiet.
// Desktop/live-wallpaper hosts (such as Wallpaper Engine) trigger QUNS_BUSY without
// being a foreground fullscreen game and are exempted.
func IsFullscreenOrBusy() (uint32, bool) {
	if err := procSHQueryUserNotificationState.Find(); err != nil {
		return 0, false
	}
	var state uint32
	hr, _, _ := procSHQueryUserNotificationState.Call(uintptr(unsafe.Pointer(&state)))
	if int32(hr) < 0 {
		return 0, false
	}
	switch state {
	case qunsRunningD3DFullscreen, qunsPresentationMode:
		return state, true
	case qunsBusy:
		fgPID := ForegroundPID()
		fgName := ProcessNameByPID(fgPID)
		if isFullscreenBusyExempt(fgPID, fgName) {
			return qunsAcceptsNotifications, false
		}
		return state, true
	default:
		return state, false
	}
}

func isFullscreenBusyExempt(fgPID uint32, fgName string) bool {
	if fgPID == 0 {
		return true
	}
	switch strings.ToLower(fgName) {
	case "", "explorer.exe", "wallpaper64.exe", "wallpaper32.exe", "sihost.exe",
		"shellexperiencehost.exe", "searchhost.exe", "applicationframehost.exe",
		"lockapp.exe":
		return true
	default:
		return false
	}
}

// NotificationStateLabel turns SHQueryUserNotificationState into a concise label.
func NotificationStateLabel(state uint32) string {
	switch state {
	case qunsBusy:
		return "全屏应用"
	case qunsRunningD3DFullscreen:
		return "全屏D3D游戏"
	case qunsPresentationMode:
		return "演示模式"
	case qunsNotPresent:
		return "屏保/锁屏"
	default:
		return "正常桌面"
	}
}

// VisibleWindowPIDs returns all PIDs that own at least one visible top-level window.
func VisibleWindowPIDs() map[uint32]bool {
	enumWinMu.Lock()
	defer enumWinMu.Unlock()

	visible := make(map[uint32]bool, 64)
	enumWinMap = visible
	_, _, _ = procEnumWindows.Call(enumWinCB, 0)
	enumWinMap = nil
	return visible
}

// FamilyPIDs collects rootPID, all of its descendant processes, and its direct
// non-shell launcher parent (e.g. Steam/launcher hosting a foreground game),
// protecting the active user tree from throttling or working-set trims.
func FamilyPIDs(procs []Info, rootPID uint32) map[uint32]bool {
	out := map[uint32]bool{}
	if rootPID == 0 {
		return out
	}

	byPID := make(map[uint32]Info, len(procs))
	children := make(map[uint32][]uint32, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
		if p.ParentPID != 0 && p.ParentPID != p.PID {
			children[p.ParentPID] = append(children[p.ParentPID], p.PID)
		}
	}

	out[rootPID] = true
	queue := []uint32{rootPID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, ch := range children[cur] {
			if !out[ch] {
				out[ch] = true
				queue = append(queue, ch)
			}
		}
	}

	if root, ok := byPID[rootPID]; ok && root.ParentPID != 0 && root.ParentPID != 4 {
		if parent, pok := byPID[root.ParentPID]; pok && !IsShellOrSystemHost(parent.Name) {
			out[parent.PID] = true
		}
	}
	return out
}

// IsShellOrSystemHost identifies generic OS/shell process launchers whose other
// unrelated children should not inherit foreground immunity or orphan blame.
func IsShellOrSystemHost(name string) bool {
	switch strings.ToLower(name) {
	case "system", "smss.exe", "csrss.exe", "wininit.exe", "winlogon.exe",
		"services.exe", "svchost.exe", "explorer.exe", "cmd.exe",
		"powershell.exe", "pwsh.exe", "windowsterminal.exe", "wt.exe",
		"openconsole.exe", "conhost.exe", "taskmgr.exe", "sihost.exe",
		"userinit.exe", "runtimebroker.exe", "applicationframehost.exe":
		return true
	default:
		return false
	}
}
