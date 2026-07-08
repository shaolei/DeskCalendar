package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shaolei/DeskCalendar/internal/infra/config"
)

func TestDefault(t *testing.T) {
	c := config.Default()
	if c.Theme.Mode != "system" {
		t.Error("default theme mode should be system")
	}
	if c.Window.CornerRadius != 16 {
		t.Error("default corner radius should be 16")
	}
	if c.Version != 1 {
		t.Error("default version should be 1")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "DeskCalendar", "config.json")

	c := config.Default()
	c.Theme.Mode = "dark"
	c.Window.CornerRadius = 8
	c.Startup.AutoStart = true

	if err := config.Save(p, c); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := config.Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Theme.Mode != "dark" || got.Window.CornerRadius != 8 || !got.Startup.AutoStart {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestLoadMissingReturnsDefault(t *testing.T) {
	got, err := config.Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("load missing should not error: %v", err)
	}
	if got.Theme.Mode != "system" {
		t.Fatal("missing file should yield default")
	}
}

// S1 回归：部分 JSON 不应使未出现字段静默零值。
func TestLoadPartialJSONKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(p, []byte(`{"theme":{"mode":"dark"}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := config.Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Theme.Mode != "dark" {
		t.Errorf("mode = %q, want dark", got.Theme.Mode)
	}
	// 未出现的字段应保留默认值，而非零值
	if got.Theme.Accent != "#4C8DFF" {
		t.Errorf("accent = %q, want default #4C8DFF", got.Theme.Accent)
	}
	if got.Window.CornerRadius != 16 {
		t.Errorf("corner_radius = %d, want default 16", got.Window.CornerRadius)
	}
	if got.Window.PositionMode != "tray" {
		t.Errorf("position_mode = %q, want default tray", got.Window.PositionMode)
	}
	if got.Version != 1 {
		t.Errorf("version = %d, want default 1", got.Version)
	}
}

// S1 回归：非法枚举值应回落默认值，而非脏字符串。
func TestLoadInvalidEnumClampsToDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(p, []byte(`{"theme":{"mode":"RAINBOW"},"window":{"position_mode":"NOWHERE"}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := config.Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Theme.Mode != "system" {
		t.Errorf("invalid mode should clamp to system, got %q", got.Theme.Mode)
	}
	if got.Window.PositionMode != "tray" {
		t.Errorf("invalid position_mode should clamp to tray, got %q", got.Window.PositionMode)
	}
}

// N5 回归：非法 JSON 应返回错误而非静默零值或 panic。
func TestLoadMalformedJSONReturnsError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(p, []byte(`{not valid json`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := config.Load(p); err == nil {
		t.Fatal("load of malformed JSON should return error")
	}
}

// S1 回归：非法 Accent 格式应回落默认 #4C8DFF。
func TestLoadInvalidAccentClampsToDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(p, []byte(`{"theme":{"accent":"not-a-color"}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := config.Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Theme.Accent != "#4C8DFF" {
		t.Errorf("invalid accent should clamp to #4C8DFF, got %q", got.Theme.Accent)
	}
}
