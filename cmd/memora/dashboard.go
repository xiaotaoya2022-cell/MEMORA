package main

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"

	"memora/internal/config"
	"memora/internal/engine"
	"memora/internal/journal"
	"memora/internal/ledger"
	"memora/internal/proc"
	"memora/internal/winmem"
)

const maxHistoryPoints = 60

//go:embed memora.ico
var embeddedAppICO []byte

//go:embed memora_logo.png
var embeddedLogoPNG []byte

type historyPoint struct {
	Time        string  `json:"time"`
	InUsePct    float64 `json:"in_use_pct"`
	AvailGB     float64 `json:"avail_gb"`
	PrivateWSGB float64 `json:"private_ws_gb"`
	SpawnRate   float64 `json:"spawn_rate"`
	VidGB       float64 `json:"vid_gb"`
	ZombieGB    float64 `json:"zombie_gb"`
}

type assessmentDTO struct {
	At              time.Time              `json:"at"`
	TotalPhysBytes  uint64                 `json:"total_phys_bytes"`
	InUseBytes      uint64                 `json:"in_use_bytes"`
	AvailPhysBytes  uint64                 `json:"avail_phys_bytes"`
	InUsePct        float64                `json:"in_use_pct"`
	AvailPct        float64                `json:"avail_pct"`
	CommitBytes     uint64                 `json:"commit_bytes"`
	TotalCommit     uint64                 `json:"total_commit_bytes"`
	KernelTotal     uint64                 `json:"kernel_total_bytes"`
	KernelPaged     uint64                 `json:"kernel_paged_bytes"`
	KernelNonpaged  uint64                 `json:"kernel_nonpaged_bytes"`
	StandbyBytes    uint64                 `json:"standby_bytes"`
	ModifiedBytes   uint64                 `json:"modified_bytes"`
	FreeBytes       uint64                 `json:"free_bytes"`
	HasPages        bool                   `json:"has_pages"`
	WSBytes         uint64                 `json:"ws_bytes"`
	PrivateWSBytes  uint64                 `json:"private_ws_bytes"`
	PrivateBytes    uint64                 `json:"private_bytes"`
	ProcessCount    int                    `json:"process_count"`
	ProcessComplete bool                   `json:"process_complete"`
	ResidualBytes   int64                  `json:"residual_bytes"`
	InUseSlope      float64                `json:"in_use_slope"`
	CommitSlope     float64                `json:"commit_slope"`
	StandbySlope    float64                `json:"standby_slope"`
	Kind            string                 `json:"kind"`
	Severity        int                    `json:"severity"`
	FullscreenState uint32                 `json:"fullscreen_state"`
	FullscreenQuiet bool                   `json:"fullscreen_quiet"`
	FullscreenLabel string                 `json:"fullscreen_label"`
	FamilyProtected int                    `json:"family_protected"`
	SoftManaged     int                    `json:"soft_managed"`
	SmartTier             int                    `json:"smart_tier"`
	SmartLabel            string                 `json:"smart_label"`
	SpawnRatePerMin       float64                `json:"spawn_rate_per_min"`
	TopSpawners           []engine.SpawnerStat   `json:"top_spawners"`
	Orphans               []engine.OrphanProcess `json:"orphans"`
	AdaptiveCooldownProcs []string               `json:"adaptive_cooldown_procs,omitempty"`
}

type bloatCulpritDTO struct {
	Name             string  `json:"name"`
	TrimHits         int     `json:"hits"`
	MaxWorkingSetMB  float64 `json:"max_ws_mb"`
	TotalReclaimedMB float64 `json:"reclaimed_mb"`
	AdaptiveCooldown bool    `json:"adaptive_cooldown"`
}

type baselineComparisonDTO struct {
	DaysTracked      int     `json:"days_tracked"`
	FirstDate        string  `json:"first_date"`
	FirstMinInUseMB  float64 `json:"first_min_in_use_mb"`
	LatestDate       string  `json:"latest_date"`
	LatestMinInUseMB float64 `json:"latest_min_in_use_mb"`
	DriftMB          float64 `json:"drift_mb"`
	Stable           bool    `json:"stable"`
}

type ledgerDTO struct {
	Data          ledger.Data `json:"data"`
	RestraintRate float64     `json:"restraint_rate"`
	AvgReclaimMB  float64     `json:"avg_reclaim_mb"`
}

type dashboardState struct {
	Version            string                      `json:"version"`
	Mode               string                      `json:"mode"`
	IsAdmin            bool                        `json:"is_admin"`
	UpdatedAt          time.Time                   `json:"updated_at"`
	SelfWSBytes        uint64                      `json:"self_ws_bytes"`
	SelfHandles        uint32                      `json:"self_handles"`
	Config             config.Config               `json:"config"`
	Assessment         assessmentDTO               `json:"assessment"`
	Breakdown          engine.MemoryBreakdown      `json:"breakdown"`
	Deep               engine.DeepDiagnosis        `json:"deep"`
	ManagedProcesses   []engine.ManagedProcessInfo `json:"managed_processes"`
	RecentJournal      []journal.Entry             `json:"recent_journal"`
	Ledger             ledgerDTO                   `json:"ledger"`
	BloatCulprits      []bloatCulpritDTO           `json:"bloat_culprits"`
	BaselineComparison baselineComparisonDTO       `json:"baseline_comparison"`
	History            []historyPoint              `json:"history"`
}

