package main

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"memora/internal/config"
	"memora/internal/engine"
	"memora/internal/journal"
	"memora/internal/ledger"
)

func TestMibIfRowSizeAndNetSampler(t *testing.T) {
	if got := unsafe.Sizeof(mibIfRow{}); got != 860 {
		t.Fatalf("expected MIB_IFROW size 860 on windows/amd64, got %d", got)
	}
	if got := unsafe.Sizeof(bitmapInfoHeader{}); got != 40 {
		t.Fatalf("expected BITMAPINFOHEADER size 40, got %d", got)
	}
	if got := unsafe.Sizeof(blendFunction{}); got != 4 {
		t.Fatalf("expected BLENDFUNCTION size 4, got %d", got)
	}

	s := newEdgeNetSampler()
	down1, up1 := s.Sample()
	if down1 < 0 || up1 < 0 {
		t.Fatalf("unexpected negative net speeds: down=%v up=%v", down1, up1)
	}
	if len(s.prev) == 0 {
		t.Log("warning: GetIfTable returned 0 active interfaces in sandbox")
	}
	down2, up2 := s.Sample()
	if down2 < 0 || up2 < 0 {
		t.Fatalf("unexpected negative net speeds on second sample: down=%v up=%v", down2, up2)
	}
}

func TestEdgeHorizonConfigDefaultAndLegacyMigration(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)

	// Simulate an existing v0.2/v0.3 config.json without edge_horizon or edge_widget keys.
	cfgDir := filepath.Join(tmp, "MEMORA")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`{"mode":"guard","interval_seconds":10,"actions":{"smart_memory_schedule":true}}`)
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), legacy, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if !loaded.Actions.EdgeHorizon {
		t.Fatalf("expected EdgeHorizon default true when omitted from legacy config")
	}
	if loaded.EdgeWidget.Side != "left" || loaded.EdgeWidget.YPercent < 0.21 || loaded.EdgeWidget.YPercent > 0.23 {
		t.Fatalf("unexpected default EdgeWidget: %+v", loaded.EdgeWidget)
	}
}

func TestEdgeHorizonDashboardToggle(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)

	cfg := config.Default()
	jr, err := journal.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer jr.Close()
	lg, err := ledger.Open(tmp)
	if err != nil {
		t.Fatal(err)
	}

	eng := engine.New(cfg, jr)
	eng.SetLedger(lg)
	var lastCfg config.Config
	dash, err := newDashboardServer(
		cfg,
		lg,
		func(fn func(*engine.Engine)) error {
			fn(eng)
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer dash.Close()
	dash.SetOnConfigChange(func(c config.Config) {
		lastCfg = c
	})

	a, err := eng.Step(engine.Options{Act: false})
	if err != nil {
		t.Fatal(err)
	}
	dash.UpdateFromEngine(eng, a, true, false)

	callToggle := func(enabled bool) dashboardState {
		body, _ := json.Marshal(toggleRequest{Key: "edge_horizon", Enabled: enabled})
		r, _ := http.NewRequest(http.MethodPost, dash.BaseURL()+"/api/toggle", bytes.NewReader(body))
		r.Header.Set("X-Memora-Token", dash.token)
		r.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("toggle edge_horizon(%v) status = %d", enabled, resp.StatusCode)
		}
		var out dashboardState
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	stOff := callToggle(false)
	if stOff.Config.Actions.EdgeHorizon || lastCfg.Actions.EdgeHorizon {
		t.Fatalf("expected edge_horizon disabled after toggle false")
	}
	stOn := callToggle(true)
	if !stOn.Config.Actions.EdgeHorizon || !lastCfg.Actions.EdgeHorizon {
		t.Fatalf("expected edge_horizon enabled after toggle true")
	}
}

func TestEdgeHorizonFormattingAndHelpers(t *testing.T) {
	if got := fmtNetSpeed(120); got != "0 K/s" {
		t.Fatalf("fmtNetSpeed(120) = %q", got)
	}
	if got := fmtNetSpeed(21 * 1024); got != "21 K/s" {
		t.Fatalf("fmtNetSpeed(21KB) = %q", got)
	}
	if got := fmtNetSpeed(1536 * 1024); got != "1.5 M/s" {
		t.Fatalf("fmtNetSpeed(1.5MB) = %q", got)
	}
	if normalizeEdgeSide("RIGHT") != "right" || normalizeEdgeSide("unknown") != "left" {
		t.Fatalf("normalizeEdgeSide unexpected")
	}
	if clampEdgeY(-1) != 0.22 || clampEdgeY(0.01) != 0.04 || clampEdgeY(0.99) != 0.92 {
		t.Fatalf("clampEdgeY unexpected bounds")
	}
	line, _ := buildEdgeStatusLine(edgeHorizonSnapshot{mode: "guard", softManaged: 5}, false, "", false)
	if line != "基线托管 5 项" {
		t.Fatalf("unexpected status line: %q", line)
	}
	lineAvail, _ := buildEdgeStatusLine(edgeHorizonSnapshot{mode: "guard", softManaged: 5, availBytes: 20 << 30}, false, "", false)
	if lineAvail != "可用 20.0G · 托管 5" {
		t.Fatalf("unexpected avail+managed status line: %q", lineAvail)
	}
	if txt, _, _, _, _ := edgeTierPillStyle(edgeHorizonSnapshot{mode: "guard", smartTier: 1}, false); txt != "巡航" {
		t.Fatalf("unexpected tier1 pill: %q", txt)
	}
	if txt, _, _, _, _ := edgeTierPillStyle(edgeHorizonSnapshot{mode: "guard", smartTier: 3}, false); txt != "高压" {
		t.Fatalf("unexpected tier3 pill: %q", txt)
	}
	if txt, _, _, _, _ := edgeTierPillStyle(edgeHorizonSnapshot{mode: "observe"}, false); txt != "观测" {
		t.Fatalf("unexpected observe pill: %q", txt)
	}
	if txt, _, _, _, _ := edgeTierPillStyle(edgeHorizonSnapshot{mode: "guard"}, true); txt != "深释" {
		t.Fatalf("unexpected optimizing pill: %q", txt)
	}
}

func TestCrispTrayIconAndSelfMB(t *testing.T) {
	if got := popupSelfMB(0); got != 0 {
		t.Fatalf("popupSelfMB(0) = %d, want 0", got)
	}
	if got := popupSelfMB(89 * 1024 * 1024); got != 89 {
		t.Fatalf("popupSelfMB(89MB) = %d, want 89", got)
	}

	out, err := renderCrispTrayPNG(embeddedLogoPNG, 24)
	if err != nil {
		t.Fatalf("renderCrispTrayPNG(24): %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 24 || b.Dy() != 24 {
		t.Fatalf("unexpected rendered icon bounds: %v", b)
	}

	_, _, _, cornerA := img.At(0, 0).RGBA()
	if uint8(cornerA>>8) > 40 {
		t.Fatalf("expected transparent squircle corner (0,0), got alpha=%d", uint8(cornerA>>8))
	}

	var porcelainCount, inkCount int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(bl>>8), uint8(a>>8)
			if a8 > 200 {
				if r8 > 235 && g8 > 235 && b8 > 235 {
					porcelainCount++
				} else if r8 < 120 && g8 < 120 && b8 < 120 {
					inkCount++
				}
			}
		}
	}
	if porcelainCount < 20 || inkCount < 20 {
		t.Fatalf("expected high-contrast porcelain badge & crisp ink strokes, got porcelain=%d ink=%d", porcelainCount, inkCount)
	}
}

