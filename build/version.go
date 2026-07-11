// Package build 集中管理版本元数据与全局嵌入资产。
//
// Version/Commit/BuildTime/TargetOS/TargetArch 由 Makefile 或 CI 经
// -ldflags "-X github.com/shaolei/DeskCalendar/build.Version=..." 注入，
// 请勿在代码中手动赋值（默认值仅用于本地 dev 构建）。
//
// build 是叶子包：仅依赖 embed 标准库与（可选的）infra/log，不依赖任何
// feature 包，避免构建期引入业务耦合（见 docs/100-Release/Build.md §1）。
package build

// Version / Commit / BuildTime / TargetOS / TargetArch 为 -ldflags 注入目标。
var (
	Version    = "dev"
	Commit     = "none"
	BuildTime  = "unknown"
	TargetOS   = "windows"
	TargetArch = "amd64"
)

// VersionInfo 是运行时可读的构建元数据快照。
type VersionInfo struct {
	Version    string
	Commit     string
	BuildTime  string
	TargetOS   string
	TargetArch string
	CGOEnabled bool
}

// Info 返回当前二进制注入的版本信息。
func Info() VersionInfo {
	return VersionInfo{
		Version:    Version,
		Commit:     Commit,
		BuildTime:  BuildTime,
		TargetOS:   TargetOS,
		TargetArch: TargetArch,
		CGOEnabled: false, // 硬约束：本工程永不启用 CGO（ADR-01/06）
	}
}
