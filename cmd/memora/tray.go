package main

import (
	"bytes"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"memora/internal/config"
	"memora/internal/engine"
	"memora/internal/journal"
	"memora/internal/ledger"
	"memora/internal/proc"
	"memora/internal/winmem"
)

const (
	wmDestroy          = 0x0002
	wmClose            = 0x0010
	wmCommand          = 0x0111
	wmUser             = 0x0400
	wmTrayCallback     = wmUser + 1
	wmUpdateTooltip    = wmUser + 2
	wmOpenDashboard    = wmUser + 10
	wmShowTrayPopup    = wmUser + 11
	wmRefreshTrayPopup = wmUser + 12
	wmLButtonUp        = 0x0202
	wmLButtonDblClk    = 0x0203
	wmRButtonUp        = 0x0205
	wmContextMenu      = 0x007B
	wmNull             = 0x0000

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mfString     = 0x00000000
	mfSeparator  = 0x00000800
	mfChecked    = 0x00000008
	tpmRightBtn  = 0x0002
	tpmBottomAln = 0x0020

	idiShield      = 32518
	idiApplication = 32512
	smCxSmIcon     = 49
	lrDefaultColor = 0x0000
	mbOK           = 0x00000000
	mbIconInfo     = 0x00000040
	mbSetFg        = 0x00010000

	idTrayOpenUI     = 1000
	idTrayStatus     = 1001
	idTrayDeepStatus = 1002
	idTrayLedger     = 1003
	idTrayToggleMode = 1004
	idTrayRestore    = 1005
	idTrayOpenDir    = 1006
	idTrayExit       = 1007
	idTrayOptimize   = 1008
	idTrayToggleEdge = 1009
)

//go:embed memora_tray.png
var embeddedTrayPNG []byte

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	shell32                 = syscall.NewLazyDLL("shell32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMsg        = user32.NewProc("TranslateMessage")
	procDispatchMsg         = user32.NewProc("DispatchMessageW")
	procLoadIcon            = user32.NewProc("LoadIconW")
	procCreateIconFromResEx = user32.NewProc("CreateIconFromResourceEx")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procDestroyIcon         = user32.NewProc("DestroyIcon")
	procShellNotifyIcon        = shell32.NewProc("Shell_NotifyIconW")
	procShellExecute           = shell32.NewProc("ShellExecuteW")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procDestroyMenu            = user32.NewProc("DestroyMenu")
	procAppendMenu             = user32.NewProc("AppendMenuW")
	procTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procSetForeground          = user32.NewProc("SetForegroundWindow")
	procPostMessage            = user32.NewProc("PostMessageW")
	procFindWindow             = user32.NewProc("FindWindowW")
	procChangeWindowMsgFilterEx = user32.NewProc("ChangeWindowMessageFilterEx")
	procRegisterWinMsg         = user32.NewProc("RegisterWindowMessageW")
	procMessageBox             = user32.NewProc("MessageBoxW")
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// notifyIconData matches Windows NOTIFYICONDATAW (976 bytes on windows/amd64).
type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         guid
	HBalloonIcon     uintptr
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type winPoint struct {
	X int32
	Y int32
}

type winMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      winPoint
}

type engineCmd struct {
	fn   func(*engine.Engine)
	done chan struct{}
}

