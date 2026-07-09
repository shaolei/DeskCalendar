package settings

import (
	"context"
	"testing"

	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// fakeStartup 记录 Enable/Disable 调用，模拟 HKCU Run 注册表。
type fakeStartup struct {
	enabled       bool
	enableCalls   int
	disableCalls  int
}

func (f *fakeStartup) Enable(context.Context) error {
	f.enableCalls++
	f.enabled = true
	return nil
}
func (f *fakeStartup) Disable(context.Context) error {
	f.disableCalls++
	f.enabled = false
	return nil
}
func (f *fakeStartup) Enabled(context.Context) (bool, error) {
	return f.enabled, nil
}

var _ platform.StartupManager = (*fakeStartup)(nil)

// findItem 在菜单树（含子菜单）中按 label 查找项。
func findItem(items []*platform.MenuItem, label string) *platform.MenuItem {
	for _, it := range items {
		if it == nil {
			continue
		}
		if it.Label == label {
			return it
		}
		if it.Submenu != nil {
			if found := findItem(it.Submenu, label); found != nil {
				return found
			}
		}
	}
	return nil
}

// newDeps 构造带计数器的测试依赖；返回 Deps、真实 ThemeProvider（供断言
// Current().Scheme）与 persist 调用计数。
func newDeps(cfg *config.Config, su platform.StartupManager) (Deps, *theme.ThemeProvider, *int) {
	persistCalls := 0
	d := Deps{
		Config:  cfg,
		Startup: su,
		Ctx:     context.Background(),
		Persist: func() error { persistCalls++; return nil },
		SendCmd: func(platform.TrayCommand) {},
	}
	// 注入一个真实 ThemeProvider（跨平台可用），便于验证 ApplyMode。
	tp, _ := theme.NewProvider(theme.WithInitialScheme(theme.SchemeLight))
	d.Theme = tp
	return d, tp, &persistCalls
}

func TestBuildTrayMenu_Structure(t *testing.T) {
	cfg := config.Default()
	d, _, _ := newDeps(&cfg, nil)
	menu := BuildTrayMenu(d)

	labels := make([]string, 0, len(menu.Items))
	for _, it := range menu.Items {
		if it.Separator {
			labels = append(labels, "---")
			continue
		}
		labels = append(labels, it.Label)
	}
	want := []string{"显示/隐藏", "---", "显示农历", "显示节假日", "开机启动", "主题", "---", "退出"}
	if len(labels) != len(want) {
		t.Fatalf("menu labels = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Errorf("item %d = %q, want %q (full: %v)", i, labels[i], want[i], labels)
		}
	}

	// 显示农历初始勾选态应等于 config.Display.ShowLunar（default true）。
	if ln := findItem(menu.Items, "显示农历"); ln == nil || !ln.Checked {
		t.Errorf("显示农历 checkbox should be checked by default (ShowLunar=true)")
	}
	// 主题子菜单应含浅色/深色/跟随系统。
	themeItem := findItem(menu.Items, "主题")
	if themeItem == nil || themeItem.Submenu == nil {
		t.Fatal("主题 should be a submenu")
	}
	for _, sub := range []string{"浅色", "深色", "跟随系统"} {
		if findItem(themeItem.Submenu, sub) == nil {
			t.Errorf("主题 submenu missing %q", sub)
		}
	}
}

func TestBuildTrayMenu_AutoStartToggle(t *testing.T) {
	cfg := config.Default() // AutoStart=false
	su := &fakeStartup{enabled: false}
	d, _, persistCalls := newDeps(&cfg, su)
	menu := BuildTrayMenu(d)

	auto := findItem(menu.Items, "开机启动")
	if auto == nil {
		t.Fatal("开机启动 item missing")
	}
	if auto.Checked {
		t.Errorf("开机启动 initial checked should be false (registry disabled)")
	}
	// 勾选 → Enable + 写 config + 持久化。
	auto.OnToggle(true)
	if su.enableCalls != 1 {
		t.Errorf("Enable called %d times, want 1", su.enableCalls)
	}
	if !cfg.Startup.AutoStart {
		t.Errorf("config.Startup.AutoStart = %v, want true", cfg.Startup.AutoStart)
	}
	if *persistCalls != 1 {
		t.Errorf("Persist called %d times, want 1", *persistCalls)
	}
	// 取消勾选 → Disable + 写 config + 持久化。
	auto.OnToggle(false)
	if su.disableCalls != 1 {
		t.Errorf("Disable called %d times, want 1", su.disableCalls)
	}
	if cfg.Startup.AutoStart {
		t.Errorf("config.Startup.AutoStart = %v, want false", cfg.Startup.AutoStart)
	}
	if *persistCalls != 2 {
		t.Errorf("Persist called %d times, want 2", *persistCalls)
	}
}

func TestBuildTrayMenu_ThemeSelect(t *testing.T) {
	cfg := config.Default() // Mode=system
	d, tp, persistCalls := newDeps(&cfg, nil)
	menu := BuildTrayMenu(d)
	themeItem := findItem(menu.Items, "主题")

	// 选浅色 → mode=light + ApplyMode + 持久化。
	findItem(themeItem.Submenu, "浅色").OnClick()
	if cfg.Theme.Mode != "light" {
		t.Errorf("Theme.Mode = %q, want light", cfg.Theme.Mode)
	}
	if tp.Current().Scheme != theme.SchemeLight {
		t.Errorf("applied theme scheme = %v, want light", tp.Current().Scheme)
	}
	if *persistCalls != 1 {
		t.Errorf("Persist called %d times, want 1", *persistCalls)
	}

	// 选深色 → mode=dark + ApplyMode。
	findItem(themeItem.Submenu, "深色").OnClick()
	if cfg.Theme.Mode != "dark" {
		t.Errorf("Theme.Mode = %q, want dark", cfg.Theme.Mode)
	}
	if tp.Current().Scheme != theme.SchemeDark {
		t.Errorf("applied theme scheme = %v, want dark", tp.Current().Scheme)
	}

	// 选跟随系统 → mode=system + ApplyMode 清除覆盖（回到系统浅/深）。
	findItem(themeItem.Submenu, "跟随系统").OnClick()
	if cfg.Theme.Mode != "system" {
		t.Errorf("Theme.Mode = %q, want system", cfg.Theme.Mode)
	}
}

func TestBuildTrayMenu_ShowLunarToggle(t *testing.T) {
	cfg := config.Default() // ShowLunar=true
	d, _, _ := newDeps(&cfg, nil)
	menu := BuildTrayMenu(d)

	ln := findItem(menu.Items, "显示农历")
	// 取消勾选 → ShowLunar=false。
	ln.OnToggle(false)
	if cfg.Display.ShowLunar {
		t.Errorf("Display.ShowLunar = %v, want false", cfg.Display.ShowLunar)
	}
	// 重新勾选 → true。
	ln.OnToggle(true)
	if !cfg.Display.ShowLunar {
		t.Errorf("Display.ShowLunar = %v, want true", cfg.Display.ShowLunar)
	}
}

func TestBuildTrayMenu_Commands(t *testing.T) {
	cfg := config.Default()
	var got platform.TrayCommand = -1
	d := Deps{
		Config:  &cfg,
		Ctx:     context.Background(),
		Persist: func() error { return nil },
		SendCmd: func(c platform.TrayCommand) { got = c },
	}
	menu := BuildTrayMenu(d)

	findItem(menu.Items, "显示/隐藏").OnClick()
	if got != platform.CmdToggle {
		t.Errorf("显示/隐藏 sent %v, want CmdToggle", got)
	}
	findItem(menu.Items, "退出").OnClick()
	if got != platform.CmdQuit {
		t.Errorf("退出 sent %v, want CmdQuit", got)
	}
}
