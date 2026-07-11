# 代码审查报告 — Phase 4（UI 交互 / 发布前打磨，ADR-08 降级延续）

> 审查日期：2026-07-11 ｜ 审查者：CodeReviewExpert
> 审查对象：Phase 4 提交 `a69f908`（发布前打磨 + 修复 app.Run 退出死锁）及工作区未提交改动
> （`internal/ui/hittest.go`、`internal/ui/animator/`、`internal/app/app.go`+`app_test.go`，
> `internal/calendar/calendar.go`、`internal/platform/win32/{window.go,window_windows.go}`、
> `internal/ui/calendar_view.go`）。
> 对照：`docs/90-UI/*`、`docs/ADR-08-降级脱离gogpu上游阻塞.md`、`Issue #146`。

---

## 0. 摘要

**质量评级：B+（产品代码 A-；1 个 🔴 = 新增集成测试未通过，套件非绿，阻断「Phase 4 完成」宣告）。**

Phase 4 在 Phase 3 之上稳健扩展了**鼠标交互**：点击命中测试（#113）+ 月导航/选中/今天按钮（#114）+ 选中驱动重渲，并把退出死锁修复、每日精确跨午夜刷新（替代 30min 轮询）一并落地。产品代码质量高——单写者铁律守住、双循环纪律到位、绘制/命中几何单一来源、DPI 坐标换算正确、零 CGO/vet/build 全绿。

但**提交态并非全绿**：新增集成测试 `TestRun_ClickNavigatesAndSelects` 失败。根因是测试 setup 把「显示月」与「今天」混淆，产品 `GoToToday` 实现是正确的——修复在测试侧。按本仓前几轮「build/vet/test 全绿」的硬门槛，此 🔴 不修不能宣布 Phase 4 完成。

---

## 1. 验证事实（工具实证，非推测）

| 项 | 结果 |
|---|---|
| `go build ./...` | ✅ BUILD_OK |
| `CGO_ENABLED=0 go build ./...` | ✅ CGO_OK（零 CGO 守住，ADR-06） |
| `go vet ./...` | ✅ VET_OK |
| `go test ./...` | ❌ **RED**：`internal/app` 的 `TestRun_ClickNavigatesAndSelects` 失败（见 🔴 B1）；`ui`(87.9%) / `calendar`(86.0%) / `animator`(91.4%) 绿；`win32` 真实窗口套件（慢）沿 Phase 3 基线 83.4%+ |
| 依赖方向（ADR-07a） | ✅ `ui` 仅 `calendar`+`theme`+`gg`；`app` 为组合根；`animator` 零依赖；全仓**无** `gogpu` 主仓/`gogpu/ui`/`wgpu` 源码引用（ADR-08 干净） |

---

## 2. 🔴 Blockers（必须修）

### B1 — 新增集成测试 `TestRun_ClickNavigatesAndSelects` 未通过（套件 RED）

失败点：`internal/app/app_test.go:427`

```
after today click selected = 2026-07-11 11:48:11 ..., want 2026-07-09 12:00:00
```

**根因（测试 setup bug，非产品 bug）：**

测试用 `calendar.WithSelected(time.Date(2026,7,9,12,0,0,0,time.Local))` 固定了**显示月**（7 月），
但断言「今天」按钮点击后 `SelectedDate() == 2026-07-09`——即它期望 `GoToToday()` 回到**初始选中日**。
而 `GoToToday()` 正确返回**真实系统今天**：

```go
// internal/calendar/calendar.go:213
func (s *calendarService) GoToToday() { s.selected = s.todayDate() }
// todayDate() → todayFixed 时返回固定值，否则 time.Now()  → 2026-07-11
```

月断言（`Month == initialMonth`）恰好通过，是因为 `2026-07-11` 也在 7 月——**掩盖了 `selected` 的错误**，
使这个 bug 只在 `SelectedDate()` 精确比较时才暴露。

