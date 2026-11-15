package main

import (
	"bytes"
	"encoding/json"
	"memora/internal/config"
	"memora/internal/engine"
	"memora/internal/journal"
	"memora/internal/ledger"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func TestTruncateName(t *testing.T) {
	if got := truncateName("short.exe", 20); got != "short.exe" {
		t.Fatalf("unexpected short name: %q", got)
	}
	got := truncateName("Flydigi Space Station Plus Ultra.exe", 26)
	if len(got) != 26 || !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated length 26 with ..., got %q (len=%d)", got, len(got))
	}
}

func TestFormatTagNote(t *testing.T) {
	f := engine.PoolFinding{
		Tag:     "ConT",
		Desc:    "",
		Drivers: []string{"mtkwl6ex.sys"},
		Allocs:  100,
		Frees:   0,
	}
	got := formatTagNote(f)
	if !strings.Contains(got, "疑似驱动: mtkwl6ex.sys") || !strings.Contains(got, "0释放") {
		t.Fatalf("unexpected formatTagNote output: %q", got)
	}
}

func TestConclusionDetectsKernelLeakGrowth(t *testing.T) {
	deep := engine.DeepDiagnosis{
		Leak: engine.KernelLeakSummary{
			Suspected:      true,
			ProcessObjects: 180000,
		},
	}
	got := conclusion(0, 0, 0, 0, 0, 0, 10, 5, deep)
	if !strings.Contains(got, "180000") || !strings.Contains(got, "EPROCESS") {
		t.Fatalf("expected kernel leak conclusion, got: %q", got)
	}
}

func TestFormatLedgerTextAndNotifyStructSize(t *testing.T) {
	if got := unsafe.Sizeof(notifyIconData{}); got != 976 {
		t.Fatalf("expected NOTIFYICONDATAW amd64 size 976, got %d", got)
	}
	text := formatLedgerText(ledger.Data{
		StartedAt:         time.Now().Add(-48 * time.Hour),
		Sessions:          3,
		TotalGuardSeconds: 36000,
		TotalSamples:      100,
		RestrainedSamples: 98,
		FullscreenSkips:   12,
		SoftDowngrades:    15,
		SoftRestores:      14,
		TrimActions:       2,
		PeakSaves:         2,
		TotalReclaimedMB:  700,
		OrphansDetected:   4,
		DailyBaselines: []ledger.DailyBaseline{
			{Date: "2026-11-13", MinInUseMB: 10240, MinPrivateMB: 6144},
		},
	})
	for _, want := range []string{"10.0 小时", "98.0%", "全屏静音", "12 次", "4 个", "2026-11-13"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in ledger text, got:\n%s", want, text)
		}
	}
}

func TestEmbeddedTrayIconLoads(t *testing.T) {
	if len(embeddedTrayPNG) < 8 || string(embeddedTrayPNG[:4]) != "\x89PNG" {
		t.Fatalf("expected embedded PNG header, got len=%d", len(embeddedTrayPNG))
	}
	icon, owned := loadTrayIcon()
	if icon == 0 {
		t.Fatalf("expected non-zero HICON from loadTrayIcon")
	}
	if owned {
		_, _, _ = procDestroyIcon.Call(icon)
	}
}

