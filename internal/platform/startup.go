package platform

import (
	"context"
	"errors"
	"os"
)

// RegistryKey 自启注册表项（仅当前用户，HKCU）。
const RegistryKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// ValueName 注册表值名（应用标识）。
const ValueName = "DeskCalendar"

// startupValueSuffix 写入注册表的值后缀（自启时带参数启动即驻托盘）。
const startupValueSuffix = " --minimized"

// StartupManager 开机自启管理器（当前用户）。
type StartupManager interface {
	// Enable 写入注册表 Run，值为 exe 绝对路径 + " --minimized"。
	Enable(ctx context.Context) error
	// Disable 删除注册表 Run 值。
	Disable(ctx context.Context) error
	// Enabled 查询当前是否已注册自启（值与本进程 exe 路径一致才视为启用）。
	Enabled(ctx context.Context) (bool, error)
}

// registryBackend 封装注册表读写（seam，便于测试注入内存 fake）。
type registryBackend interface {
	setString(key, valueName, value string) error
	deleteValue(key, valueName string) error
	queryString(key, valueName string) (string, bool, error)
}

// regStartupManager 基于 HKCU\...\Run 的实现。
type regStartupManager struct {
	key       string
	valueName string
	exePath   string
	backend   registryBackend
}

// NewStartupManager 构造默认实现（零 CGO，纯注册表 API 封装）。
func NewStartupManager() (StartupManager, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err // 仅初始化失败可返回错误
	}
	return &regStartupManager{
		key:       RegistryKey,
		valueName: ValueName,
		exePath:   exe,
		backend:   newPlatformRegistryBackend(),
	}, nil
}

// intendedValue 返回应写入注册表的完整值（exe 路径 + 自启参数）。
func (m *regStartupManager) intendedValue() string {
	return m.exePath + startupValueSuffix
}

// Enable 写入自启项。
func (m *regStartupManager) Enable(ctx context.Context) error {
	if m.backend == nil {
		return errStartupUnavailable
	}
	return m.backend.setString(m.key, m.valueName, m.intendedValue())
}

// Disable 删除自启项。
func (m *regStartupManager) Disable(ctx context.Context) error {
	if m.backend == nil {
		return errStartupUnavailable
	}
	return m.backend.deleteValue(m.key, m.valueName)
}

// Enabled 查询是否已自启（值须与本进程 exe 完全一致，过滤旧路径/其他程序同名值）。
func (m *regStartupManager) Enabled(ctx context.Context) (bool, error) {
	if m.backend == nil {
		return false, errStartupUnavailable
	}
	v, ok, err := m.backend.queryString(m.key, m.valueName)
	if err != nil {
		return false, err
	}
	return ok && v == m.intendedValue(), nil
}

// errStartupUnavailable 表示当前平台无注册表 backend（如非 Windows 测试环境）。
var errStartupUnavailable = errors.New("startup: registry backend unavailable")