func cmdTray(args []string) {
	enablePerMonitorDPIAwareness()
	fs := flag.NewFlagSet("tray", flag.ExitOnError)
	hidden := fs.Bool("hidden", false, "隐藏控制台窗口")
	openUI := fs.Bool("open-ui", false, "启动后打开性能调度与真相中心界面")
	observe := fs.Bool("observe", false, "强制仅观察，不主动干预")
	noElevate := fs.Bool("no-elevate", false, "禁止自动触发 UAC 提权")
	interval := fs.Duration("interval", 0, "采样间隔，例如 10s")
	_ = fs.Parse(args)

	if *hidden {
		winmem.HideConsole()
	}
	release, ok := winmem.AcquireSingleInstance(`Local\MEMORA.Watch`)
	if !ok {
		cls, _ := syscall.UTF16PtrFromString("MEMORATrayClass")
		if existing, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(cls)), 0); existing != 0 {
			_, _, _ = procPostMessage.Call(existing, wmOpenDashboard, 0, 0)
			return
		}
		showMessageBox("MEMORA", "MEMORA 正在无托盘命令行模式 (memora watch) 运行中，请先停止该进程后再打开托盘界面。")
		return
	}
	if !*noElevate && !winmem.IsAdmin() {
		release()
		if relaunchElevated(args) {
			return
		}
		// User declined UAC or launch failed: re-acquire mutex and continue unelevated.
		release, ok = winmem.AcquireSingleInstance(`Local\MEMORA.Watch`)
		if !ok {
			return
		}
	}
	defer release()
	winmem.EnableGuardianPrivileges()

	cfg, err := config.Load()
	if err != nil {
		fail("读取配置失败: %v", err)
	}
	if *observe {
		cfg.Mode = "observe"
	}
	jr, err := journal.Open(config.Dir())
	if err != nil {
		fail("打开日志失败: %v", err)
	}
	defer jr.Close()
	lg, _ := ledger.Open(config.Dir())

	eng := engine.New(cfg, jr)
	if lg != nil {
		eng.SetLedger(lg)
		_ = lg.StartSession()
	}

	tickDur := *interval
	if tickDur <= 0 {
		tickDur = time.Duration(cfg.IntervalSeconds) * time.Second
	}
	if tickDur < 2*time.Second {
		tickDur = 10 * time.Second
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var stateMu sync.Mutex
	currentMode := cfg.Mode
	currentEdgeOn := cfg.Actions.EdgeHorizon
	currentLang := cfg.Language
	var latest engine.Assessment
	var latestSelfWS uint64
	var lastOptNote string
	hasLatest := false
	modeCh := make(chan string, 4)
	restoreCh := make(chan struct{}, 1)
	cmdCh := make(chan engineCmd, 16)
	done := make(chan struct{})

	var hwnd atomic.Uintptr
	postMsg := func(msg uint32) {
		if h := hwnd.Load(); h != 0 {
			_, _, _ = procPostMessage.Call(h, uintptr(msg), 0, 0)
		}
	}

	runOnEngine := func(fn func(*engine.Engine)) error {
		c := engineCmd{fn: fn, done: make(chan struct{})}
		select {
		case cmdCh <- c:
		case <-ctx.Done():
			return fmt.Errorf("engine stopping")
		case <-time.After(5 * time.Second):
			return fmt.Errorf("engine busy")
		}
		select {
		case <-c.done:
			return nil
		case <-ctx.Done():
			return fmt.Errorf("engine stopping")
		case <-time.After(20 * time.Second):
			return fmt.Errorf("engine timeout")
		}
	}

	dash, dashErr := newDashboardServer(cfg, lg, runOnEngine, func(next string) {
		stateMu.Lock()
		currentMode = next
		stateMu.Unlock()
		postMsg(wmUpdateTooltip)
	})
	if dashErr == nil {
		dash.SetOnConfigChange(func(c config.Config) {
			stateMu.Lock()
			currentMode = c.Mode
			currentEdgeOn = c.Actions.EdgeHorizon
			currentLang = c.Language
			stateMu.Unlock()
			postMsg(wmUpdateTooltip)
		})
		defer dash.Close()
	}

	go func() {
		defer close(done)
		opts := engine.Options{
			Interval: tickDur,
			Act:      currentMode == "guard",
			OnReading: func(s engine.Sample) {
				sws, sh := selfStats(s.Procs)
				stateMu.Lock()
				if sws > 0 {
					latestSelfWS = sws
				}
				stateMu.Unlock()
				if dash != nil {
					dash.RecordSample(s, sws, sh)
				}
			},
		}
		// Take an initial reading quickly so tray tooltip & dashboard are populated immediately.
		if a, err := eng.Step(opts); err == nil {
			stateMu.Lock()
			latest = a
			hasLatest = true
			stateMu.Unlock()
			if dash != nil {
				dash.UpdateFromEngine(eng, a, true, false)
			}
			postMsg(wmUpdateTooltip)
		}
		if *openUI && dash != nil {
			go dash.OpenWindow()
		}
		if winmem.IsAdmin() && cfg.Actions.MitigateConflictDrivers {
			go func() {
				ctxMit, cancel := context.WithTimeout(ctx, 25*time.Second)
				defer cancel()
				_, _ = mitigateConflictDrivers(ctxMit, false)
				_ = runOnEngine(func(eng *engine.Engine) {
					if a, err := eng.Step(engine.Options{Act: false}); err == nil {
						stateMu.Lock()
						latest = a
						hasLatest = true
						stateMu.Unlock()
						if dash != nil {
							dash.UpdateFromEngine(eng, a, true, true)
						}
					}
				})
			}()
		}

		ticker := time.NewTicker(tickDur)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				eng.RestoreAll()
				if lg != nil {
					_ = lg.Flush()
				}
				postMsg(wmDestroy)
				return
			case c := <-cmdCh:
				if c.fn != nil {
					c.fn(eng)
				}
				opts.Act = eng.Mode() == "guard"
				stateMu.Lock()
				currentMode = eng.Mode()
				stateMu.Unlock()
				close(c.done)
				postMsg(wmUpdateTooltip)
			case <-restoreCh:
				eng.RestoreAll()
				if dash != nil {
					stateMu.Lock()
					a := latest
					stateMu.Unlock()
					a.SoftManaged = 0
					dash.UpdateFromEngine(eng, a, false, false)
				}
				postMsg(wmUpdateTooltip)
			case m := <-modeCh:
				_ = eng.UpdateConfig(func(c *config.Config) {
					c.Mode = m
				})
				opts.Act = m == "guard"
				var nextA engine.Assessment
				var hasNext bool
				if m == "guard" {
					if aStep, sErr := eng.Step(engine.Options{Act: true}); sErr == nil {
						nextA = aStep
						hasNext = true
					}
				}
				stateMu.Lock()
				currentMode = m
				if hasNext {
					latest = nextA
					hasLatest = true
				}
				a := latest
				stateMu.Unlock()
				if dash != nil {
					a.SoftManaged = len(eng.ManagedProcesses())
					dash.UpdateFromEngine(eng, a, false, false)
				}
				postMsg(wmUpdateTooltip)
			case <-ticker.C:
				opts.Act = eng.Mode() == "guard"
				if a, err := eng.Step(opts); err == nil {
					stateMu.Lock()
					latest = a
					hasLatest = true
					currentMode = eng.Mode()
					stateMu.Unlock()
					if dash != nil {
						dash.UpdateFromEngine(eng, a, false, false)
					}
					postMsg(wmUpdateTooltip)
				}
			}
		}
	}()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandle.Call(0)
	icon, ownedIcon := loadTrayIcon()
	if ownedIcon {
		defer func() { _, _, _ = procDestroyIcon.Call(icon) }()
	}
	popupIcon, ownedPopupIcon := loadPopupHeaderIcon()
	if !ownedPopupIcon {
		popupIcon = icon
	} else {
		defer func() { _, _, _ = procDestroyIcon.Call(popupIcon) }()
	}

	className, _ := syscall.UTF16PtrFromString("MEMORATrayClass")
	winTitle, _ := syscall.UTF16PtrFromString("MEMORA")
	taskbarCreatedMsg, _, _ := procRegisterWinMsg.Call(uintptr(unsafe.Pointer(mustUTF16("TaskbarCreated"))))

	updateTip := func(h uintptr) {
		stateMu.Lock()
		m := currentMode
		a := latest
		okSample := hasLatest
		edgeOn := currentEdgeOn
		sws := latestSelfWS
		lang := currentLang
		stateMu.Unlock()
		isEn := strings.HasPrefix(strings.ToLower(lang), "en")
		tip := fmt.Sprintf("MEMORA %s · %s", version, m)
		if okSample {
			statusTag := "智能调度"
			if isEn {
				statusTag = "Smart Dispatch"
			}
			if a.FullscreenQuiet {
				if isEn {
					statusTag = a.FullscreenLabel + " Quiet"
				} else {
					statusTag = a.FullscreenLabel + "免打扰"
				}
			} else if m != "guard" {
				if isEn {
					statusTag = "Disabled"
				} else {
					statusTag = "已关闭"
				}
			} else if a.SmartLabel != "" {
				statusTag = a.SmartLabel
			}
			if isEn {
				tip = fmt.Sprintf("MEMORA %s · %s · Avail %.0f%%", version, statusTag, a.AvailPct)
			} else {
				tip = fmt.Sprintf("MEMORA %s · %s · 可用 %.0f%%", version, statusTag, a.AvailPct)
			}
		}
		var nid notifyIconData
		nid.CbSize = uint32(unsafe.Sizeof(nid))
		nid.HWnd = h
		nid.UID = 1
		nid.UFlags = nifTip
		copyUTF16(nid.SzTip[:], tip)
		_, _, _ = procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
		if isModernTrayPopupOpen() {
			updateModernTrayPopupState(m, a.AvailPct, popupSelfMB(sws), okSample, edgeOn, false, "", lang)
		}
	}

	addIcon := func(h uintptr) {
		var nid notifyIconData
		nid.CbSize = uint32(unsafe.Sizeof(nid))
		nid.HWnd = h
		nid.UID = 1
		nid.UFlags = nifMessage | nifIcon | nifTip
		nid.UCallbackMessage = wmTrayCallback
		nid.HIcon = icon
		copyUTF16(nid.SzTip[:], fmt.Sprintf("MEMORA %s · 正在初始化", version))
		_, _, _ = procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
		updateTip(h)
	}

	removeIcon := func(h uintptr) {
		var nid notifyIconData
		nid.CbSize = uint32(unsafe.Sizeof(nid))
		nid.HWnd = h
		nid.UID = 1
		_, _, _ = procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	}

	showStatusDialog := func() {
		snap, err := winmem.Collect()
		if err != nil {
			showMessageBox("MEMORA 状态体检", fmt.Sprintf("采集内存失败: %v", err))
			return
		}
		procs, complete, _ := proc.ListAll()
		diag := engine.InspectDeep(snap, procs, false)
		stateMu.Lock()
		m := currentMode
		a := latest
		okSample := hasLatest
		stateMu.Unlock()
		text := buildTrayStatusSummary(snap, diag, procs, complete, m, a, okSample)
		showMessageBox("MEMORA 状态与真相账本", text)
	}

	showPopup := func(h uintptr) {
		stateMu.Lock()
		m := currentMode
		a := latest
		okSample := hasLatest
		edgeOn := currentEdgeOn
		sws := latestSelfWS
		lang := currentLang
		stateMu.Unlock()
		showModernTrayPopup(h, popupIcon, m, a.AvailPct, popupSelfMB(sws), okSample, edgeOn, lang)
	}

	var lastLeftClickAt time.Time
	wndProc := syscall.NewCallback(func(h uintptr, msg uint32, wParam, lParam uintptr) uintptr {
		if taskbarCreatedMsg != 0 && uint32(taskbarCreatedMsg) == msg {
			addIcon(h)
			return 0
		}
		switch msg {
		case wmUpdateTooltip:
			updateTip(h)
			return 0
		case wmRefreshTrayPopup:
			stateMu.Lock()
			m := currentMode
			a := latest
			okSample := hasLatest
			edgeOn := currentEdgeOn
			sws := latestSelfWS
			note := lastOptNote
			lang := currentLang
			stateMu.Unlock()
			updateModernTrayPopupState(m, a.AvailPct, popupSelfMB(sws), okSample, edgeOn, true, note, lang)
			return 0
		case wmOpenDashboard:
			if dash != nil {
				go dash.OpenWindow()
			} else {
				go showStatusDialog()
			}
			return 0
		case wmShowTrayPopup:
			showPopup(h)
			return 0
		case wmTrayCallback:
			switch uint32(lParam) {
			case wmLButtonUp, wmLButtonDblClk:
				now := time.Now()
				if now.Sub(lastLeftClickAt) < 380*time.Millisecond {
					return 0
				}
				lastLeftClickAt = now
				dismissModernTrayPopup()
				if dash != nil {
					go dash.OpenWindow()
				} else {
					go showStatusDialog()
				}
			case wmRButtonUp, wmContextMenu:
				showPopup(h)
			}
			return 0
		case wmCommand:
			id := uint16(wParam & 0xffff)
			switch id {
			case idTrayOpenUI:
				if dash != nil {
					go dash.OpenWindow()
				} else {
					go showStatusDialog()
				}
			case idTrayOptimize:
				go func() {
					var res engine.OptimizeResult
					var opErr error
					popupActive := isModernTrayPopupOpen()
					if err := runOnEngine(func(eng *engine.Engine) {
						res, opErr = eng.OptimizeNow()
						if opErr == nil {
							if a, sErr := eng.Step(engine.Options{
								Act: false,
								OnReading: func(s engine.Sample) {
									sws, sh := selfStats(s.Procs)
									stateMu.Lock()
									if sws > 0 {
										latestSelfWS = sws
									}
									stateMu.Unlock()
									if dash != nil {
										dash.RecordSample(s, sws, sh)
									}
								},
							}); sErr == nil {
								stateMu.Lock()
								latest = a
								hasLatest = true
								if res.AvailGainMB >= 1 {
									lastOptNote = fmt.Sprintf("净增 %.0fMB", res.AvailGainMB)
								} else if res.WSShrunkMB >= 1 {
									lastOptNote = fmt.Sprintf("收敛 %.0fMB", res.WSShrunkMB)
								} else if res.TrimmedCount > 0 {
									lastOptNote = fmt.Sprintf("紧缩 %d 项", res.TrimmedCount)
								} else {
									lastOptNote = "已稳态"
								}
								stateMu.Unlock()
								if dash != nil {
									dash.UpdateFromEngine(eng, a, true, false)
								}
							}
						}
					}); err == nil && opErr == nil {
						postMsg(wmRefreshTrayPopup)
						if !popupActive && !isModernTrayPopupOpen() {
							showMessageBox("MEMORA 深度释压完成", fmt.Sprintf("已安全收敛 %d 个闲置后台进程工作集 %.0f MB\n系统可用物理内存净增 %.0f MB（耗时 %d ms）",
								res.TrimmedCount, res.WSShrunkMB, res.AvailGainMB, res.DurationMS))
						}
					} else {
						stateMu.Lock()
						lastOptNote = "稍后再试"
						stateMu.Unlock()
						postMsg(wmRefreshTrayPopup)
					}
				}()
			case idTrayStatus:
				go showStatusDialog()
			case idTrayDeepStatus:
				go launchDeepStatusTerminal()
			case idTrayLedger:
				go func() {
					if lg == nil {
						showMessageBox("MEMORA 静默账本", "当前账本未开启。")
						return
					}
					showMessageBox("MEMORA 静默守护账本", formatLedgerText(lg.Snapshot()))
				}()
			case idTrayToggleMode:
				stateMu.Lock()
				next := "observe"
				if currentMode != "guard" {
					next = "guard"
				}
				stateMu.Unlock()
				select {
				case modeCh <- next:
				default:
				}
			case idTrayToggleEdge:
				go func() {
					_ = runOnEngine(func(eng *engine.Engine) {
						var nextOn bool
						_ = eng.UpdateConfig(func(c *config.Config) {
							c.Actions.EdgeHorizon = !c.Actions.EdgeHorizon
							nextOn = c.Actions.EdgeHorizon
						})
						stateMu.Lock()
						currentEdgeOn = nextOn
						a := latest
						okS := hasLatest
						stateMu.Unlock()
						if dash != nil && okS {
							dash.UpdateFromEngine(eng, a, false, false)
						} else if okS {
							updateEdgeHorizonState(eng.ConfigSnapshot(), a, engine.MemoryBreakdown{}, true)
						} else {
							updateEdgeHorizonState(eng.ConfigSnapshot(), engine.Assessment{}, engine.MemoryBreakdown{}, false)
						}
					})
				}()
			case idTrayRestore:
				select {
				case restoreCh <- struct{}{}:
				default:
				}
				go showMessageBox("MEMORA", "已恢复所有经由 MEMORA 软调度的进程默认内存优先级与 EcoQoS 状态。")
			case idTrayOpenDir:
				_ = exec.Command("explorer.exe", config.Dir()).Start()
			case idTrayExit:
				stop()
				_, _, _ = procDestroyWindow.Call(h)
			}
			return 0
		case wmClose:
			stop()
			_, _, _ = procDestroyWindow.Call(h)
			return 0
		case wmDestroy:
			closeEdgeHorizon()
			removeIcon(h)
			stop()
			_, _, _ = procPostQuitMessage.Call(0)
			return 0
		default:
			r, _, _ := procDefWindowProc.Call(h, uintptr(msg), wParam, lParam)
			return r
		}
	})

	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   wndProc,
		HInstance:     hInst,
		HIcon:         icon,
		LpszClassName: className,
	}
	_, _, _ = procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	h, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(winTitle)),
		0,
		0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if h == 0 {
		stop()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		fail("创建系统托盘窗口失败: %v", err)
	}
	hwnd.Store(h)
	_, _, _ = procChangeWindowMsgFilterEx.Call(h, wmOpenDashboard, 1, 0)
	_, _, _ = procChangeWindowMsgFilterEx.Call(h, wmClose, 1, 0)
	addIcon(h)
	initEdgeHorizon(
		h,
		cfg,
		func() (engine.OptimizeResult, error) {
			var res engine.OptimizeResult
			var opErr error
			err := runOnEngine(func(eng *engine.Engine) {
				res, opErr = eng.OptimizeNow()
				if opErr == nil {
					if a, sErr := eng.Step(engine.Options{Act: false}); sErr == nil {
						stateMu.Lock()
						latest = a
						hasLatest = true
						stateMu.Unlock()
						if dash != nil {
							dash.UpdateFromEngine(eng, a, true, false)
						}
					}
				}
			})
			if err != nil {
				return res, err
			}
			return res, opErr
		},
		func() {
			if dash != nil {
				dash.OpenWindow()
			} else {
				showStatusDialog()
			}
		},
		func() {
			postMsg(wmShowTrayPopup)
		},
		func(side string, yPct float64) {
			_ = runOnEngine(func(eng *engine.Engine) {
				_ = eng.UpdateConfig(func(c *config.Config) {
					c.EdgeWidget.Side = normalizeEdgeSide(side)
					c.EdgeWidget.YPercent = clampEdgeY(yPct)
				})
				stateMu.Lock()
				a := latest
				okS := hasLatest
				stateMu.Unlock()
				if dash != nil && okS {
					dash.UpdateFromEngine(eng, a, false, false)
				} else if okS {
					updateEdgeHorizonState(eng.ConfigSnapshot(), a, engine.MemoryBreakdown{}, true)
				} else {
					updateEdgeHorizonState(eng.ConfigSnapshot(), engine.Assessment{}, engine.MemoryBreakdown{}, false)
				}
			})
		},
	)
	if dash != nil {
		snapSt := dash.snapshotState()
		stateMu.Lock()
		aInit := latest
		okInit := hasLatest
		stateMu.Unlock()
		if okInit {
			updateEdgeHorizonState(snapSt.Config, aInit, snapSt.Breakdown, true)
		} else {
			updateEdgeHorizonState(snapSt.Config, engine.Assessment{}, engine.MemoryBreakdown{}, false)
		}
	}

	var m winMsg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		_, _, _ = procTranslateMsg.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMsg.Call(uintptr(unsafe.Pointer(&m)))
	}
	stop()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

