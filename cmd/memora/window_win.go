package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"memora/internal/config"
	"memora/internal/proc"
)

//go:embed webview2loader_amd64.dll
var embeddedWebView2LoaderDLL []byte

const (
	dashboardTitlePrefix = "MEMORA · 智能性能调度与内存真相中心"
	dashboardClassName   = "MEMORADashboardHostClass"
	dashboardAppUserID   = "MEMORA.PerformanceTruthCenter"

	minDashboardWidth  = 980
	minDashboardHeight = 640

	defaultDashWidth  = 1280
	defaultDashHeight = 830

	swHide     = 0
	swShow     = 5
	swMinimize = 6
	swRestore  = 9
	swMaximize = 3

	wmCreate        = 0x0001
	wmMove          = 0x0003
	wmSize          = 0x0005
	wmSetFocus      = 0x0007
	wmGetMinMaxInfo = 0x0024
	wmSetIcon       = 0x0080
	wmNcCalcSize    = 0x0083
	wmSysCommand    = 0x0112
	wmDpiChanged    = 0x02E0
	wmDashAction    = 0x8000 + 210

	iconSmall = 0
	iconBig   = 1

	wsOverlappedWindow = 0x00CF0000
	wsClipChildren     = 0x02000000
	wsClipSiblings     = 0x04000000
	wsExAppWindow      = 0x00040000

	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020

	dwmWindowCornerPreference = 33
	dwmBorderColor            = 34
	dwmCornerRound            = 2

	monitorDefaultToNearest = 2
	idcArrow                = 32512
	vkLButton               = 0x01
	coinitApartmentThreaded = 0x2

	scMoveCaption = 0xF012
	scSizeW       = 0xF001
	scSizeE       = 0xF002
	scSizeN       = 0xF003
	scSizeNW      = 0xF004
	scSizeNE      = 0xF005
	scSizeS       = 0xF006
	scSizeSW      = 0xF007
	scSizeSE      = 0xF008

	dashActShowFocus = 1
	dashActMinimize  = 2
	dashActMaximize  = 3
	dashActHide      = 4
	dashActDrag      = 5
	dashActResizeN   = 6
	dashActResizeS   = 7
	dashActResizeW   = 8
	dashActResizeE   = 9
	dashActResizeNW  = 10
	dashActResizeNE  = 11
	dashActResizeSW          = 12
	dashActResizeSE          = 13
	dashActDestroy           = 14
	dashActHibernateIfHidden = 15

	dashTimerHibernateID = 201
	dashHibernateIdleMS  = 18 * 1000 // 18s in Lightweight Mode -> release WebView2 renderer & trim working set
)

var (
	dwmapi = syscall.NewLazyDLL("dwmapi.dll")
	ole32  = syscall.NewLazyDLL("ole32.dll")

	procDwmSetWindowAttribute            = dwmapi.NewProc("DwmSetWindowAttribute")
	procDwmExtendFrameIntoClient         = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
	procCoInitializeEx                   = ole32.NewProc("CoInitializeEx")
	procEnumWindows                      = user32.NewProc("EnumWindows")
	procIsWindow                         = user32.NewProc("IsWindow")
	procIsWindowVisible                  = user32.NewProc("IsWindowVisible")
	procIsIconic                         = user32.NewProc("IsIconic")
	procIsZoomed                         = user32.NewProc("IsZoomed")
	procGetWindowTextW                   = user32.NewProc("GetWindowTextW")
	procShowWindow                       = user32.NewProc("ShowWindow")
	procBringWindowToTop                 = user32.NewProc("BringWindowToTop")
	procGetForegroundWindow              = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcID            = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput                = user32.NewProc("AttachThreadInput")
	procGetCurrentThreadId               = kernel32.NewProc("GetCurrentThreadId")
	procGetWindowRect                    = user32.NewProc("GetWindowRect")
	procGetClientRect                    = user32.NewProc("GetClientRect")
	procSetWindowPos                     = user32.NewProc("SetWindowPos")
	procSendMessageW                     = user32.NewProc("SendMessageW")
	procReleaseCapture                   = user32.NewProc("ReleaseCapture")
	procLoadImageW                       = user32.NewProc("LoadImageW")
	procLoadCursorW                      = user32.NewProc("LoadCursorW")
	procMonitorFromWindow                = user32.NewProc("MonitorFromWindow")
	procGetDpiForWindow                  = user32.NewProc("GetDpiForWindow")
	procGetAsyncKeyState                 = user32.NewProc("GetAsyncKeyState")
	procSetProcessExplicitAppUserModelID = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
)

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type minMaxInfo struct {
	Reserved     winPoint
	MaxSize      winPoint
	MaxPosition  winPoint
	MinTrackSize winPoint
	MaxTrackSize winPoint
}

