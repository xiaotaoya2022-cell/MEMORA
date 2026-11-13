// Package config holds MEMORA's on-disk settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Actions selects which management behaviours are allowed at all. A disabled
// action is never attempted, regardless of how bad the system looks.
type Actions struct {
	TrimIdleWorkingSets bool `json:"trim_idle_working_sets"`
	PurgeStandby        bool `json:"purge_standby"`
}

// Thresholds are the numbers the policy uses to decide. They are deliberately
// conservative; lowering them is how a user turns MEMORA into a cleaner, and
// that is their explicit choice, not a default.
type Thresholds struct {
	AvailableLowPercent    float64 `json:"available_low_percent"`
	TrimMinWorkingSetMB    float64 `json:"trim_min_working_set_mb"`
	IdleMinutes            float64 `json:"idle_minutes"`
	CooldownMinutes        float64 `json:"cooldown_minutes"`
	MaxActionsPerHour      int     `json:"max_actions_per_hour"`
	ReclaimBudgetMBPerHour float64 `json:"reclaim_budget_mb_per_hour"`
	GrowthWindowMinutes    float64 `json:"growth_window_minutes"`
	GrowthSignalMB         float64 `json:"growth_signal_mb"`
	StandbyPurgeMinMB      float64 `json:"standby_purge_min_mb"`
}

// Config is the whole settings file.
type Config struct {
	Mode            string     `json:"mode"` // observe | guard
	IntervalSeconds int        `json:"interval_seconds"`
	Actions         Actions    `json:"actions"`
	Thresholds      Thresholds `json:"thresholds"`
	Protected       []string   `json:"protected_processes"`
}

// Default returns the shipped defaults. Guard mode is on, but only the
// least destructive action is enabled, and only under real pressure.
func Default() Config {
	return Config{
		Mode:            "guard",
		IntervalSeconds: 10,
		Actions: Actions{
			TrimIdleWorkingSets: true,
			PurgeStandby:        false,
		},
		Thresholds: Thresholds{
			AvailableLowPercent:    12,
			TrimMinWorkingSetMB:    180,
			IdleMinutes:            20,
			CooldownMinutes:        10,
			MaxActionsPerHour:      4,
			ReclaimBudgetMBPerHour: 1024,
			GrowthWindowMinutes:    30,
			GrowthSignalMB:         400,
			StandbyPurgeMinMB:      1536,
		},
		Protected: []string{
			"system", "registry", "memory compression", "secure system",
			"smss.exe", "csrss.exe", "wininit.exe", "winlogon.exe",
			"services.exe", "lsass.exe", "svchost.exe", "dwm.exe",
			"explorer.exe", "audiodg.exe", "msmpeng.exe", "securityhealthservice.exe",
			"nissrv.exe", "memora.exe",
		},
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
	if cfg.IntervalSeconds < 2 {
		cfg.IntervalSeconds = 2
	}
	if cfg.Mode != "observe" && cfg.Mode != "guard" {
		cfg.Mode = "observe"
	}
	return cfg, nil
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