func relaunchElevated(args []string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	var b strings.Builder
	b.WriteString("tray")
	hasNoElev := false
	showCmd := uintptr(1)
	for _, a := range args {
		if a == "--no-elevate" {
			hasNoElev = true
		}
		if a == "--hidden" {
			showCmd = 0
		}
		b.WriteByte(' ')
		if strings.ContainsAny(a, " \t\"") {
			b.WriteByte('"')
			b.WriteString(strings.ReplaceAll(a, `"`, `\"`))
			b.WriteByte('"')
		} else {
			b.WriteString(a)
		}
	}
	if !hasNoElev {
		b.WriteString(" --no-elevate")
	}
	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(mustUTF16("runas"))),
		uintptr(unsafe.Pointer(mustUTF16(exe))),
		uintptr(unsafe.Pointer(mustUTF16(b.String()))),
		0,
		showCmd,
	)
	return ret > 32
}

func buildTrayStatusSummary(snap winmem.Snapshot, diag engine.DeepDiagnosis, procs []proc.Info, complete bool, mode string, latest engine.Assessment, hasLatest bool) string {
	var b strings.Builder
	var priv, ws, privWS uint64
	for _, p := range procs {
		priv += p.Private
		ws += p.WorkingSet
		privWS += p.PrivateWS
	}
	selfWS, selfHandles := selfStats(procs)
	modeDesc := "智能调度中"
	if mode != "guard" {
		modeDesc = "已关闭（仅采样观测）"
	}
	if hasLatest && latest.FullscreenQuiet {
		modeDesc += "（当前全屏/游戏免打扰锁已生效）"
	}
	fmt.Fprintf(&b, "状态: %s   MEMORA 自身 %s / %d 句柄\n\n", modeDesc, human(selfWS), selfHandles)
	fmt.Fprintf(&b, "物理内存: %s 总量 | %s 已用 (%.1f%%) | %s 可用\n",
		human(snap.TotalPhys), human(snap.InUse()), 100-snap.AvailablePercent(), human(snap.AvailPhys))
	fmt.Fprintf(&b, "提交与池: 提交 %s / %s | 内核池 %s（非分页 %s）\n",
		human(snap.Commit), human(snap.TotalCommit), human(snap.KernelTotal), human(snap.KernelNonpaged))
	if snap.HasPages {
		fmt.Fprintf(&b, "页链表  : 待机缓存 %s（可回收缓存）| 已修改 %s\n",
			human(snap.Pages.Standby), human(snap.Pages.Modified))
	}
	readFlag := "完整读取"
	if !complete {
		readFlag = "部分读取"
	}
	fmt.Fprintf(&b, "活跃进程: %d 个（%s）| 私有提交 %s | 独占工作集 %s\n",
		len(procs), readFlag, human(priv), human(privWS))
	if diag.Extras.HasVid && diag.Extras.VidBytes > 0 {
		fmt.Fprintf(&b, "虚机锁页: Hyper-V / WSL2 Vid 锁住 %s 物理内存\n", human(diag.Extras.VidBytes))
	}
	if tight, ok := engine.NUMADisparity(diag.Extras.NUMANodes); ok {
		fmt.Fprintf(&b, "NUMA预警: Node%s 可用仅剩 %d/%d MB\n", tight.ID, tight.AvailableMB, tight.TotalMB)
	}
	if diag.Leak.Suspected {
		fmt.Fprintf(&b, "内核异常: 疑似僵尸 Process 对象泄漏（%d 对象 vs %d 进程，残骸 %s）\n",
			diag.Leak.ProcessObjects, diag.Leak.ActiveProcesses, human(diag.Leak.RelatedPoolByte))
	}
	if hasLatest && len(latest.Orphans) > 0 {
		fmt.Fprintf(&b, "孤儿残留: 捕获 %d 个原父进程已退出的无窗子进程\n", len(latest.Orphans))
	}
	if len(diag.ProcGroups) > 0 {
		fmt.Fprintf(&b, "\n占用前三套件:\n")
		for i := 0; i < len(diag.ProcGroups) && i < 3; i++ {
			g := diag.ProcGroups[i]
			fmt.Fprintf(&b, "  • %s (%d个)  私有 %s  独占 %s\n", g.Name, g.Count, human(g.Private), human(g.PrivateWS))
		}
	}
	return b.String()
}