type toggleRequest struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

// dashboardServer serves the loopback Control & Truth UI and JSON API.
type dashboardServer struct {
	token          string
	listener       net.Listener
	server         *http.Server
	runOnEngine    func(func(*engine.Engine)) error
	onModeChange   func(string)
	onConfigChange func(config.Config)
	lg             *ledger.Ledger

	mu         sync.RWMutex
	cfg        config.Config
	latest     engine.Assessment
	hasLatest  bool
	lastProcs  []proc.Info
	selfWS     uint64
	selfHandle uint32
	deep           engine.DeepDiagnosis
	hasDeep        bool
	lastDeepAt     time.Time
	breakdown      engine.MemoryBreakdown
	managed        []engine.ManagedProcessInfo
	recentJ        []journal.Entry
	culprits       []bloatCulpritDTO
	lastCulpritsAt time.Time
	history        []historyPoint
}

func newDashboardServer(
	cfg config.Config,
	lg *ledger.Ledger,
	runOnEngine func(func(*engine.Engine)) error,
	onModeChange func(string),
) (*dashboardServer, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("generate dashboard token: %w", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen loopback: %w", err)
	}

	d := &dashboardServer{
		token:        hex.EncodeToString(raw[:]),
		listener:     ln,
		runOnEngine:  runOnEngine,
		onModeChange: onModeChange,
		lg:           lg,
		cfg:          cfg,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", d.handleIndex)
	mux.HandleFunc("/favicon.ico", d.handleFavicon)
	mux.HandleFunc("/logo.png", d.handleLogoPNG)
	mux.HandleFunc("/manifest.json", d.handleManifest)
	mux.HandleFunc("/api/state", d.withAuth(d.handleState))
	mux.HandleFunc("/api/deep", d.withAuth(d.handleDeep))
	mux.HandleFunc("/api/toggle", d.withAuth(d.handleToggle))
	mux.HandleFunc("/api/optimize", d.withAuth(d.handleOptimize))
	mux.HandleFunc("/api/restore", d.withAuth(d.handleRestore))
	mux.HandleFunc("/api/suppress-storms", d.withAuth(d.handleSuppressStorms))
	mux.HandleFunc("/api/storm-list", d.withAuth(d.handleStormList))
	mux.HandleFunc("/api/whitelist", d.withAuth(d.handleWhitelist))
	mux.HandleFunc("/api/proc-icon", d.withAuth(d.handleProcIcon))
	mux.HandleFunc("/api/export-report", d.withAuth(d.handleExportReport))
	mux.HandleFunc("/api/window-ctl", d.withAuth(d.handleWindowCtl))
	mux.HandleFunc("/api/wsl-shutdown", d.withAuth(d.handleWSLShutdown))
	mux.HandleFunc("/api/wsl-reclaim", d.withAuth(d.handleWSLReclaim))
	mux.HandleFunc("/api/fix-kernel-leak", d.withAuth(d.handleFixKernelLeak))

	d.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = d.server.Serve(ln)
	}()
	return d, nil
}

func (d *dashboardServer) SetOnConfigChange(fn func(config.Config)) {
	d.mu.Lock()
	d.onConfigChange = fn
	d.mu.Unlock()
}

func (d *dashboardServer) URL() string {
	if d.listener == nil {
		return ""
	}
	return fmt.Sprintf("http://%s/?token=%s", d.listener.Addr().String(), d.token)
}

func (d *dashboardServer) BaseURL() string {
	if d.listener == nil {
		return ""
	}
	return fmt.Sprintf("http://%s", d.listener.Addr().String())
}

func (d *dashboardServer) Close() {
	closeDashboardWindow()
	if d.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = d.server.Shutdown(ctx)
	}
}

func (d *dashboardServer) OpenWindow() {
	u := d.URL()
	if u != "" {
		openDashboardWindow(u)
	}
}

// RecordSample stores raw process snapshot metadata received from Options.OnReading.
func (d *dashboardServer) RecordSample(s engine.Sample, selfWS uint64, selfHandles uint32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastProcs = s.Procs
	d.selfWS = selfWS
	d.selfHandle = selfHandles
}

