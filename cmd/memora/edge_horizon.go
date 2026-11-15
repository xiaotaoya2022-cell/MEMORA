package main

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"memora/internal/config"
	"memora/internal/engine"
)

var (
	iphlpapi = syscall.NewLazyDLL("iphlpapi.dll")
	winmm    = syscall.NewLazyDLL("winmm.dll")
	pdhDLL   = syscall.NewLazyDLL("pdh.dll")
	nvmlDLL  = syscall.NewLazyDLL("nvml.dll")

	procUpdateLayeredWindow         = user32.NewProc("UpdateLayeredWindow")
	procSetTimer                    = user32.NewProc("SetTimer")
	procKillTimer                   = user32.NewProc("KillTimer")
	procSetCursorWin                = user32.NewProc("SetCursor")
	procCreateDIBSection            = gdi32.NewProc("CreateDIBSection")
	procD3DKMTOpenAdapterFromGDI    = gdi32.NewProc("D3DKMTOpenAdapterFromGdiDisplayName")
	procD3DKMTQueryAdapterInfo      = gdi32.NewProc("D3DKMTQueryAdapterInfo")
	procD3DKMTCloseAdapter          = gdi32.NewProc("D3DKMTCloseAdapter")
	procGetIfTable                  = iphlpapi.NewProc("GetIfTable")
	procTimeBeginPeriod             = winmm.NewProc("timeBeginPeriod")
	procTimeEndPeriod               = winmm.NewProc("timeEndPeriod")
	procPdhOpenQueryW               = pdhDLL.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounterW       = pdhDLL.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData         = pdhDLL.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterArray = pdhDLL.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery               = pdhDLL.NewProc("PdhCloseQuery")
	procNvmlInitV2                  = nvmlDLL.NewProc("nvmlInit_v2")
	procNvmlDeviceGetCountV2        = nvmlDLL.NewProc("nvmlDeviceGetCount_v2")
	procNvmlDeviceGetHandleByIndex  = nvmlDLL.NewProc("nvmlDeviceGetHandleByIndex_v2")
	procNvmlDeviceGetTemperature    = nvmlDLL.NewProc("nvmlDeviceGetTemperature")
)

const (
	wsExLayered      = 0x00080000
	wsExNoActivate   = 0x08000000
	swShowNoActivate = 4

	ulwAlpha      = 0x00000002
	wmSetCursor   = 0x0020
	wmTimer       = 0x0113
	wmLButtonDown = 0x0201
	wmDisplayChg  = 0x007E
	wmEdgeSync    = wmUser + 31
	wmEdgeExpand  = wmUser + 32

	idcHand    = 32649
	idcSizeAll = 32646

	edgeTimerAnimID = 101
	edgeTimerNetID  = 102

	ifTypeLoopback    = 24
	ifTypeTunnel      = 131
	ifOperConnected   = 4
	ifOperOperational = 5
)

type winSize struct {
	Cx int32
	Cy int32
}

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
	BmiColors [1]uint32
}

// mibIfRow mirrors Windows SDK MIB_IFROW (860 bytes, zero 8-byte alignment padding).
type mibIfRow struct {
	Name          [256]uint16
	Index         uint32
	Type          uint32
	Mtu           uint32
	Speed         uint32
	PhysAddrLen   uint32
	PhysAddr      [8]byte
	AdminStatus   uint32
	OperStatus    uint32
	LastChange    uint32
	InOctets      uint32
	InUcastPkts   uint32
	InNUcastPkts  uint32
	InDiscards    uint32
	InErrors      uint32
	InUnknown     uint32
	OutOctets     uint32
	OutUcastPkts  uint32
	OutNUcastPkts uint32
	OutDiscards   uint32
	OutErrors     uint32
	OutQLen       uint32
	DescrLen      uint32
	Descr         [256]byte
}

type netIfCounter struct {
	in  uint32
	out uint32
}

type edgeNetSampler struct {
	mu      sync.Mutex
	buf     []byte
	prev    map[uint32]netIfCounter
	lastAt  time.Time
	downBps float64
	upBps   float64
}

func newEdgeNetSampler() *edgeNetSampler {
	return &edgeNetSampler{
		buf:  make([]byte, 64*1024),
		prev: make(map[uint32]netIfCounter, 16),
	}
}

func isVirtualOrTunnelDescr(descr string) bool {
	if descr == "" {
		return false
	}
	l := strings.ToLower(descr)
	virtualHints := []string{
		"hyper-v", "vethernet", "vmware", "virtualbox",
		"meta tunnel", "wintun", "wireguard", "tap-", "npcap",
		"loopback", "teredo", "isatap", "qos packet", "wfp ", "filter",
	}
	for _, h := range virtualHints {
		if strings.Contains(l, h) {
			return true
		}
	}
	return false
}

func (ns *edgeNetSampler) Sample() (downBps, upBps float64) {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	if procGetIfTable.Find() != nil {
		return ns.downBps, ns.upBps
	}

	size := uint32(len(ns.buf))
	ret, _, _ := procGetIfTable.Call(
		uintptr(unsafe.Pointer(&ns.buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
	)
	if ret == 122 { // ERROR_INSUFFICIENT_BUFFER
		if size > 2*1024*1024 {
			size = 2 * 1024 * 1024
		}
		ns.buf = make([]byte, size)
		ret, _, _ = procGetIfTable.Call(
			uintptr(unsafe.Pointer(&ns.buf[0])),
			uintptr(unsafe.Pointer(&size)),
			0,
		)
	}
	if ret != 0 || len(ns.buf) < 4 {
		return ns.downBps, ns.upBps
	}

	numEntries := *(*uint32)(unsafe.Pointer(&ns.buf[0]))
	rowSize := unsafe.Sizeof(mibIfRow{})
	maxRows := (uintptr(len(ns.buf)) - 4) / rowSize
	if uintptr(numEntries) > maxRows {
		numEntries = uint32(maxRows)
	}

	type rowSnap struct {
		idx     uint32
		in      uint32
		out     uint32
		virtual bool
	}
	rows := make([]rowSnap, 0, numEntries)
	hasPhysical := false

	for i := uint32(0); i < numEntries; i++ {
		off := uintptr(4) + uintptr(i)*rowSize
		row := (*mibIfRow)(unsafe.Pointer(&ns.buf[off]))
		if row.Type == ifTypeLoopback || row.Type == ifTypeTunnel {
			continue
		}
		if row.OperStatus != ifOperOperational && row.OperStatus != ifOperConnected {
			continue
		}
		dLen := int(row.DescrLen)
		if dLen > len(row.Descr) {
			dLen = len(row.Descr)
		}
		descr := strings.TrimRight(string(row.Descr[:dLen]), "\x00 ")
		virt := isVirtualOrTunnelDescr(descr) || row.Type == 53 // IF_TYPE_PROP_VIRTUAL
		if !virt {
			hasPhysical = true
		}
		rows = append(rows, rowSnap{
			idx:     row.Index,
			in:      row.InOctets,
			out:     row.OutOctets,
			virtual: virt,
		})
	}

	now := time.Now()
	dt := now.Sub(ns.lastAt).Seconds()
	firstRun := ns.lastAt.IsZero()
	ns.lastAt = now

	var dIn, dOut uint64
	nextMap := make(map[uint32]netIfCounter, len(rows))
	for _, r := range rows {
		nextMap[r.idx] = netIfCounter{in: r.in, out: r.out}
		if hasPhysical && r.virtual {
			continue
		}
		if prev, ok := ns.prev[r.idx]; ok && !firstRun && dt > 0.05 {
			// Unsigned uint32 subtraction naturally handles 4GB rollover per sample.
			dIn += uint64(r.in - prev.in)
			dOut += uint64(r.out - prev.out)
		}
	}
	ns.prev = nextMap

	if !firstRun && dt > 0.05 {
		instDown := float64(dIn) / dt
		instUp := float64(dOut) / dt
		// Light EWMA smoothing so speeds look fluid without trailing lag.
		if ns.downBps == 0 && ns.upBps == 0 {
			ns.downBps = instDown
			ns.upBps = instUp
		} else {
			ns.downBps = ns.downBps*0.35 + instDown*0.65
			ns.upBps = ns.upBps*0.35 + instUp*0.65
		}
	}
	return ns.downBps, ns.upBps
}

const (
	pdhFmtDouble          = 0x00000200
	pdhMoreData           = 0x800007D2
	kmtQaiTypeAdapterPerf = 62
)

type winLUID struct {
	LowPart  uint32
	HighPart int32
}

type d3dkmtOpenAdapterFromGdiDisplayName struct {
	DeviceName    [32]uint16
	HAdapter      uint32
	AdapterLuid   winLUID
	VidPnSourceId uint32
}

type d3dkmtCloseAdapter struct {
	HAdapter uint32
}

type d3dkmtQueryAdapterInfo struct {
	HAdapter              uint32
	Type                  uint32
	PPrivateDriverData    unsafe.Pointer
	PrivateDriverDataSize uint32
}

type d3dkmtAdapterPerfData struct {
	PhysicalAdapterIndex uint32
	MemoryFrequency      uint64
	MaxMemoryFrequency   uint64
	MaxMemoryFrequencyOC uint64
	MemoryBandwidth      uint64
	PCIEBandwidth        uint64
	FanRPM               uint32
	Power                uint32
	Temperature          uint32
	PowerStateOverride   byte
}

type pdhFmtCounterValueItemW struct {
	SzName      *uint16
	CStatus     uint32
	DoubleValue float64
}

type edgeThermalSampler struct {
	mu           sync.Mutex
	pdhInitTried bool
	pdhQuery     uintptr
	pdhHighPrec  uintptr
	pdhBasicTemp uintptr
	nvmlInited   bool
	nvmlFailed   bool

	lastAt     time.Time
	cpuTempC   float64
	gpuTempC   float64
	hasCPUTemp bool
	hasGPUTemp bool
}

func newEdgeThermalSampler() *edgeThermalSampler {
	return &edgeThermalSampler{}
}

func (ts *edgeThermalSampler) Close() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.pdhQuery != 0 {
		_, _, _ = procPdhCloseQuery.Call(ts.pdhQuery)
		ts.pdhQuery = 0
		ts.pdhHighPrec = 0
		ts.pdhBasicTemp = 0
	}
}

func (ts *edgeThermalSampler) ensurePDHLocked() bool {
	if ts.pdhInitTried {
		return ts.pdhQuery != 0
	}
	ts.pdhInitTried = true
	if procPdhOpenQueryW.Find() != nil ||
		procPdhAddEnglishCounterW.Find() != nil ||
		procPdhCollectQueryData.Find() != nil ||
		procPdhGetFormattedCounterArray.Find() != nil {
		return false
	}

	var q uintptr
	if r, _, _ := procPdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&q))); uint32(r) != 0 || q == 0 {
		return false
	}

	hiPath := mustUTF16(`\Thermal Zone Information(*)\High Precision Temperature`)
	var hiC uintptr
	if r, _, _ := procPdhAddEnglishCounterW.Call(q, uintptr(unsafe.Pointer(hiPath)), 0, uintptr(unsafe.Pointer(&hiC))); uint32(r) == 0 {
		ts.pdhHighPrec = hiC
	}

	loPath := mustUTF16(`\Thermal Zone Information(*)\Temperature`)
	var loC uintptr
	if r, _, _ := procPdhAddEnglishCounterW.Call(q, uintptr(unsafe.Pointer(loPath)), 0, uintptr(unsafe.Pointer(&loC))); uint32(r) == 0 {
		ts.pdhBasicTemp = loC
	}

	if ts.pdhHighPrec == 0 && ts.pdhBasicTemp == 0 {
		_, _, _ = procPdhCloseQuery.Call(q)
		return false
	}
	ts.pdhQuery = q
	return true
}