type dwmMargins struct {
	CxLeftWidth    int32
	CxRightWidth   int32
	CyTopHeight    int32
	CyBottomHeight int32
}

type comCallbackObj struct {
	vtbl *uintptr
}

var (
	iidIUnknown = guid{
		Data1: 0x00000000,
		Data2: 0x0000,
		Data3: 0x0000,
		Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46},
	}
	iidEnvCompletedHandler = guid{
		Data1: 0x4e8a3389,
		Data2: 0xc9d8,
		Data3: 0x4bd2,
		Data4: [8]byte{0xb6, 0xb5, 0x12, 0x4f, 0xee, 0x6c, 0xc1, 0x4d},
	}
	iidCtrlCompletedHandler = guid{
		Data1: 0x6c4819f3,
		Data2: 0xc9b7,
		Data3: 0x4260,
		Data4: [8]byte{0x81, 0x27, 0xc9, 0xf5, 0xbd, 0xe7, 0xf6, 0x8c},
	}
)

var (
	dashMu          sync.Mutex
	dashStarted     bool
	dashURL         string
	dashHWND        atomic.Uintptr
	dashHiddenSince atomic.Int64
	dashWVEnv       uintptr
	dashWVCtrl      uintptr
	dashWVCore      uintptr
	dashBigIcon   uintptr
	dashSmallIcon uintptr
	dashBgBrush   uintptr

	wv2LoaderOnce sync.Once
	wv2CreateProc *syscall.LazyProc

	envCompletedVtbl = [4]uintptr{
		syscall.NewCallback(envHandlerQueryInterface),
		syscall.NewCallback(comHandlerAddRef),
		syscall.NewCallback(comHandlerRelease),
		syscall.NewCallback(envHandlerInvoke),
	}
	ctrlCompletedVtbl = [4]uintptr{
		syscall.NewCallback(ctrlHandlerQueryInterface),
		syscall.NewCallback(comHandlerAddRef),
		syscall.NewCallback(comHandlerRelease),
		syscall.NewCallback(ctrlHandlerInvoke),
	}
	envCompletedObj  = comCallbackObj{vtbl: &envCompletedVtbl[0]}
	ctrlCompletedObj = comCallbackObj{vtbl: &ctrlCompletedVtbl[0]}

	enumDashTarget uintptr
	enumDashCB     = syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		vis, _, _ := procIsWindowVisible.Call(hwnd)
		if vis == 0 {
			return 1
		}
		var buf [256]uint16
		n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n == 0 {
			return 1
		}
		title := syscall.UTF16ToString(buf[:n])
		if strings.HasPrefix(title, dashboardTitlePrefix) {
			enumDashTarget = hwnd
			return 0
		}
		return 1
	})
)

func init() {
	_, _, _ = procSetProcessExplicitAppUserModelID.Call(uintptr(unsafe.Pointer(mustUTF16(dashboardAppUserID))))
}