// UpdateFromEngine refreshes the cached dashboard view after an engine step or mutation.
func (d *dashboardServer) UpdateFromEngine(eng *engine.Engine, a engine.Assessment, forceDeep bool, matchDrivers bool) {
	cfg := eng.ConfigSnapshot()
	managed := eng.ManagedProcesses()
	recentJ := eng.RecentJournal(30)

	d.mu.Lock()
	procs := d.lastProcs
	shouldDeep := forceDeep || !d.hasDeep || time.Since(d.lastDeepAt) >= 15*time.Second
	shouldCulprits := forceDeep || d.lastCulpritsAt.IsZero() || time.Since(d.lastCulpritsAt) >= 10*time.Second
	d.mu.Unlock()

	if len(procs) == 0 && shouldDeep {
		procs, _, _ = proc.ListAll()
	}

	var deep engine.DeepDiagnosis
	var bd engine.MemoryBreakdown
	if shouldDeep {
		deep = engine.InspectDeep(a.Snap, procs, matchDrivers)
		bd = eng.ComputeAdaptiveBreakdown(a.Snap, a.PrivateWS, procs, deep)
	}

	var culprits []bloatCulpritDTO
	if shouldCulprits {
		entries, _ := journal.Tail(filepath.Join(config.Dir(), "journal.jsonl"), 400)
		_, culprits = aggregateJournalTargets(entries, a.AdaptiveCooldownProcs, 5)
	}

	d.mu.Lock()
	d.cfg = cfg
	d.latest = a
	d.hasLatest = true
	d.managed = managed
	d.recentJ = recentJ
	if shouldCulprits {
		d.culprits = culprits
		d.lastCulpritsAt = time.Now()
	}
	if shouldDeep {
		d.deep = deep
		d.hasDeep = true
		d.lastDeepAt = time.Now()
		d.breakdown = bd
	} else {
		d.breakdown = eng.ComputeAdaptiveBreakdown(a.Snap, a.PrivateWS, procs, d.deep)
	}
	finalBD := d.breakdown
	cb := d.onConfigChange
	d.appendHistoryLocked(a, finalBD)
	d.mu.Unlock()

	if cb != nil {
		cb(cfg)
	}
	updateEdgeHorizonState(cfg, a, finalBD, true)
	maybeHibernateHiddenDashboard(a.AvailPct)
}

func (d *dashboardServer) appendHistoryLocked(a engine.Assessment, bd engine.MemoryBreakdown) {
	at := a.At
	if at.IsZero() {
		at = time.Now()
	}
	pt := historyPoint{
		Time:        at.Format("15:04:05"),
		InUsePct:    100 - a.AvailPct,
		AvailGB:     float64(a.Snap.AvailPhys) / (1 << 30),
		PrivateWSGB: float64(a.PrivateWS) / (1 << 30),
		SpawnRate:   a.SpawnRatePerMin,
		VidGB:       float64(bd.VidBytes) / (1 << 30),
		ZombieGB:    float64(bd.ZombieLeakBytes) / (1 << 30),
	}
	if len(d.history) >= maxHistoryPoints {
		copy(d.history, d.history[1:])
		d.history[maxHistoryPoints-1] = pt
	} else {
		d.history = append(d.history, pt)
	}
}

func (d *dashboardServer) snapshotState() dashboardState {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var lData ledger.Data
	if d.lg != nil {
		lData = d.lg.Snapshot()
	}
	deepCopy := d.deep
	if len(deepCopy.ProcGroups) > 20 {
		deepCopy.ProcGroups = deepCopy.ProcGroups[:20]
	}
	histCopy := make([]historyPoint, len(d.history))
	copy(histCopy, d.history)
	culpritCopy := make([]bloatCulpritDTO, len(d.culprits))
	copy(culpritCopy, d.culprits)

	var baseCmp baselineComparisonDTO
	if len(lData.DailyBaselines) > 0 {
		win := lData.DailyBaselines
		if len(win) > 7 {
			win = win[len(win)-7:]
		}
		first := win[0]
		latest := win[len(win)-1]
		drift := latest.MinInUseMB - first.MinInUseMB
		baseCmp = baselineComparisonDTO{
			DaysTracked:      len(win),
			FirstDate:        first.Date,
			FirstMinInUseMB:  first.MinInUseMB,
			LatestDate:       latest.Date,
			LatestMinInUseMB: latest.MinInUseMB,
			DriftMB:          drift,
			Stable:           drift <= 512,
		}
	}

	return dashboardState{
		Version:          version,
		Mode:             d.cfg.Mode,
		IsAdmin:          winmem.IsAdmin(),
		UpdatedAt:        d.latest.At,
		SelfWSBytes:      d.selfWS,
		SelfHandles:      d.selfHandle,
		Config:           d.cfg,
		Assessment:       toAssessmentDTO(d.latest),
		Breakdown:        d.breakdown,
		Deep:             deepCopy,
		ManagedProcesses: d.managed,
		RecentJournal:    d.recentJ,
		Ledger: ledgerDTO{
			Data:          lData,
			RestraintRate: lData.RestraintRate(),
			AvgReclaimMB:  lData.AvgReclaimMB(),
		},
		BloatCulprits:      culpritCopy,
		BaselineComparison: baseCmp,
		History:            histCopy,
	}
}