func queryPdhThermalMaxC(counter uintptr, highPrecision bool) (float64, bool) {
	if counter == 0 {
		return 0, false
	}
	var bufSize, itemCount uint32
	ret, _, _ := procPdhGetFormattedCounterArray.Call(
		counter,
		pdhFmtDouble,
		uintptr(unsafe.Pointer(&bufSize)),
		uintptr(unsafe.Pointer(&itemCount)),
		0,
	)
	if uint32(ret) != pdhMoreData || bufSize == 0 || bufSize > 256*1024 {
		return 0, false
	}
	buf := make([]byte, bufSize)
	ret, _, _ = procPdhGetFormattedCounterArray.Call(
		counter,
		pdhFmtDouble,
		uintptr(unsafe.Pointer(&bufSize)),
		uintptr(unsafe.Pointer(&itemCount)),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if uint32(ret) != 0 || itemCount == 0 {
		return 0, false
	}
	itemSize := unsafe.Sizeof(pdhFmtCounterValueItemW{})
	if uintptr(itemCount)*itemSize > uintptr(len(buf)) {
		return 0, false
	}
	items := unsafe.Slice((*pdhFmtCounterValueItemW)(unsafe.Pointer(&buf[0])), itemCount)
	best := 0.0
	ok := false
	for _, it := range items {
		if it.CStatus != 0 && it.CStatus != 1 {
			continue
		}
		var c float64
		if highPrecision {
			c = it.DoubleValue/10.0 - 273.15
		} else if it.DoubleValue > 1000.0 {
			c = it.DoubleValue/10.0 - 273.15
		} else {
			c = it.DoubleValue - 273.15
		}
		if c >= 10.0 && c <= 125.0 && (!ok || c > best) {
			best = c
			ok = true
		}
	}
	return best, ok
}

func (ts *edgeThermalSampler) sampleNVML() (float64, bool) {
	if ts.nvmlFailed {
		return 0, false
	}
	if !ts.nvmlInited {
		if procNvmlInitV2.Find() != nil ||
			procNvmlDeviceGetCountV2.Find() != nil ||
			procNvmlDeviceGetHandleByIndex.Find() != nil ||
			procNvmlDeviceGetTemperature.Find() != nil {
			ts.nvmlFailed = true
			return 0, false
		}
		if r, _, _ := procNvmlInitV2.Call(); r != 0 {
			ts.nvmlFailed = true
			return 0, false
		}
		ts.nvmlInited = true
	}

	var count uint32
	if r, _, _ := procNvmlDeviceGetCountV2.Call(uintptr(unsafe.Pointer(&count))); r != 0 || count == 0 {
		return 0, false
	}
	best := 0.0
	ok := false
	for i := uint32(0); i < count && i < 8; i++ {
		var dev uintptr
		if r, _, _ := procNvmlDeviceGetHandleByIndex.Call(uintptr(i), uintptr(unsafe.Pointer(&dev))); r != 0 || dev == 0 {
			continue
		}
		var temp uint32
		if r, _, _ := procNvmlDeviceGetTemperature.Call(dev, 0, uintptr(unsafe.Pointer(&temp))); r == 0 {
			c := float64(temp)
			if c >= 10.0 && c <= 125.0 && (!ok || c > best) {
				best = c
				ok = true
			}
		}
	}
	return best, ok
}

func sampleD3DKMTGPU() (float64, bool) {
	if procD3DKMTOpenAdapterFromGDI.Find() != nil ||
		procD3DKMTQueryAdapterInfo.Find() != nil ||
		procD3DKMTCloseAdapter.Find() != nil {
		return 0, false
	}
	best := 0.0
	ok := false
	for i := 1; i <= 4; i++ {
		var open d3dkmtOpenAdapterFromGdiDisplayName
		name := fmt.Sprintf(`\\.\DISPLAY%d`, i)
		if u, err := syscall.UTF16FromString(name); err == nil {
			copy(open.DeviceName[:len(open.DeviceName)-1], u)
		}
		if st, _, _ := procD3DKMTOpenAdapterFromGDI.Call(uintptr(unsafe.Pointer(&open))); uint32(st) != 0 || open.HAdapter == 0 {
			continue
		}
		var perf d3dkmtAdapterPerfData
		q := d3dkmtQueryAdapterInfo{
			HAdapter:              open.HAdapter,
			Type:                  kmtQaiTypeAdapterPerf,
			PPrivateDriverData:    unsafe.Pointer(&perf),
			PrivateDriverDataSize: uint32(unsafe.Sizeof(perf)),
		}
		qst, _, _ := procD3DKMTQueryAdapterInfo.Call(uintptr(unsafe.Pointer(&q)))
		runtime.KeepAlive(&perf)
		cl := d3dkmtCloseAdapter{HAdapter: open.HAdapter}
		_, _, _ = procD3DKMTCloseAdapter.Call(uintptr(unsafe.Pointer(&cl)))
		if uint32(qst) != 0 {
			continue
		}
		c := float64(perf.Temperature) / 10.0
		if c >= 10.0 && c <= 125.0 && (!ok || c > best) {
			best = c
			ok = true
		}
	}
	return best, ok
}

func (ts *edgeThermalSampler) Sample() (cpuC float64, hasCPU bool, gpuC float64, hasGPU bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	now := time.Now()
	smoothEligible := !ts.lastAt.IsZero() && now.Sub(ts.lastAt) <= 10*time.Second

	var rawCPU float64
	var gotCPU bool
	if ts.ensurePDHLocked() {
		if r, _, _ := procPdhCollectQueryData.Call(ts.pdhQuery); uint32(r) == 0 {
			if rawCPU, gotCPU = queryPdhThermalMaxC(ts.pdhHighPrec, true); !gotCPU {
				rawCPU, gotCPU = queryPdhThermalMaxC(ts.pdhBasicTemp, false)
			}
		}
	}

	rawGPU, gotGPU := ts.sampleNVML()
	if !gotGPU {
		rawGPU, gotGPU = sampleD3DKMTGPU()
	}

	if gotCPU {
		if smoothEligible && ts.hasCPUTemp {
			ts.cpuTempC = ts.cpuTempC*0.35 + rawCPU*0.65
		} else {
			ts.cpuTempC = rawCPU
		}
		ts.hasCPUTemp = true
	} else {
		ts.hasCPUTemp = false
	}

	if gotGPU {
		if smoothEligible && ts.hasGPUTemp {
			ts.gpuTempC = ts.gpuTempC*0.35 + rawGPU*0.65
		} else {
			ts.gpuTempC = rawGPU
		}
		ts.hasGPUTemp = true
	} else {
		ts.hasGPUTemp = false
	}

	ts.lastAt = now
	return ts.cpuTempC, ts.hasCPUTemp, ts.gpuTempC, ts.hasGPUTemp
}

type edgeHorizonSnapshot struct {
	enabled         bool
	mode            string
	fullscreenQuiet bool
	side            string
	yPct            float64
	hasSample       bool
	inUsePct        float64
	smartTier       int
	smartLabel      string
	softManaged     int
	inUseBytes      uint64
	availBytes      uint64
	totalBytes      uint64
	anomalyBytes    uint64 // Zombie + Vid (or Zombie + IdleBg accent)
	secondaryBytes  uint64 // Vid (when VM active) or IdleBg (universal host)
	slot2Mode       string // "vid" | "idle_bg"
	zombieBytes     uint64
	vidBytes        uint64
}

type edgeHorizonWidget struct {
	mu        sync.Mutex
	state     edgeHorizonSnapshot
	callbacks struct {
		onOptimize func() (engine.OptimizeResult, error)
		onOpenUI   func()
		onShowMenu func()
		onSavePos  func(side string, yPct float64)
	}

	// UI thread fields (strictly touched on OS-locked tray message thread).
	parentHwnd   uintptr
	hwnd         uintptr
	dpi          int32
	visible      bool
	netTimerOn     bool
	animTimerOn    bool
	periodActive   bool
	netSampler     *edgeNetSampler
	thermalSampler *edgeThermalSampler

	downBps    float64
	upBps      float64
	cpuTempC   float64
	gpuTempC   float64
	hasCPUTemp bool
	hasGPUTemp bool

	hovering            bool
	trackingMouse       bool
	ringHovered         bool
	suppressUntilLeave  bool
	leaveTimerPending   bool
	leaveDeadline       time.Time
	autoRetractPending  bool
	autoRetractDeadline time.Time

	dragging      bool
	dragMoved     bool
	dragStartCur  winPoint
	dragCur       winPoint
	dragStartWin  winPoint
	dragStartVisX int32
	dragStartVisY int32
	dragOffsetX   float64
	dragVelX      float64

	// Critically-damped spring state for zero-jerk 120Hz expand/collapse transitions.
	expandProg   float64
	expandVel    float64
	targetExpand float64
	dispPct      float64
	hasDispPct   bool

	optimizing    bool
	spinAngle     float64
	optResultText string
	optResultAt   time.Time

	lastTick time.Time
	workRect winRect

	// Cached persistent GDI/DIB surface (1x output + 2x supersampled render canvas) and AA fonts.
	cachedDPI  int32
	bufW       int32
	bufH       int32
	memDC      uintptr
	dib        uintptr
	oldBmp     uintptr
	bitsPtr    unsafe.Pointer
	ssMemDC    uintptr
	ssDib      uintptr
	ssOldBmp   uintptr
	ssBitsPtr  unsafe.Pointer
	savedAlpha []byte
	fontTitle  uintptr
	fontPill   uintptr
	fontNet    uintptr
	fontSub    uintptr
	fontPct    uintptr

	// Cached window position & hit-test geometry.
	winX       int32
	winY       int32
	winW       int32
	winH       int32
	ringCX     float64
	ringCY     float64
	ringOuterR float64
	cardLeft   float64
	cardRight  float64
	cardTop    float64
	cardBottom float64
}

var (
	edgeWidgetMu   sync.Mutex
	edgeWidgetInst *edgeHorizonWidget
	edgeClassOnce  sync.Once
)

func initEdgeHorizon(
	parentHwnd uintptr,
	initialCfg config.Config,
	onOptimize func() (engine.OptimizeResult, error),
	onOpenUI func(),
	onShowMenu func(),
	onSavePos func(side string, yPct float64),
) {
	w := &edgeHorizonWidget{
		parentHwnd:     parentHwnd,
		netSampler:     newEdgeNetSampler(),
		thermalSampler: newEdgeThermalSampler(),
		workRect:       winRect{Left: 0, Top: 0, Right: 1920, Bottom: 1032},
	}
	w.state = edgeHorizonSnapshot{
		enabled: initialCfg.Actions.EdgeHorizon,
		mode:    initialCfg.Mode,
		side:    normalizeEdgeSide(initialCfg.EdgeWidget.Side),
		yPct:    clampEdgeY(initialCfg.EdgeWidget.YPercent),
	}
	w.callbacks.onOptimize = onOptimize
	w.callbacks.onOpenUI = onOpenUI
	w.callbacks.onShowMenu = onShowMenu
	w.callbacks.onSavePos = onSavePos

	edgeWidgetMu.Lock()
	edgeWidgetInst = w
	edgeWidgetMu.Unlock()

	createEdgeHorizonWindow(w)
}

func updateEdgeHorizonState(cfg config.Config, a engine.Assessment, bd engine.MemoryBreakdown, hasSample bool) {
	edgeWidgetMu.Lock()
	w := edgeWidgetInst
	edgeWidgetMu.Unlock()
	if w == nil {
		return
	}

	w.mu.Lock()
	w.state.enabled = cfg.Actions.EdgeHorizon
	w.state.mode = cfg.Mode
	w.state.fullscreenQuiet = cfg.Thresholds.QuietOnFullscreen && a.FullscreenQuiet
	if s := normalizeEdgeSide(cfg.EdgeWidget.Side); s != "" {
		w.state.side = s
	}
	if cfg.EdgeWidget.YPercent > 0 {
		w.state.yPct = clampEdgeY(cfg.EdgeWidget.YPercent)
	}
	if hasSample {
		w.state.hasSample = true
		if a.Snap.TotalPhys > 0 {
			w.state.inUsePct = 100 - a.AvailPct
			if w.state.inUsePct < 0 {
				w.state.inUsePct = 0
			} else if w.state.inUsePct > 100 {
				w.state.inUsePct = 100
			}
		}
		w.state.smartTier = a.SmartTier
		w.state.smartLabel = a.SmartLabel
		w.state.softManaged = a.SoftManaged
		w.state.inUseBytes = a.Snap.InUse()
		w.state.availBytes = a.Snap.AvailPhys
		w.state.totalBytes = a.Snap.TotalPhys
		w.state.zombieBytes = bd.ZombieLeakBytes
		w.state.vidBytes = bd.VidBytes
		w.state.slot2Mode = bd.Slot2Mode
		if bd.Slot2Mode == "vid" {
			w.state.secondaryBytes = bd.VidBytes
			w.state.anomalyBytes = bd.ZombieLeakBytes + bd.VidBytes
		} else {
			w.state.secondaryBytes = bd.IdleAppWSBytes + bd.VidBytes
			w.state.anomalyBytes = bd.ZombieLeakBytes
		}
	}
	hwnd := w.hwnd
	w.mu.Unlock()

	if hwnd != 0 {
		_, _, _ = procPostMessage.Call(hwnd, wmEdgeSync, 0, 0)
	}
}

func closeEdgeHorizon() {
	edgeWidgetMu.Lock()
	w := edgeWidgetInst
	edgeWidgetInst = nil
	edgeWidgetMu.Unlock()
	if w == nil {
		return
	}
	w.mu.Lock()
	hwnd := w.hwnd
	w.hwnd = 0
	ts := w.thermalSampler
	w.mu.Unlock()
	if hwnd != 0 {
		_, _, _ = procDestroyWindow.Call(hwnd)
	}
	if ts != nil {
		ts.Close()
	}
}

func normalizeEdgeSide(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "right") {
		return "right"
	}
	return "left"
}

