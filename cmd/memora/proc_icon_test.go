package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestShouldInheritParentIconAndHelperDetection(t *testing.T) {
	if !shouldInheritParentIcon(
		`C:\Users\demo\AppData\Roaming\Tencent\xwechat\radium\WeChatAppEx.exe`,
		"WeChatAppEx.exe",
		`C:\Program Files\Tencent\weixin\Weixin.exe`,
		"Weixin.exe",
	) {
		t.Fatalf("expected WeChatAppEx.exe to inherit Weixin.exe icon")
	}

	if !shouldInheritParentIcon(
		`C:\Users\demo\AppData\Local\Programs\antigravity\resources\bin\language_server_windows_x64.exe`,
		"language_server_windows_x64.exe",
		`C:\Users\demo\AppData\Local\Programs\antigravity\Antigravity.exe`,
		"Antigravity.exe",
	) {
		t.Fatalf("expected language_server_windows_x64.exe inside Antigravity tree to inherit parent icon")
	}

	if shouldInheritParentIcon(
		`C:\Program Files\Git\cmd\git.exe`,
		"git.exe",
		`C:\Users\demo\AppData\Local\Programs\antigravity\Antigravity.exe`,
		"Antigravity.exe",
	) {
		t.Fatalf("expected standalone git.exe in different root NOT to inherit IDE icon")
	}
}

func TestParseAppxManifestLogosAndAssetLookup(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="utf-8"?>
<Package>
  <Properties>
    <Logo>Assets\StoreLogo.png</Logo>
  </Properties>
  <Applications>
    <Application Id="App">
      <uap:VisualElements Square44x44Logo="Assets\AppList.png" Square150x150Logo="Assets\MedTile.png" />
    </Application>
  </Applications>
</Package>`)
	logos := parseAppxManifestLogos(raw)
	if len(logos) < 3 || logos[0] != `Assets\AppList.png` {
		t.Fatalf("expected Square44x44Logo prioritized first, got %+v", logos)
	}

	tmp := t.TempDir()
	assets := filepath.Join(tmp, "Assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	f100 := filepath.Join(assets, "AppList.scale-100.png")
	fTarget48 := filepath.Join(assets, "AppList.targetsize-48_altform-unplated.png")
	if err := os.WriteFile(f100, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fTarget48, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	best := findBestAppxLogoFile(tmp, `Assets\AppList.png`)
	if best != fTarget48 {
		t.Fatalf("expected unplated targetsize-48 logo, got %q", best)
	}
}

func TestExtractEmbeddedPEIconPNGFromSelf(t *testing.T) {
	// C:\Windows\System32\notepad.exe has a classic embedded PE icon on every Windows machine.
	notepad := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	if _, err := os.Stat(notepad); err != nil {
		t.Skip("notepad.exe not found")
	}
	pngBytes := extractEmbeddedPEIconPNG(notepad)
	if len(pngBytes) < 32 {
		t.Fatalf("expected valid PNG bytes from notepad.exe, got len=%d", len(pngBytes))
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("invalid PNG output: %v", err)
	}
	if cfg.Width < 16 || cfg.Height < 16 {
		t.Fatalf("unexpected icon dimensions: %dx%d", cfg.Width, cfg.Height)
	}
}

func TestResolveProcessIconForWeChatHelperIfInstalled(t *testing.T) {
	pngBytes := findWellKnownHelperAppIconPNG("WeChatAppEx.exe")
	if len(pngBytes) == 0 {
		t.Skip("Weixin/WeChat not installed in standard Program Files paths")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("invalid WeChat helper icon PNG: %v", err)
	}
	if cfg.Width < 16 || cfg.Height < 16 {
		t.Fatalf("unexpected WeChat icon dimensions: %dx%d", cfg.Width, cfg.Height)
	}
}
