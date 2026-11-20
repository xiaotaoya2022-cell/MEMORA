package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	gdi32 = syscall.NewLazyDLL("gdi32.dll")

	procGetDC                 = user32.NewProc("GetDC")
	procReleaseDC             = user32.NewProc("ReleaseDC")
	procBeginPaint            = user32.NewProc("BeginPaint")
	procEndPaint              = user32.NewProc("EndPaint")
	procFillRect              = user32.NewProc("FillRect")
	procDrawTextW             = user32.NewProc("DrawTextW")
	procDrawIconEx            = user32.NewProc("DrawIconEx")
	procInvalidateRect        = user32.NewProc("InvalidateRect")
	procTrackMouseEvent       = user32.NewProc("TrackMouseEvent")
	procMonitorFromPoint      = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW       = user32.NewProc("GetMonitorInfoW")
	procSetCapture            = user32.NewProc("SetCapture")
	procGetDpiForSystem       = user32.NewProc("GetDpiForSystem")
	procSetProcessDpiAwareCtx = user32.NewProc("SetProcessDpiAwarenessContext")
	procSetFocus              = user32.NewProc("SetFocus")

	procCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	procDeleteDC               = gdi32.NewProc("DeleteDC")
	procSelectObject           = gdi32.NewProc("SelectObject")
	procDeleteObject           = gdi32.NewProc("DeleteObject")
	procBitBlt                 = gdi32.NewProc("BitBlt")
	procCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	procCreatePen              = gdi32.NewProc("CreatePen")
	procRoundRect              = gdi32.NewProc("RoundRect")
	procEllipse                = gdi32.NewProc("Ellipse")
	procSetBkMode              = gdi32.NewProc("SetBkMode")
	procSetTextColor           = gdi32.NewProc("SetTextColor")
	procCreateFontW            = gdi32.NewProc("CreateFontW")
	procEnumFontFamiliesExW    = gdi32.NewProc("EnumFontFamiliesExW")
)

const (
	wsPopup      = 0x80000000
	wsVisible    = 0x10000000
	wsExTopmost  = 0x00000008
	wsExToolWin  = 0x00000080

	csVRedraw    = 0x0001
	csHRedraw    = 0x0002
	csDropShadow = 0x00020000

	wmActivate   = 0x0006
	wmKillFocus  = 0x0008
	wmPaint      = 0x000F
	wmEraseBkgnd = 0x0014
	wmKeyDown    = 0x0100
	wmMouseMove  = 0x0200
	wmMouseLeave = 0x02A3

	vkEscape               = 0x1B
	vkRButton              = 0x02
	vkMButton              = 0x04
	trayMenuDismissTimerID = 1
	dtSingleLine           = 0x00000020
	dtVCenter    = 0x00000004
	dtLeft       = 0x00000000
	dtRight      = 0x00000002
	dtEndEllipsis = 0x00008000

	srcCopy       = 0x00CC0020
	tmeLeave      = 0x00000002
	monitorNearPt = 0x00000002
)

type paintStruct struct {
	HDC        uintptr
	FErase     int32
	RcPaint    winRect
	FRestore   int32
	FIncUpdate int32
	RgbReserve [32]byte
}

type trackMouseEventStruct struct {
	CbSize      uint32
	DwFlags     uint32
	HWndTrack   uintptr
	DwHoverTime uint32
}

type monitorInfo struct {
	CbSize    uint32
	RcMonitor winRect
	RcWork    winRect
	DwFlags   uint32
}

type logFontW struct {
	LfHeight         int32
	LfWidth          int32
	LfEscapement     int32
	LfOrientation    int32
	LfWeight         int32
	LfItalic         byte
	LfUnderline      byte
	LfStrikeOut      byte
	LfCharSet        byte
	LfOutPrecision   byte
	LfClipPrecision  byte
	LfQuality        byte
	LfPitchAndFamily byte
	LfFaceName       [32]uint16
}

type trayMenuItem struct {
	ID       uint16
	Label    string
	Badge    string
	Sep      bool
	Bold     bool
	Danger   bool
	KeepOpen bool
	DotCol   uint32 // COLORREF 0x00BBGGRR, 0 if none
	Rect     winRect
}