func toAssessmentDTO(a engine.Assessment) assessmentDTO {
	inUsePct := 0.0
	if a.Snap.TotalPhys > 0 {
		inUsePct = 100 - a.AvailPct
	}
	return assessmentDTO{
		At:              a.At,
		TotalPhysBytes:  a.Snap.TotalPhys,
		InUseBytes:      a.Snap.InUse(),
		AvailPhysBytes:  a.Snap.AvailPhys,
		InUsePct:        inUsePct,
		AvailPct:        a.AvailPct,
		CommitBytes:     a.Snap.Commit,
		TotalCommit:     a.Snap.TotalCommit,
		KernelTotal:     a.Snap.KernelTotal,
		KernelPaged:     a.Snap.KernelPaged,
		KernelNonpaged:  a.Snap.KernelNonpaged,
		StandbyBytes:    a.Snap.Pages.Standby,
		ModifiedBytes:   a.Snap.Pages.Modified,
		FreeBytes:       a.Snap.Pages.Free,
		HasPages:        a.Snap.HasPages,
		WSBytes:         a.WS,
		PrivateWSBytes:  a.PrivateWS,
		PrivateBytes:    a.Private,
		ProcessCount:    a.ProcessCount,
		ProcessComplete: a.ProcessComplete,
		ResidualBytes:   a.Residual,
		InUseSlope:      a.InUseSlope,
		CommitSlope:     a.CommitSlope,
		StandbySlope:    a.StandbySlope,
		Kind:            a.Kind,
		Severity:        a.Severity,
		FullscreenState: a.FullscreenState,
		FullscreenQuiet: a.FullscreenQuiet,
		FullscreenLabel: a.FullscreenLabel,
		FamilyProtected: a.FamilyProtected,
		SoftManaged:     a.SoftManaged,
		SmartTier:             a.SmartTier,
		SmartLabel:            a.SmartLabel,
		SpawnRatePerMin:       a.SpawnRatePerMin,
		TopSpawners:           a.TopSpawners,
		Orphans:               a.Orphans,
		AdaptiveCooldownProcs: a.AdaptiveCooldownProcs,
	}
}

func (d *dashboardServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Memora-Token") != d.token {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (d *dashboardServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("token") != d.token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(strings.Replace(dashboardHTML, "__MEMORA_TOKEN__", d.token, 1)))
}

func (d *dashboardServer) handleFavicon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/x-icon")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(embeddedAppICO)
}

func (d *dashboardServer) handleLogoPNG(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(embeddedLogoPNG)
}

func (d *dashboardServer) handleManifest(w http.ResponseWriter, r *http.Request) {
	manifest := map[string]any{
		"name":             "MEMORA · 智能性能调度与内存真相中心",
		"short_name":       "MEMORA",
		"display":          "standalone",
		"background_color": "#0f172a",
		"theme_color":      "#0f172a",
		"icons": []map[string]any{
			{"src": "/logo.png", "sizes": "256x256", "type": "image/png"},
			{"src": "/favicon.ico", "sizes": "any", "type": "image/x-icon"},
		},
	}
	writeJSON(w, http.StatusOK, manifest)
}

func (d *dashboardServer) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, d.snapshotState())
}

func (d *dashboardServer) handleDeep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap, err := winmem.Collect()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	procs, complete, _ := proc.ListAll()
	var privWS, ws, priv uint64
	for _, p := range procs {
		privWS += p.PrivateWS
		ws += p.WorkingSet
		priv += p.Private
	}
	selfWS, selfHandles := selfStats(procs)
	deep := engine.InspectDeep(snap, procs, true)
	bd := d.computeBreakdown(snap, privWS, procs, deep)

	d.mu.Lock()
	d.lastProcs = procs
	d.selfWS = selfWS
	d.selfHandle = selfHandles
	d.deep = deep
	d.hasDeep = true
	d.lastDeepAt = time.Now()
	d.breakdown = bd
	d.latest.Snap = snap
	d.latest.AvailPct = snap.AvailablePercent()
	d.latest.WS = ws
	d.latest.PrivateWS = privWS
	d.latest.Private = priv
	d.latest.ProcessCount = len(procs)
	d.latest.ProcessComplete = complete
	d.mu.Unlock()

	writeJSON(w, http.StatusOK, d.snapshotState())
}

