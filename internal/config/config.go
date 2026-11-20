// Package config holds MEMORA's on-disk settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Actions selects which management behaviours are allowed at all. A disabled
// action is never attempted, regardless of how bad the system looks.
type Actions struct {
	SmartMemorySchedule     bool `json:"smart_memory_schedule"`
	SoftMemoryPriority      bool `json:"soft_memory_priority"`
	EcoQoSBackground        bool `json:"eco_qos_background"`
	SuppressSpawnStorm      bool `json:"suppress_spawn_storm"`
	AutoLearnStormList      bool `json:"auto_learn_storm_list"`
	TrimIdleWorkingSets     bool `json:"trim_idle_working_sets"`
	PurgeStandby            bool `json:"purge_standby"`
	MitigateConflictDrivers bool `json:"mitigate_conflict_drivers"`
	EdgeHorizon             bool `json:"edge_horizon"`
}

// EdgeWidgetConfig configures the native edge-docked "贴边真相光尺 · Edge Horizon" HUD.
type EdgeWidgetConfig struct {
	Side     string  `json:"side"`      // "left" | "right"
	YPercent float64 `json:"y_percent"` // 0.04 .. 0.92
}

// Thresholds are the numbers the policy uses to decide.
type Thresholds struct {
	AvailableLowPercent    float64 `json:"available_low_percent"`
	TrimMinWorkingSetMB    float64 `json:"trim_min_working_set_mb"`
	SoftIdleMinutes        float64 `json:"soft_idle_minutes"`
	OrphanIdleMinutes      float64 `json:"orphan_idle_minutes"`
	IdleMinutes            float64 `json:"idle_minutes"`
	CooldownMinutes        float64 `json:"cooldown_minutes"`
	MaxActionsPerHour      int     `json:"max_actions_per_hour"`
	ReclaimBudgetMBPerHour float64 `json:"reclaim_budget_mb_per_hour"`
	GrowthWindowMinutes    float64 `json:"growth_window_minutes"`
	GrowthSignalMB         float64 `json:"growth_signal_mb"`
	StandbyPurgeMinMB      float64 `json:"standby_purge_min_mb"`
	QuietOnFullscreen      bool    `json:"quiet_on_fullscreen"`
}

// Config is the whole settings file.
type Config struct {
	Mode               string           `json:"mode"` // observe | guard
	Language           string           `json:"language,omitempty"` // "zh-CN" | "en-US"
	IntervalSeconds    int              `json:"interval_seconds"`
	Actions            Actions          `json:"actions"`
	Thresholds         Thresholds       `json:"thresholds"`
	EdgeWidget         EdgeWidgetConfig `json:"edge_widget"`
	Protected          []string         `json:"protected_processes"`
	Whitelist          []string         `json:"whitelist,omitempty"`
	SuppressedSpawners []string         `json:"suppressed_spawners,omitempty"`
}

// DefaultSuppressedSpawners returns the shipped default persistent spawner suppression list.
// Shipped empty so fresh installations never hardcode author/OEM-specific process assumptions;
// AutoLearnStormList and user rules populate it strictly from observed behavior on the host PC.
func DefaultSuppressedSpawners() []string {
	return []string{}
}