type trayMenuState struct {
	ParentHwnd  uintptr
	MenuHwnd    uintptr
	Icon        uintptr
	DPI         int32
	Width       int32
	Height      int32
	HeaderH     int32
	Mode        string
	AvailPct    float64
	SelfMB      uint64
	HasSample   bool
	EdgeEnabled bool
	OptBusy     bool
	OptNote     string
	OptNoteAt   time.Time
	Language    string
	ModeLabel   string
	ModeColor   uint32
	MemBadge    string
	Items       []trayMenuItem
	HoverIdx    int
	Tracking    bool
	OpenedAt    time.Time
}

var (
	trayMenuClassOnce sync.Once
	activeTrayMenu    *trayMenuState
	trayPopupOpen     atomic.Bool

	resolvedFontsMu   sync.Mutex
	resolvedFontCache = make(map[string]bool)
	enumFontProbeMu   sync.Mutex
	enumFontFound     atomic.Bool
	enumFontCheckCB   = syscall.NewCallback(func(lpelfe, lpntme, fontType, lParam uintptr) uintptr {
		enumFontFound.Store(true)
		return 0
	})
)

func isFontInstalled(faceName string) bool {
	if faceName == "" {
		return false
	}
	resolvedFontsMu.Lock()
	if ok, seen := resolvedFontCache[faceName]; seen {
		resolvedFontsMu.Unlock()
		return ok
	}
	resolvedFontsMu.Unlock()

	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return false
	}
	defer procReleaseDC.Call(0, hdc)

	var lf logFontW
	lf.LfCharSet = 1 // DEFAULT_CHARSET
	if u, err := syscall.UTF16FromString(faceName); err == nil {
		copy(lf.LfFaceName[:len(lf.LfFaceName)-1], u)
	}

	enumFontProbeMu.Lock()
	enumFontFound.Store(false)
	_, _, _ = procEnumFontFamiliesExW.Call(
		hdc,
		uintptr(unsafe.Pointer(&lf)),
		enumFontCheckCB,
		0,
		0,
	)
	isInstalled := enumFontFound.Load()
	enumFontProbeMu.Unlock()

	resolvedFontsMu.Lock()
	resolvedFontCache[faceName] = isInstalled
	resolvedFontsMu.Unlock()
	return isInstalled
}

func resolveInstalledFont(candidates ...string) string {
	for _, c := range candidates {
		if isFontInstalled(c) {
			return c
		}
	}
	return "Microsoft YaHei UI"
}

func enablePerMonitorDPIAwareness() {
	if procSetProcessDpiAwareCtx.Find() == nil {
		ctx := ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
		_, _, _ = procSetProcessDpiAwareCtx.Call(ctx)
	}
}

func scalePx(v int32, dpi int32) int32 {
	if dpi <= 0 {
		dpi = 96
	}
	return (v*dpi + 48) / 96
}