func (d *dashboardServer) handleToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req toggleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var valid = true
	var nextMode string
	var saveErr error
	err := d.runOnEngine(func(eng *engine.Engine) {
		saveErr = eng.UpdateConfig(func(c *config.Config) {
			switch req.Key {
			case "mode":
				if req.Enabled {
					c.Mode = "guard"
				} else {
					c.Mode = "observe"
				}
				nextMode = c.Mode
			case "smart_memory_schedule":
				c.Actions.SmartMemorySchedule = req.Enabled
			case "soft_memory_priority":
				c.Actions.SoftMemoryPriority = req.Enabled
			case "eco_qos_background":
				c.Actions.EcoQoSBackground = req.Enabled
			case "suppress_spawn_storm":
				c.Actions.SuppressSpawnStorm = req.Enabled
			case "auto_learn_storm_list":
				c.Actions.AutoLearnStormList = req.Enabled
			case "purge_standby":
				c.Actions.PurgeStandby = req.Enabled
			case "quiet_on_fullscreen":
				c.Thresholds.QuietOnFullscreen = req.Enabled
			case "edge_horizon":
				c.Actions.EdgeHorizon = req.Enabled
			default:
				valid = false
			}
		})
		if valid && saveErr == nil {
			if req.Key == "mode" && req.Enabled {
				if a, stepErr := eng.Step(engine.Options{Act: true}); stepErr == nil {
					a.SoftManaged = len(eng.ManagedProcesses())
					d.UpdateFromEngine(eng, a, false, false)
					return
				}
			}
			d.mu.RLock()
			a := d.latest
			hasL := d.hasLatest
			d.mu.RUnlock()
			if hasL {
				a.SoftManaged = len(eng.ManagedProcesses())
				d.UpdateFromEngine(eng, a, false, false)
			} else {
				cfgSnap := eng.ConfigSnapshot()
				d.mu.Lock()
				d.cfg = cfgSnap
				cb := d.onConfigChange
				d.mu.Unlock()
				if cb != nil {
					cb(cfgSnap)
				}
				updateEdgeHorizonState(cfgSnap, engine.Assessment{}, engine.MemoryBreakdown{}, false)
			}
		}
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown toggle key"})
		return
	}
	if saveErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": saveErr.Error()})
		return
	}
	if req.Key == "mode" && d.onModeChange != nil {
		d.onModeChange(nextMode)
	}
	writeJSON(w, http.StatusOK, d.snapshotState())
}

func (d *dashboardServer) handleOptimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var res engine.OptimizeResult
	var opErr error
	err := d.runOnEngine(func(eng *engine.Engine) {
		res, opErr = eng.OptimizeNow()
		if opErr != nil {
			return
		}
		if a, stepErr := eng.Step(engine.Options{Act: false}); stepErr == nil {
			d.UpdateFromEngine(eng, a, true, false)
		}
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if opErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": opErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"result": res,
		"state":  d.snapshotState(),
	})
}

func (d *dashboardServer) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	err := d.runOnEngine(func(eng *engine.Engine) {
		eng.RestoreAll()
		d.mu.RLock()
		a := d.latest
		d.mu.RUnlock()
		a.SoftManaged = 0
		d.UpdateFromEngine(eng, a, false, false)
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"state": d.snapshotState(),
	})
}

func (d *dashboardServer) handleSuppressStorms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var newly, total int
	err := d.runOnEngine(func(eng *engine.Engine) {
		_ = eng.UpdateConfig(func(c *config.Config) {
			c.Actions.SuppressSpawnStorm = true
			c.Actions.EcoQoSBackground = true
		})
		newly, total = eng.SuppressActiveSpawnersNow()
		d.mu.RLock()
		a := d.latest
		d.mu.RUnlock()
		a.SoftManaged = len(eng.ManagedProcesses())
		d.UpdateFromEngine(eng, a, false, false)
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"throttled_count": newly,
		"total_throttled": total,
		"state":           d.snapshotState(),
	})
}

func (d *dashboardServer) computeBreakdown(snap winmem.Snapshot, privWS uint64, procs []proc.Info, deep engine.DeepDiagnosis) engine.MemoryBreakdown {
	bd := engine.ComputeBreakdown(snap, privWS, deep)
	if d.runOnEngine != nil {
		_ = d.runOnEngine(func(eng *engine.Engine) {
			bd = eng.ComputeAdaptiveBreakdown(snap, privWS, procs, deep)
		})
	}
	return bd
}

func (d *dashboardServer) refreshSnapshotNow(matchDrivers bool) (winmem.Snapshot, winmem.SystemExtras, bool) {
	snap, err := winmem.Collect()
	if err != nil {
		return snap, winmem.SystemExtras{}, false
	}
	procs, complete, _ := proc.ListAll()
	var privWS, ws, priv uint64
	for _, p := range procs {
		privWS += p.PrivateWS
		ws += p.WorkingSet
		priv += p.Private
	}
	selfWS, selfHandles := selfStats(procs)
	deep := engine.InspectDeep(snap, procs, matchDrivers)
	bd := d.computeBreakdown(snap, privWS, procs, deep)

	d.mu.Lock()
	d.lastProcs = procs
	d.selfWS = selfWS
	d.selfHandle = selfHandles
	d.deep = deep
	d.hasDeep = true
	d.lastDeepAt = time.Now()
	d.breakdown = bd
	d.latest.Snap = snap
	d.latest.AvailPct = snap.AvailablePercent()
	d.latest.WS = ws
	d.latest.PrivateWS = privWS
	d.latest.Private = priv
	d.latest.ProcessCount = len(procs)
	d.latest.ProcessComplete = complete
	d.appendHistoryLocked(d.latest, bd)
	d.mu.Unlock()

	return snap, winmem.ReadSystemExtras(snap.PageSize), true
}

