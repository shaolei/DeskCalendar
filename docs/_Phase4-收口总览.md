# Phase 4 收口总览（代码审查核验 + 修复 + 关闭 issue）

> 日期：2026-07-11 ｜ 提交：`5004be2` ｜ 审查报告：`docs/_代码审查_Phase4.md`

## 核验结论
对 `docs/_代码审查_Phase4.md` 逐项实证，**6 项全部属实**：

| 报告项 | 核验 | 说明 |
|---|---|---|
| 🔴 B1 集成测试失败 | ✅ 属实 | 测试仅 `WithSelected` 未固定 today，`GoToToday` 返回真实今天 ≠ 期望 |
| 🟡 S1 `onClick` 跨 goroutine 竞争 | ✅ 属实 | 主 goroutine 写、窗口线程读，无同步 |
| 🟡 S2 Animation.md 漂移 | ✅ 属实 | §9/§10 仍描述旧 API 与 v1.0 接入显隐 |
| 🟡 S3 animator 死代码 | ✅ 属实 | 全仓无 import（但 #122 刻意占位） |
| 🟡 S4 cmdCh 缓冲 16 缓解死锁 | ✅ 属实 | 报告自认真实负载安全 |
| 💭 N1 `wmDpiChanged` 未回写 `w.dpi` | ✅ 属实 | 换屏后点击坐标用旧 DPI 反算偏移 |

## 修复清单（提交 `5004be2`，12 文件 +858 / -89）
- **B1**：`app_test.go` `TestRun_ClickNavigatesAndSelects` 补 `WithToday(2026-07-09)`（**测试侧**，产品 `GoToToday` 不动）→ 测试 PASS。
- **S1**：`win32Window.onClick` 改 `atomic.Pointer[func(int, int)]`（Load/Store），消除跨 goroutine 竞争，与 `w.dpi` 固化模式互补。
- **N1**：`wmDpiChanged` 内 `w.dpi = newDPI`（同窗口线程，安全）。
- **S2**：`docs/90-UI/Animation.md` §1/§2/§8/§9/集成示例/§10 全对齐 `internal/ui/animator` 现实（包名 `animator`、`New/Start/Tick(float64,bool)/Active`、`Easing float64`、`Kind` 含 `KindNone`、`Spec{Duration,Easing,Kind}`），标注 **v1.2+ 预留、v1.0 不接入显隐**。

## 处置决定
- **S3 保留**：`internal/ui/animator` 死代码属 #122 刻意「类型占位」（Plan C 要求），不删。
- **S4 留作 🟡 follow-up**：cmdCh 缓冲 16 为缓解，非根治；报告自认真实负载下安全，未引入独立 quitCh。

## 验证
- `CGO_ENABLED=0 go build/vet/test ./...` **全绿**（含 `internal/app`、`internal/ui/animator`）
- `GOOS=windows CGO_ENABLED=0 go build ./...` **通过**

## 关闭的 13 个 Phase 4 子 issue
`#106 #107 #108 #109 #111 #112 #113 #114 #116 #117 #118 #121 #122`
（保留 OPEN：`#119` SettingsView 独立窗 v1.3 后备；`#123` 视觉润色路线 v1.1+）

## 遗留
- **Phase 2 发布门 S5**：`2026.json` 仍是占位 SEED，v1.0 前须换真实 holiday-cn 数据。
- **S4**（退出死锁根治）：低优先级 follow-up。
- **是否推送 GitHub**：待用户确认（SSH 22 被墙 → HTTPS+gh）。
