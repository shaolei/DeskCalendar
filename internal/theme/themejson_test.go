package theme

import (
	"context"
	"strings"
	"testing"
)

const validLightJSON = `{
  "name": "浅色",
  "scheme": "light",
  "cornerRadius": 12,
  "alpha": 0.98,
  "colors": {
    "background":  "#F7F7F7FF",
    "surface":     "#FFFFFFFF",
    "foreground":  "#1A1A1AFF",
    "muted":       "#9AA0A6FF",
    "accent":      "#2D7FF9FF",
    "holidayRed":  "#E53935FF",
    "todayBlue":   "#2D7FF9FF",
    "border":      "#E0E0E0FF"
  },
  "shadow": { "blur": 24, "offsetY": 8, "color": "#00000066", "opacity": 0.35 }
}`

// TestParseBytes_Valid 验证合法 JSON → *Theme，alpha 生效。
func TestParseBytes_Valid(t *testing.T) {
	th, err := ParseBytes(context.Background(), []byte(validLightJSON))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if th.Name != "浅色" || th.Scheme != SchemeLight {
		t.Errorf("parsed theme = %+v", th)
	}
	if th.Alpha != 0.98 {
		t.Errorf("alpha = %v, want 0.98", th.Alpha)
	}
	if th.Palette.Background.A != 0xff {
		t.Errorf("background alpha = %d, want 255", th.Palette.Background.A)
	}
	if th.Shadow.Color.A != 0x66 {
		t.Errorf("shadow color alpha = %d, want 0x66", th.Shadow.Color.A)
	}
}

// TestParseBytes_MissingColor 验证缺失必填色返回 error。
func TestParseBytes_MissingColor(t *testing.T) {
	bad := strings.Replace(validLightJSON, `"accent":      "#2D7FF9FF",`, "", 1)
	if _, err := ParseBytes(context.Background(), []byte(bad)); err == nil {
		t.Error("missing required color should error, got nil")
	}
}

// TestValidate_BadAlpha 验证 alpha 越界被拒。
func TestValidate_BadAlpha(t *testing.T) {
	th, err := ParseBytes(context.Background(), []byte(validLightJSON))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	th.Alpha = 1.5
	if err := Validate(th); err == nil {
		t.Error("alpha 1.5 should fail Validate")
	}
}

// TestValidate_BadRadius 验证 cornerRadius 越界被拒。
func TestValidate_BadRadius(t *testing.T) {
	th, err := ParseBytes(context.Background(), []byte(validLightJSON))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	th.CornerRadius = 128
	if err := Validate(th); err == nil {
		t.Error("cornerRadius 128 should fail Validate")
	}
}

// TestLoadEmbedded 验证内嵌默认主题可加载出 light + dark 两套。
func TestLoadEmbedded(t *testing.T) {
	themes, err := LoadEmbedded(context.Background())
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	var haveLight, haveDark bool
	for _, th := range themes {
		if th.Scheme == SchemeLight {
			haveLight = true
		}
		if th.Scheme == SchemeDark {
			haveDark = true
		}
	}
	if !haveLight || !haveDark {
		t.Errorf("LoadEmbedded themes = %d (light=%v dark=%v), want both", len(themes), haveLight, haveDark)
	}
}