func (d *dashboardServer) handleWSLShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	beforeSnap, _ := winmem.Collect()
	beforeExtras := winmem.ReadSystemExtras(beforeSnap.PageSize)

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", "--shutdown")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	out, cmdErr := cmd.CombinedOutput()
	if cmdErr == nil {
		time.Sleep(1200 * time.Millisecond)
	}

	afterSnap, afterExtras, ok := d.refreshSnapshotNow(false)
	// Fallback via elevated UAC when non-admin execution cannot terminate Hyper-V/WSL2 Vid.
	if !winmem.IsAdmin() && (cmdErr != nil || (ok && afterExtras.VidBytes > 512<<20)) {
		elevCmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command",
			"$p = Start-Process -FilePath 'wsl.exe' -ArgumentList '--shutdown' -Verb RunAs -WindowStyle Hidden -Wait -PassThru; exit $p.ExitCode")
		elevCmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000,
		}
		elevOut, elevErr := elevCmd.CombinedOutput()
		if len(elevOut) > 0 {
			out = elevOut
		}
		cmdErr = elevErr
		if cmdErr == nil {
			time.Sleep(1200 * time.Millisecond)
			afterSnap, afterExtras, _ = d.refreshSnapshotNow(false)
		}
	}

	wslConfigUpdated := ensureWSLAutoReclaimGradual()

	availGainMB := 0.0
	if afterSnap.AvailPhys > beforeSnap.AvailPhys {
		availGainMB = float64(afterSnap.AvailPhys-beforeSnap.AvailPhys) / (1 << 20)
	}
	vidReleasedMB := 0.0
	if beforeExtras.VidBytes > afterExtras.VidBytes {
		vidReleasedMB = float64(beforeExtras.VidBytes-afterExtras.VidBytes) / (1 << 20)
	}

	res := map[string]any{
		"ok":                cmdErr == nil,
		"output":            strings.TrimSpace(string(out)),
		"wslconfig_updated": wslConfigUpdated,
		"before_avail_mb":   float64(beforeSnap.AvailPhys) / (1 << 20),
		"after_avail_mb":    float64(afterSnap.AvailPhys) / (1 << 20),
		"avail_gain_mb":     availGainMB,
		"vid_released_mb":   vidReleasedMB,
		"state":             d.snapshotState(),
	}
	if cmdErr != nil {
		res["error"] = cmdErr.Error()
	}
	writeJSON(w, http.StatusOK, res)
}

func (d *dashboardServer) handleWSLReclaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	beforeSnap, _ := winmem.Collect()
	beforeExtras := winmem.ReadSystemExtras(beforeSnap.PageSize)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", "--user", "root", "sh", "-c", "sync; echo 3 > /proc/sys/vm/drop_caches")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	out, cmdErr := cmd.CombinedOutput()
	if cmdErr == nil {
		time.Sleep(800 * time.Millisecond)
	}
	afterSnap, afterExtras, _ := d.refreshSnapshotNow(false)
	vidReleasedMB := 0.0
	if beforeExtras.VidBytes > afterExtras.VidBytes {
		vidReleasedMB = float64(beforeExtras.VidBytes-afterExtras.VidBytes) / (1 << 20)
	}
	availGainMB := 0.0
	if afterSnap.AvailPhys > beforeSnap.AvailPhys {
		availGainMB = float64(afterSnap.AvailPhys-beforeSnap.AvailPhys) / (1 << 20)
	}
	res := map[string]any{
		"ok":              cmdErr == nil,
		"output":          strings.TrimSpace(string(out)),
		"vid_released_mb": vidReleasedMB,
		"avail_gain_mb":   availGainMB,
		"state":           d.snapshotState(),
	}
	if cmdErr != nil {
		res["error"] = cmdErr.Error()
	}
	writeJSON(w, http.StatusOK, res)
}

