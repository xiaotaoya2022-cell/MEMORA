package config

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultProtectedIncludesVirtualizationHosts(t *testing.T) {
	cfg := Default()
	if cfg.Mode != "guard" {
		t.Fatalf("expected default mode guard, got %q", cfg.Mode)
	}
	joined := strings.ToLower(strings.Join(cfg.Protected, ","))
	for _, must := range []string{"system", "vmmemwsl", "memora.exe"} {
		if !strings.Contains(joined, must) {
			t.Fatalf("expected %q in default protected list", must)
		}
	}
}

func TestLoadCreatesDefaultAndSanitizesInvalidValues(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("initial Load failed: %v", err)
	}
	cfg.Mode = "unknown"
	cfg.IntervalSeconds = 1
	if err := Save(cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if reloaded.Mode != "observe" {
		t.Fatalf("expected invalid mode to degrade to observe, got %q", reloaded.Mode)
	}
	if reloaded.IntervalSeconds != 2 {
		t.Fatalf("expected interval clamped to 2s, got %d", reloaded.IntervalSeconds)
	}
}

func TestLegacyConfigInheritsV02Defaults(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"mode":"guard","interval_seconds":10,"actions":{"trim_idle_working_sets":true}}`
	if err := os.WriteFile(Path(), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load legacy config failed: %v", err)
	}
	if !cfg.Actions.SmartMemorySchedule || !cfg.Actions.SoftMemoryPriority || !cfg.Actions.EcoQoSBackground || !cfg.Actions.SuppressSpawnStorm || !cfg.Thresholds.QuietOnFullscreen {
		t.Fatalf("expected v0.3 safety/QoS defaults inherited: %+v", cfg)
	}
	if cfg.Thresholds.SoftIdleMinutes <= 0 || cfg.Thresholds.OrphanIdleMinutes <= 0 {
		t.Fatalf("unexpected default idle thresholds: %+v", cfg.Thresholds)
	}
}

func TestMigratesV02ShippedDefaultsToV03(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmp)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	v02 := `{"mode":"guard","interval_seconds":10,"actions":{"soft_memory_priority":true,"eco_qos_background":true,"trim_idle_working_sets":true,"purge_standby":false},"thresholds":{"available_low_percent":12,"trim_min_working_set_mb":180,"idle_minutes":20,"max_actions_per_hour":4}}`
	if err := os.WriteFile(Path(), []byte(v02), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load v0.2 config failed: %v", err)
	}
	if cfg.Thresholds.AvailableLowPercent != 22 || cfg.Thresholds.IdleMinutes != 4 || !cfg.Actions.PurgeStandby || !cfg.Actions.SmartMemorySchedule {
		t.Fatalf("expected v0.2 shipped thresholds migrated to v0.3, got %+v", cfg)
	}
}

func TestDefaultAutoLearnStormListAndSuppressedSpawners(t *testing.T) {
	cfg := Default()
	if !cfg.Actions.AutoLearnStormList {
		t.Fatalf("expected AutoLearnStormList enabled by default")
	}
	if len(cfg.SuppressedSpawners) != 0 {
		t.Fatalf("expected empty default SuppressedSpawners to avoid overfitting, got %v", cfg.SuppressedSpawners)
	}
}