func ptrOf(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

func comQueryInterfaceFor(expected guid, this, riidPtr, ppvPtr uintptr) uintptr {
	if ppvPtr == 0 {
		return 0x80004003 // E_POINTER
	}
	out := (*uintptr)(ptrOf(ppvPtr))
	if riidPtr == 0 {
		*out = 0
		return 0x80004002 // E_NOINTERFACE
	}
	riid := *(*guid)(ptrOf(riidPtr))
	if riid == iidIUnknown || riid == expected {
		*out = this
		return 0 // S_OK
	}
	*out = 0
	return 0x80004002 // E_NOINTERFACE
}

func envHandlerQueryInterface(this, riid, ppv uintptr) uintptr {
	return comQueryInterfaceFor(iidEnvCompletedHandler, this, riid, ppv)
}

func ctrlHandlerQueryInterface(this, riid, ppv uintptr) uintptr {
	return comQueryInterfaceFor(iidCtrlCompletedHandler, this, riid, ppv)
}

func comHandlerAddRef(this uintptr) uintptr { return 1 }
func comHandlerRelease(this uintptr) uintptr { return 1 }

func comCall(obj uintptr, slot int, args ...uintptr) uintptr {
	if obj == 0 {
		return 0x80004003
	}
	vtbl := *(*uintptr)(ptrOf(obj))
	fn := *(*uintptr)(unsafe.Add(ptrOf(vtbl), uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func comAddRef(obj uintptr)  { _ = comCall(obj, 1) }
func comRelease(obj uintptr) { _ = comCall(obj, 2) }

func envHandlerInvoke(this uintptr, hr uintptr, env uintptr) uintptr {
	if int32(hr) < 0 || env == 0 {
		fallbackFromNativeHost()
		return 0
	}
	dashWVEnv = env
	comAddRef(env)
	hwnd := dashHWND.Load()
	if hwnd == 0 {
		return 0
	}
	res := comCall(env, 3, hwnd, uintptr(unsafe.Pointer(&ctrlCompletedObj))) // CreateCoreWebView2Controller
	if int32(res) < 0 {
		fallbackFromNativeHost()
	}
	return 0
}

func ctrlHandlerInvoke(this uintptr, hr uintptr, ctrl uintptr) uintptr {
	if int32(hr) < 0 || ctrl == 0 {
		fallbackFromNativeHost()
		return 0
	}
	dashWVCtrl = ctrl
	comAddRef(ctrl)

	hwnd := dashHWND.Load()
	syncWebView2Bounds(hwnd)
	_ = comCall(ctrl, 4, 1) // put_IsVisible(TRUE)

	var core uintptr
	if int32(comCall(ctrl, 25, uintptr(unsafe.Pointer(&core)))) >= 0 && core != 0 { // get_CoreWebView2
		dashWVCore = core
		var settings uintptr
		if int32(comCall(core, 3, uintptr(unsafe.Pointer(&settings)))) >= 0 && settings != 0 { // get_Settings
			_ = comCall(settings, 10, 0) // put_IsStatusBarEnabled(FALSE)
			_ = comCall(settings, 14, 0) // put_AreDefaultContextMenusEnabled(FALSE)
			_ = comCall(settings, 18, 0) // put_IsZoomControlEnabled(FALSE)
			comRelease(settings)
		}
		if dashURL != "" {
			_ = comCall(core, 5, uintptr(unsafe.Pointer(mustUTF16(dashURL)))) // Navigate
		}
	}
	syncWebView2Bounds(hwnd)
	_ = comCall(ctrl, 23) // NotifyParentWindowPositionChanged
	return 0
}

func syncWebView2Bounds(hwnd uintptr) {
	if dashWVCtrl == 0 || hwnd == 0 {
		return
	}
	var rc winRect
	ok, _, _ := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	if ok == 0 {
		return
	}
	_ = comCall(dashWVCtrl, 6, uintptr(unsafe.Pointer(&rc))) // put_Bounds (tagRECT passed by ptr on x64)
}

func ensureWebView2CreateProc() *syscall.LazyProc {
	wv2LoaderOnce.Do(func() {
		var candidates []string
		if exe, err := os.Executable(); err == nil && exe != "" {
			exeDir := filepath.Dir(exe)
			candidates = append(candidates, filepath.Join(exeDir, "WebView2Loader.dll"))
		}
		binDir := filepath.Join(config.Dir(), "bin")
		extracted := filepath.Join(binDir, "WebView2Loader.dll")
		if len(embeddedWebView2LoaderDLL) > 0 {
			needWrite := true
			if info, err := os.Stat(extracted); err == nil && info.Size() == int64(len(embeddedWebView2LoaderDLL)) {
				needWrite = false
			}
			if needWrite {
				_ = os.MkdirAll(binDir, 0o755)
				_ = os.WriteFile(extracted, embeddedWebView2LoaderDLL, 0o644)
			}
			candidates = append(candidates, extracted)
		}
		for _, p := range candidates {
			if p == "" {
				continue
			}
			if _, err := os.Stat(p); err == nil {
				dll := syscall.NewLazyDLL(p)
				proc := dll.NewProc("CreateCoreWebView2EnvironmentWithOptions")
				if proc.Find() == nil {
					wv2CreateProc = proc
					return
				}
			}
		}
	})
	return wv2CreateProc
}

func openDashboardWindow(url string) {
	dashMu.Lock()
	dashURL = url
	if hwnd := dashHWND.Load(); hwnd != 0 {
		if ok, _, _ := procIsWindow.Call(hwnd); ok != 0 {
			dashMu.Unlock()
			_, _, _ = procPostMessage.Call(hwnd, wmDashAction, dashActShowFocus, 0)
			return
		}
		dashHWND.Store(0)
		dashStarted = false
	}
	if dashStarted {
		dashMu.Unlock()
		return
	}
	if ensureWebView2CreateProc() == nil {
		dashMu.Unlock()
		openDashboardWindowFallback(url)
		return
	}
	dashStarted = true
	dashMu.Unlock()

	go runNativeDashboardWindow()
}

func closeDashboardWindow() {
	if hwnd := dashHWND.Load(); hwnd != 0 {
		_, _, _ = procPostMessage.Call(hwnd, wmDashAction, dashActDestroy, 0)
	}
}

func isDashboardWindowMaximized() bool {
	hwnd := dashHWND.Load()
	if hwnd == 0 {
		return false
	}
	if ok, _, _ := procIsWindow.Call(hwnd); ok == 0 {
		return false
	}
	zoomed, _, _ := procIsZoomed.Call(hwnd)
	return zoomed != 0
}

func controlDashboardWindow(action string) bool {
	hwnd := dashHWND.Load()
	if hwnd == 0 {
		hwnd = findDashboardHWND()
		if hwnd == 0 {
			return false
		}
	}
	act := strings.ToLower(strings.TrimSpace(action))
	if dashHWND.Load() == hwnd {
		var code uintptr
		switch act {
		case "focus":
			code = dashActShowFocus
		case "minimize":
			code = dashActMinimize
		case "maximize":
			code = dashActMaximize
		case "close":
			code = dashActHide
		case "drag":
			code = dashActDrag
		case "resize_n":
			code = dashActResizeN
		case "resize_s":
			code = dashActResizeS
		case "resize_w":
			code = dashActResizeW
		case "resize_e":
			code = dashActResizeE
		case "resize_nw":
			code = dashActResizeNW
		case "resize_ne":
			code = dashActResizeNE
		case "resize_sw":
			code = dashActResizeSW
		case "resize_se":
			code = dashActResizeSE
		case "state":
			return true
		default:
			return false
		}
		_, _, _ = procPostMessage.Call(hwnd, wmDashAction, code, 0)
		return true
	}

	// Fallback if running external Edge/Chrome window.
	switch act {
	case "minimize":
		_, _, _ = procShowWindow.Call(hwnd, swMinimize)
	case "maximize":
		zoomed, _, _ := procIsZoomed.Call(hwnd)
		if zoomed != 0 {
			_, _, _ = procShowWindow.Call(hwnd, swRestore)
		} else {
			_, _, _ = procShowWindow.Call(hwnd, swMaximize)
		}
	case "close":
		_, _, _ = procPostMessage.Call(hwnd, wmClose, 0, 0)
	case "focus":
		focusDashboardHWND(hwnd)
	default:
		return false
	}
	return true
}

func fallbackFromNativeHost() {
	url := dashURL
	hwnd := dashHWND.Load()
	if hwnd != 0 {
		_, _, _ = procPostMessage.Call(hwnd, wmDashAction, dashActDestroy, 0)
	}
	if url != "" {
		go openDashboardWindowFallback(url)
	}
}

func runNativeDashboardWindow() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	_, _, _ = procCoInitializeEx.Call(0, coinitApartmentThreaded)
	_, _, _ = procSetProcessExplicitAppUserModelID.Call(uintptr(unsafe.Pointer(mustUTF16(dashboardAppUserID))))

	defer func() {
		dashMu.Lock()
		dashStarted = false
		dashHWND.Store(0)
		dashMu.Unlock()
	}()

	bigIcon, smallIcon := loadAppIcons()
	dashBigIcon = bigIcon
	dashSmallIcon = smallIcon
	if dashBgBrush == 0 {
		// Dark slate matching titlebar to avoid any white flash prior to WebView2 initial paint.
		dashBgBrush, _, _ = procCreateSolidBrush.Call(0x002A170F)
	}

	hInst, _, _ := procGetModuleHandle.Call(0)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	className := mustUTF16(dashboardClassName)

	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   syscall.NewCallback(dashWndProc),
		HInstance:     hInst,
		HIcon:         bigIcon,
		HIconSm:       smallIcon,
		HCursor:       cursor,
		HbrBackground: dashBgBrush,
		LpszClassName: className,
	}
	_, _, _ = procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	dpi := int32(96)
	if procGetDpiForSystem.Find() == nil {
		if d, _, _ := procGetDpiForSystem.Call(); d >= 96 {
			dpi = int32(d)
		}
	}
	winW := scalePx(defaultDashWidth, dpi)
	winH := scalePx(defaultDashHeight, dpi)
	minW := scalePx(minDashboardWidth, dpi)
	minH := scalePx(minDashboardHeight, dpi)

	xPos := int32(100)
	yPos := int32(80)
	var pt winPoint
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	packedPt := uintptr(uint32(pt.X)) | (uintptr(uint32(pt.Y)) << 32)
	if hMon, _, _ := procMonitorFromPoint.Call(packedPt, monitorNearPt); hMon != 0 {
		var mi monitorInfo
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		if ok, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); ok != 0 {
			wk := mi.RcWork
			availW := wk.Right - wk.Left
			availH := wk.Bottom - wk.Top
			if availW > 0 && winW > availW-scalePx(40, dpi) {
				winW = availW - scalePx(40, dpi)
			}
			if availH > 0 && winH > availH-scalePx(40, dpi) {
				winH = availH - scalePx(40, dpi)
			}
			if winW < minW {
				winW = minW
			}
			if winH < minH {
				winH = minH
			}
			xPos = wk.Left + (availW-winW)/2
			yPos = wk.Top + (availH-winH)/2
			if xPos < wk.Left {
				xPos = wk.Left
			}
			if yPos < wk.Top {
				yPos = wk.Top
			}
		}
	}

	titlePtr := mustUTF16(dashboardTitlePrefix)
	hwnd, _, _ := procCreateWindowEx.Call(
		wsExAppWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titlePtr)),
		wsOverlappedWindow|wsClipChildren|wsClipSiblings,
		uintptr(xPos), uintptr(yPos),
		uintptr(winW), uintptr(winH),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		openDashboardWindowFallback(dashURL)
		return
	}
	dashHWND.Store(hwnd)
	dashHiddenSince.Store(0)

	if bigIcon != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconBig, bigIcon)
	}
	if smallIcon != 0 {
		_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, iconSmall, smallIcon)
	}

	corner := uint32(dwmCornerRound)
	_, _, _ = procDwmSetWindowAttribute.Call(hwnd, dwmWindowCornerPreference, uintptr(unsafe.Pointer(&corner)), 4)
	margins := dwmMargins{CyBottomHeight: 1}
	_, _, _ = procDwmExtendFrameIntoClient.Call(hwnd, uintptr(unsafe.Pointer(&margins)))
	_, _, _ = procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpFrameChanged)

	_, _, _ = procShowWindow.Call(hwnd, swShow)
	focusDashboardHWND(hwnd)

	// Create WebView2 Environment asynchronously on this STA thread.
	proc := ensureWebView2CreateProc()
	if proc == nil {
		fallbackFromNativeHost()
		return
	}
	userDataDir := filepath.Join(config.Dir(), "wv2_data")
	_ = os.MkdirAll(userDataDir, 0o755)
	hr, _, _ := proc.Call(
		0,
		uintptr(unsafe.Pointer(mustUTF16(userDataDir))),
		0,
		uintptr(unsafe.Pointer(&envCompletedObj)),
	)
	if int32(hr) < 0 {
		fallbackFromNativeHost()
	}

	var msg winMsg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		_, _, _ = procTranslateMsg.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = procDispatchMsg.Call(uintptr(unsafe.Pointer(&msg)))
	}

	cleanupWebView2COM()
}