func (st *trayMenuState) rebuildItems() {
	inUsePct := 0.0
	if st.HasSample {
		inUsePct = 100 - st.AvailPct
		if inUsePct < 0 {
			inUsePct = 0
		} else if inUsePct > 100 {
			inUsePct = 100
		}
	}

	isEn := strings.HasPrefix(strings.ToLower(st.Language), "en")

	modeItemLabel := "智能调度模式：已开启"
	modeItemBadge := "切换关闭"
	modeBadgeCol := uint32(0x00699605) // #059669 emerald
	headerStatus := "智能调度中"
	if isEn {
		modeItemLabel = "Smart Dispatch: Enabled"
		modeItemBadge = "Turn Off"
		headerStatus = "Smart Dispatching"
	}
	if st.Mode != "guard" {
		modeItemLabel = "智能调度模式：已关闭"
		modeItemBadge = "点击开启"
		modeBadgeCol = uint32(0x008B7464) // #64748B slate
		headerStatus = "已关闭"
		if isEn {
			modeItemLabel = "Smart Dispatch: Disabled"
			modeItemBadge = "Turn On"
			headerStatus = "Disabled"
		}
	}

	edgeItemLabel := "贴边真相光尺：已开启"
	edgeItemBadge := "点击关闭"
	edgeBadgeCol := uint32(0x00EB6325) // #2563EB MEMORA blue
	if isEn {
		edgeItemLabel = "Edge Horizon: Enabled"
		edgeItemBadge = "Turn Off"
	}
	if !st.EdgeEnabled {
		edgeItemLabel = "贴边真相光尺：已关闭"
		edgeItemBadge = "点击开启"
		edgeBadgeCol = uint32(0x008B7464)
		if isEn {
			edgeItemLabel = "Edge Horizon: Disabled"
			edgeItemBadge = "Turn On"
		}
	}

	optLabel := "立即深度释压"
	optBadge := "单次"
	if isEn {
		optLabel = "Deep Pressure Relief Now"
		optBadge = "Action"
	}
	optDotCol := uint32(0x00EB6325)
	if st.OptBusy {
		optLabel = "正在深度释压..."
		optBadge = "执行中"
		if isEn {
			optLabel = "Relieving Pressure..."
			optBadge = "Running"
		}
	} else if st.OptNote != "" && time.Since(st.OptNoteAt) <= 8*time.Second {
		optLabel = "深度释压 (" + strings.TrimSpace(st.OptNote) + ")"
		optBadge = "已完成"
		if isEn {
			optLabel = "Pressure Relief (" + strings.TrimSpace(st.OptNote) + ")"
			optBadge = "Done"
		}
		optDotCol = uint32(0x00699605)
	}

	memBadge := "采样中"
	if isEn {
		memBadge = "Sampling..."
	}
	if st.HasSample {
		if st.SelfMB > 0 {
			if isEn {
				memBadge = fmt.Sprintf("Used %.0f%% · Avail %.0f%% · Self %dMB", inUsePct, st.AvailPct, st.SelfMB)
			} else {
				memBadge = fmt.Sprintf("已用 %.0f%% · 可用 %.0f%% · 自身占用 %dMB", inUsePct, st.AvailPct, st.SelfMB)
			}
		} else {
			if isEn {
				memBadge = fmt.Sprintf("Used %.0f%% · Avail %.0f%%", inUsePct, st.AvailPct)
			} else {
				memBadge = fmt.Sprintf("已用 %.0f%% · 可用 %.0f%%", inUsePct, st.AvailPct)
			}
		}
	} else if st.SelfMB > 0 {
		if isEn {
			memBadge = fmt.Sprintf("Sampling... · Self %dMB", st.SelfMB)
		} else {
			memBadge = fmt.Sprintf("采样中 · 自身占用 %dMB", st.SelfMB)
		}
	}

	var items []trayMenuItem
	if isEn {
		items = []trayMenuItem{
			{ID: idTrayOpenUI, Label: "Open MEMORA Dispatch Center", Badge: "Home", Bold: true, DotCol: 0x00EB6325},
			{ID: idTrayOptimize, Label: optLabel, Badge: optBadge, DotCol: optDotCol, KeepOpen: true},
			{Sep: true},
			{ID: idTrayToggleMode, Label: modeItemLabel, Badge: modeItemBadge, DotCol: modeBadgeCol, KeepOpen: true},
			{ID: idTrayToggleEdge, Label: edgeItemLabel, Badge: edgeItemBadge, DotCol: edgeBadgeCol, KeepOpen: true},
			{Sep: true},
			{ID: idTrayStatus, Label: "Quick Status Summary...", DotCol: 0x009B7464},
			{ID: idTrayDeepStatus, Label: "Terminal Deep Scan (status --deep)", DotCol: 0x009B7464},
			{ID: idTrayLedger, Label: "View Quiet Ledger...", DotCol: 0x009B7464},
			{ID: idTrayOpenDir, Label: "Open Logs & Config Folder", DotCol: 0x009B7464},
			{Sep: true},
			{ID: idTrayExit, Label: "Exit MEMORA", Danger: true, DotCol: 0x002626DC},
		}
	} else {
		items = []trayMenuItem{
			{ID: idTrayOpenUI, Label: "打开 MEMORA 性能调度中心", Badge: "主页", Bold: true, DotCol: 0x00EB6325},
			{ID: idTrayOptimize, Label: optLabel, Badge: optBadge, DotCol: optDotCol, KeepOpen: true},
			{Sep: true},
			{ID: idTrayToggleMode, Label: modeItemLabel, Badge: modeItemBadge, DotCol: modeBadgeCol, KeepOpen: true},
			{ID: idTrayToggleEdge, Label: edgeItemLabel, Badge: edgeItemBadge, DotCol: edgeBadgeCol, KeepOpen: true},
			{Sep: true},
			{ID: idTrayStatus, Label: "快速状态弹窗摘要...", DotCol: 0x009B7464},
			{ID: idTrayDeepStatus, Label: "终端深度扫描 (status --deep)", DotCol: 0x009B7464},
			{ID: idTrayLedger, Label: "查看长期静默账本...", DotCol: 0x009B7464},
			{ID: idTrayOpenDir, Label: "打开日志与配置目录", DotCol: 0x009B7464},
			{Sep: true},
			{ID: idTrayExit, Label: "退出 MEMORA", Danger: true, DotCol: 0x002626DC},
		}
	}

	padX := scalePx(8, st.DPI)
	itemH := scalePx(34, st.DPI)
	sepH := scalePx(9, st.DPI)
	y := st.HeaderH + scalePx(6, st.DPI)
	for i := range items {
		if items[i].Sep {
			items[i].Rect = winRect{Left: padX + scalePx(6, st.DPI), Top: y, Right: st.Width - padX - scalePx(6, st.DPI), Bottom: y + sepH}
			y += sepH
			continue
		}
		items[i].Rect = winRect{Left: padX, Top: y, Right: st.Width - padX, Bottom: y + itemH}
		y += itemH + scalePx(2, st.DPI)
	}
	st.ModeLabel = headerStatus
	st.ModeColor = modeBadgeCol
	st.MemBadge = memBadge
	st.Items = items
}