func clampEdgeY(y float64) float64 {
	if y <= 0 {
		return 0.22
	}
	if y < 0.04 {
		return 0.04
	}
	if y > 0.92 {
		return 0.92
	}
	return y
}

func createEdgeHorizonWindow(w *edgeHorizonWidget) {
	hInst, _, _ := procGetModuleHandle.Call(0)
	clsName := mustUTF16("MEMORAEdgeHorizonClass")

	edgeClassOnce.Do(func() {
		wc := wndClassExW{
			CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			Style:         csHRedraw | csVRedraw,
			LpfnWndProc:   syscall.NewCallback(edgeHorizonWndProc),
			HInstance:     hInst,
			LpszClassName: clsName,
		}
		_, _, _ = procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	})

	title := mustUTF16("MEMORA 贴边真相光尺")
	hwnd, _, _ := procCreateWindowEx.Call(
		wsExLayered|wsExTopmost|wsExToolWin|wsExNoActivate,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		0, 200, 230, 96,
		0,
		0,
		hInst,
		0,
	)
	if hwnd == 0 {
		return
	}

	w.mu.Lock()
	w.hwnd = hwnd
	w.mu.Unlock()

	w.dpi = getWindowDPI(hwnd)
	w.refreshWorkAreaForCurrentSide()
	w.cpuTempC, w.hasCPUTemp, w.gpuTempC, w.hasGPUTemp = w.thermalSampler.Sample()
	w.syncVisibilityAndPaint()
}

func edgeHorizonWndProc(h uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	edgeWidgetMu.Lock()
	w := edgeWidgetInst
	edgeWidgetMu.Unlock()
	if w == nil || (w.hwnd != 0 && w.hwnd != h) {
		r, _, _ := procDefWindowProc.Call(h, uintptr(msg), wParam, lParam)
		return r
	}

	switch msg {
	case wmEraseBkgnd:
		return 1

	case wmEdgeSync:
		w.syncVisibilityAndPaint()
		return 0

	case wmEdgeExpand:
		w.onOptimizeDoneUI()
		return 0

	case wmDisplayChg:
		w.dpi = getWindowDPI(h)
		w.refreshWorkAreaForCurrentSide()
		w.syncVisibilityAndPaint()
		return 0

	case wmDpiChanged:
		w.dpi = getWindowDPI(h)
		w.refreshWorkAreaForCurrentSide()
		w.syncVisibilityAndPaint()
		return 0

	case wmSetCursor:
		curID := uintptr(idcArrow)
		if w.dragging {
			curID = idcSizeAll
		} else if w.ringHovered || w.expandProg > 0.45 {
			curID = idcHand
		}
		cur, _, _ := procLoadCursorW.Call(0, curID)
		if cur != 0 {
			_, _, _ = procSetCursorWin.Call(cur)
			return 1
		}

	case wmTimer:
		switch wParam {
		case edgeTimerNetID:
			if w.visible {
				if w.expandProg > 0.02 || w.hovering || w.optimizing {
					if w.thermalSampler != nil {
						w.cpuTempC, w.hasCPUTemp, w.gpuTempC, w.hasGPUTemp = w.thermalSampler.Sample()
					}
					w.renderLayered()
				}
			}
		case edgeTimerAnimID:
			w.stepAnimation()
		}
		return 0

	case wmMouseMove:
		if w.dragging {
			var cur winPoint
			_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
			dx := cur.X - w.dragStartCur.X
			dy := cur.Y - w.dragStartCur.Y
			if !w.dragMoved && (dx*dx+dy*dy >= 16) {
				w.dragMoved = true
			}
			if w.dragMoved {
				w.handleDragMove(cur, dy)
			}
			return 0
		}

		if !w.trackingMouse {
			tme := trackMouseEventStruct{
				CbSize:    uint32(unsafe.Sizeof(trackMouseEventStruct{})),
				DwFlags:   tmeLeave,
				HWndTrack: h,
			}
			_, _, _ = procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			w.trackingMouse = true
		}

		mx := int32(int16(lParam & 0xffff))
		my := int32(int16((lParam >> 16) & 0xffff))

		w.leaveTimerPending = false
		w.autoRetractPending = false
		if !w.suppressUntilLeave {
			if !w.hovering {
				w.hovering = true
				w.targetExpand = 1
				if w.thermalSampler != nil {
					w.cpuTempC, w.hasCPUTemp, w.gpuTempC, w.hasGPUTemp = w.thermalSampler.Sample()
				}
				w.ensureAnimTimer(true)
			}
		}

		nextRingHover := w.hitTestRing(mx, my)
		if nextRingHover != w.ringHovered {
			w.ringHovered = nextRingHover
			w.renderLayered()
		}
		return 0

	case wmMouseLeave:
		w.trackingMouse = false
		w.suppressUntilLeave = false
		w.ringHovered = false
		if !w.dragging && w.hovering {
			if !w.autoRetractPending {
				w.leaveTimerPending = true
				w.leaveDeadline = time.Now().Add(220 * time.Millisecond)
				w.ensureAnimTimer(true)
			}
		}
		return 0

	case wmLButtonDown:
		var cur winPoint
		_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
		w.dragStartCur = cur
		w.dragCur = cur
		w.dragStartWin = winPoint{X: w.winX, Y: w.winY}
		w.mu.Lock()
		startSide := w.state.side
		w.mu.Unlock()
		if startSide == "right" {
			w.dragStartVisX = w.winX + int32(math.Round(w.cardRight))
		} else {
			w.dragStartVisX = w.winX + int32(math.Round(w.cardLeft))
		}
		w.dragStartVisY = w.winY + int32(math.Round(w.cardTop))
		w.dragOffsetX = 0
		w.dragVelX = 0
		w.leaveTimerPending = false
		w.autoRetractPending = false
		w.dragging = true
		w.dragMoved = false
		_, _, _ = procSetCapture.Call(h)
		return 0

	case wmLButtonUp:
		if !w.dragging {
			return 0
		}
		_, _, _ = procReleaseCapture.Call()
		wasMoved := w.dragMoved
		var dropCur winPoint
		_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&dropCur)))
		w.dragging = false
		w.dragMoved = false

		if wasMoved {
			wk := w.workRect
			packedPt := uintptr(uint32(dropCur.X)) | (uintptr(uint32(dropCur.Y)) << 32)
			if hMon, _, _ := procMonitorFromPoint.Call(packedPt, monitorDefaultToNearest); hMon != 0 {
				var mi monitorInfo
				mi.CbSize = uint32(unsafe.Sizeof(mi))
				if ok, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); ok != 0 {
					wk = mi.RcWork
					w.workRect = wk
				}
			}

			dpi := w.dpi
			if dpi < 96 {
				dpi = 96
			}
			sc := float64(dpi) / 96.0
			p := clamp01(w.expandProg)
			curW := lerpF(9.0*sc, 214.0*sc, p)
			edgeGap := lerpF(2.0*sc, 6.0*sc, p)

			curVisLeft := w.winX + int32(math.Round(w.cardLeft))
			visW := int32(math.Round(w.cardRight - w.cardLeft))
			visCenterX := curVisLeft + visW/2
			midX := (wk.Left + wk.Right) / 2
			newSide := "left"
			if visCenterX >= midX {
				newSide = "right"
			}

			var newLeftX float64
			var newDockWinX int32
			if newSide == "right" {
				newLeftX = float64(w.bufW) - edgeGap - curW
				newDockWinX = wk.Right - w.bufW
			} else {
				newLeftX = edgeGap
				newDockWinX = wk.Left
			}

			w.dragOffsetX = float64(curVisLeft) - (float64(newDockWinX) + newLeftX)
			w.dragVelX = 0

			w.mu.Lock()
			w.state.side = newSide
			yPct := w.state.yPct
			saveFn := w.callbacks.onSavePos
			w.mu.Unlock()
			if saveFn != nil {
				go saveFn(newSide, yPct)
			}

			w.leaveTimerPending = false
			w.autoRetractPending = true
			w.autoRetractDeadline = time.Now().Add(360 * time.Millisecond)
			w.ensureAnimTimer(true)
			w.renderLayered()
			return 0
		}

		mx := int32(int16(lParam & 0xffff))
		my := int32(int16((lParam >> 16) & 0xffff))
		if w.hitTestRing(mx, my) {
			w.triggerOptimize()
		} else if w.expandProg > 0.45 && w.hitTestCard(mx, my) {
			w.mu.Lock()
			openFn := w.callbacks.onOpenUI
			w.mu.Unlock()
			if openFn != nil {
				go openFn()
			}
		} else {
			w.suppressUntilLeave = false
			w.hovering = true
			w.targetExpand = 1
			w.ensureAnimTimer(true)
		}
		return 0

	case wmRButtonUp:
		w.leaveTimerPending = false
		w.mu.Lock()
		menuFn := w.callbacks.onShowMenu
		w.mu.Unlock()
		if menuFn != nil {
			menuFn()
		}
		return 0

	case wmDestroy:
		w.ensureAnimTimer(false)
		w.ensureNetTimer(false)
		w.destroySurfaceLocked()
		return 0
	}

	r, _, _ := procDefWindowProc.Call(h, uintptr(msg), wParam, lParam)
	return r
}