// Default returns the shipped defaults with continuous multi-tier smart scheduling.
func Default() Config {
	return Config{
		Mode:            "guard",
		Language:        "zh-CN",
		IntervalSeconds: 10,
		Actions: Actions{
			SmartMemorySchedule:     true,
			SoftMemoryPriority:      true,
			EcoQoSBackground:        true,
			SuppressSpawnStorm:      true,
			AutoLearnStormList:      true,
			TrimIdleWorkingSets:     true,
			PurgeStandby:            true,
			MitigateConflictDrivers: true,
			EdgeHorizon:             true,
		},
		Thresholds: Thresholds{
			AvailableLowPercent:    22,
			TrimMinWorkingSetMB:    35,
			SoftIdleMinutes:        2,
			OrphanIdleMinutes:      1,
			IdleMinutes:            4,
			CooldownMinutes:        2,
			MaxActionsPerHour:      30,
			ReclaimBudgetMBPerHour: 8192,
			GrowthWindowMinutes:    30,
			GrowthSignalMB:         400,
			StandbyPurgeMinMB:      1024,
			QuietOnFullscreen:      true,
		},
		EdgeWidget: EdgeWidgetConfig{
			Side:     "left",
			YPercent: 0.22,
		},
		Protected: []string{
			"system", "registry", "memory compression", "secure system",
			"vmmem", "vmmem.exe", "vmmemwsl", "vmmemwsl.exe", "vmcompute.exe", "vmwp.exe",
			"smss.exe", "csrss.exe", "wininit.exe", "winlogon.exe",
			"services.exe", "lsass.exe", "svchost.exe", "dwm.exe",
			"explorer.exe", "audiodg.exe", "msmpeng.exe", "securityhealthservice.exe",
			"nissrv.exe", "memora.exe",
		},
		Whitelist:          []string{},
		SuppressedSpawners: DefaultSuppressedSpawners(),
	}
}

// Dir is the per-user state directory.
func Dir() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return filepath.Join(v, "MEMORA")
	}
	if v, err := os.UserConfigDir(); err == nil {
		return filepath.Join(v, "MEMORA")
	}
	return "MEMORA"
}

// Path is the settings file location.
func Path() string { return filepath.Join(Dir(), "config.json") }

// Load reads the settings, creating the file with defaults on first run.
func Load() (Config, error) {
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := Default()
			if saveErr := Save(cfg); saveErr != nil {
				return cfg, fmt.Errorf("create default config: %w", saveErr)
			}
			return cfg, nil
		}
		return Default(), err
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", path, err)
	}
	migrated := sanitize(&cfg)
	if migrated {
		_ = Save(cfg)
	}
	return cfg, nil
}