func cleanupWebView2COM() {
	hadWV := dashWVCore != 0 || dashWVCtrl != 0 || dashWVEnv != 0
	if dashWVCore != 0 {
		comRelease(dashWVCore)
		dashWVCore = 0
	}
	if dashWVCtrl != 0 {
		_ = comCall(dashWVCtrl, 24) // Close()
		comRelease(dashWVCtrl)
		dashWVCtrl = 0
	}
	if dashWVEnv != 0 {
		comRelease(dashWVEnv)
		dashWVEnv = 0
	}
	if hadWV {
		go func() {
			time.Sleep(150 * time.Millisecond)
			runtime.GC()
			debug.FreeOSMemory()
			_ = proc.TrimWorkingSet(uint32(os.Getpid()))
		}()
	}
}

func getWindowDPI(hwnd uintptr) int32 {
	if hwnd != 0 && procGetDpiForWindow.Find() == nil {
		if d, _, _ := procGetDpiForWindow.Call(hwnd); d >= 96 {
			return int32(d)
		}
	}
	if procGetDpiForSystem.Find() == nil {
		if d, _, _ := procGetDpiForSystem.Call(); d >= 96 {
			return int32(d)
		}
	}
	return 96
}

func dashWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmNcCalcSize:
		if wParam != 0 {
			zoomed, _, _ := procIsZoomed.Call(hwnd)
			if zoomed != 0 {
				if hMon, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest); hMon != 0 {
					var mi monitorInfo
					mi.CbSize = uint32(unsafe.Sizeof(mi))
					if ok, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); ok != 0 {
						rgrc0 := (*winRect)(ptrOf(lParam))
						*rgrc0 = mi.RcWork
					}
				}
			}
			return 0
		}
		return 0

	case wmGetMinMaxInfo:
		if lParam != 0 {
			dpi := getWindowDPI(hwnd)
			mmi := (*minMaxInfo)(ptrOf(lParam))
			mmi.MinTrackSize.X = scalePx(minDashboardWidth, dpi)
			mmi.MinTrackSize.Y = scalePx(minDashboardHeight, dpi)
		}
		return 0

	case wmDpiChanged:
		if lParam != 0 {
			sug := (*winRect)(ptrOf(lParam))
			_, _, _ = procSetWindowPos.Call(
				hwnd, 0,
				uintptr(sug.Left), uintptr(sug.Top),
				uintptr(sug.Right-sug.Left), uintptr(sug.Bottom-sug.Top),
				swpNoZOrder|swpNoActivate,
			)
			syncWebView2Bounds(hwnd)
		}
		return 0

	case wmEraseBkgnd:
		if dashWVCtrl != 0 {
			return 1
		}
		if wParam != 0 && dashBgBrush != 0 {
			var rc winRect
			_, _, _ = procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
			_, _, _ = procFillRect.Call(wParam, uintptr(unsafe.Pointer(&rc)), dashBgBrush)
			return 1
		}

	case wmSize:
		syncWebView2Bounds(hwnd)
		return 0

	case wmMove:
		if dashWVCtrl != 0 {
			_ = comCall(dashWVCtrl, 23) // NotifyParentWindowPositionChanged
		}
		return 0

	case wmSetFocus:
		if dashWVCtrl != 0 {
			_ = comCall(dashWVCtrl, 12, 0) // MoveFocus(Programmatic)
		}
		return 0

	case wmDashAction:
		handleDashAction(hwnd, wParam)
		return 0

	case wmTimer:
		if wParam == dashTimerHibernateID {
			_, _, _ = procKillTimer.Call(hwnd, dashTimerHibernateID)
			vis, _, _ := procIsWindowVisible.Call(hwnd)
			if vis == 0 {
				cleanupWebView2COM()
				_, _, _ = procDestroyWindow.Call(hwnd)
			}
			return 0
		}

	case wmClose:
		dashHiddenSince.Store(time.Now().Unix())
		_, _, _ = procSetTimer.Call(hwnd, dashTimerHibernateID, dashHibernateIdleMS, 0)
		if dashWVCtrl != 0 {
			_ = comCall(dashWVCtrl, 4, 0) // put_IsVisible(FALSE)
		}
		_, _, _ = procShowWindow.Call(hwnd, swHide)
		return 0

	case wmDestroy:
		_, _, _ = procKillTimer.Call(hwnd, dashTimerHibernateID)
		dashHiddenSince.Store(0)
		cleanupWebView2COM()
		dashMu.Lock()
		dashHWND.Store(0)
		dashStarted = false
		dashMu.Unlock()
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}

	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func handleDashAction(hwnd uintptr, act uintptr) {
	switch act {
	case dashActShowFocus:
		dashHiddenSince.Store(0)
		_, _, _ = procKillTimer.Call(hwnd, dashTimerHibernateID)
		if dashWVCtrl != 0 {
			_ = comCall(dashWVCtrl, 4, 1) // put_IsVisible(TRUE)
		}
		focusDashboardHWND(hwnd)
		syncWebView2Bounds(hwnd)
		if dashWVCtrl != 0 {
			_ = comCall(dashWVCtrl, 23)
			_ = comCall(dashWVCtrl, 12, 0)
		}
		if dashWVCore != 0 && dashURL != "" {
			// Leave live React/DOM state intact if already loaded on this token.
		}

	case dashActMinimize:
		_, _, _ = procShowWindow.Call(hwnd, swMinimize)

	case dashActMaximize:
		zoomed, _, _ := procIsZoomed.Call(hwnd)
		if zoomed != 0 {
			_, _, _ = procShowWindow.Call(hwnd, swRestore)
		} else {
			_, _, _ = procShowWindow.Call(hwnd, swMaximize)
		}
		syncWebView2Bounds(hwnd)

	case dashActHide:
		dashHiddenSince.Store(time.Now().Unix())
		_, _, _ = procSetTimer.Call(hwnd, dashTimerHibernateID, dashHibernateIdleMS, 0)
		if dashWVCtrl != 0 {
			_ = comCall(dashWVCtrl, 4, 0)
		}
		_, _, _ = procShowWindow.Call(hwnd, swHide)

	case dashActDrag:
		ks, _, _ := procGetAsyncKeyState.Call(vkLButton)
		if (ks & 0x8000) == 0 {
			return
		}
		var pt winPoint
		_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		if zoomed, _, _ := procIsZoomed.Call(hwnd); zoomed != 0 {
			_, _, _ = procShowWindow.Call(hwnd, swRestore)
			var r winRect
			if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok != 0 {
				w := r.Right - r.Left
				h := r.Bottom - r.Top
				dpi := getWindowDPI(hwnd)
				nx := pt.X - w/2
				ny := pt.Y - scalePx(20, dpi)
				_, _, _ = procSetWindowPos.Call(hwnd, 0, uintptr(nx), uintptr(ny), uintptr(w), uintptr(h), swpNoZOrder|swpNoActivate)
				syncWebView2Bounds(hwnd)
			}
		}
		_, _, _ = procReleaseCapture.Call()
		lp := uintptr(uint16(pt.X)) | (uintptr(uint16(pt.Y)) << 16)
		_, _, _ = procSendMessageW.Call(hwnd, wmSysCommand, scMoveCaption, lp)

	case dashActResizeN, dashActResizeS, dashActResizeW, dashActResizeE,
		dashActResizeNW, dashActResizeNE, dashActResizeSW, dashActResizeSE:
		if zoomed, _, _ := procIsZoomed.Call(hwnd); zoomed != 0 {
			return
		}
		ks, _, _ := procGetAsyncKeyState.Call(vkLButton)
		if (ks & 0x8000) == 0 {
			return
		}
		var sc uintptr
		switch act {
		case dashActResizeN:
			sc = scSizeN
		case dashActResizeS:
			sc = scSizeS
		case dashActResizeW:
			sc = scSizeW
		case dashActResizeE:
			sc = scSizeE
		case dashActResizeNW:
			sc = scSizeNW
		case dashActResizeNE:
			sc = scSizeNE
		case dashActResizeSW:
			sc = scSizeSW
		case dashActResizeSE:
			sc = scSizeSE
		}
		var pt winPoint
		_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		_, _, _ = procReleaseCapture.Call()
		lp := uintptr(uint16(pt.X)) | (uintptr(uint16(pt.Y)) << 16)
		_, _, _ = procSendMessageW.Call(hwnd, wmSysCommand, sc, lp)

	case dashActHibernateIfHidden:
		vis, _, _ := procIsWindowVisible.Call(hwnd)
		if vis == 0 {
			_, _, _ = procKillTimer.Call(hwnd, dashTimerHibernateID)
			cleanupWebView2COM()
			_, _, _ = procDestroyWindow.Call(hwnd)
		}

	case dashActDestroy:
		_, _, _ = procKillTimer.Call(hwnd, dashTimerHibernateID)
		cleanupWebView2COM()
		_, _, _ = procDestroyWindow.Call(hwnd)
	}
}

