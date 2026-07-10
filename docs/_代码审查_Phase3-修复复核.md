# 代码审查 · Phase 3 修复复核（ADR-08 降级方案）

> 复核对象：`DeskCalendar` Phase 3 修复后代码（提交 `a53e107` … `a542b79`）
> 复核基准：`docs/_代码审查_Phase3.md` 中的 1🔴 + 6🟡 + 3💭 + 3 附加项
> 复核日期：2026-07-11
> 方法：工具实证（build / vet / zero-CGO / test / cover / go list / grep）+ 逐文件精读 + 回归测试核对

---

## 一、总体结论

**质量评级：A（较首轮 A- 上调）。Phase 3 首轮发现的全部缺陷已闭环，且每一项都有对应的回归测试守护。无新引入的 🔴 blocker。**

首轮报告 11 项问题（B1 / S1–S6 / N1–N3 / WindowStyle drift）现已 **100% 关闭**。其中 8 项由真实或专项回归测试直接固化（B1、S2、S3、S4、S5、N1 有 Windows 门控真机/专项测试；S2、S4 另有纯单测）。修复不是「补丁式」的，而是把设计漏洞从源头堵死（B1 是跨线程通道缺 `Store`；S1 是并发写缺乏单写者约束），并配测试防回潮。

**验证事实（本轮实测，非推测）**

| 项 | 结果 |
|---|---|
| `go build ./...` | ✅ 通过 |
| `CGO_ENABLED=0 go build ./...` | ✅ 通过（零 CGO 守住，ADR-06） |
| `go vet ./...` | ✅ 通过 |
| `go test ./...` | ✅ 全绿（含 Windows 门控真机测试，本机交互窗口站下真实运行） |
| 依赖方向 `ui` | ✅ 仅 `gg` / `calendar` / `theme` + stdlib；**不** import `app`/`win32`/`shell`/`settings`（ADR-07a） |
| ADR-08 源码符合 | ✅ 全仓**零 code import** 到 `gogpu` 主仓 / `gogpu/ui` / `gpucontext` / `webgpu`（`grep` 命中仅限注释） |
| 整体覆盖率 | **74.7%**（首轮 64.0%，越过 ≥60% 目标） |
| `win32` 覆盖率 | **83.4%**（首轮 16% — 真机烟测直接拉起，B1 修复的最大受益者） |
| `ui` / `app` / `shell` / `settings` / `calendar` / `theme` | 84.3% / 78.0% / 100% / 75.0% / 87.5% / 71.5% |

---

## 二、修复闭环逐项核对

