package config

import (
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