func maybeHibernateHiddenDashboard(availPct float64) {
	if availPct <= 0 || availPct >= 18.0 {
		return
	}
	hwnd := dashHWND.Load()
	hiddenSince := dashHiddenSince.Load()
	if hwnd == 0 || hiddenSince == 0 {
		return
	}
	if time.Now().Unix()-hiddenSince < 15 {
		return
	}
	_, _, _ = procPostMessage.Call(hwnd, wmDashAction, dashActHibernateIfHidden, 0)
}

func findDashboardHWND() uintptr {
	if h := dashHWND.Load(); h != 0 {
		ok, _, _ := procIsWindow.Call(h)
		if ok != 0 {
			return h
		}
		dashHWND.Store(0)
	}
	enumDashTarget = 0
	_, _, _ = procEnumWindows.Call(enumDashCB, 0)
	return enumDashTarget
}

func focusDashboardHWND(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	iconic, _, _ := procIsIconic.Call(hwnd)
	if iconic != 0 {
		_, _, _ = procShowWindow.Call(hwnd, swRestore)
	} else {
		_, _, _ = procShowWindow.Call(hwnd, swShow)
	}
	_, _, _ = procBringWindowToTop.Call(hwnd)

	fg, _, _ := procGetForegroundWindow.Call()
	if fg != 0 && fg != hwnd {
		fgTid, _, _ := procGetWindowThreadProcID.Call(fg, 0)
		curTid, _, _ := procGetCurrentThreadId.Call()
		if fgTid != 0 && fgTid != curTid {
			_, _, _ = procAttachThreadInput.Call(curTid, fgTid, 1)
			_, _, _ = procSetForeground.Call(hwnd)
			_, _, _ = procBringWindowToTop.Call(hwnd)
			_, _, _ = procAttachThreadInput.Call(curTid, fgTid, 0)
			return
		}
	}
	_, _, _ = procSetForeground.Call(hwnd)
}