| # | 首轮问题 | 修复提交 | 修复方式（实证） | 回归测试 |
|---|---|---|---|---|
| 🔴 **B1** | `win32Window` 跨线程 `pendingRect`/`pendingBmp` 只 `Load` 未 `Store` → 真机 `AnchorAboveTray`/`Present` 为空操作（弹在 (0,0) + 永远空白） | `a53e107` | `AnchorAboveTray`（window_windows.go:471）与 `Present`（:481）补 `.Store()`；`wndProc` 的 `wmUserAnchor`/`wmUserPresent` 分支 `Load` 后消费并写 `lastTray`/`lastBmp`。堆拷贝 `new(image.Rectangle)` 保证生命周期 | `TestWin32Window_AnchorAndPresentReachWindowThread`、`TestWin32Window_RenderAndPresentFullPipeline`（真机 DIB 非中性灰底断言） |
| 🟡 **S1** | `settings` 托盘回调在 systray goroutine 直改 `config`/`theme`，midnight goroutine 改 `calendar.today`，`render()` 在 3 goroutine 读 —— 非原子并发读写（且零 CGO 使 `-race` 不可用） | `df9dc81` | **单写者模式**：`app.Run` 内 `applyConfigCommand`（app.go:120-171）是**唯一**写 `opts.Config`/`opts.Theme`/`opts.Calendar`/`opts.Startup` 之处，仅由主循环 select 调用；`settings.BuildTrayMenu` 回调只 `SendCmd`（settings.go:78/83/88/93…）；theme-watch 与 midnight goroutine 仅 `SendCommand` 投命令，绝不直读共享态 | `lifecycle_test` / `app_test` 既有；并发契约由代码审阅确认（单一写者路径） |
| 🟡 **S2** | 网格 `WeekStart=Monday`（列0=周一），UI 表头写死周日首「日一二三四五六」→ 表头与网格错位一天 | `a53e107` | `Model.Weekdays = rotateWeekdays(grid.WeekStart)`（model.go:63），表头按 `grid.WeekStart` 旋转，与 `GenMonthGrid` 同列对齐；任意周首都正确 | `TestNewMonthModel_HeaderFollowsWeekStart`（Sunday/Monday/Saturday 三态遍历，逐列断言表头==首行格星期）+ 真机管道测试 |
| 🟡 **S3** | 显示后系统 reclaim 焦点，首个 `WM_ACTIVATE(WA_INACTIVE)` 即自隐藏 → 「点托盘闪一下就没」 | `d2d6710` | `wmUserShow` 显式 `AllowSetForegroundWindow(ASFW_ANY)` + `SetForegroundWindow` 抢前台，并将 `activated` 置 0；`wmActivate` 仅当 `activated==1` 才在 `WA_INACTIVE` 下自隐藏（window_windows.go:339-377） | `TestWin32Window_S3_ActivateGuard`（模拟「显示即失焦」「激活后失焦」「再显示重置」三序列） |
| 🟡 **S4** | `applyFont` 每次 `LoadFontFace` 重读 ~20MB `msyh.ttc`，单次 `Render` ~86 次 → 巨大 I/O | `140141d` | `resolveCJKFont` 缓存 `cjkFontSrc`（font.go:38-56），`applyFont` 复用 `src.Face(points)` 不再读盘；包 `loadFontSource` seam 供测试注入计数 | `TestApplyFont_ReadsFontFileOnce`（计数 seam 断言 86 次调用仅解析 1 次）+ `TestApplyFont_NoPanicWhenNoFont` |
| 🟡 **S5** | `destroy`/`createDIB` 删除仍被 `memDC` 选中的 GDI 位图 → Win32 未定义行为，跨 DPI 累积泄漏 | `83666f2` | `createDIB` 先 `selectObject` 新位图把旧位图「顶出」再删；`destroy` 删 `hbmp` 前先 `selectObject(memDC, origBmp)` 顶出（window_windows.go:277-328） | `TestWin32Window_S5_DIBLifecycle`（注入 `deleteObject` seam，每次删除断言对象已不在 `memDC` 选中） |
| 🟡 **S6** | 多 `win32Window` 共用同一 `RegisterClassExW` 类名 → `wndProc` 槽被首实例占，第二窗消息误派发 + DIB 已释放 → 崩溃 | `d2d6710` 前序 + 实测 | `classSeq int64` + `atomic.AddInt64` 给每实例唯一类名 `DeskCalendarWin32_%d`（window_windows.go:150/229） | 静态审查确认；单实例场景下无回归（多实例路径由类名隔离保证） |
| 🟡 **N1** | `Quit` 仅 `PostQuitMessage` 不阻塞 → 窗口消息泵 goroutine 泄漏至进程退出 | `7d301fe` | `Quit()` 经 `SendMessage(WM_DESTROY)` → `postQuitMessage` → run 退出 → `destroy()` + `close(done)`；`Quit()` 阻塞 `<-w.done`（window_windows.go:457-463） | `TestWin32Window_N1_QuitStopsMessagePump`（3s 超时断言 goroutine 退出 + `done` 关闭） |
| 🟡 **N2** | `cmd/` 入口零测试（覆盖 0%） | `4a89074` | 抽 `buildOptions` + 全 fake 集成测试，`cmd` 覆盖 0% → 55% | `cmd/deskcalendar` 集成测试 |
| 🟡 **N3** | Phase 3 文档与 ADR-08 现实漂移（路径 D 描述滞后） | `9bf9f9a` | 全量对齐 ADR-08 现实（自拥 win32 普通弹窗 / gg / 弃 gogpu.ui） | 文档审阅 |
| 🟡 **WindowStyle drift** | `internal/platform/windowstyle` 代码层与 ADR-08 措辞不一致 | `a542b79` | 代码层 drift 对齐 ADR-08（注释/枚举与降级现实一致） | — |

> 注：首轮报告末尾的 3 个 💭 nit 实质已随上述修复一并消解或降为非问题（见第四节）。

---

## 三、架构层面的两点关键确认

