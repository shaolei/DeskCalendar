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

// TestProvider_SetOverride_ClearOverride_DarkSystem 是 S2 的回归测试：
// 暗色系统下设置暗色覆盖再清除，必须回到暗色（之前错误地回退到亮色）。
func TestProvider_SetOverride_ClearOverride_DarkSystem(t *testing.T) {
	p, err := NewProvider(WithInitialScheme(SchemeDark))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Current() == nil || p.Current().Scheme != SchemeDark {
		t.Fatalf("init scheme = %v, want dark", p.Current().Scheme)
	}
	custom := &Theme{Name: "我的暗", Builtin: false, Scheme: SchemeDark}
	p.SetOverride(custom)
	if p.Current() != custom {
		t.Errorf("after SetOverride Current = %+v, want custom", p.Current())
	}
	p.ClearOverride()
	if p.Current() == nil {
		t.Fatal("after ClearOverride Current is nil")
	}
	if p.Current().Name == "我的暗" {
		t.Errorf("after ClearOverride still showing override %+v", p.Current())
	}
	if p.Current().Scheme != SchemeDark {
		t.Errorf("after ClearOverride scheme = %v, want dark (system scheme must be restored)", p.Current().Scheme)
	}
}

// TestProvider_Watch_UpdatesCurrentOnSystemChange 是 S3 的跨平台测试：
// 系统方案切换时应同步更新 Current()，使其实时跟随（无需消费者自行 Resolve）。
func TestProvider_Watch_UpdatesCurrentOnSystemChange(t *testing.T) {
	p, err := NewProvider(WithInitialScheme(SchemeLight))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	ch := make(chan Scheme, 1)
	p.onSystemSchemeChanged(ch, SchemeDark)
	if p.Current() == nil || p.Current().Scheme != SchemeDark {
		t.Errorf("after system change Current scheme = %v, want dark", p.Current().Scheme)
	}
	select {
	case s := <-ch:
		if s != SchemeDark {
			t.Errorf("channel got %v, want dark", s)
		}
	default:
		t.Error("expected scheme pushed to channel")
	}

	// 有覆盖时，系统变化不应覆盖用户选择，但仍应记录 systemScheme 并推送。
	custom := &Theme{Name: "锁定", Builtin: false, Scheme: SchemeLight}
	p.SetOverride(custom)
	p.onSystemSchemeChanged(ch, SchemeLight)
	if p.Current() != custom {
		t.Errorf("with override, Current should stay custom, got %+v", p.Current())
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