func loadAppIcons() (bigIcon uintptr, smallIcon uintptr) {
	bigIcon = loadIconFromICO(embeddedAppICO, 256)
	smallIcon = loadIconFromICO(embeddedAppICO, 32)
	hInst, _, _ := procGetModuleHandle.Call(0)
	if bigIcon == 0 && hInst != 0 {
		bigIcon, _, _ = procLoadImageW.Call(hInst, 1, 1, 256, 256, 0)
	}
	if smallIcon == 0 && hInst != 0 {
		smallIcon, _, _ = procLoadImageW.Call(hInst, 1, 1, 32, 32, 0)
	}
	if bigIcon == 0 && len(embeddedLogoPNG) > 0 {
		bigIcon, _, _ = procCreateIconFromResEx.Call(
			uintptr(unsafe.Pointer(&embeddedLogoPNG[0])),
			uintptr(len(embeddedLogoPNG)),
			1, 0x00030000, 256, 256, lrDefaultColor,
		)
	}
	if smallIcon == 0 && len(embeddedTrayPNG) > 0 {
		smallIcon, _, _ = procCreateIconFromResEx.Call(
			uintptr(unsafe.Pointer(&embeddedTrayPNG[0])),
			uintptr(len(embeddedTrayPNG)),
			1, 0x00030000, 32, 32, lrDefaultColor,
		)
	}
	return bigIcon, smallIcon
}

