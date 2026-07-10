package settings

import (
	"context"
	"testing"

	"github.com/shaolei/DeskCalendar/internal/infra/config"
	"github.com/shaolei/DeskCalendar/internal/platform"
)

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

// newDeps 构造带命令捕获的测试依赖。单写者约束下，菜单回调只经 SendCmd 投递
// 命令，不直接写 Config；故 Deps 仅需 Config + SendCmd + Ctx。
func newDeps(cfg *config.Config) (Deps, *platform.TrayCommand) {
	var last platform.TrayCommand = -1
	d := Deps{
		Config:  cfg,
		Ctx:     context.Background(),
		SendCmd: func(c platform.TrayCommand) { last = c },
	}
	return d, &last
}

func TestBuildTrayMenu_Structure(t *testing.T) {
	cfg := config.Default()
	d, _ := newDeps(&cfg)
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

// TestBuildTrayMenu_AutoStartSendsCommand 验证「开机启动」勾选仅经 SendCmd 投递
// CmdToggleStartup，不直改 Config（单写者：配置写收口主循环）。
func TestBuildTrayMenu_AutoStartSendsCommand(t *testing.T) {
	cfg := config.Default() // AutoStart=false
	d, last := newDeps(&cfg)
	menu := BuildTrayMenu(d)

	auto := findItem(menu.Items, "开机启动")
	if auto == nil {
		t.Fatal("开机启动 item missing")
	}
	if auto.Checked {
		t.Errorf("开机启动 initial checked should be false (config AutoStart=false)")
	}
	// 勾选 → 仅发命令，不直改 config。
	auto.OnToggle(true)
	if *last != platform.CmdToggleStartup {
		t.Errorf("勾选 sent %v, want CmdToggleStartup", *last)
	}
	if cfg.Startup.AutoStart {
		t.Errorf("callback must not mutate config (single-writer) — AutoStart=%v", cfg.Startup.AutoStart)
	}
	// 取消勾选 → 仍发同一命令（值由主循环依当前态翻转）。
	auto.OnToggle(false)
	if *last != platform.CmdToggleStartup {
		t.Errorf("取消勾选 sent %v, want CmdToggleStartup", *last)
	}
	if cfg.Startup.AutoStart {
		t.Errorf("callback must not mutate config (single-writer)")
	}
}

// TestBuildTrayMenu_ThemeSelectSendsCommand 验证主题子菜单项仅投递对应命令，不直改
// Config.Theme.Mode（单写者）。主题实际应用由 app.Run 主循环落地。
func TestBuildTrayMenu_ThemeSelectSendsCommand(t *testing.T) {
	cfg := config.Default() // Mode=system
	d, last := newDeps(&cfg)
	menu := BuildTrayMenu(d)
	themeItem := findItem(menu.Items, "主题")

	findItem(themeItem.Submenu, "浅色").OnClick()
	if *last != platform.CmdThemeLight {
		t.Errorf("浅色 sent %v, want CmdThemeLight", *last)
	}
	if cfg.Theme.Mode != "system" {
		t.Errorf("callback must not mutate config (single-writer) — Mode=%q", cfg.Theme.Mode)
	}

	findItem(themeItem.Submenu, "深色").OnClick()
	if *last != platform.CmdThemeDark {
		t.Errorf("深色 sent %v, want CmdThemeDark", *last)
	}

	findItem(themeItem.Submenu, "跟随系统").OnClick()
	if *last != platform.CmdThemeSystem {
		t.Errorf("跟随系统 sent %v, want CmdThemeSystem", *last)
	}
}

// TestBuildTrayMenu_ShowLunarSendsCommand 验证「显示农历」勾选仅投递 CmdToggleLunar，
// 不直改 Config.Display.ShowLunar（单写者）。
func TestBuildTrayMenu_ShowLunarSendsCommand(t *testing.T) {
	cfg := config.Default() // ShowLunar=true
	d, last := newDeps(&cfg)
	menu := BuildTrayMenu(d)

	ln := findItem(menu.Items, "显示农历")
	ln.OnToggle(false)
	if *last != platform.CmdToggleLunar {
		t.Errorf("取消勾选 sent %v, want CmdToggleLunar", *last)
	}
	if !cfg.Display.ShowLunar {
		t.Errorf("callback must not mutate config (single-writer) — ShowLunar=%v", cfg.Display.ShowLunar)
	}
	ln.OnToggle(true)
	if *last != platform.CmdToggleLunar {
		t.Errorf("重新勾选 sent %v, want CmdToggleLunar", *last)
	}
	if !cfg.Display.ShowLunar {
		t.Errorf("callback must not mutate config (single-writer)")
	}
}

// TestBuildTrayMenu_Commands 验证各菜单项经 SendCmd 投递正确的 TrayCommand（含
// 显示/隐藏、退出，以及 S1 单写者新增的配置切换命令）。
func TestBuildTrayMenu_Commands(t *testing.T) {
	cfg := config.Default()
	d, last := newDeps(&cfg)
	menu := BuildTrayMenu(d)

	findItem(menu.Items, "显示/隐藏").OnClick()
	if *last != platform.CmdToggle {
		t.Errorf("显示/隐藏 sent %v, want CmdToggle", *last)
	}
	findItem(menu.Items, "退出").OnClick()
	if *last != platform.CmdQuit {
		t.Errorf("退出 sent %v, want CmdQuit", *last)
	}

	findItem(menu.Items, "显示农历").OnToggle(false)
	if *last != platform.CmdToggleLunar {
		t.Errorf("显示农历 sent %v, want CmdToggleLunar", *last)
	}
	findItem(menu.Items, "显示节假日").OnToggle(false)
	if *last != platform.CmdToggleHoliday {
		t.Errorf("显示节假日 sent %v, want CmdToggleHoliday", *last)
	}
	findItem(menu.Items, "开机启动").OnToggle(true)
	if *last != platform.CmdToggleStartup {
		t.Errorf("开机启动 sent %v, want CmdToggleStartup", *last)
	}

	themeItem := findItem(menu.Items, "主题")
	findItem(themeItem.Submenu, "浅色").OnClick()
	if *last != platform.CmdThemeLight {
		t.Errorf("浅色 sent %v, want CmdThemeLight", *last)
	}
	findItem(themeItem.Submenu, "深色").OnClick()
	if *last != platform.CmdThemeDark {
		t.Errorf("深色 sent %v, want CmdThemeDark", *last)
	}
	findItem(themeItem.Submenu, "跟随系统").OnClick()
	if *last != platform.CmdThemeSystem {
		t.Errorf("跟随系统 sent %v, want CmdThemeSystem", *last)
	}
}