**为什么产品代码是对的：** 「今天」按钮的语义就是跳到真实今天并选中今天；`GoToToday` 实现正确。

**修复（改测试，勿改产品）：** 测试应像固定显示月一样固定「今天」——把

```go
calendar.NewDefaultCalendarService(nil, calendar.WithSelected(time.Date(2026,7,9,12,0,0,0,time.Local)))
```

改为**同时** `calendar.WithToday(time.Date(2026,7,9,12,0,0,0,time.Local))`。
这样 `todayDate()` 返回固定的 `2026-07-09`，「今天」点击 → 7 月 + 选中 `2026-07-09`，断言全绿且确定性。

> ⚠️ **警告**：此红测试极易诱使工程师去改 `GoToToday` 让它返回 `s.selected` 来「让测试变绿」——
> 那会**破坏真实功能**（点击「今天」不再跳到今天）。**请勿动产品代码，只修测试 setup。**

**影响：** Phase 4 自述「已完成」但套件非绿，按本仓规范不可宣布 done，须先修。

---

## 3. 🟡 Suggestions（应修）

### S1 — `win32Window.onClick` 跨 goroutine 无同步写读（与本项目自有标准不一致）

`window_windows.go:513` `OnClick(fn)` 在**主 goroutine**写入 `w.onClick`——而它在 `app.go:245` 被调用，
**晚于** `newNativeWindow` 中的 `go w.run()`（`window_windows.go:229`）。`wndProc` 在**窗口线程**
（`WM_LBUTTONDOWN`，`:399-408`）读取 `w.onClick`。

- 本仓 Phase 3 因同样的跨 goroutine 字段问题，把 `pendingRect/pendingBmp` 改为 `atomic`（B1 修复）；
  但 `onClick` 用了普通函数字段，**标准不一致**。
- 实践上无碍：OS 消息队列在 `OnClick` 注册（setup）与用户点击（远晚）之间提供 happens-before。
- 但本仓**禁用 `-race`（CGO_ENABLED=0）**，此类隐患 CI 永远看不见，只能靠设计规避。

**建议：** `w.onClick` 用 `atomic.Pointer[func(int,int)]`；或改在 `newNativeWindow` 经 `Options` 注入，
于 `go w.run()` 前固化（与 `w.dpi` 同款安全模式，见 S1 下方 N1 关联）。约 2 行改动。

### S2 — `docs/90-UI/Animation.md` 文档漂移（承接前几轮 S1 类）

- §9 `Animator` API：`NewAnimator()`/`Play`/`Stop`/`Tick(now) bool`/`Spec{FromY,ToY,FromAlpha,ToAlpha,OnFrame,OnDone}`、
  `Easing func(float32) float32` —— 与实现 `internal/ui/animator`（`New(Spec)`/`Start`/`Tick(now)(float64,bool)`、
  无 `Play/Stop/OnFrame/OnDone`、`Easing func(float64) float64`、额外 `KindNone`）**不一致**。
- §10 v1.0 T2 承诺「app.Run 显隐接入 `KindFadeSlideIn`」被代码**显式推迟到 v1.2+**（animator.go 注释 #121/#122）。

实现是刻意精简的占位（注释清楚），但文档承诺未兑现，属前几轮反复出现的「设计文档 §9 超前于落地代码」漂移。

**建议：** 像前几轮那样做 doc-sweep，把 §9/§10 标注为「v1.2+ 预留，MVP 不接入显隐」。

### S3 — `internal/ui/animator` 是死代码（未被任何包 import）

`grep -rln "ui/animator" --include=*.go internal cmd` 为空。包本身干净、零依赖、测试完善（91.4%），
但当前无人使用。

**建议：** 要么在 v1.2 真正接线时再引入（YAGNI，先删）；要么保留占位但明确「仅类型预留」。
倾向：**现在删**，v1.2 需要时再按当时对齐后的 API 重写（届时 Animation.md 也已 sweep）。

