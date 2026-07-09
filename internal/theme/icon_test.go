package theme

import (
	"testing"
)

// TestIconProvider_TrayIcon 验证按 Scheme 返回非空托盘图标字节。
func TestIconProvider_TrayIcon(t *testing.T) {
	p, err := NewIconProvider()
	if err != nil {
		t.Fatalf("NewIconProvider: %v", err)
	}
	for _, s := range []Scheme{SchemeLight, SchemeDark} {
		b, err := p.TrayIcon(s)
		if err != nil {
			t.Fatalf("TrayIcon(%s): %v", s, err)
		}
		if len(b) == 0 {
			t.Errorf("TrayIcon(%s) returned empty bytes", s)
		}
		// 校验是合法 PNG（签名头）。
		if len(b) < 8 || string(b[0:4]) != "\x89PNG" {
			t.Errorf("TrayIcon(%s) not a PNG (bad signature)", s)
		}
	}
}

// TestIconProvider_WinIcon 验证高分屏分辨率返回字节。
func TestIconProvider_WinIcon(t *testing.T) {
	p, err := NewIconProvider()
	if err != nil {
		t.Fatalf("NewIconProvider: %v", err)
	}
	b, err := p.WinIcon(SchemeLight, R256)
	if err != nil {
		t.Fatalf("WinIcon(light,256): %v", err)
	}
	if len(b) == 0 {
		t.Error("WinIcon(light,256) returned empty bytes")
	}
}

// TestIconProvider_ForScheme 验证明暗各自返回独立映射。
func TestIconProvider_ForScheme(t *testing.T) {
	p, err := NewIconProvider()
	if err != nil {
		t.Fatalf("NewIconProvider: %v", err)
	}
	light := p.ForScheme(SchemeLight)
	dark := p.ForScheme(SchemeDark)
	if light == nil || dark == nil {
		t.Fatal("ForScheme returned nil map")
	}
	if _, ok := light[R16]; !ok {
		t.Error("light set missing R16")
	}
	if _, ok := dark[R256]; !ok {
		t.Error("dark set missing R256")
	}
}