func mitigateConflictDrivers(ctx context.Context, elevateIfNeeded bool) (string, error) {
	// Unload minifilter drivers (fltmc unload) and stop/disable Lenovo conflict services.
	// Even when lrtp.sys protects HKLM\...\Start=1 across reboots, runtime mitigation
	// unloads lnvscenter and stops LnvSvcFdn / LenovoPcManagerService cleanly on startup.
	const script = `
$ErrorActionPreference = 'Continue'
$actions = New-Object System.Collections.Generic.List[string]
foreach ($svc in @('lnvscenter', 'lrtp', 'netrtp')) {
  $q = & sc.exe query $svc 2>$null
  if ($LASTEXITCODE -eq 0) {
    $qText = $q -join ' '
    if ($qText -match 'RUNNING') {
      & fltmc.exe unload $svc *> $null
      if ($LASTEXITCODE -eq 0) {
        $actions.Add("已卸载内核过滤驱动 $svc")
      } else {
        & sc.exe stop $svc *> $null
        if ($LASTEXITCODE -eq 0) {
          $actions.Add("已停止内核驱动 $svc")
        }
      }
    }
    & sc.exe config $svc start= disabled *> $null
    $regPath = "HKLM\SYSTEM\CurrentControlSet\Services\$svc"
    & reg.exe add $regPath /v Start /t REG_DWORD /d 4 /f *> $null
    if ($LASTEXITCODE -eq 0) {
      $actions.Add("$svc 已写入禁用自启")
    }
  }
}
foreach ($svc in @('LnvSvcFdn', 'LenovoPcManagerService')) {
  $q = & sc.exe query $svc 2>$null
  if ($LASTEXITCODE -eq 0) {
    $qText = $q -join ' '
    if ($qText -match 'RUNNING') {
      & sc.exe stop $svc *> $null
      if ($LASTEXITCODE -eq 0) {
        $actions.Add("已停止关联宿主服务 $svc")
      }
    }
    & sc.exe config $svc start= demand *> $null
    if ($LASTEXITCODE -eq 0) {
      $actions.Add("$svc 已改为按需(demand)")
    }
  }
}
foreach ($pname in @('LenovoPcManagerService', 'LenovoPcManager', 'LnvSvcFdn')) {
  $ps = Get-Process -Name $pname -ErrorAction SilentlyContinue
  if ($null -ne $ps) {
    $ps | Stop-Process -Force -ErrorAction SilentlyContinue
    $actions.Add("已清理残留进程 $pname")
  }
}
$esProcs = Get-Process -Name 'ESurfingSvr' -ErrorAction SilentlyContinue
$maxHandles = 0
if ($null -ne $esProcs) {
  $maxHandles = ($esProcs | Measure-Object -Property Handles -Maximum).Maximum
}
if ($maxHandles -gt 5000) {
  & sc.exe query 'ESurfingSvr' *> $null
  if ($LASTEXITCODE -eq 0) {
    & sc.exe stop 'ESurfingSvr' *> $null
    for ($i = 0; $i -lt 20; $i++) {
      Start-Sleep -Milliseconds 200
      $st = (& sc.exe query 'ESurfingSvr' 2>$null) -join ' '
      if ($st -match 'STOPPED') { break }
    }
    & sc.exe start 'ESurfingSvr' *> $null
    $actions.Add("已重启高句柄宿主 ESurfingSvr (原 ${maxHandles} 句柄)")
  }
}
if ($actions.Count -eq 0) {
  Write-Output "已完成冲突驱动卸载与服务校验（无正在运行的冲突项）"
} else {
  Write-Output ($actions -join '；')
}
`
	out, cmdErr := runHiddenPowerShell(ctx, script, elevateIfNeeded)
	engine.ResetDriverScanCache()
	return strings.TrimSpace(string(out)), cmdErr
}

func (d *dashboardServer) handleFixKernelLeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	_ = d.runOnEngine(func(eng *engine.Engine) {
		_ = eng.UpdateConfig(func(c *config.Config) {
			c.Actions.MitigateConflictDrivers = true
		})
	})

	msg, cmdErr := mitigateConflictDrivers(ctx, !winmem.IsAdmin())
	_, _, _ = d.refreshSnapshotNow(true)

	if cmdErr != nil {
		if msg == "" {
			msg = "冲突服务修复未能完成（若弹出 UAC 请选择允许）: " + cmdErr.Error()
		}
	} else if msg == "" {
		msg = "已运行时卸载冲突过滤驱动并停止/降档联想后台服务（已在 config.json 启用每次开机自动静默压制）"
	} else {
		msg = msg + "（已启用重启后开机自动静默压制）"
	}

	res := map[string]any{
		"ok":      cmdErr == nil,
		"message": msg,
		"output":  msg,
		"state":   d.snapshotState(),
	}
	if cmdErr != nil {
		res["error"] = cmdErr.Error()
	}
	writeJSON(w, http.StatusOK, res)
}

func runHiddenPowerShell(ctx context.Context, script string, elevate bool) ([]byte, error) {
	encoded := encodePowerShellCommand(script)
	if !elevate {
		cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", encoded)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000,
		}
		return cmd.CombinedOutput()
	}
	wrapper := fmt.Sprintf("$p = Start-Process -FilePath 'powershell.exe' -ArgumentList @('-NoProfile','-NonInteractive','-WindowStyle','Hidden','-EncodedCommand','%s') -Verb RunAs -Wait -PassThru; exit $p.ExitCode", encoded)
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", wrapper)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	return cmd.CombinedOutput()
}

func encodePowerShellCommand(script string) string {
	u16 := utf16.Encode([]rune(script))
	buf := make([]byte, len(u16)*2)
	for i, v := range u16 {
		buf[i*2] = byte(v)
		buf[i*2+1] = byte(v >> 8)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func ensureWSLAutoReclaimGradual() bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	cfgPath := filepath.Join(home, ".wslconfig")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return false
	}
	text := string(raw)
	lines := strings.Split(text, "\n")
	changed := false
	for i, line := range lines {
		clean := strings.TrimRight(line, "\r")
		trimmed := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(clean)), " ", "")
		if trimmed == "automemoryreclaim=disabled" {
			suffix := ""
			if strings.HasSuffix(line, "\r") {
				suffix = "\r"
			}
			lines[i] = "autoMemoryReclaim=gradual" + suffix
			changed = true
		}
	}
	if !changed {
		return false
	}
	return os.WriteFile(cfgPath, []byte(strings.Join(lines, "\n")), 0o644) == nil
}

type stormListRequest struct {
	Action string `json:"action"`
	Name   string `json:"name"`
}