func TestDashboardAuthAndAPIEndpoints(t *testing.T) {
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
	var engMu sync.Mutex
	var modeChanges []string

	dash, err := newDashboardServer(
		cfg,
		lg,
		func(fn func(*engine.Engine)) error {
			engMu.Lock()
			defer engMu.Unlock()
			fn(eng)
			return nil
		},
		func(mode string) {
			modeChanges = append(modeChanges, mode)
		},
	)
	if err != nil {
		t.Fatalf("newDashboardServer: %v", err)
	}
	defer dash.Close()

	a, err := eng.Step(engine.Options{Act: false})
	if err != nil {
		t.Fatalf("initial Step: %v", err)
	}
	dash.UpdateFromEngine(eng, a, true, false)

	base := dash.BaseURL()

	// 1. Missing token must be rejected with 403.
	resp, err := http.Get(base + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("GET /api/state without token = %d, want 403", resp.StatusCode)
	}

	respRootBad, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	respRootBad.Body.Close()
	if respRootBad.StatusCode != http.StatusForbidden {
		t.Fatalf("GET / without token = %d, want 403", respRootBad.StatusCode)
	}

	respRootOK, err := http.Get(dash.URL())
	if err != nil {
		t.Fatal(err)
	}
	respRootOK.Body.Close()
	if respRootOK.StatusCode != http.StatusOK {
		t.Fatalf("GET /?token=... = %d, want 200", respRootOK.StatusCode)
	}

	// 2. Authenticated GET /api/state returns coherent snapshot.
	req, _ := http.NewRequest(http.MethodGet, base+"/api/state", nil)
	req.Header.Set("X-Memora-Token", dash.token)
	resState, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var st dashboardState
	if err := json.NewDecoder(resState.Body).Decode(&st); err != nil {
		resState.Body.Close()
		t.Fatalf("decode /api/state: %v", err)
	}
	resState.Body.Close()
	if resState.StatusCode != http.StatusOK || st.Version != version || st.Breakdown.TotalBytes == 0 {
		t.Fatalf("unexpected /api/state status=%d version=%q total=%d", resState.StatusCode, st.Version, st.Breakdown.TotalBytes)
	}

	// 3. POST /api/toggle flips settings & invalid key yields 400.
	toggleCall := func(key string, enabled bool) (*http.Response, dashboardState) {
		body, _ := json.Marshal(toggleRequest{Key: key, Enabled: enabled})
		r, _ := http.NewRequest(http.MethodPost, base+"/api/toggle", bytes.NewReader(body))
		r.Header.Set("X-Memora-Token", dash.token)
		r.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatalf("POST /api/toggle(%s): %v", key, err)
		}
		var out dashboardState
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		return resp, out
	}

	rBad, _ := toggleCall("nonexistent_flag", false)
	if rBad.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown toggle status = %d, want 400", rBad.StatusCode)
	}
	rMode, stMode := toggleCall("mode", false)
	if rMode.StatusCode != http.StatusOK || stMode.Mode != "observe" || len(modeChanges) != 1 || modeChanges[0] != "observe" {
		t.Fatalf("mode toggle failed: code=%d mode=%s changes=%v", rMode.StatusCode, stMode.Mode, modeChanges)
	}
	rSmart, stSmart := toggleCall("smart_memory_schedule", false)
	if rSmart.StatusCode != http.StatusOK || stSmart.Config.Actions.SmartMemorySchedule {
		t.Fatalf("smart toggle failed: code=%d smart=%v", rSmart.StatusCode, stSmart.Config.Actions.SmartMemorySchedule)
	}
	_, _ = toggleCall("mode", true)
	_, _ = toggleCall("smart_memory_schedule", true)

	// 4. POST /api/optimize then POST /api/restore.
	reqOpt, _ := http.NewRequest(http.MethodPost, base+"/api/optimize", bytes.NewReader([]byte("{}")))
	reqOpt.Header.Set("X-Memora-Token", dash.token)
	resOpt, err := http.DefaultClient.Do(reqOpt)
	if err != nil {
		t.Fatal(err)
	}
	var optPayload struct {
		Result engine.OptimizeResult `json:"result"`
		State  dashboardState        `json:"state"`
	}
	if err := json.NewDecoder(resOpt.Body).Decode(&optPayload); err != nil {
		resOpt.Body.Close()
		t.Fatalf("decode /api/optimize: %v", err)
	}
	resOpt.Body.Close()
	if resOpt.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/optimize status = %d", resOpt.StatusCode)
	}

	reqRest, _ := http.NewRequest(http.MethodPost, base+"/api/restore", bytes.NewReader([]byte("{}")))
	reqRest.Header.Set("X-Memora-Token", dash.token)
	resRest, err := http.DefaultClient.Do(reqRest)
	if err != nil {
		t.Fatal(err)
	}
	var restPayload struct {
		OK    bool           `json:"ok"`
		State dashboardState `json:"state"`
	}
	_ = json.NewDecoder(resRest.Body).Decode(&restPayload)
	resRest.Body.Close()
	if resRest.StatusCode != http.StatusOK || !restPayload.OK || len(restPayload.State.ManagedProcesses) != 0 {
		t.Fatalf("POST /api/restore failed: code=%d ok=%v managed=%d", resRest.StatusCode, restPayload.OK, len(restPayload.State.ManagedProcesses))
	}
}
