// Package packaging 提供 DeskCalendar 的发布打包能力（见 docs/100-Release/Package.md）：
//   - NSISPackager：调用外部 makensis 编译 build/nsis/installer.nsi 生成单文件安装器
//   - PortablePackager：将单一 exe 压入 zip 生成免安装便携版
//
// 依赖纪律（ADR-07）：本包仅依赖标准库，不引入任何业务/feature 包，
// 保持为纯构建期工具，可在 Linux CI runner 上编译与单元测试。
package packaging

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// InstallConfig 描述一次打包的配置。
// InstallDir 使用 NSIS 变量表达式（如 "$LOCALAPPDATA\DeskCalendar"），
// 由安装器在目标机器展开，无需在构建期解析。
type InstallConfig struct {
	AppName                 string
	Version                 string
	Arch                    string // amd64 / arm64，用于产物命名
	SourceEXE               string // Build 产物路径（必填）
	OutDir                  string // 产物输出目录，缺省为 SourceEXE 所在目录
	IconPath                string // 安装器/快捷方式图标（.ico），可空
	InstallDir              string // NSIS 安装目录表达式，缺省 $LOCALAPPDATA\DeskCalendar
	CreateDesktopShortcut   bool
	CreateStartMenuShortcut bool
	// AutoStart 仅控制安装向导默认是否勾选“开机自动启动”；
	// 勾选时安装器直接写入 HKCU\Software\Microsoft\Windows\CurrentVersion\Run，
	// 与 internal/platform/startup 的注册表模型一致（取消亦由应用设置统一回收）。
	AutoStart bool
}

// Packager 抽象一种打包形态，便于扩展（NSIS / 便携 zip / 未来 MSIX）。
type Packager interface {
	// Package 产出安装包/压缩包文件，返回产物路径。
	Package(ctx context.Context, cfg InstallConfig) (path string, err error)
}

// Package 是便捷入口：用给定 Packager 执行一次打包。
func Package(ctx context.Context, cfg InstallConfig, p Packager) (string, error) {
	return p.Package(ctx, cfg)
}

// resolve 填充缺省值并返回校验后的配置副本。
func (c InstallConfig) resolve() (InstallConfig, error) {
	if c.SourceEXE == "" {
		return c, fmt.Errorf("packaging: SourceEXE 不能为空")
	}
	if fi, err := os.Stat(c.SourceEXE); err != nil || fi.IsDir() {
		return c, fmt.Errorf("packaging: SourceEXE 不存在或不是文件: %s", c.SourceEXE)
	}
	if c.AppName == "" {
		c.AppName = "DeskCalendar"
	}
	if c.Version == "" {
		c.Version = "dev"
	}
	if c.Arch == "" {
		c.Arch = "amd64"
	}
	if c.OutDir == "" {
		c.OutDir = filepath.Dir(c.SourceEXE)
	}
	if c.InstallDir == "" {
		c.InstallDir = `$LOCALAPPDATA\DeskCalendar`
	}
	return c, nil
}

// PortablePackager 生成免安装便携版（单 exe 的 zip）。
type PortablePackager struct{}

// Package 将 SourceEXE 压入 zip，返回 Portable.zip 路径。
func (p PortablePackager) Package(ctx context.Context, cfg InstallConfig) (string, error) {
	cfg, err := cfg.resolve()
	if err != nil {
		return "", err
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return "", fmt.Errorf("packaging: 创建输出目录失败: %w", err)
	}
	innerName := fmt.Sprintf("deskcalendar-%s.exe", cfg.Arch)
	out := filepath.Join(cfg.OutDir, fmt.Sprintf("DeskCalendar-Portable-%s.zip", cfg.Arch))
	if err := zipFile(cfg.SourceEXE, out, innerName); err != nil {
		return "", err
	}
	return out, nil
}

// zipFile 将 src 作为 nameInZip 压入 dest。
func zipFile(src, dest, nameInZip string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("packaging: 读取源文件失败: %w", err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("packaging: 创建输出目录失败: %w", err)
	}
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("packaging: 创建 zip 失败: %w", err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	defer zw.Close()
	w, err := zw.Create(nameInZip)
	if err != nil {
		return fmt.Errorf("packaging: 写入 zip 条目失败: %w", err)
	}
	if _, err := io.Copy(w, in); err != nil {
		return fmt.Errorf("packaging: 复制数据失败: %w", err)
	}
	return nil
}