func launchDeepStatusTerminal() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := fmt.Sprintf("& '%s' status --deep; Write-Host ''; Read-Host '按回车键关闭窗口'", strings.ReplaceAll(exe, `'`, `''`))
	_ = exec.Command("powershell.exe", "-NoProfile", "-Command", cmd).Start()
}

func showMessageBox(title, body string) {
	_, _, _ = procMessageBox.Call(
		0,
		uintptr(unsafe.Pointer(mustUTF16(body))),
		uintptr(unsafe.Pointer(mustUTF16(title))),
		mbOK|mbIconInfo|mbSetFg,
	)
}

func appendMenuItem(menu, flags, id uintptr, text string) {
	var p uintptr
	if text != "" {
		p = uintptr(unsafe.Pointer(mustUTF16(text)))
	}
	_, _, _ = procAppendMenu.Call(menu, flags, id, p)
}

func mustUTF16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

func copyUTF16(dst []uint16, s string) {
	u, err := syscall.UTF16FromString(s)
	if err != nil || len(dst) == 0 {
		return
	}
	n := copy(dst[:len(dst)-1], u)
	dst[n] = 0
}

func popupSelfMB(ws uint64) uint64 {
	if ws == 0 {
		return 0
	}
	mb := (ws + 512*1024) / (1024 * 1024)
	if mb == 0 {
		mb = 1
	}
	return mb
}

