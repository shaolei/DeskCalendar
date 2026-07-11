package packaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// NSISPackager 通过调用外部 makensis 编译 installer.nsi 生成单文件安装器。
type NSISPackager struct {
	Makensis string // makensis 可执行路径，缺省 "makensis"
}

// Package 生成 DeskCalendar-Setup-${Arch}.exe。
func (n NSISPackager) Package(ctx context.Context, cfg InstallConfig) (string, error) {
	cfg, err := cfg.resolve()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return "", fmt.Errorf("packaging: 创建输出目录失败: %w", err)
	}

	// NSIS 的 File/OutFile 相对路径以 .nsi 脚本所在目录为基准，
	// 为避免 cwd 与脚本目录不一致导致找不到文件，统一转为绝对路径。
	srcAbs, err := filepath.Abs(cfg.SourceEXE)
	if err != nil {
		return "", fmt.Errorf("packaging: 解析源路径失败: %w", err)
	}
	outDirAbs, err := filepath.Abs(cfg.OutDir)
	if err != nil {
		return "", fmt.Errorf("packaging: 解析输出目录失败: %w", err)
	}
	iconAbs := ""
	if cfg.IconPath != "" {
		iconAbs, err = filepath.Abs(cfg.IconPath)
		if err != nil {
			return "", fmt.Errorf("packaging: 解析图标路径失败: %w", err)
		}
	}

	script := findNSISScript()
	makensis := n.Makensis
	if makensis == "" {
		makensis = "makensis"
	}
	out := filepath.Join(outDirAbs, fmt.Sprintf("DeskCalendar-Setup-%s.exe", cfg.Arch))

	args := []string{
		"-V2",
		"-DAPPNAME=" + cfg.AppName,
		"-DVERSION=" + cfg.Version,
		"-DEXE=" + filepath.Base(srcAbs),
		"-DSOURCE_EXE=" + srcAbs,
		"-DINSTALLDIR=" + cfg.InstallDir,
		"-DCREATE_DESKTOP=" + b2i(cfg.CreateDesktopShortcut),
		"-DCREATE_STARTMENU=" + b2i(cfg.CreateStartMenuShortcut),
		"-DAUTOSTART=" + b2i(cfg.AutoStart),
		"-DOUTFILE=" + out,
	}
	if iconAbs != "" {
		args = append(args, "-DICON="+iconAbs)
	}
	args = append(args, script)

	cmd := exec.CommandContext(ctx, makensis, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("packaging: 未找到 makensis（请安装 NSIS：https://nsis.sourceforge.io/）；%w", err)
		}
		return "", fmt.Errorf("packaging: makensis 编译失败: %v\n%s", err, msg)
	}
	return out, nil
}

// findNSISScript 定位 build/nsis/installer.nsi：优先环境变量覆盖，
// 其次从当前工作目录向上查找（兼容 make / go run / go test 各调用场景）。
func findNSISScript() string {
	if p := os.Getenv("DESKCALENDAR_NSIS_SCRIPT"); p != "" {
		return p
	}
	dir, err := os.Getwd()
	if err != nil {
		return "build/nsis/installer.nsi"
	}
	for i := 0; i < 8; i++ {
		cand := filepath.Join(dir, "build", "nsis", "installer.nsi")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "build/nsis/installer.nsi"
}

func b2i(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