func isModernTrayPopupOpen() bool {
	return trayPopupOpen.Load()
}

func updateModernTrayPopupState(mode string, availPct float64, selfMB uint64, hasSample bool, edgeEnabled bool, clearOptBusy bool, optNote string, lang string) {
	if activeTrayMenu == nil || activeTrayMenu.MenuHwnd == 0 {
		return
	}
	st := activeTrayMenu
	if mode != "" {
		st.Mode = mode
	}
	if hasSample {
		st.AvailPct = availPct
		st.HasSample = true
	}
	if selfMB > 0 {
		st.SelfMB = selfMB
	}
	st.EdgeEnabled = edgeEnabled
	if lang != "" {
		st.Language = lang
	}
	if clearOptBusy {
		st.OptBusy = false
	}
	if optNote != "" {
		st.OptNote = optNote
		st.OptNoteAt = time.Now()
	}
	st.rebuildItems()
	_, _, _ = procInvalidateRect.Call(st.MenuHwnd, 0, 0)
}

func showModernTrayPopup(parentHwnd, icon uintptr, mode string, availPct float64, selfMB uint64, hasSample bool, edgeEnabled bool, lang string) {
	if activeTrayMenu != nil && activeTrayMenu.MenuHwnd != 0 {
		old := activeTrayMenu.MenuHwnd
		activeTrayMenu.MenuHwnd = 0
		_, _, _ = procDestroyWindow.Call(old)
	}

	dpi := int32(96)
	if procGetDpiForSystem.Find() == nil {
		if d, _, _ := procGetDpiForSystem.Call(); d >= 96 {
			dpi = int32(d)
		}
	}

	w := scalePx(352, dpi)
	hdrH := scalePx(56, dpi)
	padBottom := scalePx(8, dpi)

	st := &trayMenuState{
		ParentHwnd:  parentHwnd,
		Icon:        icon,
		DPI:         dpi,
		Width:       w,
		HeaderH:     hdrH,
		Mode:        mode,
		AvailPct:    availPct,
		SelfMB:      selfMB,
		HasSample:   hasSample,
		EdgeEnabled: edgeEnabled,
		Language:    lang,
		HoverIdx:    -1,
	}
	st.rebuildItems()
	lastRect := st.Items[len(st.Items)-1].Rect
	totalH := lastRect.Bottom + padBottom
	st.Height = totalH

	trayMenuClassOnce.Do(func() {
		hInst, _, _ := procGetModuleHandle.Call(0)
		clsName := mustUTF16("MEMORATrayMenuClass")
		wc := wndClassExW{
			CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			Style:         csHRedraw | csVRedraw | csDropShadow,
			LpfnWndProc:   syscall.NewCallback(trayMenuWndProc),
			HInstance:     hInst,
			LpszClassName: clsName,
		}
		_, _, _ = procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	})

	var pt winPoint
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	xPos := pt.X - scalePx(12, dpi)
	yPos := pt.Y - totalH + scalePx(8, dpi)

	packedPt := uintptr(uint32(pt.X)) | (uintptr(uint32(pt.Y)) << 32)
	hMon, _, _ := procMonitorFromPoint.Call(packedPt, monitorNearPt)
	if hMon != 0 {
		var mi monitorInfo
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		if ok, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); ok != 0 {
			wk := mi.RcWork
			if xPos+w > wk.Right-scalePx(6, dpi) {
				xPos = wk.Right - w - scalePx(6, dpi)
			}
			if xPos < wk.Left+scalePx(6, dpi) {
				xPos = wk.Left + scalePx(6, dpi)
			}
			if yPos+totalH > wk.Bottom-scalePx(6, dpi) {
				yPos = wk.Bottom - totalH - scalePx(6, dpi)
			}
			if yPos < wk.Top+scalePx(6, dpi) {
				yPos = wk.Top + scalePx(6, dpi)
			}
		}
	}

	activeTrayMenu = st

	hInst, _, _ := procGetModuleHandle.Call(0)
	clsName := mustUTF16("MEMORATrayMenuClass")
	title := mustUTF16("MEMORAMenu")
	menuHwnd, _, _ := procCreateWindowEx.Call(
		wsExTopmost|wsExToolWin,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(title)),
		wsPopup|wsVisible,
		uintptr(xPos),
		uintptr(yPos),
		uintptr(w),
		uintptr(totalH),
		parentHwnd,
		0,
		hInst,
		0,
	)
	if menuHwnd == 0 {
		return
	}
	st.MenuHwnd = menuHwnd

	// Windows 11 native rounded corners + subtle border.
	corner := uint32(dwmCornerRound)
	borderCol := uint32(0x00CBD5E1) // #CBD5E1
	_, _, _ = procDwmSetWindowAttribute.Call(menuHwnd, dwmWindowCornerPreference, uintptr(unsafe.Pointer(&corner)), 4)
	_, _, _ = procDwmSetWindowAttribute.Call(menuHwnd, dwmBorderColor, uintptr(unsafe.Pointer(&borderCol)), 4)

	st.OpenedAt = time.Now()
	trayPopupOpen.Store(true)
	activatePopupHWND(menuHwnd)
	_, _, _ = procSetFocus.Call(menuHwnd)
	_, _, _ = procSetTimer.Call(menuHwnd, trayMenuDismissTimerID, 50, 0)
}