func loadTrayIcon() (uintptr, bool) {
	cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	dpi := getWindowDPI(0)
	targetPx := int(scalePx(16, dpi))
	if int(cx) > targetPx {
		targetPx = int(cx)
	}
	if targetPx < 16 {
		targetPx = 16
	}
	if h, ok := buildCrispTrayHICON(targetPx); ok {
		return h, true
	}
	if len(embeddedTrayPNG) > 0 {
		sz := uintptr(targetPx)
		h, _, _ := procCreateIconFromResEx.Call(
			uintptr(unsafe.Pointer(&embeddedTrayPNG[0])),
			uintptr(len(embeddedTrayPNG)),
			1,
			0x00030000,
			sz,
			sz,
			lrDefaultColor,
		)
		if h != 0 {
			return h, true
		}
	}
	h, _, _ := procLoadIcon.Call(0, uintptr(idiShield))
	if h == 0 {
		h, _, _ = procLoadIcon.Call(0, uintptr(idiApplication))
	}
	return h, false
}

func loadPopupHeaderIcon() (uintptr, bool) {
	targetPx := int(scalePx(24, getWindowDPI(0)))
	if targetPx < 24 {
		targetPx = 24
	}
	return buildCrispTrayHICON(targetPx)
}

func buildCrispTrayHICON(targetPx int) (uintptr, bool) {
	src := embeddedLogoPNG
	if len(src) == 0 {
		src = embeddedTrayPNG
	}
	pngBytes, err := renderCrispTrayPNG(src, targetPx)
	if err != nil || len(pngBytes) == 0 {
		return 0, false
	}
	h, _, _ := procCreateIconFromResEx.Call(
		uintptr(unsafe.Pointer(&pngBytes[0])),
		uintptr(len(pngBytes)),
		1,
		0x00030000,
		uintptr(targetPx),
		uintptr(targetPx),
		lrDefaultColor,
	)
	return h, h != 0
}