### 1. 单写者约束（S1）是「结构正确」而非「补丁正确」
修复后的并发模型清晰：**所有跨 goroutine 共享状态的读写都收口于 `app.Run` 主循环**。`tray` 回调、`theme.Watch`、`midnight ticker` 三个后台源全部只往 `cmdCh` 投命令（`SendCommand` 非阻塞 select，绝不会在后台 goroutine 阻塞或泄漏 —— 见 `platform/tray.go:58`）。`render()` 仅由主循环调用（`app.go:114/168/246`）。这意味着即便 `go test -race` 因零 CGO 不可用而无法跑，**数据流本身不存在数据竞争** —— 这正是本项目的正确规避方式（用设计消弭而非靠工具发现）。

附带收益：Phase 2 遗留的 S4（`calendar.today` 跨午夜陈旧）也借此闭环 —— `CmdRefreshToday` 由主循环调用 `Calendar.RefreshToday()`，today 基准不再长期陈旧（`app.go:150-153, 216-228`）。

### 2. B1 的真机烟测填补了 ADR-08 行动项 #5 的缺口
首轮已指出：早前的像素验收 harness（`cmd/accept`）走 `ui.Render→PNG` 直出，**绕过了 `win32Window.Present`/`AnchorAboveTray`**，故漏掉 B1。本轮新增的 `window_windows_test.go` 直接构造真实 `win32Window`、经 `SendMessage` 派发到窗口线程、断言 `lastTray`/`lastBmp` 被写入、DIB 被日历位图覆盖 —— 真正覆盖了「真实窗口线程显隐 + 锚定 + 推送」路径。这是本次修复质量的最高光点。

---

## 四、残余项 / 💭 轻微建议（均非阻塞）

1. **💭 真机烟测在非交互窗口站会 Skip**：`window_windows_test.go` 全部以 `if wc.hwnd == 0 { t.Skip(...) }` 兜底（headless CI / 服务会话无交互窗口站时跳过）。本机（有交互窗口站）真实运行，覆盖充分；但若将来接入无头 CI，这些最有价值的测试将静默 Skip，无法在 CI 兜底 B1 类回归。建议：发布流水线确保有一台带交互会话的 Windows runner，或在 CI 显式标注「win32 真机测试需交互窗口站」。

2. **💭 `present()` 持有 `w.lastBmp` 指针的隐式所有权契约**：`present()` 把调用方缓冲存为 `lastBmp` 供 DPI 变化时重绘，契约依赖「`ui.Render` 每次返回全新缓冲、调用方不重用/释放」。当前成立（有注释说明），但属于跨包隐式约定，无编译期保障。可考虑在 `WindowController.Present` 文档中显式标注所有权（调用方在下次 `Present` 前不得复用该 `*image.RGBA`）。

3. **💭 midnight ticker 粒度 30 分钟**（`app.go:217`）：跨午夜后「今天」高亮最多可能延迟 30 分钟纠正。对托盘日历弹窗可接受；若追求精确，可改为「每日 00:00 触发一次」的定时器。属体验微调，非缺陷。

---

## 五、与历史阶段的连贯性

- **Phase 0 / 1 / 2 遗留项连续性**：Phase 1 的 S1（文档 drift sweep）/ S2（测试补强）/ S3（Startup 归一化）已在当时落地；Phase 2 的 S2–S7（theme 逻辑、today 陈旧、SEED、Builtin、闰月、doc-sweep）本轮确认全部已闭环。
- **review → fix 闭环有效**：连续四轮（P0→P3）审查均呈现「发现问题 → 提交修复 + 回归测试 → 复核关闭」的良性节奏，工程师对 🔴/🟡 的响应质量高、且普遍补了防回潮测试，而非仅做最小修补。这是项目目前质量稳定的根本原因。

---

## 六、结论与下一步

**Phase 3（ADR-08 降级方案）修复已通过复核，质量达 A 级，可放心进入 Phase 4（UI 细化 / v1.0 打磨）或发布准备。**

建议的下一步（按优先级）：
1. **发布前必做**：把 `2026.json` 占位 SEED 换成真实 holiday-cn 数据（Phase 2 S5 遗留，属发布门），并在 `joinDate` 处加无效日期校验。
2. **CI 加固**（💭#1）：确保 Windows 交互窗口站 runner，让 win32 真机烟测在 CI 真正执行。
3. 余下 💭 为体验/文档微调，可随 Phase 4 一并处理。

---

## 七、Phase 4 实施闭环（2026-07-11）

按第六节「建议的下一步」逐条收口，发布门遗留全部清零，文档/timing 微调闭合。