func (w *edgeHorizonWidget) hitTestRing(mx, my int32) bool {
	if w.expandProg < 0.45 {
		return false
	}
	dx := float64(mx) - w.ringCX
	dy := float64(my) - w.ringCY
	lim := w.ringOuterR + float64(scalePx(5, w.dpi))
	return dx*dx+dy*dy <= lim*lim
}

func (w *edgeHorizonWidget) hitTestCard(mx, my int32) bool {
	return float64(mx) >= w.cardLeft && float64(mx) <= w.cardRight &&
		float64(my) >= w.cardTop && float64(my) <= w.cardBottom
}

func (w *edgeHorizonWidget) triggerOptimize() {
	if w.optimizing {
		return
	}
	w.mu.Lock()
	optFn := w.callbacks.onOptimize
	w.mu.Unlock()
	if optFn == nil {
		return
	}
	w.optimizing = true
	w.optResultText = ""
	w.autoRetractPending = false
	w.leaveTimerPending = false
	w.ensureAnimTimer(true)
	w.renderLayered()

	hwnd := w.hwnd
	go func() {
		res, err := optFn()
		msg := "已维持基线稳态"
		if err != nil {
			msg = "深度释压失败"
		} else if res.AvailGainMB >= 1 {
			msg = fmt.Sprintf("✓ 已净增 %s", fmtEdgeMB(res.AvailGainMB))
		} else if res.WSShrunkMB >= 1 {
			msg = fmt.Sprintf("✓ 已收敛 %s", fmtEdgeMB(res.WSShrunkMB))
		} else if res.TrimmedCount > 0 {
			msg = fmt.Sprintf("✓ 已紧缩 %d 项", res.TrimmedCount)
		}

		w.mu.Lock()
		w.optResultText = msg
		w.optResultAt = time.Now()
		w.mu.Unlock()
		if hwnd != 0 {
			_, _, _ = procPostMessage.Call(hwnd, wmEdgeExpand, 0, 0)
		}
	}()
}

func (w *edgeHorizonWidget) onOptimizeDoneUI() {
	w.optimizing = false
	w.autoRetractPending = true
	w.autoRetractDeadline = time.Now().Add(2000 * time.Millisecond)
	w.ensureAnimTimer(true)
	w.renderLayered()
}

func (w *edgeHorizonWidget) refreshWorkAreaForCurrentSide() {
	w.mu.Lock()
	yPct := w.state.yPct
	w.mu.Unlock()

	probeX := int32(0)
	if w.workRect.Right > w.workRect.Left {
		w.mu.Lock()
		if w.state.side == "right" {
			probeX = w.workRect.Right - 4
		} else {
			probeX = w.workRect.Left + 4
		}
		w.mu.Unlock()
	}
	probeY := w.workRect.Top + int32(float64(w.workRect.Bottom-w.workRect.Top)*yPct)
	packedPt := uintptr(uint32(probeX)) | (uintptr(uint32(probeY)) << 32)
	if hMon, _, _ := procMonitorFromPoint.Call(packedPt, monitorDefaultToNearest); hMon != 0 {
		var mi monitorInfo
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		if ok, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); ok != 0 {
			w.workRect = mi.RcWork
		}
	}
}

func (w *edgeHorizonWidget) handleDragMove(cur winPoint, dy int32) {
	wk := w.workRect
	packedPt := uintptr(uint32(cur.X)) | (uintptr(uint32(cur.Y)) << 32)
	if hMon, _, _ := procMonitorFromPoint.Call(packedPt, monitorDefaultToNearest); hMon != 0 {
		var mi monitorInfo
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		if ok, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi))); ok != 0 {
			wk = mi.RcWork
			w.workRect = wk
		}
	}

	w.dragCur = cur

	margin := scalePx(8, w.dpi)
	minY := wk.Top + margin
	maxY := wk.Bottom - w.winH - margin
	if maxY < minY {
		maxY = minY
	}
	ny := w.dragStartWin.Y + dy
	if ny < minY {
		ny = minY
	} else if ny > maxY {
		ny = maxY
	}

	availSpan := maxY - minY
	yPct := 0.22
	if availSpan > 0 {
		yPct = 0.04 + float64(ny-minY)/float64(availSpan)*0.88
	}

	w.mu.Lock()
	w.state.yPct = clampEdgeY(yPct)
	w.mu.Unlock()

	w.renderLayered()
}

func (w *edgeHorizonWidget) syncVisibilityAndPaint() {
	if w.hwnd == 0 {
		return
	}
	w.mu.Lock()
	snap := w.state
	w.mu.Unlock()

	shouldShow := snap.enabled && !snap.fullscreenQuiet
	if snap.hasSample {
		if !w.hasDispPct {
			w.dispPct = snap.inUsePct
			w.hasDispPct = true
		} else if math.Abs(snap.inUsePct-w.dispPct) > 0.15 {
			w.ensureAnimTimer(true)
		}
	}

	if !shouldShow {
		if w.visible {
			w.visible = false
			w.hovering = false
			w.ringHovered = false
			w.expandProg = 0
			w.expandVel = 0
			w.targetExpand = 0
			w.leaveTimerPending = false
			w.autoRetractPending = false
			w.ensureAnimTimer(false)
			w.ensureNetTimer(false)
			_, _, _ = procShowWindow.Call(w.hwnd, swHide)
		}
		return
	}

	if !w.visible {
		w.visible = true
		w.refreshWorkAreaForCurrentSide()
		if w.thermalSampler != nil {
			w.cpuTempC, w.hasCPUTemp, w.gpuTempC, w.hasGPUTemp = w.thermalSampler.Sample()
		}
		w.ensureNetTimer(true)
		w.renderLayered()
		_, _, _ = procShowWindow.Call(w.hwnd, swShowNoActivate)
		_, _, _ = procSetWindowPos.Call(w.hwnd, ^uintptr(0) /* HWND_TOPMOST */, 0, 0, 0, 0,
			swpNoMove|swpNoSize|swpNoActivate)
		return
	}

	w.renderLayered()
}

func (w *edgeHorizonWidget) ensureAnimTimer(on bool) {
	if w.hwnd == 0 || w.animTimerOn == on {
		return
	}
	w.animTimerOn = on
	if on {
		if !w.periodActive {
			_, _, _ = procTimeBeginPeriod.Call(1)
			w.periodActive = true
		}
		w.lastTick = time.Now()
		_, _, _ = procSetTimer.Call(w.hwnd, edgeTimerAnimID, 8, 0) // ~120Hz silky-smooth spring step
	} else {
		_, _, _ = procKillTimer.Call(w.hwnd, edgeTimerAnimID)
		if w.periodActive {
			_, _, _ = procTimeEndPeriod.Call(1)
			w.periodActive = false
		}
	}
}

func (w *edgeHorizonWidget) ensureNetTimer(on bool) {
	if w.hwnd == 0 || w.netTimerOn == on {
		return
	}
	w.netTimerOn = on
	if on {
		_, _, _ = procSetTimer.Call(w.hwnd, edgeTimerNetID, 1000, 0)
	} else {
		_, _, _ = procKillTimer.Call(w.hwnd, edgeTimerNetID)
	}
}