### S4 — 退出死锁修复依赖缓冲大小（健壮性质疑）

`app.go:215` `cmdCh` 缓冲 16，`platform.SendCommand` 非阻塞（`select-default`）。
注释说明：若缓冲=1，theme-watch 的 `CmdRender` 占满后 `CmdQuit` 被静默丢弃→主循环收不到退出→死锁；
缓冲 16 可容纳一次性突发（点击+主题变更+每日刷新）。这是**缓解**而非**根治**。

**建议：** 给 `CmdQuit` 走**阻塞发送**或独立 `quitCh`（unbuffered，保证必达），不再依赖缓冲 sizing。
当前 16 在真实负载下足够安全，标为低优先级 🟡。

---

## 4. 💭 Nits

- **N1（DPI）**：`wmDpiChanged`（`window_windows.go:415-434`）按新 DPI 重建 DIB 并重新锚定，
  但**未刷新 `w.dpi`**。若窗口被拖到不同 DPI 的显示器后用户点击，点击坐标用创建期旧 DPI 反算→轻微偏移。
  建议 handler 内 `w.dpi = newDPI`（顺带把 S1 的 `onClick` 一并改为创建期注入更彻底）。
- **N2（UX）**：`HitTest` 不判断 `InMonth`，点击上/下月补白灰字格会 `SetSelectedDate` 到相邻月（并跳月）。
  多数日历 App 对此类点击是「导航」而非「选中」。MVP 可接受，记录供 v1.1 决策。
- **N3/N4（文档）**：`Easing` 类型 `float64` vs 文档 `float32`、`Kind` 枚举含 `KindNone` 且 iota 起点不同——
  均属 S2 文档 sweep 一并处理。

---

## 5. 👍 亮点

- **单写者铁律守住**：`handleClick`（`app.go:183`）与 `applyConfigCommand` 同在主循环；
  窗口线程 `OnClick` 回调仅经 `clickCh` **非阻塞投递坐标**，绝不直改业务状态（ADR-02）。
  `s.selected`/`s.today` 全部主循环写，无并发竞争。
- **绘制/命中几何单一来源**：`computeNav`（`hittest.go`）同时被 `CalendarView.Draw`（`calendar_view.go:56`）
  与 `HitTest` 使用，表头/导航/网格布局不可能漂移——正是前几轮强调的「画在哪点哪一致」最佳实践，落地干净。
- **DPI 坐标换算正确**：`lx = px*96/dpi`（`window_windows.go:405`）与 `scaleLogical(opts.Width,dpi)` 互为逆，
  逻辑坐标与 `ui.HitTest`/渲染基准（96-DPI）对齐。
- **集成测试已就位（只差 WithToday）**：`TestRun_ClickNavigatesAndSelects` 端到端覆盖
  点击→HitTest→月导航/选中→重渲，是 Phase 3「真机烟测补 B1」精神的延续。
- `win32Window` 的 `dpi`/`lastTray` 同步模式（`go` happens-before / `SendMessage` 同步）正确；
  退出 `Quit()` 阻塞等待消息泵（N1 既往修复）保持；每日刷新改为精确 `nextMidnight`（`app.go:317`，`AddDate` 避夏令时）。
- 构建/零 CGO/vet 全绿；animator 包纯函数 + 完备测试（91.4%）。

---

## 6. 遗留 / 跨阶段

- **Phase 2 发布门 S5**：`2026.json` 仍是占位 SEED，v1.0 前须换真实 holiday-cn 数据；
  `joinDate` 无效日期校验仍未做（非 Phase 4 范围，但仍是 v1.0 门禁）。
- **Phase 3 全部闭环确认**：B1/S1–S6/N1–N3 在 Phase 3 修复复核中已 100% 闭环，Phase 4 在其上稳健扩展。
- 🔴 B1 是当前唯一阻断项；S1–S4 为发布前建议（S1/S2/S3 尤建议清掉，与历史 drift 一起治理）。