func loadIconFromICO(icoData []byte, desiredPx int) uintptr {
	if len(icoData) < 6 {
		return 0
	}
	if binary.LittleEndian.Uint16(icoData[0:2]) != 0 || binary.LittleEndian.Uint16(icoData[2:4]) != 1 {
		return 0
	}
	count := int(binary.LittleEndian.Uint16(icoData[4:6]))
	if count <= 0 || len(icoData) < 6+count*16 {
		return 0
	}

	bestOff, bestSize := 0, 0
	bestW, bestDiff := 0, 1<<30
	for i := 0; i < count; i++ {
		ent := icoData[6+i*16 : 6+(i+1)*16]
		w := int(ent[0])
		if w == 0 {
			w = 256
		}
		bytesInRes := int(binary.LittleEndian.Uint32(ent[8:12]))
		imgOffset := int(binary.LittleEndian.Uint32(ent[12:16]))
		if imgOffset < 0 || bytesInRes <= 0 || imgOffset+bytesInRes > len(icoData) {
			continue
		}
		diff := w - desiredPx
		if diff < 0 {
			diff = -diff
		}
		// Prefer exact match, or PNG-encoded large entry when desired >= 128.
		isPNG := bytes.HasPrefix(icoData[imgOffset:imgOffset+bytesInRes], []byte{0x89, 'P', 'N', 'G'})
		if diff < bestDiff || (diff == bestDiff && isPNG) {
			bestDiff = diff
			bestOff = imgOffset
			bestSize = bytesInRes
			bestW = w
		}
	}
	if bestSize == 0 {
		return 0
	}
	slice := icoData[bestOff : bestOff+bestSize]
	reqW := desiredPx
	if reqW <= 0 {
		reqW = bestW
	}
	h, _, _ := procCreateIconFromResEx.Call(
		uintptr(unsafe.Pointer(&slice[0])),
		uintptr(len(slice)),
		1,
		0x00030000,
		uintptr(reqW),
		uintptr(reqW),
		lrDefaultColor,
	)
	return h
}