func (w *edgeHorizonWidget) destroySurfaceLocked() {
	if w.ssMemDC != 0 && w.ssOldBmp != 0 {
		_, _, _ = procSelectObject.Call(w.ssMemDC, w.ssOldBmp)
		w.ssOldBmp = 0
	}
	if w.ssDib != 0 {
		_, _, _ = procDeleteObject.Call(w.ssDib)
		w.ssDib = 0
		w.ssBitsPtr = nil
	}
	if w.ssMemDC != 0 {
		_, _, _ = procDeleteDC.Call(w.ssMemDC)
		w.ssMemDC = 0
	}
	if w.memDC != 0 && w.oldBmp != 0 {
		_, _, _ = procSelectObject.Call(w.memDC, w.oldBmp)
		w.oldBmp = 0
	}
	if w.dib != 0 {
		_, _, _ = procDeleteObject.Call(w.dib)
		w.dib = 0
		w.bitsPtr = nil
	}
	if w.memDC != 0 {
		_, _, _ = procDeleteDC.Call(w.memDC)
		w.memDC = 0
	}
	for _, fp := range []*uintptr{&w.fontTitle, &w.fontPill, &w.fontNet, &w.fontSub, &w.fontPct} {
		if *fp != 0 {
			_, _, _ = procDeleteObject.Call(*fp)
			*fp = 0
		}
	}
	w.cachedDPI = 0
	w.bufW = 0
	w.bufH = 0
}

func createTopDownDIB(screenDC uintptr, width, height int32) (memDC, dib, oldBmp uintptr, bitsPtr unsafe.Pointer, ok bool) {
	memDC, _, _ = procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return 0, 0, 0, nil, false
	}
	bmi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       width,
			BiHeight:      -height, // top-down
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: 0,
		},
	}
	dib, _, _ = procCreateDIBSection.Call(
		memDC,
		uintptr(unsafe.Pointer(&bmi)),
		0, // DIB_RGB_COLORS
		uintptr(unsafe.Pointer(&bitsPtr)),
		0,
		0,
	)
	if dib == 0 || bitsPtr == nil {
		_, _, _ = procDeleteDC.Call(memDC)
		return 0, 0, 0, nil, false
	}
	oldBmp, _, _ = procSelectObject.Call(memDC, dib)
	return memDC, dib, oldBmp, bitsPtr, true
}

func (w *edgeHorizonWidget) ensureSurfaceAndFontsLocked(dpi, bufW, bufH int32) bool {
	if w.memDC != 0 && w.dib != 0 && w.bitsPtr != nil &&
		w.ssMemDC != 0 && w.ssDib != 0 && w.ssBitsPtr != nil &&
		w.cachedDPI == dpi && w.bufW == bufW && w.bufH == bufH {
		return true
	}

	w.destroySurfaceLocked()

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return false
	}
	memDC, dib, oldBmp, bitsPtr, ok1 := createTopDownDIB(screenDC, bufW, bufH)
	var ssMemDC, ssDib, ssOldBmp uintptr
	var ssBitsPtr unsafe.Pointer
	ok2 := false
	if ok1 {
		ssMemDC, ssDib, ssOldBmp, ssBitsPtr, ok2 = createTopDownDIB(screenDC, bufW*2, bufH*2)
	}
	_, _, _ = procReleaseDC.Call(0, screenDC)
	if !ok1 || !ok2 {
		if ok1 {
			_, _, _ = procSelectObject.Call(memDC, oldBmp)
			_, _, _ = procDeleteObject.Call(dib)
			_, _, _ = procDeleteDC.Call(memDC)
		}
		return false
	}

	w.memDC = memDC
	w.dib = dib
	w.oldBmp = oldBmp
	w.bitsPtr = bitsPtr
	w.ssMemDC = ssMemDC
	w.ssDib = ssDib
	w.ssOldBmp = ssOldBmp
	w.ssBitsPtr = ssBitsPtr
	w.cachedDPI = dpi
	w.bufW = bufW
	w.bufH = bufH

	needCap := int(bufW * 2 * bufH * 2)
	if cap(w.savedAlpha) < needCap {
		w.savedAlpha = make([]byte, needCap)
	}

	// Render fonts at 2x supersampled pixel height on ssMemDC for razor-sharp HiDPI clarity.
	ssDPI := dpi * 2
	titleFace := resolveInstalledFont("Bahnschrift", "Segoe UI Variable Display", "Segoe UI", "Microsoft YaHei UI")
	netFace := resolveInstalledFont("Segoe UI Variable Text", "Segoe UI", "Microsoft YaHei UI")
	cjkFace := resolveInstalledFont("Noto Sans SC", "Microsoft YaHei UI")

	w.fontTitle = createAAFontFace(scalePx(12, ssDPI), 700, titleFace)
	w.fontPill = createAAFontFace(scalePx(11, ssDPI), 700, cjkFace)
	w.fontNet = createAAFontFace(scalePx(11, ssDPI), 600, netFace)
	w.fontSub = createAAFontFace(scalePx(11, ssDPI), 600, cjkFace)
	w.fontPct = createAAFontFace(scalePx(12, ssDPI), 700, titleFace)
	_, _, _ = procSetBkMode.Call(ssMemDC, 1) // TRANSPARENT
	return true
}

func (w *edgeHorizonWidget) stepAnimation() {
	now := time.Now()
	dt := now.Sub(w.lastTick).Seconds()
	if dt <= 0 || dt > 0.05 {
		dt = 0.008
	}
	w.lastTick = now

	if w.leaveTimerPending && !now.Before(w.leaveDeadline) {
		w.leaveTimerPending = false
		w.hovering = false
		w.targetExpand = 0
	}

	if w.autoRetractPending && !now.Before(w.autoRetractDeadline) {
		w.autoRetractPending = false
		w.hovering = false
		w.ringHovered = false
		w.targetExpand = 0
		if w.trackingMouse {
			w.suppressUntilLeave = true
		}
	}

	w.mu.Lock()
	targetPct := w.state.inUsePct
	hasSample := w.state.hasSample
	optAt := w.optResultAt
	optText := w.optResultText
	w.mu.Unlock()

	if optText != "" && !w.optimizing && now.Sub(optAt) > 3500*time.Millisecond {
		w.mu.Lock()
		w.optResultText = ""
		w.mu.Unlock()
	}

	keepTimer := w.leaveTimerPending || w.autoRetractPending

	// Critically damped spring (omega=23 -> settles smoothly in ~250ms with C1 continuity).
	diff := w.targetExpand - w.expandProg
	if math.Abs(diff) > 0.0015 || math.Abs(w.expandVel) > 0.008 {
		const subSteps = 4
		const omega = 23.0
		subDt := dt / float64(subSteps)
		for i := 0; i < subSteps; i++ {
			acc := omega*omega*(w.targetExpand-w.expandProg) - 2.0*omega*w.expandVel
			w.expandVel += acc * subDt
			w.expandProg += w.expandVel * subDt
		}
		if w.expandProg < 0 {
			w.expandProg = 0
			w.expandVel = 0
		} else if w.expandProg > 1 {
			w.expandProg = 1
			w.expandVel = 0
		}
		keepTimer = true
	} else {
		w.expandProg = w.targetExpand
		w.expandVel = 0
	}

	// Critically damped horizontal drop spring (dragOffsetX -> 0) for silky full-screen edge docking.
	if !w.dragging {
		if math.Abs(w.dragOffsetX) > 0.35 || math.Abs(w.dragVelX) > 2.0 {
			const subSteps = 4
			const omegaX = 22.0
			subDt := dt / float64(subSteps)
			for i := 0; i < subSteps; i++ {
				acc := -omegaX*omegaX*w.dragOffsetX - 2.0*omegaX*w.dragVelX
				w.dragVelX += acc * subDt
				w.dragOffsetX += w.dragVelX * subDt
			}
			keepTimer = true
		} else {
			w.dragOffsetX = 0
			w.dragVelX = 0
		}
	}

	// Animate percentage gauge readout smoothly.
	if hasSample {
		if !w.hasDispPct {
			w.dispPct = targetPct
			w.hasDispPct = true
		} else if math.Abs(targetPct-w.dispPct) > 0.08 {
			w.dispPct += (targetPct - w.dispPct) * math.Min(1, dt*10.0)
			keepTimer = true
		} else {
			w.dispPct = targetPct
		}
	}

	if w.optimizing {
		w.spinAngle = math.Mod(w.spinAngle+dt*2*math.Pi*1.5, 2*math.Pi)
		keepTimer = true
	}

	w.renderLayered()

	if !keepTimer {
		w.ensureAnimTimer(false)
	}
}