func activatePopupHWND(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
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

func dismissModernTrayPopup() {
	trayPopupOpen.Store(false)
	if activeTrayMenu == nil || activeTrayMenu.MenuHwnd == 0 {
		return
	}
	h := activeTrayMenu.MenuHwnd
	activeTrayMenu.MenuHwnd = 0
	_, _, _ = procDestroyWindow.Call(h)
}

func trayMenuWndProc(h uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	st := activeTrayMenu
	if st == nil || (st.MenuHwnd != 0 && st.MenuHwnd != h) {
		r, _, _ := procDefWindowProc.Call(h, uintptr(msg), wParam, lParam)
		return r
	}
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmKillFocus:
		dismissModernTrayPopup()
		return 0
	case wmActivate:
		if (wParam & 0xffff) == 0 {
			dismissModernTrayPopup()
			return 0
		}
	case wmKeyDown:
		if wParam == vkEscape {
			dismissModernTrayPopup()
			return 0
		}
	case wmMouseMove:
		if !st.Tracking {
			tme := trackMouseEventStruct{
				CbSize:    uint32(unsafe.Sizeof(trackMouseEventStruct{})),
				DwFlags:   tmeLeave,
				HWndTrack: h,
			}
			_, _, _ = procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			st.Tracking = true
		}
		mx := int32(int16(lParam & 0xffff))
		my := int32(int16((lParam >> 16) & 0xffff))
		idx := hitTestTrayItem(st, mx, my)
		if idx != st.HoverIdx {
			st.HoverIdx = idx
			_, _, _ = procInvalidateRect.Call(h, 0, 0)
		}
		return 0
	case wmMouseLeave:
		st.Tracking = false
		if st.HoverIdx != -1 {
			st.HoverIdx = -1
			_, _, _ = procInvalidateRect.Call(h, 0, 0)
		}
		return 0
	case wmLButtonUp:
		mx := int32(int16(lParam & 0xffff))
		my := int32(int16((lParam >> 16) & 0xffff))
		idx := hitTestTrayItem(st, mx, my)
		if idx < 0 || idx >= len(st.Items) || st.Items[idx].Sep {
			return 0
		}
		it := st.Items[idx]
		parent := st.ParentHwnd
		if it.KeepOpen {
			switch it.ID {
			case idTrayToggleMode:
				if st.Mode == "guard" {
					st.Mode = "observe"
				} else {
					st.Mode = "guard"
				}
			case idTrayToggleEdge:
				st.EdgeEnabled = !st.EdgeEnabled
			case idTrayOptimize:
				if st.OptBusy {
					return 0
				}
				st.OptBusy = true
				st.OptNote = ""
			}
			st.rebuildItems()
			_, _, _ = procInvalidateRect.Call(h, 0, 0)
			if parent != 0 {
				_, _, _ = procPostMessage.Call(parent, wmCommand, uintptr(it.ID), 0)
			}
			return 0
		}
		dismissModernTrayPopup()
		if it.ID != 0 && parent != 0 {
			_, _, _ = procPostMessage.Call(parent, wmCommand, uintptr(it.ID), 0)
		}
		return 0
	case wmRButtonUp:
		dismissModernTrayPopup()
		return 0
	case wmTimer:
		if wParam == trayMenuDismissTimerID {
			lBtn, _, _ := procGetAsyncKeyState.Call(vkLButton)
			rBtn, _, _ := procGetAsyncKeyState.Call(vkRButton)
			mBtn, _, _ := procGetAsyncKeyState.Call(vkMButton)
			if (lBtn&0x8000) != 0 || (rBtn&0x8000) != 0 || (mBtn&0x8000) != 0 {
				var cur winPoint
				var rc winRect
				_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
				if ok, _, _ := procGetWindowRect.Call(h, uintptr(unsafe.Pointer(&rc))); ok != 0 {
					if cur.X < rc.Left || cur.X >= rc.Right || cur.Y < rc.Top || cur.Y >= rc.Bottom {
						dismissModernTrayPopup()
						return 0
					}
				}
			}
			if !st.OpenedAt.IsZero() && time.Since(st.OpenedAt) > 180*time.Millisecond {
				fg, _, _ := procGetForegroundWindow.Call()
				if fg != 0 && fg != h {
					dismissModernTrayPopup()
					return 0
				}
			}
			return 0
		}
	case wmPaint:
		paintModernTrayMenu(h, st)
		return 0
	case wmDestroy:
		_, _, _ = procKillTimer.Call(h, trayMenuDismissTimerID)
		trayPopupOpen.Store(false)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(h, uintptr(msg), wParam, lParam)
	return r
}

func hitTestTrayItem(st *trayMenuState, x, y int32) int {
	for i, it := range st.Items {
		if it.Sep {
			continue
		}
		if x >= it.Rect.Left && x < it.Rect.Right && y >= it.Rect.Top && y < it.Rect.Bottom {
			return i
		}
	}
	return -1
}

func paintModernTrayMenu(h uintptr, st *trayMenuState) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(h, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(h, uintptr(unsafe.Pointer(&ps)))

	memDC, _, _ := procCreateCompatibleDC.Call(hdc)
	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(st.Width), uintptr(st.Height))
	oldBmp, _, _ := procSelectObject.Call(memDC, bmp)
	defer func() {
		_, _, _ = procSelectObject.Call(memDC, oldBmp)
		_, _, _ = procDeleteObject.Call(bmp)
		_, _, _ = procDeleteDC.Call(memDC)
	}()

	dpi := st.DPI
	bgBrush, _, _ := procCreateSolidBrush.Call(0x00FFFFFF)
	hdrBrush, _, _ := procCreateSolidBrush.Call(0x00FAFAF8) // Porcelain slate-white #F8FAFA
	sepBrush, _, _ := procCreateSolidBrush.Call(0x00F0E8E2) // #E2E8F0
	hoverExactBrush, _, _ := procCreateSolidBrush.Call(0x00FFF6EF) // #EFF6FF
	hoverDangerBrush, _, _ := procCreateSolidBrush.Call(0x00F2F2FE) // #FEF2F2
	defer func() {
		_, _, _ = procDeleteObject.Call(bgBrush)
		_, _, _ = procDeleteObject.Call(hdrBrush)
		_, _, _ = procDeleteObject.Call(sepBrush)
		_, _, _ = procDeleteObject.Call(hoverExactBrush)
		_, _, _ = procDeleteObject.Call(hoverDangerBrush)
	}()

	fullRect := winRect{0, 0, st.Width, st.Height}
	_, _, _ = procFillRect.Call(memDC, uintptr(unsafe.Pointer(&fullRect)), bgBrush)
	hdrRect := winRect{0, 0, st.Width, st.HeaderH}
	_, _, _ = procFillRect.Call(memDC, uintptr(unsafe.Pointer(&hdrRect)), hdrBrush)
	hdrLine := winRect{0, st.HeaderH - 1, st.Width, st.HeaderH}
	_, _, _ = procFillRect.Call(memDC, uintptr(unsafe.Pointer(&hdrLine)), sepBrush)

	_, _, _ = procSetBkMode.Call(memDC, 1) // TRANSPARENT

	brandFace := resolveInstalledFont("Bahnschrift", "Segoe UI Variable Display", "Noto Sans SC", "Microsoft YaHei UI")
	cjkFace := resolveInstalledFont("Noto Sans SC", "Microsoft YaHei UI")
	cjkMedFace := resolveInstalledFont("Noto Sans SC Medium", "Noto Sans SC", "Microsoft YaHei UI")

	fontBrand := createMenuFontFace(scalePx(14, dpi), 700, brandFace)
	fontNorm := createMenuFontFace(scalePx(13, dpi), 400, cjkFace)
	fontBold := createMenuFontFace(scalePx(13, dpi), 600, cjkMedFace)
	fontSmall := createMenuFontFace(scalePx(11, dpi), 500, cjkMedFace)
	defer func() {
		_, _, _ = procDeleteObject.Call(fontBrand)
		_, _, _ = procDeleteObject.Call(fontNorm)
		_, _, _ = procDeleteObject.Call(fontBold)
		_, _, _ = procDeleteObject.Call(fontSmall)
	}()

	// Draw header brand icon + title + live memory summary (with self WS MB).
	if st.Icon != 0 {
		icSz := scalePx(24, dpi)
		icY := (st.HeaderH - icSz) / 2
		_, _, _ = procDrawIconEx.Call(memDC, uintptr(scalePx(14, dpi)), uintptr(icY), st.Icon, uintptr(icSz), uintptr(icSz), 0, 0, 0x0003)
	}
	_, _, _ = procSelectObject.Call(memDC, fontBrand)
	_, _, _ = procSetTextColor.Call(memDC, 0x002A170F) // #0F172A
	titleRc := winRect{scalePx(46, dpi), scalePx(6, dpi), st.Width - scalePx(12, dpi), scalePx(28, dpi)}
	drawTextLine(memDC, fmt.Sprintf("MEMORA %s", version), &titleRc, dtLeft|dtSingleLine|dtVCenter)

	_, _, _ = procSelectObject.Call(memDC, fontSmall)
	_, _, _ = procSetTextColor.Call(memDC, uintptr(st.ModeColor))
	subRc := winRect{scalePx(46, dpi), scalePx(28, dpi), st.Width - scalePx(10, dpi), st.HeaderH - scalePx(5, dpi)}
	drawTextLine(memDC, fmt.Sprintf("%s · %s", st.ModeLabel, st.MemBadge), &subRc, dtLeft|dtSingleLine|dtVCenter|dtEndEllipsis)

	// Draw menu items with crisp vector status dots (zero Unicode emojis).
	nullPen, _, _ := procCreatePen.Call(5 /* PS_NULL */, 0, 0)
	defer procDeleteObject.Call(nullPen)

	for i, it := range st.Items {
		if it.Sep {
			mid := (it.Rect.Top + it.Rect.Bottom) / 2
			lineRc := winRect{it.Rect.Left, mid, it.Rect.Right, mid + 1}
			_, _, _ = procFillRect.Call(memDC, uintptr(unsafe.Pointer(&lineRc)), sepBrush)
			continue
		}

		hovered := i == st.HoverIdx
		if hovered {
			br := hoverExactBrush
			if it.Danger {
				br = hoverDangerBrush
			}
			oldP, _, _ := procSelectObject.Call(memDC, nullPen)
			oldB, _, _ := procSelectObject.Call(memDC, br)
			rad := scalePx(9, dpi)
			_, _, _ = procRoundRect.Call(memDC, uintptr(it.Rect.Left), uintptr(it.Rect.Top), uintptr(it.Rect.Right), uintptr(it.Rect.Bottom), uintptr(rad), uintptr(rad))
			_, _, _ = procSelectObject.Call(memDC, oldB)
			_, _, _ = procSelectObject.Call(memDC, oldP)
		}

		labelLeft := it.Rect.Left + scalePx(12, dpi)
		if it.DotCol != 0 {
			dotSz := scalePx(6, dpi)
			if dotSz < 5 {
				dotSz = 5
			}
			dotX := it.Rect.Left + scalePx(11, dpi)
			dotY := (it.Rect.Top + it.Rect.Bottom - dotSz) / 2
			dotBr, _, _ := procCreateSolidBrush.Call(uintptr(it.DotCol))
			oldP, _, _ := procSelectObject.Call(memDC, nullPen)
			oldB, _, _ := procSelectObject.Call(memDC, dotBr)
			_, _, _ = procEllipse.Call(memDC, uintptr(dotX), uintptr(dotY), uintptr(dotX+dotSz), uintptr(dotY+dotSz))
			_, _, _ = procSelectObject.Call(memDC, oldB)
			_, _, _ = procSelectObject.Call(memDC, oldP)
			_, _, _ = procDeleteObject.Call(dotBr)
			labelLeft = dotX + dotSz + scalePx(8, dpi)
		}

		if it.Bold {
			_, _, _ = procSelectObject.Call(memDC, fontBold)
		} else {
			_, _, _ = procSelectObject.Call(memDC, fontNorm)
		}
		txtColor := uint32(0x002A170F) // #0F172A high-contrast primary ink
		if it.Danger {
			txtColor = 0x002626DC // #DC2626
		} else if hovered {
			txtColor = 0x00D84E1D // #1D4ED8
		}
		_, _, _ = procSetTextColor.Call(memDC, uintptr(txtColor))

		labelRc := winRect{
			Left:   labelLeft,
			Top:    it.Rect.Top,
			Right:  it.Rect.Right - scalePx(12, dpi),
			Bottom: it.Rect.Bottom,
		}
		if it.Badge != "" {
			labelRc.Right -= scalePx(66, dpi)
		}
		drawTextLine(memDC, it.Label, &labelRc, dtLeft|dtSingleLine|dtVCenter|dtEndEllipsis)

		if it.Badge != "" {
			_, _, _ = procSelectObject.Call(memDC, fontSmall)
			badgeCol := uint32(0x00695547) // #475569 crisp Slate 600
			if it.DotCol != 0 && it.DotCol != 0x009B7464 {
				badgeCol = it.DotCol
			} else if hovered {
				badgeCol = 0x00D84E1D
			}
			_, _, _ = procSetTextColor.Call(memDC, uintptr(badgeCol))
			badgeRc := winRect{
				Left:   it.Rect.Right - scalePx(74, dpi),
				Top:    it.Rect.Top,
				Right:  it.Rect.Right - scalePx(12, dpi),
				Bottom: it.Rect.Bottom,
			}
			drawTextLine(memDC, it.Badge, &badgeRc, dtRight|dtSingleLine|dtVCenter)
		}
	}

	_, _, _ = procBitBlt.Call(hdc, 0, 0, uintptr(st.Width), uintptr(st.Height), memDC, 0, 0, srcCopy)
}

func createMenuFont(height int32, weight int32) uintptr {
	return createMenuFontFace(height, weight, resolveInstalledFont("Noto Sans SC", "Microsoft YaHei UI"))
}

func createMenuFontFace(height int32, weight int32, faceName string) uintptr {
	face := mustUTF16(faceName)
	h, _, _ := procCreateFontW.Call(
		uintptr(int64(-height)),
		0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		1, // DEFAULT_CHARSET
		0, 0,
		5, // CLEARTYPE_QUALITY
		0,
		uintptr(unsafe.Pointer(face)),
	)
	return h
}

func drawTextLine(hdc uintptr, text string, rc *winRect, flags uint32) {
	u := mustUTF16(text)
	_, _, _ = procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(u)), uintptr(^uintptr(0)), uintptr(unsafe.Pointer(rc)), uintptr(flags))
}
