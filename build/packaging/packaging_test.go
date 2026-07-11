package packaging

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func writeTempEXE(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("MZfake-exe-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPortablePackager(t *testing.T) {
	dir := t.TempDir()
	src := writeTempEXE(t, dir, "deskcalendar-amd64.exe")

	cfg := InstallConfig{SourceEXE: src, Arch: "amd64", OutDir: dir}
	pp := PortablePackager{}
	out, err := pp.Package(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Package: %v", err)
	}

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	if len(zr.File) != 1 {
		t.Fatalf("zip 条目数=%d, want 1", len(zr.File))
	}
	f := zr.File[0]
	if f.Name != "deskcalendar-amd64.exe" {
		t.Fatalf("zip 内文件名=%q, want deskcalendar-amd64.exe", f.Name)
	}
	rc, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	data := make([]byte, 64)
	n, _ := rc.Read(data)
	if string(data[:n]) != "MZfake-exe-bytes" {
		t.Fatalf("zip 内容不匹配: %q", string(data[:n]))
	}
}

func TestInstallConfigResolve(t *testing.T) {
	dir := t.TempDir()
	src := writeTempEXE(t, dir, "app.exe")

	cfg, err := InstallConfig{SourceEXE: src}.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppName != "DeskCalendar" || cfg.Version != "dev" || cfg.Arch != "amd64" {
		t.Fatalf("默认值错误: %+v", cfg)
	}
	if cfg.OutDir != dir {
		t.Fatalf("OutDir=%q want %q", cfg.OutDir, dir)
	}
	if cfg.InstallDir != `$LOCALAPPDATA\DeskCalendar` {
		t.Fatalf("InstallDir=%q", cfg.InstallDir)
	}

	// 缺失源文件应报错
	bad := InstallConfig{SourceEXE: filepath.Join(dir, "nope.exe")}
	if _, err := bad.resolve(); err == nil {
		t.Fatal("期望源文件缺失报错")
	}
}

func TestNSISPackagerMissingMakensis(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("非 Windows 环境验证 makensis 未找到的错误路径")
	}
	dir := t.TempDir()
	src := writeTempEXE(t, dir, "app.exe")
	cfg := InstallConfig{SourceEXE: src, Arch: "amd64", OutDir: dir}

	_, err := NSISPackager{Makensis: "this-makensis-does-not-exist-xyz"}.Package(context.Background(), cfg)
	if err == nil {
		t.Fatal("期望 makensis 未找到错误")
	}
}

func TestNSISScriptPresent(t *testing.T) {
	p := findNSISScript()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("未找到 installer.nsi: %s", p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, need := range []string{"MUI2.nsh", "RequestExecutionLevel user", "WriteUninstaller", "SOURCE_EXE", "DeskCalendar"} {
		if !contains(s, need) {
			t.Fatalf("installer.nsi 缺少关键内容: %s", need)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
