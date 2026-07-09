package theme

import (
	"context"
	"testing"
	"time"
)

// TestNewProvider_ResolvesEmbedded 验证 Provider 从内嵌默认主题初始化 light/dark。
func TestNewProvider_ResolvesEmbedded(t *testing.T) {
	p, err := NewProvider(WithInitialScheme(SchemeLight))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Resolve(SchemeLight) == nil || p.Resolve(SchemeLight).Scheme != SchemeLight {
		t.Error("Resolve(light) should return builtin light theme")
	}
	if p.Resolve(SchemeDark) == nil || p.Resolve(SchemeDark).Scheme != SchemeDark {
		t.Error("Resolve(dark) should return builtin dark theme")
	}
	if p.Current() == nil || p.Current().Scheme != SchemeLight {
		t.Errorf("Current() scheme = %v, want light", p.Current().Scheme)
	}
}

// TestProvider_SetOverride_ClearOverride 验证覆盖/清除切换当前主题。
func TestProvider_SetOverride_ClearOverride(t *testing.T) {
	p, err := NewProvider(WithInitialScheme(SchemeLight))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	custom := &Theme{Name: "我的蓝", Builtin: false, Scheme: SchemeDark}
	p.SetOverride(custom)
	if p.Current() != custom {
		t.Errorf("after SetOverride Current = %+v, want custom", p.Current())
	}
	p.ClearOverride()
	if p.Current() == nil || p.Current().Name == "我的蓝" {
		t.Errorf("after ClearOverride Current = %+v, want builtin", p.Current())
	}
	if p.Current().Scheme != SchemeLight {
		t.Errorf("after ClearOverride scheme = %v, want light", p.Current().Scheme)
	}
}

// TestProvider_Watch_DeliversInitial 验证 Watch 立即推送当前方案。
func TestProvider_Watch_DeliversInitial(t *testing.T) {
	p, err := NewProvider(WithInitialScheme(SchemeDark))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := p.Watch(ctx)
	select {
	case s := <-ch:
		if s != SchemeDark {
			t.Errorf("Watch initial = %v, want dark", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not deliver initial scheme within 2s")
	}
}

// TestSchemeFromString 验证字符串解析（非法回退 light）。
func TestSchemeFromString(t *testing.T) {
	if SchemeFromString("dark") != SchemeDark {
		t.Error("SchemeFromString(dark) != dark")
	}
	if SchemeFromString("light") != SchemeLight {
		t.Error("SchemeFromString(light) != light")
	}
	if SchemeFromString("bogus") != SchemeLight {
		t.Error("SchemeFromString(bogus) should fall back to light")
	}
}