func sanitize(cfg *Config) bool {
	changed := false
	if cfg.IntervalSeconds < 2 {
		cfg.IntervalSeconds = 2
		changed = true
	}
	if cfg.Mode != "observe" && cfg.Mode != "guard" {
		cfg.Mode = "observe"
		changed = true
	}
	switch strings.ToLower(cfg.Language) {
	case "en", "en-us", "english":
		if cfg.Language != "en-US" {
			cfg.Language = "en-US"
			changed = true
		}
	default:
		if cfg.Language != "zh-CN" {
			cfg.Language = "zh-CN"
			changed = true
		}
	}
	if cfg.EdgeWidget.Side != "left" && cfg.EdgeWidget.Side != "right" {
		cfg.EdgeWidget.Side = "left"
		changed = true
	}
	if cfg.EdgeWidget.YPercent <= 0 {
		cfg.EdgeWidget.YPercent = 0.22
		changed = true
	} else if cfg.EdgeWidget.YPercent < 0.04 {
		cfg.EdgeWidget.YPercent = 0.04
		changed = true
	} else if cfg.EdgeWidget.YPercent > 0.92 {
		cfg.EdgeWidget.YPercent = 0.92
		changed = true
	}
	if cfg.Whitelist == nil {
		cfg.Whitelist = []string{}
		changed = true
	} else {
		dedupedWL := dedupeLowerNames(cfg.Whitelist)
		if len(dedupedWL) != len(cfg.Whitelist) {
			cfg.Whitelist = dedupedWL
			changed = true
		}
	}
	if cfg.SuppressedSpawners == nil {
		cfg.SuppressedSpawners = DefaultSuppressedSpawners()
		changed = true
	} else {
		deduped := dedupeLowerNames(cfg.SuppressedSpawners)
		filtered := make([]string, 0, len(deduped))
		for _, item := range deduped {
			if !ShouldPruneSuppressedSpawner(item) {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) != len(cfg.SuppressedSpawners) {
			cfg.SuppressedSpawners = filtered
			changed = true
		}
	}
	// Migrate unmodified v0.1/v0.2 conservative shipped thresholds to v0.3 smart scheduling.
	t := &cfg.Thresholds
	if t.AvailableLowPercent == 12 &&
		t.TrimMinWorkingSetMB == 180 &&
		t.IdleMinutes == 20 &&
		t.MaxActionsPerHour == 4 {
		def := Default()
		cfg.Thresholds = def.Thresholds
		cfg.Actions.SmartMemorySchedule = true
		cfg.Actions.SuppressSpawnStorm = true
		cfg.Actions.AutoLearnStormList = true
		cfg.Actions.PurgeStandby = true
		return true
	}
	return changed
}

func dedupeLowerNames(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, s := range items {
		k := strings.ToLower(strings.TrimSpace(s))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

// ShouldPruneSuppressedSpawner reports whether an entry in SuppressedSpawners (or UI logs)
// should be pruned because it is protected or belongs to uninstalled Lenovo PC Manager.
func ShouldPruneSuppressedSpawner(name string) bool {
	if IsDisallowedStormTarget(name) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "lenovopcmanagerservice.exe", "lenovopcmanager.exe", "lnvsvcfdn.exe":
		return !hasLenovoPCManagerInstalled()
	default:
		return false
	}
}

// IsDisallowedStormTarget reports whether a process name belongs to protected OS daemons,
// developer CLI toolchains, IDEs, browsers, or temporary installers
// that should never be throttled or learned into storm suppression lists.
func IsDisallowedStormTarget(name string) bool {
	low := strings.ToLower(strings.TrimSpace(name))
	if low == "" {
		return true
	}
	if strings.Contains(low, "setup") || strings.Contains(low, "install") || strings.Contains(low, "unins") {
		return true
	}
	switch low {
	case "system", "registry", "memory compression", "secure system",
		"vmmem", "vmmem.exe", "vmmemwsl", "vmmemwsl.exe", "vmcompute.exe", "vmwp.exe",
		"wsl.exe", "wslservice.exe", "docker.exe", "docker desktop.exe", "com.docker.backend.exe", "com.docker.build.exe",
		"smss.exe", "csrss.exe", "wininit.exe", "winlogon.exe",
		"services.exe", "lsass.exe", "svchost.exe", "dwm.exe",
		"explorer.exe", "audiodg.exe", "msmpeng.exe", "securityhealthservice.exe",
		"nissrv.exe", "memora.exe", "hipsdaemon.exe",
		"msedge.exe", "msedgewebview2.exe", "chrome.exe", "chrome-headless-shell.exe", "firefox.exe", "electron.exe",
		"zcode.exe", "antigravity.exe", "code.exe", "devenv.exe", "omnicode.exe",
		"language_server.exe", "language_server_windows_x64.exe",
		"gopls.exe", "rust-analyzer.exe", "pyright.exe", "claude.exe", "codex.exe",
		"go.exe", "node.exe", "npm.exe", "pnpm.exe", "yarn.exe", "bun.exe", "deno.exe",
		"python.exe", "pythonw.exe", "pip.exe", "uv.exe",
		"git.exe", "bash.exe", "sh.exe", "zsh.exe", "pwsh.exe", "powershell.exe", "cmd.exe", "conhost.exe",
		"cargo.exe", "rustc.exe", "rustup.exe", "gcc.exe", "g++.exe", "clang.exe", "cl.exe", "link.exe",
		"dotnet.exe", "msbuild.exe", "java.exe", "javaw.exe", "javac.exe",
		"cmake.exe", "ninja.exe", "make.exe", "rg.exe", "fd.exe", "fzf.exe",
		"wezterm-gui.exe", "windowsterminal.exe":
		return true
	default:
		return false
	}
}

func hasLenovoPCManagerInstalled() bool {
	for _, p := range []string{
		`C:\Program Files (x86)\Lenovo\PCManager`,
		`C:\Program Files\Lenovo\PCManager`,
	} {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// Save writes the settings as indented JSON.
func Save(cfg Config) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0o644)
}