| # | 来源 | 内容 | 改动文件 | 验证 |
|---|---|---|---|---|
| **P4-1** | 发布门（Phase 2 S5） | `2026.json` 占位 SEED → **真实 holiday-cn 数据**（NateScarlet/holiday-cn, MIT）：33 休 + 6 补班，国庆补班 `09-20`（非 `09-26`），`_comment` 标源 gov.cn + fetch 日期 | `internal/calendar/embed/holidays/2026.json` | 新增 `TestEmbedHolidayRepository_Real2026Schedule` 锁真值；`TestEmbedHolidayRepository_Refresh` 期望名 `"国庆节"` 对齐 |
| **P4-2** | 发布门 | `joinDate` 无效日期校验 —— 核查 `holiday_embed.go:107-116` **已实现** MM/DD 区间 + 往返归一化拒绝（`TestJoinDate_RoundTrip` 覆盖 `02-30`/`13-01`/`02-32`/`0222`），确认早已闭环，**无需改动** | — | 免改，已覆盖 |
| **P4-3** | 💭#2 | `WindowController.Present` 显式标注所有权契约：`Present` 返回后、下次 `Present` 前调用方不得复用/修改/释放该 `*image.RGBA`（窗口线程持 `lastBmp` 指针供 DPI 重绘）；`ui.Render` 每次返回全新缓冲，满足契约 | `internal/platform/win32/window.go` | 注释契约，编译通过 |
| **P4-4** | 💭#3 | midnight ticker 30min 粒度 → **每日 00:00 精确触发**：新增纯函数 `nextMidnight(now)` + `time.NewTimer(time.Until(nextMidnight))`，到点仅 `SendCommand(CmdRefreshToday)` 由主循环重渲 | `internal/app/app.go` | 新增 `TestNextMidnight` 4 用例（13:40 / 00:00 / 23:59:59 / 年末跨年） |
| **P4-6** | 文档 drift（Phase 0 N2） | `Signal.md` 去全部 `Observe(old,new)` 漂移，对齐真实 `coregx/signals@v0.1.0` API（`New[T]` / `Get`/`Set`/`Update`/`AsReadonly` / `Subscribe(ctx,fn)` / `SubscribeForever(fn)`）；`DataFlow.md`/`Store.md` 同步 `Observe→Subscribe` | `docs/30-State/Signal.md`、`DataFlow.md`、`Store.md` | `grep -rn Observe docs/30-State/` = CLEAN |
| **P4-5** | 💭#2 文档 | 归并入 P4-3（同一所有权契约） | — | — |
| **P4-7** | 验证期发现 · `app.Run` 退出死锁（**既有缺陷，非 P4 回归**） | `cmdCh` 缓冲 1 → 16。根因：主循环每处理一条命令后同步 `render()`（慢操作，期间不在 `select` 接收）；`SendCommand` 非阻塞发送，若缓冲被 theme-watch 的 `CmdRender` 占满，`退出` 的 `CmdQuit` 被**静默丢弃** → 主循环永不收到退出命令而死锁。高负载（全量 `./...` 并行 / 机器繁忙）下 `render` 越慢越易触发，隔离低负载下偶发难现。修复：`internal/app/app.go` 缓冲提到 16，容纳一次性命令突发，保证 `CmdQuit` 必达 | `internal/app/app.go` | 全量 `CGO_ENABLED=0 go test ./...` 由死锁（120s 超时 FAIL）转为全绿；`TestRun_Integration_AllFakesMenuQuit` 隔离 20× 复现此前亦死锁，修复后稳定通过 |

**未处理项**
- **P4-CI（💭#1 交互式 Windows runner）**：低优先级，可延后至发布流水线搭建时一并处理；本机（有交互窗口站）真机烟测已真实运行兜底，非阻塞。

**验证结论**
- 改动包 `calendar` + `app`：`CGO_ENABLED=0 go test` 各自全绿。
- 全仓 `go build / vet / test`（零 CGO）**全绿**：`CGO_ENABLED=0 go test -timeout 120s ./...` 通过（含 Windows 真机门控测试，无窗口站则 Skip）。
- **P4-7 退出死锁修复已验证**：同一全量命令由死锁（120s 超时 FAIL）转为全绿；该 cmd 集成测试隔离 20× 复现此前亦死锁，修复后稳定通过。
- Phase 4 不引入新 🔴；发布门（真实节假日 + 无效日期校验）已闭环；顺带消解一处既有退出竞态。
