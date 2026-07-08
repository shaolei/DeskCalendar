package platform

import (
	"context"
	"runtime"
	"testing"
)

// memRegistryBackend 内存版注册表 fake，用于单测 StartupManager 逻辑。
type memRegistryBackend struct {
	store map[string]map[string]string // key -> (valueName -> value)
}

func newMemRegistryBackend() *memRegistryBackend {
	return &memRegistryBackend{store: map[string]map[string]string{}}
}

func (b *memRegistryBackend) setString(key, valueName, value string) error {
	if b.store[key] == nil {
		b.store[key] = map[string]string{}
	}
	b.store[key][valueName] = value
	return nil
}

func (b *memRegistryBackend) deleteValue(key, valueName string) error {
	if b.store[key] != nil {
		delete(b.store[key], valueName)
	}
	return nil
}

func (b *memRegistryBackend) queryString(key, valueName string) (string, bool, error) {
	v, ok := b.store[key][valueName]
	return v, ok, nil
}

func newTestStartupManager(t *testing.T, backend registryBackend, exePath string) *regStartupManager {
	t.Helper()
	return &regStartupManager{
		key:       RegistryKey,
		valueName: ValueName,
		exePath:   exePath,
		backend:   backend,
	}
}

func TestStartupManager_EnableDisableEnabledRoundTrip(t *testing.T) {
	m := newTestStartupManager(t, newMemRegistryBackend(), `C:\apps\DeskCalendar.exe`)

	ctx := context.Background()
	if err := m.Enable(ctx); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	on, err := m.Enabled(ctx)
	if err != nil {
		t.Fatalf("Enabled after enable: %v", err)
	}
	if !on {
		t.Error("expected Enabled()==true after Enable()")
	}

	if err := m.Disable(ctx); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	on, err = m.Enabled(ctx)
	if err != nil {
		t.Fatalf("Enabled after disable: %v", err)
	}
	if on {
		t.Error("expected Enabled()==false after Disable()")
	}
}

func TestStartupManager_EnabledFalseWhenAbsent(t *testing.T) {
	m := newTestStartupManager(t, newMemRegistryBackend(), `C:\apps\DeskCalendar.exe`)
	on, err := m.Enabled(context.Background())
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	if on {
		t.Error("expected Enabled()==false when registry value absent")
	}
}

func TestStartupManager_EnabledFalseWhenStalePath(t *testing.T) {
	// 注册表里有同名值，但指向旧 exe 路径（或不同程序）→ 不应视为启用。
	b := newMemRegistryBackend()
	_ = b.setString(RegistryKey, ValueName, `C:\old\DeskCalendar.exe --minimized`)
	m := newTestStartupManager(t, b, `C:\apps\DeskCalendar.exe`)
	on, err := m.Enabled(context.Background())
	if err != nil {
		t.Fatalf("Enabled: %v", err)
	}
	if on {
		t.Error("expected Enabled()==false when registry value points to a different exe path")
	}
}

func TestNewStartupManager_WindowsWiresBackend(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("registry backend only on windows")
	}
	m, err := NewStartupManager()
	if err != nil {
		t.Fatalf("NewStartupManager: %v", err)
	}
	rm, ok := m.(*regStartupManager)
	if !ok {
		t.Fatalf("NewStartupManager returned %T, want *regStartupManager", m)
	}
	if rm.backend == nil {
		t.Error("expected non-nil registry backend on windows")
	}
}