func TestEdgeHorizonFullScreenDragHelpers(t *testing.T) {
	wk := winRect{Left: 0, Top: 0, Right: 1920, Bottom: 1040}
	if got := computeEdgeDragWinX("left", wk, 214, 6, 600, 6, 220); got != 600 {
		t.Fatalf("left free drag winX = %d, want 600", got)
	}
	if got := computeEdgeDragWinX("left", wk, 214, 6, 5000, 6, 220); got != 1700 {
		t.Fatalf("left right-edge clamped winX = %d, want 1700", got)
	}
	if got := computeEdgeDragWinX("left", wk, 214, 6, -500, 6, 220); got != -6 {
		t.Fatalf("left left-edge clamped winX = %d, want -6", got)
	}
	if got := computeEdgeDragWinX("right", wk, 214, 1914, -900, 10, 224); got != 790 {
		t.Fatalf("right free drag winX = %d, want 790", got)
	}

	wk2 := winRect{Left: 1920, Top: 0, Right: 3840, Bottom: 1080}
	if got := computeEdgeDragWinX("left", wk2, 214, 6, 2500, 6, 220); got != 2500 {
		t.Fatalf("multi-monitor drag winX = %d, want 2500", got)
	}
}

func TestEdgeHorizonThermalSamplerAndFormatting(t *testing.T) {
	if got := unsafe.Sizeof(d3dkmtAdapterPerfData{}); got != 64 {
		t.Fatalf("expected D3DKMT_ADAPTER_PERFDATA size 64 on windows/amd64, got %d", got)
	}
	if got := unsafe.Sizeof(pdhFmtCounterValueItemW{}); got != 24 {
		t.Fatalf("expected PDH_FMT_COUNTERVALUE_ITEM_W size 24 on windows/amd64, got %d", got)
	}

	line, col := fmtEdgeTempLine(62.4, true, 48.2, true)
	if line != "CPU 62°C · GPU 48°C" || col != 0x00554133 {
		t.Fatalf("normal temp line = %q col=0x%08x", line, col)
	}
	lineWarm, colWarm := fmtEdgeTempLine(76.0, true, 55.0, true)
	if lineWarm != "CPU 76°C · GPU 55°C" || colWarm != 0x000953B4 {
		t.Fatalf("warm temp line = %q col=0x%08x", lineWarm, colWarm)
	}
	lineHot, colHot := fmtEdgeTempLine(65.0, true, 88.6, true)
	if lineHot != "CPU 65°C · GPU 89°C" || colHot != 0x002626DC {
		t.Fatalf("hot temp line = %q col=0x%08x", lineHot, colHot)
	}
	linePartial, colPartial := fmtEdgeTempLine(58.0, true, 0, false)
	if linePartial != "CPU 58°C · GPU --°C" || colPartial != 0x00554133 {
		t.Fatalf("partial temp line = %q col=0x%08x", linePartial, colPartial)
	}
	lineMissing, _ := fmtEdgeTempLine(0, false, 0, false)
	if lineMissing != "CPU --°C · GPU --°C" {
		t.Fatalf("missing temp line = %q", lineMissing)
	}

	ts := newEdgeThermalSampler()
	defer ts.Close()
	cpuC, hasCPU, gpuC, hasGPU := ts.Sample()
	if hasCPU && (cpuC < 10 || cpuC > 125) {
		t.Fatalf("CPU temperature out of plausible bounds: %.1fC", cpuC)
	}
	if hasGPU && (gpuC < 10 || gpuC > 125) {
		t.Fatalf("GPU temperature out of plausible bounds: %.1fC", gpuC)
	}
	t.Logf("thermal probe: CPU(has=%v %.1fC) GPU(has=%v %.1fC)", hasCPU, cpuC, hasGPU, gpuC)
}