func (d *dashboardServer) handleStormList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req stormListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ok := false
	err := d.runOnEngine(func(eng *engine.Engine) {
		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "add":
			ok = eng.AddSuppressedSpawner(req.Name)
		case "remove":
			ok = eng.RemoveSuppressedSpawner(req.Name)
		}
		d.mu.RLock()
		a := d.latest
		d.mu.RUnlock()
		a.SoftManaged = len(eng.ManagedProcesses())
		d.UpdateFromEngine(eng, a, false, false)
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    ok,
		"state": d.snapshotState(),
	})
}

func (d *dashboardServer) handleWhitelist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req stormListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ok := false
	err := d.runOnEngine(func(eng *engine.Engine) {
		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "add":
			ok = eng.AddWhitelist(req.Name)
		case "remove":
			ok = eng.RemoveWhitelist(req.Name)
		}
		d.mu.RLock()
		a := d.latest
		d.mu.RUnlock()
		a.SoftManaged = len(eng.ManagedProcesses())
		d.UpdateFromEngine(eng, a, false, false)
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    ok,
		"state": d.snapshotState(),
	})
}

func aggregateJournalTargets(entries []journal.Entry, adaptiveProcs []string, topN int) (map[string]*bloatCulpritDTO, []bloatCulpritDTO) {
	adaptiveSet := make(map[string]bool, len(adaptiveProcs))
	for _, n := range adaptiveProcs {
		adaptiveSet[strings.ToLower(strings.TrimSpace(n))] = true
	}
	procMap := map[string]*bloatCulpritDTO{}
	for _, e := range entries {
		for _, tgt := range e.Targets {
			k := strings.ToLower(strings.TrimSpace(tgt.Name))
			if k == "" {
				continue
			}
			ps, exists := procMap[k]
			if !exists {
				ps = &bloatCulpritDTO{
					Name:             tgt.Name,
					AdaptiveCooldown: adaptiveSet[k],
				}
				procMap[k] = ps
			}
			ps.TrimHits++
			if tgt.WorkingSetMB > ps.MaxWorkingSetMB {
				ps.MaxWorkingSetMB = tgt.WorkingSetMB
			}
			if len(e.Targets) > 0 && e.ReclaimedMB > 0 {
				ps.TotalReclaimedMB += e.ReclaimedMB / float64(len(e.Targets))
			}
		}
	}
	list := make([]bloatCulpritDTO, 0, len(procMap))
	for _, ps := range procMap {
		list = append(list, *ps)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].TotalReclaimedMB != list[j].TotalReclaimedMB {
			return list[i].TotalReclaimedMB > list[j].TotalReclaimedMB
		}
		if list[i].TrimHits != list[j].TrimHits {
			return list[i].TrimHits > list[j].TrimHits
		}
		return list[i].MaxWorkingSetMB > list[j].MaxWorkingSetMB
	})
	if topN > 0 && len(list) > topN {
		list = list[:topN]
	}
	return procMap, list
}

func reportExportDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dl := filepath.Join(home, "Downloads")
		if info, err := os.Stat(dl); err == nil && info.IsDir() {
			return dl
		}
	}
	dir := filepath.Join(config.Dir(), "reports")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func (d *dashboardServer) handleExportReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "open" {
		target := filepath.Clean(strings.TrimSpace(r.URL.Query().Get("path")))
		expDir := filepath.Clean(reportExportDir())
		if target == "" || !strings.HasPrefix(strings.ToLower(target), strings.ToLower(expDir)) || !strings.HasSuffix(strings.ToLower(target), ".json") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid report path"})
			return
		}
		cmd := exec.Command("explorer.exe", "/select,"+target)
		_ = cmd.Start()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	state := d.snapshotState()
	entries, _ := journal.Tail(filepath.Join(config.Dir(), "journal.jsonl"), 500)
	byAction := map[string]int{}
	for _, e := range entries {
		key := e.Kind
		if e.Action != "" {
			key += ":" + e.Action
		}
		byAction[key]++
	}
	procMap, _ := aggregateJournalTargets(entries, state.Assessment.AdaptiveCooldownProcs, 0)

	report := map[string]any{
		"exported_at":    time.Now(),
		"version":        version,
		"config_dir":     config.Dir(),
		"current_state":  state,
		"action_counts":  byAction,
		"target_summary": procMap,
		"journal_count":  len(entries),
		"journal":        entries,
	}
	fname := fmt.Sprintf("memora-diagnostic-report-%s.json", time.Now().Format("20060102-150405"))
	if mode == "save" || r.Method == http.MethodPost {
		raw, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		fullPath := filepath.Join(reportExportDir(), fname)
		if err := os.WriteFile(fullPath, raw, 0o644); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"filename":   fname,
			"path":       fullPath,
			"size_bytes": len(raw),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fname))
	w.Header().Set("Cache-Control", "no-store")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(report)
}

func (d *dashboardServer) handleWindowCtl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ok := controlDashboardWindow(req.Action)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        ok,
		"maximized": isDashboardWindowMaximized(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