// renderCrispTrayPNG tightly crops the central MEMORA ribbon logo onto a full-bleed
// high-contrast porcelain squircle at exact targetPx dimensions (zero OS downscale blur).
func renderCrispTrayPNG(srcData []byte, targetPx int) ([]byte, error) {
	if len(srcData) == 0 || targetPx <= 0 {
		return nil, fmt.Errorf("invalid icon input")
	}
	srcImg, err := png.Decode(bytes.NewReader(srcData))
	if err != nil {
		return nil, err
	}
	b := srcImg.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 16 || h < 16 {
		return nil, fmt.Errorf("source icon too small")
	}

	// Scan inner region away from outer squircle frame to find exact ribbon ink bounds.
	scanX0, scanX1 := b.Min.X+w/10, b.Max.X-w/10
	scanY0, scanY1 := b.Min.Y+h/7, b.Max.Y-h/7
	minX, minY := scanX1, scanY1
	maxX, maxY := scanX0, scanY0
	for y := scanY0; y < scanY1; y++ {
		for x := scanX0; x < scanX1; x++ {
			c := color.NRGBAModel.Convert(srcImg.At(x, y)).(color.NRGBA)
			if c.A > 90 && (c.R < 228 || c.G < 228 || c.B < 228) {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX <= minX || maxY <= minY {
		minX, maxX = b.Min.X+w/10, b.Max.X-w/10
		minY, maxY = b.Min.Y+h/5, b.Max.Y-h/5
	}

	// Expand slightly so outer ribbon curvature is never clipped, keeping a tight 78% logo fill.
	ribbonW := float64(maxX - minX + 1)
	ribbonH := float64(maxY - minY + 1)
	cropSide := ribbonW * 1.24
	if ribbonH*1.30 > cropSide {
		cropSide = ribbonH * 1.30
	}
	cxSrc := (float64(minX+maxX) + 1) / 2.0
	cySrc := (float64(minY+maxY) + 1) / 2.0
	srcLeft := cxSrc - cropSide/2.0
	srcTop := cySrc - cropSide/2.0

	dst := image.NewNRGBA(image.Rect(0, 0, targetPx, targetPx))
	fSize := float64(targetPx)
	half := fSize / 2.0
	cornerR := fSize * 0.24
	if cornerR < 3.0 {
		cornerR = 3.0
	}
	borderThick := 1.15
	if targetPx <= 18 {
		borderThick = 1.05
	}

	const sub = 4
	const subInv = 1.0 / float64(sub*sub)

	for py := 0; py < targetPx; py++ {
		yCenter := float64(py) + 0.5
		for px := 0; px < targetPx; px++ {
			xCenter := float64(px) + 0.5

			// Signed distance to porcelain squircle badge.
			qx := math.Abs(xCenter-half) - (half - 0.5 - cornerR)
			qy := math.Abs(yCenter-half) - (half - 0.5 - cornerR)
			sd := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - cornerR
			cov := 0.5 - sd
			if cov < 0 {
				cov = 0
			} else if cov > 1 {
				cov = 1
			}
			if cov <= 0.01 {
				continue
			}

			// 4x4 supersampled area average of cropped ribbon onto crisp white background.
			var sumR, sumG, sumB float64
			for sy := 0; sy < sub; sy++ {
				vy := (float64(py) + (float64(sy)+0.5)/float64(sub)) / fSize
				iy := int(math.Round(srcTop + vy*cropSide))
				if iy < b.Min.Y {
					iy = b.Min.Y
				} else if iy >= b.Max.Y {
					iy = b.Max.Y - 1
				}
				for sx := 0; sx < sub; sx++ {
					vx := (float64(px) + (float64(sx)+0.5)/float64(sub)) / fSize
					ix := int(math.Round(srcLeft + vx*cropSide))
					if ix < b.Min.X {
						ix = b.Min.X
					} else if ix >= b.Max.X {
						ix = b.Max.X - 1
					}
					c := color.NRGBAModel.Convert(srcImg.At(ix, iy)).(color.NRGBA)
					a := float64(c.A) / 255.0
					// Composite source over pure white porcelain interior.
					sr := float64(c.R)*a + 255.0*(1.0-a)
					sg := float64(c.G)*a + 255.0*(1.0-a)
					sb := float64(c.B)*a + 255.0*(1.0-a)
					sumR += sr
					sumG += sg
					sumB += sb
				}
			}

			r := sumR * subInv
			g := sumG * subInv
			bl := sumB * subInv

			// Unsharp ink deepening curve so ribbon contrast stays bold & sharp on 16-24px tray icons.
			lum := (0.299*r + 0.587*g + 0.114*bl) / 255.0
			if lum < 0.92 {
				dark := 1.0 - lum
				boost := math.Pow(dark, 0.78) * 1.18
				if boost > 1.0 {
					boost = 1.0
				}
				factor := (1.0 - boost) / math.Max(0.05, lum)
				r *= factor
				g *= factor
				bl *= factor
			} else {
				r, g, bl = 255, 255, 255
			}

			// Crisp Slate 500/600 perimeter ring so white squircle reads cleanly on light Windows 11 taskbars.
			if sd > -borderThick {
				rim := (sd + borderThick) / borderThick
				if rim < 0 {
					rim = 0
				} else if rim > 1 {
					rim = 1
				}
				rim *= 0.88
				r = r*(1-rim) + 100.0*rim
				g = g*(1-rim) + 116.0*rim
				bl = bl*(1-rim) + 139.0*rim
			}

			if r < 0 {
				r = 0
			} else if r > 255 {
				r = 255
			}
			if g < 0 {
				g = 0
			} else if g > 255 {
				g = 255
			}
			if bl < 0 {
				bl = 0
			} else if bl > 255 {
				bl = 255
			}

			dst.SetNRGBA(px, py, color.NRGBA{
				R: uint8(r + 0.5),
				G: uint8(g + 0.5),
				B: uint8(bl + 0.5),
				A: uint8(cov*255.0 + 0.5),
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