var fallbackMu sync.Mutex
var fallbackLastLaunch time.Time

func openDashboardWindowFallback(url string) {
	fallbackMu.Lock()
	if hwnd := findDashboardHWND(); hwnd != 0 {
		focusDashboardHWND(hwnd)
		fallbackMu.Unlock()
		return
	}
	if !fallbackLastLaunch.IsZero() && time.Since(fallbackLastLaunch) < 2200*time.Millisecond {
		fallbackMu.Unlock()
		return
	}
	fallbackLastLaunch = time.Now()
	fallbackMu.Unlock()

	if browser := findAppBrowser(); browser != "" {
		uiDir := filepath.Join(config.Dir(), "ui_shell")
		_ = os.MkdirAll(uiDir, 0o755)
		args := []string{
			"--user-data-dir=" + uiDir,
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-extensions",
			"--disable-sync",
			"--disable-features=Translate,msEdgeSidebarV2",
			"--app=" + url,
			"--window-size=1280,830",
		}
		cmd := exec.Command(browser, args...)
		if cmd.Start() == nil {
			return
		}
	}
	_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start()
}

func findAppBrowser() string {
	candidates := []string{}
	for _, exe := range []string{"msedge.exe", "chrome.exe"} {
		if p, err := exec.LookPath(exe); err == nil {
			candidates = append(candidates, p)
		}
	}
	pf86 := os.Getenv("ProgramFiles(x86)")
	pf := os.Getenv("ProgramFiles")
	local := os.Getenv("LOCALAPPDATA")
	known := []string{
		filepath.Join(pf86, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pf, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(local, `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(pf, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(pf86, `Google\Chrome\Application\chrome.exe`),
	}
	candidates = append(candidates, known...)
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}