func lerpF(a, b, t float64) float64 {
	return a + (b-a)*t
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func sdRoundedBox(px, py, cx, cy, halfW, halfH, r float64) float64 {
	if r > halfW {
		r = halfW
	}
	if r > halfH {
		r = halfH
	}
	qx := math.Abs(px-cx) - (halfW - r)
	qy := math.Abs(py-cy) - (halfH - r)
	dx := math.Max(qx, 0)
	dy := math.Max(qy, 0)
	return math.Hypot(dx, dy) + math.Min(math.Max(qx, qy), 0) - r
}

func blendPremulOver(pix []byte, idx int, r, g, b, a float64) {
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	sr := r * a
	sg := g * a
	sb := b * a
	inv := 1 - a

	db := float64(pix[idx])
	dg := float64(pix[idx+1])
	dr := float64(pix[idx+2])
	da := float64(pix[idx+3]) / 255.0

	nb := sb + db*inv
	ng := sg + dg*inv
	nr := sr + dr*inv
	na := (a + da*inv) * 255.0

	if nb > na {
		nb = na
	}
	if ng > na {
		ng = na
	}
	if nr > na {
		nr = na
	}
	if na > 255 {
		na = 255
	}
	pix[idx] = uint8(nb + 0.5)
	pix[idx+1] = uint8(ng + 0.5)
	pix[idx+2] = uint8(nr + 0.5)
	pix[idx+3] = uint8(na + 0.5)
}

func (w *edgeHorizonWidget) renderLayered() {
	if w.hwnd == 0 || !w.visible {
		return
	}

	w.mu.Lock()
	snap := w.state
	optText := w.optResultText
	w.mu.Unlock()

	dpi := w.dpi
	if dpi < 96 {
		dpi = 96
	}
	sc := float64(dpi) / 96.0

	// Fixed outer layered canvas keeps DWM window surface constant across frames (zero surface reallocation).
	bufW := scalePx(230, dpi)
	bufH := scalePx(96, dpi)
	if bufW%2 != 0 {
		bufW++
	}
	if bufH%2 != 0 {
		bufH++
	}

	if !w.ensureSurfaceAndFontsLocked(dpi, bufW, bufH) {
		return
	}

	p := clamp01(w.expandProg)

	// 1x layout metrics for window positioning & hit testing.
	barW1x := 9.0 * sc
	barH1x := 80.0 * sc
	capsuleW1x := 214.0 * sc
	capsuleH1x := 68.0 * sc
	curW1x := lerpF(barW1x, capsuleW1x, p)
	curH1x := lerpF(barH1x, capsuleH1x, p)
	edgeGap1x := lerpF(2.0*sc, 6.0*sc, p)

	var leftX1x float64
	if snap.side == "right" {
		leftX1x = float64(bufW) - edgeGap1x - curW1x
	} else {
		leftX1x = edgeGap1x
	}
	topY1x := (float64(bufH) - curH1x) / 2.0
	rightX1x := leftX1x + curW1x
	bottomY1x := topY1x + curH1x

	w.cardLeft = leftX1x
	w.cardRight = rightX1x
	w.cardTop = topY1x
	w.cardBottom = bottomY1x

	// Dock flush to selected work-area edge, or glide freely anywhere across the screen while dragging.
	wk := w.workRect
	margin := scalePx(8, dpi)
	minScreenY := wk.Top + margin
	maxScreenY := wk.Bottom - bufH - margin
	if maxScreenY < minScreenY {
		maxScreenY = minScreenY
	}
	normPct := (snap.yPct - 0.04) / 0.88
	if normPct < 0 {
		normPct = 0
	} else if normPct > 1 {
		normPct = 1
	}
	winY := minScreenY + int32(normPct*float64(maxScreenY-minScreenY))

	dockWinX := wk.Left
	if snap.side == "right" {
		dockWinX = wk.Right - bufW
	}
	var winX int32
	if w.dragging && w.dragMoved {
		visW := int32(math.Round(curW1x))
		dx := w.dragCur.X - w.dragStartCur.X
		winX = computeEdgeDragWinX(snap.side, wk, visW, w.dragStartVisX, dx, leftX1x, rightX1x)
		w.dragOffsetX = float64(winX - dockWinX)
	} else {
		winX = dockWinX + int32(math.Round(w.dragOffsetX))
	}
	w.winX = winX
	w.winY = winY
	w.winW = bufW
	w.winH = bufH

	// 2x supersampled render canvas coordinates (ssW x ssH).
	ssW := bufW * 2
	ssH := bufH * 2
	ssSC := sc * 2.0
	barW := 9.0 * ssSC
	barH := 80.0 * ssSC
	capsuleW := 214.0 * ssSC
	capsuleH := 68.0 * ssSC
	curW := lerpF(barW, capsuleW, p)
	curH := lerpF(barH, capsuleH, p)
	curR := lerpF(4.5*ssSC, 14.0*ssSC, p)
	edgeGap := lerpF(2.0*ssSC, 6.0*ssSC, p)

	var leftX float64
	if snap.side == "right" {
		leftX = float64(ssW) - edgeGap - curW
	} else {
		leftX = edgeGap
	}
	topY := (float64(ssH) - curH) / 2.0
	rightX := leftX + curW
	bottomY := topY + curH
	cx := (leftX + rightX) / 2.0
	cy := (topY + bottomY) / 2.0
	halfW := curW / 2.0
	halfH := curH / 2.0

	totalSSBytes := int(ssW * ssH * 4)
	pix := unsafe.Slice((*byte)(w.ssBitsPtr), totalSSBytes)
	clear(pix)

	// Unified Porcelain Daylight card alpha (subtle translucent ceramic rest -> solid porcelain expanded card).
	calmAlpha := 1.0
	if p < 0.05 && !w.hovering && !w.dragging {
		if snap.smartTier >= 2 || snap.anomalyBytes > 256*1048576 {
			calmAlpha = 0.96
		} else {
			calmAlpha = 0.90
		}
	}

	// Dual-color memory breakdown ratios aligned with Dashboard Breakdown Bar.
	inUseFrac := 0.25
	secFrac := 0.0
	zombieFrac := 0.0
	if snap.totalBytes > 0 {
		inUseFrac = clamp01(float64(snap.inUseBytes) / float64(snap.totalBytes))
		zombieFrac = clamp01(float64(snap.zombieBytes) / float64(snap.totalBytes))
		secFrac = clamp01(float64(snap.secondaryBytes) / float64(snap.totalBytes))
		if zombieFrac+secFrac > inUseFrac {
			if zombieFrac > inUseFrac {
				zombieFrac = inUseFrac
				secFrac = 0
			} else {
				secFrac = inUseFrac - zombieFrac
			}
		}
	}
	normalFrac := inUseFrac - zombieFrac - secFrac

	// Semantic Hero Gauge & Light Ruler primary accent aligned with Dashboard hero gauge.
	var baseR, baseG, baseB float64 = 16, 185, 129 // #10B981 Emerald cruise
	if snap.mode != "guard" {
		baseR, baseG, baseB = 100, 116, 139 // #64748B Slate observe
	} else if snap.smartTier >= 3 {
		baseR, baseG, baseB = 239, 68, 68 // #EF4444 Red urgent
	} else if snap.smartTier == 2 {
		baseR, baseG, baseB = 245, 158, 11 // #F59E0B Amber pressure
	}

	shadowRadius := lerpF(2.0*ssSC, 9.5*ssSC, p)
	shadowAlphaMax := lerpF(0.08, 0.14, p)

	minYBound := int(math.Floor(topY - shadowRadius - 2))
	maxYBound := int(math.Ceil(bottomY + shadowRadius + 3))
	if minYBound < 0 {
		minYBound = 0
	}
	if maxYBound >= int(ssH) {
		maxYBound = int(ssH) - 1
	}

	minXBound := int(math.Floor(leftX - shadowRadius - 2))
	maxXBound := int(math.Ceil(rightX + shadowRadius + 2))
	if minXBound < 0 {
		minXBound = 0
	}
	if maxXBound >= int(ssW) {
		maxXBound = int(ssW) - 1
	}

	// Pass 1: Porcelain #FFFFFF card surface, soft slate #CBD5E1 1px border, soft elevation shadow.
	cardAlpha := lerpF(0.92, 0.99, p) * calmAlpha
	for y := minYBound; y <= maxYBound; y++ {
		py := float64(y) + 0.5
		rowOff := y * int(ssW) * 4
		for x := minXBound; x <= maxXBound; x++ {
			px := float64(x) + 0.5
			idx := rowOff + x*4

			d := sdRoundedBox(px, py, cx, cy, halfW, halfH, curR)
			if d > 0.6 {
				if p > 0.03 {
					ds := sdRoundedBox(px, py-1.5*ssSC, cx, cy, halfW, halfH, curR)
					if ds >= 0 && ds < shadowRadius {
						t := 1.0 - ds/shadowRadius
						sa := shadowAlphaMax * t * t * p
						blendPremulOver(pix, idx, 15, 23, 42, sa)
					}
				}
				continue
			}

			bodyCov := clamp01(0.5 - d)
			if bodyCov <= 0 {
				continue
			}

			// Porcelain alabaster interior #FFFFFF.
			blendPremulOver(pix, idx, 255, 255, 255, bodyCov*cardAlpha)

			// Crisp 1.1px border #CBD5E1 (outer) -> #E2E8F0 (when settled collapsed).
			if d > -1.15*ssSC {
				rimCov := clamp01((d + 1.15*ssSC) / (1.15 * ssSC))
				blendPremulOver(pix, idx, 203, 213, 225, rimCov*bodyCov*0.92*calmAlpha)
			}
		}
	}

	// Pass 2: Screen-edge Fitts's Law invisible cushion STRICTLY on the outer 2px edge gap (never steals inward pixels).
	if p < 0.05 {
		hitTop := int(math.Floor(topY))
		hitBot := int(math.Ceil(bottomY))
		if hitTop < 0 {
			hitTop = 0
		}
		if hitBot >= int(ssH) {
			hitBot = int(ssH) - 1
		}
		var xStart, xEnd int
		if snap.side == "right" {
			xStart = int(math.Floor(rightX))
			xEnd = int(ssW) - 1
		} else {
			xStart = 0
			xEnd = int(math.Ceil(leftX))
		}
		if xStart < 0 {
			xStart = 0
		}
		if xEnd >= int(ssW) {
			xEnd = int(ssW) - 1
		}
		for y := hitTop; y <= hitBot; y++ {
			rowOff := y * int(ssW) * 4
			for x := xStart; x <= xEnd; x++ {
				if pix[rowOff+x*4+3] == 0 {
					pix[rowOff+x*4+3] = 1
				}
			}
		}
	}

	// Pass 3: Recessed Slate #F1F5F9 inner vertical meter track + top Royal Blue indicator dot + semantic fills.
	meterW := lerpF(4.0*ssSC, 3.6*ssSC, p)
	meterH := lerpF(barH-17.0*ssSC, capsuleH-18.0*ssSC, p)
	meterR := meterW / 2.0
	var meterCX float64
	if snap.side == "right" && p > 0.25 {
		meterCX = rightX - lerpF(barW/2.0, 9.5*ssSC, p)
	} else {
		meterCX = leftX + lerpF(barW/2.0, 9.5*ssSC, p)
	}
	meterCY := lerpF(cy+4.5*ssSC, cy, p)
	meterTop := meterCY - meterH/2.0
	meterBot := meterCY + meterH/2.0

	// Top status dot visible cleanly inside the Porcelain Light Ruler (fades smoothly during expand).
	dotAlpha := (1.0 - clamp01(p/0.35)) * calmAlpha
	if dotAlpha > 0.02 {
		dotCX := cx
		dotCY := topY + 6.5*ssSC
		dotR := 1.85 * ssSC
		dx0 := int(math.Floor(dotCX - dotR - 2))
		dx1 := int(math.Ceil(dotCX + dotR + 2))
		dy0 := int(math.Floor(dotCY - dotR - 2))
		dy1 := int(math.Ceil(dotCY + dotR + 2))
		var dr, dg, db float64 = 37, 99, 235 // #2563EB MEMORA Royal Blue
		if snap.mode != "guard" {
			dr, dg, db = 148, 163, 184 // #94A3B8 observe slate
		}
		for y := maxInt(0, dy0); y <= minInt(int(ssH)-1, dy1); y++ {
			py := float64(y) + 0.5
			rowOff := y * int(ssW) * 4
			for x := maxInt(0, dx0); x <= minInt(int(ssW)-1, dx1); x++ {
				px := float64(x) + 0.5
				dist := math.Hypot(px-dotCX, py-dotCY)
				cov := clamp01(dotR + 0.5 - dist)
				if cov > 0 {
					blendPremulOver(pix, rowOff+x*4, dr, dg, db, cov*dotAlpha)
				}
			}
		}
	}

	normalTop := meterBot - normalFrac*meterH
	secTop := normalTop - secFrac*meterH
	zombieTop := secTop - zombieFrac*meterH

	mX0 := maxInt(0, int(math.Floor(meterCX-meterW/2.0-2)))
	mX1 := minInt(int(ssW)-1, int(math.Ceil(meterCX+meterW/2.0+2)))
	mY0 := maxInt(0, int(math.Floor(meterTop-2)))
	mY1 := minInt(int(ssH)-1, int(math.Ceil(meterBot+2)))

	for y := mY0; y <= mY1; y++ {
		py := float64(y) + 0.5
		rowOff := y * int(ssW) * 4
		for x := mX0; x <= mX1; x++ {
			px := float64(x) + 0.5
			dm := sdRoundedBox(px, py, meterCX, meterCY, meterW/2.0, meterH/2.0, meterR)
			mcov := clamp01(0.5 - dm)
			if mcov <= 0 {
				continue
			}
			idx := rowOff + x*4

			// Recessed Slate 100 (#E2E8F0) track inside the white porcelain ruler.
			blendPremulOver(pix, idx, 226, 232, 240, mcov*0.95*calmAlpha)

			// Normal active usage fill (Emerald / Amber / Red / Slate).
			if normalFrac > 0.002 && py >= normalTop-0.5 {
				yCov := clamp01(py - (normalTop - 0.5))
				blendPremulOver(pix, idx, baseR, baseG, baseB, mcov*calmAlpha*yCov)
			}

			// Slot 2 stack (Violet #8B5CF6: WSL2/Vid lock or Idle Background WS).
			if secFrac > 0.004 && py >= secTop-0.5 && py <= normalTop+0.5 {
				topC := clamp01(py - (secTop - 0.5))
				botC := clamp01((normalTop + 0.5) - py)
				blendPremulOver(pix, idx, 139, 92, 246, mcov*calmAlpha*math.Min(topC, botC))
			}

			// Kernel zombie leak tip (Red #EF4444).
			if zombieFrac > 0.004 && py >= zombieTop-0.5 && py <= secTop+0.5 {
				topC := clamp01(py - (zombieTop - 0.5))
				botC := clamp01((secTop + 0.5) - py)
				blendPremulOver(pix, idx, 239, 68, 68, mcov*calmAlpha*math.Min(topC, botC))
			}
		}
	}

	// Pass 4: Expanded Bento Card Content (Hero Gauge Ring + MEMORA Header/Pill + Network + Status).
	contentAlpha := clamp01((p - 0.30) / 0.50)
	if contentAlpha > 0.01 {
		var ringCX, textLeft, textRight float64
		if snap.side == "right" {
			// Docked right: [ text column | gauge ring | edge meter bar ]
			ringCX = rightX - 38.0*ssSC
			textLeft = leftX + 12.0*ssSC
			textRight = ringCX - 23.0*ssSC
		} else {
			// Docked left: [ edge meter bar | gauge ring | text column ]
			ringCX = leftX + 38.0*ssSC
			textLeft = ringCX + 23.0*ssSC
			textRight = rightX - 11.0*ssSC
		}
		ringCY := cy
		ringR := 17.5 * ssSC
		ringThick := 3.5 * ssSC
		ringOuterR := ringR + ringThick/2.0 + 1.5*ssSC

		// Store 1x coordinates for pointer hit-testing.
		w.ringCX = ringCX * 0.5
		w.ringCY = ringCY * 0.5
		w.ringOuterR = ringOuterR * 0.5

		rX0 := maxInt(0, int(math.Floor(ringCX-ringOuterR-3)))
		rX1 := minInt(int(ssW)-1, int(math.Ceil(ringCX+ringOuterR+3)))
		rY0 := maxInt(0, int(math.Floor(ringCY-ringOuterR-3)))
		rY1 := minInt(int(ssH)-1, int(math.Ceil(ringCY+ringOuterR+3)))

		pct := w.dispPct
		if !w.hasDispPct {
			pct = snap.inUsePct
		}
		pctSweep := clamp01(pct/100.0) * 2 * math.Pi

		for y := rY0; y <= rY1; y++ {
			py := float64(y) + 0.5
			rowOff := y * int(ssW) * 4
			for x := rX0; x <= rX1; x++ {
				px := float64(x) + 0.5
				idx := rowOff + x*4
				dx := px - ringCX
				dy := py - ringCY
				dist := math.Hypot(dx, dy)

				if w.ringHovered && !w.optimizing && dist < ringR-ringThick/2.0 {
					inCov := clamp01((ringR - ringThick/2.0) - dist)
					blendPremulOver(pix, idx, 239, 246, 255, inCov*contentAlpha) // #EFF6FF --blue-soft
				}

				annDist := math.Abs(dist - ringR)
				ringCov := clamp01(ringThick/2.0 + 0.6 - annDist)
				if ringCov <= 0 {
					continue
				}

				// Track arc background (#E2E8F0).
				blendPremulOver(pix, idx, 226, 232, 240, ringCov*contentAlpha)

				ang := math.Atan2(dx, -dy)
				if ang < 0 {
					ang += 2 * math.Pi
				}

				if w.optimizing {
					rel := math.Mod(ang-w.spinAngle+2*math.Pi, 2*math.Pi)
					cometLen := math.Pi * 1.15
					if rel <= cometLen {
						fade := clamp01(rel / cometLen)
						blendPremulOver(pix, idx, 37, 99, 235, ringCov*contentAlpha*(0.2+0.8*fade))
					}
				} else if pctSweep > 0.01 && ang <= pctSweep {
					blendPremulOver(pix, idx, baseR, baseG, baseB, ringCov*contentAlpha)
				}
			}
		}

		// SDF Status Pill on Top-Right of Telemetry Column.
		pillText, pillBgR, pillBgG, pillBgB, pillFgCol := edgeTierPillStyle(snap, w.optimizing)
		pillW := 42.0 * ssSC
		pillH := 16.0 * ssSC
		pillR := pillH / 2.0
		pillRight := textRight
		pillLeft := pillRight - pillW
		pillCX := (pillLeft + pillRight) / 2.0
		pillTop := topY + 7.0*ssSC
		pillCY := pillTop + pillH/2.0

		pX0 := maxInt(0, int(math.Floor(pillLeft-2)))
		pX1 := minInt(int(ssW)-1, int(math.Ceil(pillRight+2)))
		pY0 := maxInt(0, int(math.Floor(pillTop-2)))
		pY1 := minInt(int(ssH)-1, int(math.Ceil(pillTop+pillH+2)))
		for y := pY0; y <= pY1; y++ {
			py := float64(y) + 0.5
			rowOff := y * int(ssW) * 4
			for x := pX0; x <= pX1; x++ {
				px := float64(x) + 0.5
				dp := sdRoundedBox(px, py, pillCX, pillCY, pillW/2.0, pillH/2.0, pillR)
				pcov := clamp01(0.5 - dp)
				if pcov > 0 {
					blendPremulOver(pix, rowOff+x*4, pillBgR, pillBgG, pillBgB, pcov*contentAlpha)
				}
			}
		}

		// Crisp 2x supersampled GDI typography over premultiplied porcelain background.
		if contentAlpha > 0.60 {
			tX0 := maxInt(0, int(math.Floor(leftX+3*ssSC)))
			tX1 := minInt(int(ssW)-1, int(math.Ceil(rightX-3*ssSC)))
			tY0 := maxInt(0, int(math.Floor(topY+3*ssSC)))
			tY1 := minInt(int(ssH)-1, int(math.Ceil(bottomY-3*ssSC)))

			bw := tX1 - tX0 + 1
			bh := tY1 - tY0 + 1
			if bw > 0 && bh > 0 && bw*bh <= len(w.savedAlpha) {
				for yy := 0; yy < bh; yy++ {
					rowOff := (tY0+yy)*int(ssW)*4 + tX0*4
					for xx := 0; xx < bw; xx++ {
						w.savedAlpha[yy*bw+xx] = pix[rowOff+xx*4+3]
					}
				}

				const dtCenter = 0x00000001

				// Row 1 Left: Brand title "MEMORA"
				_, _, _ = procSelectObject.Call(w.ssMemDC, w.fontTitle)
				_, _, _ = procSetTextColor.Call(w.ssMemDC, 0x002A170F) // #0F172A Slate 900
				rcBrand := winRect{
					Left:   int32(textLeft),
					Top:    int32(topY + 6.0*ssSC),
					Right:  int32(pillLeft - 4*ssSC),
					Bottom: int32(topY + 24.0*ssSC),
				}
				drawTextLine(w.ssMemDC, "MEMORA", &rcBrand, dtLeft|dtSingleLine|dtVCenter|dtEndEllipsis)

				// Row 1 Right: Semantic status pill text
				_, _, _ = procSelectObject.Call(w.ssMemDC, w.fontPill)
				_, _, _ = procSetTextColor.Call(w.ssMemDC, uintptr(pillFgCol))
				rcPill := winRect{
					Left:   int32(pillLeft),
					Top:    int32(pillTop),
					Right:  int32(pillRight),
					Bottom: int32(pillTop + pillH),
				}
				drawTextLine(w.ssMemDC, pillText, &rcPill, dtCenter|dtSingleLine|dtVCenter)

				// Row 2: Native Hardware CPU & GPU Thermal Telemetry
				tempLine, tempCol := fmtEdgeTempLine(w.cpuTempC, w.hasCPUTemp, w.gpuTempC, w.hasGPUTemp)
				_, _, _ = procSelectObject.Call(w.ssMemDC, w.fontNet)
				_, _, _ = procSetTextColor.Call(w.ssMemDC, uintptr(tempCol))
				rcNet := winRect{
					Left:   int32(textLeft),
					Top:    int32(topY + 25.0*ssSC),
					Right:  int32(textRight),
					Bottom: int32(topY + 43.0*ssSC),
				}
				drawTextLine(w.ssMemDC, tempLine, &rcNet, dtLeft|dtSingleLine|dtVCenter|dtEndEllipsis)

				// Row 3: Truth / Action status feedback
				statusLine, statusCol := buildEdgeStatusLine(snap, w.optimizing, optText, w.ringHovered)
				_, _, _ = procSelectObject.Call(w.ssMemDC, w.fontSub)
				_, _, _ = procSetTextColor.Call(w.ssMemDC, uintptr(statusCol))
				rcSub := winRect{
					Left:   int32(textLeft),
					Top:    int32(topY + 43.0*ssSC),
					Right:  int32(textRight),
					Bottom: int32(bottomY - 5.0*ssSC),
				}
				drawTextLine(w.ssMemDC, statusLine, &rcSub, dtLeft|dtSingleLine|dtVCenter|dtEndEllipsis)

				// Hero Gauge Center Percentage
				pctLabel := fmt.Sprintf("%.0f%%", pct)
				if !snap.hasSample {
					pctLabel = "--"
				}
				ringCol := uint32(0x002A170F)
				if w.ringHovered && !w.optimizing {
					ringCol = 0x00EB6325 // #2563EB hover accent
				}
				_, _, _ = procSelectObject.Call(w.ssMemDC, w.fontPct)
				_, _, _ = procSetTextColor.Call(w.ssMemDC, uintptr(ringCol))
				rcPct := winRect{
					Left:   int32(ringCX - ringR),
					Top:    int32(ringCY - ringR),
					Right:  int32(ringCX + ringR),
					Bottom: int32(ringCY + ringR),
				}
				drawTextLine(w.ssMemDC, pctLabel, &rcPct, dtCenter|dtSingleLine|dtVCenter)

				// Restore premultiplied alpha bytes cleared by GDI DrawTextW.
				for yy := 0; yy < bh; yy++ {
					rowOff := (tY0+yy)*int(ssW)*4 + tX0*4
					for xx := 0; xx < bw; xx++ {
						sa := w.savedAlpha[yy*bw+xx]
						pIdx := rowOff + xx*4
						if sa == 0 {
							pix[pIdx] = 0
							pix[pIdx+1] = 0
							pix[pIdx+2] = 0
							pix[pIdx+3] = 0
							continue
						}
						pix[pIdx+3] = sa
						if pix[pIdx] > sa {
							pix[pIdx] = sa
						}
						if pix[pIdx+1] > sa {
							pix[pIdx+1] = sa
						}
						if pix[pIdx+2] > sa {
							pix[pIdx+2] = sa
						}
					}
				}
			}
		}
	}

	// 2x2 supersampled box-filter downsample (ssW x ssH -> bufW x bufH) for crisp subpixel-free AA.
	outPix := unsafe.Slice((*byte)(w.bitsPtr), int(bufW*bufH*4))
	ssStride := int(ssW) * 4
	outStride := int(bufW) * 4
	for y := 0; y < int(bufH); y++ {
		sRow0 := (y * 2) * ssStride
		sRow1 := sRow0 + ssStride
		oRow := y * outStride
		for x := 0; x < int(bufW); x++ {
			s0 := sRow0 + x*8
			s1 := sRow1 + x*8
			o := oRow + x*4
			outPix[o+0] = uint8((uint32(pix[s0+0]) + uint32(pix[s0+4]) + uint32(pix[s1+0]) + uint32(pix[s1+4]) + 2) >> 2)
			outPix[o+1] = uint8((uint32(pix[s0+1]) + uint32(pix[s0+5]) + uint32(pix[s1+1]) + uint32(pix[s1+5]) + 2) >> 2)
			outPix[o+2] = uint8((uint32(pix[s0+2]) + uint32(pix[s0+6]) + uint32(pix[s1+2]) + uint32(pix[s1+6]) + 2) >> 2)
			outPix[o+3] = uint8((uint32(pix[s0+3]) + uint32(pix[s0+7]) + uint32(pix[s1+3]) + uint32(pix[s1+7]) + 2) >> 2)
		}
	}

	dstPt := winPoint{X: winX, Y: winY}
	srcPt := winPoint{X: 0, Y: 0}
	sz := winSize{Cx: bufW, Cy: bufH}
	bf := blendFunction{
		BlendOp:             0,   // AC_SRC_OVER
		BlendFlags:          0,
		SourceConstantAlpha: 255,
		AlphaFormat:         1, // AC_SRC_ALPHA
	}
	_, _, _ = procUpdateLayeredWindow.Call(
		w.hwnd,
		0,
		uintptr(unsafe.Pointer(&dstPt)),
		uintptr(unsafe.Pointer(&sz)),
		w.memDC,
		uintptr(unsafe.Pointer(&srcPt)),
		0,
		uintptr(unsafe.Pointer(&bf)),
		ulwAlpha,
	)
}

func edgeTierPillStyle(snap edgeHorizonSnapshot, optimizing bool) (text string, bgR, bgG, bgB float64, fgCOLORREF uint32) {
	if optimizing {
		return "深释", 219, 234, 254, 0x00D84E1D // #DBEAFE / #1D4ED8
	}
	if snap.mode != "guard" {
		return "观测", 241, 245, 249, 0x00695547 // #F1F5F9 / #475569
	}
	if snap.smartTier >= 3 {
		return "高压", 254, 226, 226, 0x001C1CB9 // #FEE2E2 / #B91C1C
	}
	if snap.smartTier == 2 {
		return "平衡", 254, 243, 199, 0x000953B4 // #FEF3C7 / #B45309
	}
	return "巡航", 209, 250, 229, 0x00577804 // #D1FAE5 / #047857
}

func buildEdgeStatusLine(snap edgeHorizonSnapshot, optimizing bool, optText string, ringHovered bool) (string, uint32) {
	if optimizing {
		return "正在深度释压...", 0x00EB6325 // #2563EB
	}
	if optText != "" {
		return optText, 0x00699605 // #059669 emerald
	}
	if ringHovered {
		return "点击圆环 · 立即深度释压", 0x00EB6325
	}
	if snap.mode != "guard" {
		return "仅观测 · 未主动调度", 0x008B7464
	}
	if snap.zombieBytes > 256*1048576 {
		return "泄漏 " + fmtEdgeBytes(snap.zombieBytes), 0x002626DC // #DC2626
	}
	if snap.slot2Mode == "vid" && snap.vidBytes > 256*1048576 {
		return "WSL " + fmtEdgeBytes(snap.vidBytes), 0x00D9286D // #6D28D9
	}
	if snap.softManaged > 0 && snap.availBytes > 0 {
		return fmt.Sprintf("可用 %s · 托管 %d", fmtEdgeBytes(snap.availBytes), snap.softManaged), 0x00699605
	}
	if snap.softManaged > 0 {
		return fmt.Sprintf("基线托管 %d 项", snap.softManaged), 0x00699605
	}
	if snap.availBytes > 0 {
		return fmt.Sprintf("可用物理内存 %s", fmtEdgeBytes(snap.availBytes)), 0x008B7464
	}
	if snap.inUseBytes > 0 {
		return fmt.Sprintf("实占 %s", fmtEdgeBytes(snap.inUseBytes)), 0x008B7464
	}
	return "空载守护中", 0x008B7464
}

func computeEdgeDragWinX(side string, wk winRect, visW int32, dragStartVisX, dx int32, leftX, rightX float64) int32 {
	if side == "right" {
		desVisRight := dragStartVisX + dx
		minRight := wk.Left + visW
		maxRight := wk.Right
		if maxRight < minRight {
			maxRight = minRight
		}
		if desVisRight < minRight {
			desVisRight = minRight
		} else if desVisRight > maxRight {
			desVisRight = maxRight
		}
		return desVisRight - int32(math.Round(rightX))
	}
	desVisLeft := dragStartVisX + dx
	minLeft := wk.Left
	maxLeft := wk.Right - visW
	if maxLeft < minLeft {
		maxLeft = minLeft
	}
	if desVisLeft < minLeft {
		desVisLeft = minLeft
	} else if desVisLeft > maxLeft {
		desVisLeft = maxLeft
	}
	return desVisLeft - int32(math.Round(leftX))
}

func createAAFont(height int32, weight int32) uintptr {
	return createAAFontFace(height, weight, resolveInstalledFont("Noto Sans SC", "Microsoft YaHei UI"))
}

func createAAFontFace(height int32, weight int32, faceName string) uintptr {
	face := mustUTF16(faceName)
	h, _, _ := procCreateFontW.Call(
		uintptr(int64(-height)),
		0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		1, // DEFAULT_CHARSET
		0, 0,
		4, // ANTIALIASED_QUALITY (grayscale AA, zero ClearType fringes on layered alpha DIB)
		0,
		uintptr(unsafe.Pointer(face)),
	)
	return h
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func fmtEdgeTempLine(cpuC float64, hasCPU bool, gpuC float64, hasGPU bool) (string, uint32) {
	cpuStr := "--°C"
	if hasCPU {
		cpuStr = fmt.Sprintf("%.0f°C", math.Round(cpuC))
	}
	gpuStr := "--°C"
	if hasGPU {
		gpuStr = fmt.Sprintf("%.0f°C", math.Round(gpuC))
	}
	line := fmt.Sprintf("CPU %s · GPU %s", cpuStr, gpuStr)

	maxT := 0.0
	if hasCPU && cpuC > maxT {
		maxT = cpuC
	}
	if hasGPU && gpuC > maxT {
		maxT = gpuC
	}
	switch {
	case maxT >= 88.0:
		return line, 0x002626DC // #DC2626 Red 600
	case maxT >= 75.0:
		return line, 0x000953B4 // #B45309 Amber 700
	default:
		return line, 0x00554133 // #334155 Slate 700
	}
}

func fmtNetSpeed(bps float64) string {
	if bps < 512 {
		return "0 K/s"
	}
	kbps := bps / 1024.0
	if kbps < 10 {
		return fmt.Sprintf("%.1f K/s", kbps)
	}
	if kbps < 1000 {
		return fmt.Sprintf("%.0f K/s", kbps)
	}
	mbps := kbps / 1024.0
	if mbps < 100 {
		return fmt.Sprintf("%.1f M/s", mbps)
	}
	if mbps < 1000 {
		return fmt.Sprintf("%.0f M/s", mbps)
	}
	return fmt.Sprintf("%.1f G/s", mbps/1024.0)
}

func fmtEdgeMB(mb float64) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.1f GB", mb/1024.0)
	}
	return fmt.Sprintf("%.0f MB", mb)
}

func fmtEdgeBytes(b uint64) string {
	if b >= 1<<30 {
		return fmt.Sprintf("%.1fG", float64(b)/float64(1<<30))
	}
	return fmt.Sprintf("%dM", b/(1<<20))
}
