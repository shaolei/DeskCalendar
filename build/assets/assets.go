// Package assets 持有构建期全局静态资源，运行时以只读 embed.FS 访问，零磁盘依赖。
//
// 仅承载「全局兜底资产」（应用图标、默认主题 JSON 兜底）；领域数据
// （日历节假日、主题包资源）由各自 feature 包嵌入，见 Build.md §1。
//
// 字体（font）按 Build.md 路线图推迟到 v1.3（主题/字体资源构建期可配置路径嵌入），
// 当前不在此包嵌入。
package assets

import "embed"

//go:embed icon/app.ico theme/default.json
// FS 持有构建期全局静态资源，嵌入后不可变。
var FS embed.FS

// Icon 返回应用托盘/窗口图标字节（ICO 格式）。
func Icon() ([]byte, error) { return FS.ReadFile("icon/app.ico") }

// DefaultThemeJSON 返回默认主题 JSON 兜底字节（light 结构，见 internal/theme/embedded）。
func DefaultThemeJSON() ([]byte, error) { return FS.ReadFile("theme/default.json") }
