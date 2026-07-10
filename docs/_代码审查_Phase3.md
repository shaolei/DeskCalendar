# 代码审查报告 · Phase 3（Shell 装配 / 路径 D 降级）

- **审查对象**：Phase 3 五个切片（`7f65bbc` 平台自拥弹窗、`1355237` 生命周期状态机、`a196d87` 双循环装配、`33e0a98` 托盘右键菜单、`ca5be92` 90-UI 渲染层）
- **审查依据**：`docs/ADR-08-降级脱离gogpu上游阻塞.md`、`docs/Phase3-重排计划.md`、Issue #146、各包 `§9` 设计文档
- **审查日期**：2026-07-10
- **审查方式**：逐文件精读 + 工具实证（`go build` / `CGO_ENABLED=0 go build` / `go vet` / `go test -cover` / `go list` 依赖方向 / 源码 grep）
- **质量评级**：**A-**（1 个 🔴 代码层 blocker，6 个 🟡，3 个 💭；降级方案 ADR-08 真正落地，架构纪律总体到位）

---

## 1. 总览

Phase 3 把「渲染 + 窗口」这一层从 gogpu/wgpu 切到了 **gg（CPU 光栅）+ 自拥 win32 普通弹窗**，并完成了双循环装配、托盘菜单承载设置、gg 渲染日历面板。ADR-08 的降级目标**实质性达成**：最终二进制不依赖 gogpu 主仓 / `gogpu/ui` / wgpu，依赖图大幅瘦身，零 CGO 离线构建成立。

测试与构建事实过硬：全绿、`go list` 依赖方向干净、`ui` 仅依赖 `calendar`+`theme`+`gg`、`app` 作为组合根、`win32` 完全隔离。

但有两个**被测试掩盖的真实缺陷**需在发布前修掉：

1. **🔴 B1（功能损坏）**：`win32Window` 的跨线程数据通道 `pendingRect`/`pendingBmp` 只 `Load` 从未 `Store` —— `AnchorAboveTray` 与 `Present` 在 Windows 真机上**是空操作**，窗口会弹在 (0,0) 且永远空白（不锚定、不绘日历）。测试因注入 fake 而没覆盖到真窗口，故漏网。
2. **🟡 S2（用户首屏可见）**：数据网格按 `WeekStart=Monday` 生成（列 0 = 周一），但 UI 表头写死周日为首的 `日一二三四五六` 且按列 `c` 绘制 —— **表头与网格列错位一天**。

另有并发共享状态无同步（🟡 S1，且因零 CGO 无法 `-race` 检测，必须由设计规避）、窗口激活/失焦、字体重复加载等应修项。详见下文。

**结论**：降级路线正确、架构干净，**修掉 B1 + S2 即可视为可发布到真机烟测**；S1（并发）建议发布前改为单写者设计。（S1 已于后续提交以单写者设计闭环，见 §10 已修复项）

---

## 2. 验证事实（工具实证，非推测）

| 项 | 结果 |
|---|---|
| `go build ./...` | ✅ BUILD_OK |
| `CGO_ENABLED=0 go build ./...` | ✅ CGO_OK（零 CGO 离线构建成立） |
| `go vet ./...` | ✅ VET_OK |
| `go test ./...` | ✅ 全包通过 |
| `go test -race` | ⚠️ **不可行**：`-race requires cgo`；本项目零 CGO（ADR-06）禁用 → 并发缺陷无法被 CI 检出（见 S1） |

**覆盖率（Phase 3）**

| 包 | 覆盖 | 备注 |
|---|---|---|
| internal/shell | 100.0% | 状态机完备 |
| internal/settings | 84.4% | 菜单接线完备 |
| internal/app | 80.2% | 双循环装配完备 |
| internal/ui | 82.7% | 渲染 + 模型映射 |
| internal/calendar | 87.5% | 域聚合根 |
| internal/theme | 71.5% | 含 Phase 2 S2/S3 修复 |
| internal/platform | 37.1% | 多为无头不可测 OS 胶水 |
| internal/platform/win32 | **16.0%** | 真实窗口胶水无头不可测（fake+blitScaled 已覆盖）；**B1 即藏于此未覆盖区** |
| internal/infra/config | 74.4% | |
| internal/state | 82.6% | |
| internal/plugin | 66.7% | |
| cmd/deskcalendar | 0.0% | 仅 main 装配（见 💭 N2） |
| **整体** | **64.0%** | ≥60% 目标达成（较 Phase 2 的 70.6% 略降，受 cmd/win32 新增低覆盖拖累） |

**ADR-07a 依赖倒置铁律（go list 实证）**

- `internal/ui` → `[gg, calendar, theme, image, os, strconv, sync, time]` —— **不 import app/win32/shell/settings** ✅
- `internal/app` → 组合根，依赖全部子包，无反向依赖 ✅
- `internal/platform/win32` → `[platform, x/sys/windows, image, sync/atomic, unsafe]` —— 零业务依赖、零 gg ✅
- `grep` 全仓 `internal/cmd`：**无任何对 `gogpu/gogpu` / `gogpu/ui` / `gpucontext` / `wgpu` 的 code import**（仅注释提及）。`go.mod` 中 `gg`/`systray` 为 direct（本地 replace），`wgpu` 根本未进 go.mod（gg 的 CPU 光栅路径不触碰 GPU 后端）。**ADR-08「二进制层甩掉 wgpu」成立** ✅

---

## 3. 🔴 Blockers（必须修）

### B1　`win32Window` 跨线程数据通道断裂 —— `AnchorAboveTray`/`Present` 在真机是空操作

**位置**：`internal/platform/win32/window_windows.go`
- 声明：`pendingRect atomic.Pointer[image.Rectangle]`（L153）、`pendingBmp atomic.Pointer[image.RGBA]`（L154）
- 消费：`wmUserAnchor` 分支 `if r := w.pendingRect.Load(); r != nil { w.anchor(r) }`（L295）、`wmUserPresent` 分支 `if b := w.pendingBmp.Load(); b != nil { w.present(b) }`（L300）
- **写入：全文件无 `.Store()` 调用**（grep 确认 L295/L300 仅 `Load`）
- 发送侧：`AnchorAboveTray`（L392-394）与 `Present`（L396-401）只把指针塞进 `SendMessage` 的 `lparam`，而 `wndProc` 根本不读 `lparam` → 数据从未到达窗口线程

**Why（后果）**：`Show()`/`Hide()` 走 `wmUserShow`/`wmUserHide` 在 `wndProc` 内直接处理（不需 pending 数据），所以**显示/隐藏可用**；但 `AnchorAboveTray`（L392）与 `Present`（L396）经 `wmUserAnchor`/`wmUserPresent` 派发后，`wndProc` 读到 `nil` 的 pending → **不锚定、不绘缓冲**。真机表现：点托盘 → 窗口在屏幕左上角 (0,0) 弹出、**尺寸 360×480 但底色恒为 (250,251,252) 空白**、不跟随托盘、不显示日历。相当于**首屏功能损坏**。

**Why（为何漏测）**：`newNativeWindow` 在 Windows 起真实窗口线程；但 `app`/`shell`/`win32` 测试全部注入 `fakeWindow`，真 `win32Window` 从未被单测执行，故该分支 0% 覆盖、漏网。这正是 ADR-08 行动项 #5「真机烟测」要抓的漏洞。

**Suggestion（最小修复）**：发送前 `Store`，`wndProc` 已正确 `Load` 消费（`SendMessage` 同步，指针在返回前已被窗口线程读走，生命周期安全）：

```go
func (w *win32Window) AnchorAboveTray(r image.Rectangle) {
    w.pendingRect.Store(&r)                  // ← 补这行
    sendMessage.Call(uintptr(w.hwnd), wmUserAnchor, 0, 0)
}

func (w *win32Window) Present(b *image.RGBA) {
    if b == nil {
        return
    }
    w.pendingBmp.Store(b)                     // ← 补这行
    sendMessage.Call(uintptr(w.hwnd), wmUserPresent, 0, 0)
}
```

（`lparam` 可保留 0，因 `wndProc` 读的是 atomic。）修后 `win32` 包可补一个「构造真窗口→`AnchorAboveTray`→断言 `lastTray` 被记录、`Present`→断言 `lastBmp` 被记录」的集成测试（须在 Windows CI 跑，无头环境用 fake 已覆盖纯逻辑）。

---

## 4. 🟡 Suggestions（应修）

### S1　并发共享状态无同步 + 零 CGO 无法 `-race` → 必须由设计规避

**位置**：
- 写：`settings.go` 托盘回调在 **systray goroutine** 上直接改 `cfg.Display.ShowLunar`（L89）、`cfg.Theme.Mode`（L137 `applyTheme`）、`cfg.Startup.AutoStart`（L112）；`calendar.go` `RefreshToday()` 在 **midnight goroutine** 写 `s.today`（L194）。
- 读：`app.go` `render()` 在 **主循环 + theme-watch goroutine + midnight goroutine** 三处读 `opts.Config.Display.*`（L110）、`opts.Calendar.MonthGrid()`（L109，内部读 `s.today` L173）。
- `config.Config`（config.go）是纯值对象、**无锁**；`calendarService`（calendar.go）**无 `sync` import** → 非并发安全。

**Why**：`config.Display.ShowLunar` 与 `calendarService.today` 是**非原子字段上的并发读+写**，按 Go 内存模型属未定义行为。本项目零 CGO（ADR-06）使 `go test -race` 直接不可用 —— 这类 bug 在 CI 里**永远看不到**，只能靠设计保证。`ThemeProvider` 已内部加锁（theme.go L71 `mu`），故主题安全；**Config 与 Calendar 是缺口**。

**Suggestion（与 ADR-02 双循环一致的最小改造）**：所有共享状态**写**都收口到主循环（单写者）。托盘回调只「发命令」，由主循环执行 mutation + persist + render：
- `settings.Deps` 新增 `Mutate func(func(*config.Config))` 或复用 `SendCmd` 扩展一个 `CmdApplyConfig`；菜单 `OnToggle/OnClick` 改为 `d.SendCmd(...)`，主循环在 `life.Handle` 后据命令改 `opts.Config` 并重渲。
- 或退一步：给 `config.Config` 包一层 `ConfigStore{RWMutex}`、`calendarService` 加 `sync.RWMutex` 保护 `today/selected`（`MonthGrid`/`RefreshToday`/`SelectedDate`/`SetSelectedDate` 加锁）。此路改动小但把并发责任分散到值对象，不如单写者干净。

> 注：`render()` 经 `pr.Present` → `SendMessage` 已天然串行（窗口线程只有一条），所以**推送**本身安全；风险只在 `render()` 的读 与 托盘/定时器的写 之间。

### S2　周列表头与数据网格 `WeekStart` 错位（用户首屏可见）

**位置**：
- `calendar.go:177` `GenMonthGrid(..., GridOptions{WeekStart: time.Monday, ...})` → 列 0 = **周一**（month.go `weekStart` 按 `WeekStart` 偏移，已验证生效）。
- `model.go:37/53` `Model.Weekdays = WeekdayLabels`（`[7]string{"日","一","二","三","四","五","六"}`，**周日为首**）。
- `calendar_view.go:57-60` 表头 `for i,label := range m.Weekdays { cx = colW*(i+0.5); DrawString(label) }` —— 列 `i` 画 `Weekdays[i]`。

**Why**：网格列 0 是周一，但列 0 表头写「日」。用户会看到「日 一 二 三 四 五 六」横排，其下第一列却是周一的日期 —— **整行表头右移一天**，每列标签都错。这是日历类应用最显眼的错误，且当前无测试覆盖（model 测试手工构造网格、自洽；render 测试不校验表头↔网格对齐）。

**Suggestion**：把 `WeekStart` 透传进渲染层，让表头跟随网格。`NewMonthModel` 增加 `weekStart time.Weekday` 参数，`Weekdays` 按 `weekStart` 旋转 `WeekdayLabels`（如 Monday → `一二三四五六日`）；或在 `MonthGrid` 增加 `WeekStart` 字段由 UI 直接采用。顺带补一个「表头第 0 列标签 == 网格首列星期」的对齐测试。

### S3　窗口激活/失焦：显示后可能立即被 `WM_ACTIVATE WA_INACTIVE` 自隐藏 —— **已修复**

**位置**：`window_windows.go` `wmActivate`（L312-317）收到 `WA_INACTIVE` 即 `ShowWindow(swHide)`；而 `wmUserShow`（L285-289）仅 `ShowWindow(swShow)`，**未 `SetForegroundWindow`**。

**Why**：`WS_EX_TOPMOST` 弹窗 `ShowWindow(SW_SHOW)` 不一定抢到前台。若窗口未成为前台、`WM_ACTIVATE` 带着 `WA_INACTIVE` 先到（例如原焦点窗口 reclaim），窗口会**刚显示就被自己隐藏**——点托盘「闪一下就没了」。这是 Win32 托盘弹窗的经典坑。

**Suggestion（历史方案）**：`wmUserShow` 处理里 `ShowWindow` 后补 `SetForegroundWindow(hwnd)`（跨进程需 `AllowSetForegroundWindow` 由托盘进程放行，或先 `ShowWindow(SW_SHOWNOACTIVATE)` 再 `SetForegroundWindow`）；更稳妥是加 `activated bool` 标志，仅当窗口确实激活过、后续失焦才隐藏。此条**必须在真机（Windows）烟测验证**，属于 ADR-08 行动项 #5 范围。

**Resolution（已修复）**：双保险——(1) `wmUserShow` 在 `ShowWindow(swShow)` 后调用 `AllowSetForegroundWindow(ASFW_ANY)` + `SetForegroundWindow(hwnd)` 显式抢前台（用户点击托盘属输入事件，前台锁超时通常在点击后重置，故能成功拿到焦点）；(2) `win32Window` 新增 `activated atomic.Int32` 标志，`wmUserShow` 时复位为 0；`wmActivate` 改为：仅当 `activated==1`（窗口此前确实被点开过）收到 `WA_INACTIVE` 才 `ShowWindow(swHide)` 自隐藏，`WA_ACTIVE/WA_CLICKACTIVE` 则置 `activated=1`。这彻底消除了「未激活即失焦→闪退」的竞态，同时保留「点击外部关闭」的预期行为。新增 `TestWin32Window_S3_ActivateGuard`（`//go:build windows`）直接驱动 `wndProc` 消息序列验证三态；`CGO_ENABLED=0 GOOS=windows go build/vet` 全绿。**真机有显示器/任务栏交互的端到端确认仍属 ADR-08 #5 烟测范畴**，headless CI 下该测试按设计 Skip。

### S4　`applyFont` 每帧 ~86 次重读 `.ttc` 字体文件

**位置**：`font.go:46-52` `applyFont` 每次 `dc.LoadFontFace(p, points)`（从磁盘解析字体）；`calendar_view.go` 在 `Draw` 中调用：表头(L50)、星期(L55)、**6×7 格循环内**每个格子日(L92)与农历(L103) → 单次 `Render` 约 **1+1+42×2 ≈ 86 次 `LoadFontFace`**，每次重读 `msyh.ttc`（~20MB）。

**Why**：渲染只发生在显隐/主题变更/跨午夜（非每帧），磁盘缓存下大概率不卡，但属明显的无谓 I/O。gg 支持 `dc.SetFontSize(points)` 在已加载字体上改字号而**不重读文件**。

**Suggestion**：`resolveCJKFont` 解析一次后，在 `Render` 起始 `LoadFontFace(p, 基准字号)` 一次；`applyFont` 改为 `dc.SetFontSize(points)`（不重读文件）。既提速又消除「每格读盘」的反模式。

### S5　`createDIB` 删除仍被选中的旧 bitmap

**位置**：`window_windows.go:241-268`：`createDIB` 在 `w.hbmp != 0` 时 `deleteObject.Call(w.hbmp)`（L247），随后 `createDIBSection`（L259）+ `selectObject`（L260）。但上一次 `selectObject(memDC, hbmp)`（L260 上一轮）已把旧 `hbmp` 选入 `memDC`——**删除一个仍被选中的 GDI 对象**行为未定义（多数实现延后删除，但属 footgun）。

**Suggestion**：删除前先把旧对象选出：`old,_,_ := selectObject.Call(w.memDC, getStockObject(NULL_BRUSH)); deleteObject.Call(w.hbmp); ...; selectObject.Call(w.memDC, newHbmp)`。或在 `createDIB` 入口先 `selectObject(memDC, 原默认)` 再删。

### S6　`RegisterClassEx` 同名每实例注册 → 多窗口（v1.3）派发错乱

**位置**：`window_windows.go:212` `regClassEx.Call(... "DeskCalendarWin32" ...)`，类名在 `newNativeWindow` 内硬编码；`wndProc` 闭包按实例捕获 `w`（L207）。

**Why**：v1.0 单窗口无碍。但 ADR-08 行动项 #7 / v1.3 提到「复杂设置页用同窗口工厂开第二 gg 弹窗」。若建第二个 `win32Window`：`RegisterClassExW` 同名会失败（返回 0，被 `_` 忽略），类的 `wndProc` 槽**保持首个实例的闭包** → 第二个窗口的消息全被派发到第一个实例的 `wndProc`，第二个窗口失控。

**Suggestion**：每窗口用唯一类名（如含 `hwnd`/计数器），或改用**单一全局 `wndProc` + `map[windows.Handle]*win32Window`** 按 `hwnd` 分发。v1.3 前不阻塞，但注册工厂时请先为此留接口。

---

## 5. 💭 Nits（可选）

- **N1（窗口 goroutine 泄漏）**：`app.Run` 退出路径 `tray.Remove()` + `return`（app.go:182-184、190-192），**未向窗口发 `WM_QUIT`**；`win32Window.run` 仅 `WM_DESTROY→postQuitMsg`（L348-350）才退出消息泵。故 quit 时窗口仅被 `Hide`，其消息泵 goroutine 泄漏至进程退出。进程退出会回收，但作为库使用时是泄漏。建议 quit 时 `sendMessage(hwnd, WM_CLOSE/WM_QUIT)` 让 `run` 自然 `destroy()` 退出。
- **N2（cmd 0% 覆盖）**：`cmd/deskcalendar/main.go` 仅做装配，未单测。可接受；若想保底，可加一个「`app.Run` 注入全部 fake + 经菜单退出」的集成测试（app 包已有 `TestRun_*` 范本，main 只需复用 `app.Options`）。
- **N3（文档）**：`docs/Window.md`、`docs/90-UI/*` 中仍有 `gogpuui.Node` / `gogpu.App.OnUpdate` / `RenderModeHostManaged` 等旧表述，与 ADR-08 降级后代码（自拥 `WindowController` + `ui.Render` + `desktop.Run` 已由 `app.Run` 替代）不符。Phase 2 已做 5 份文档 sweep（S1），Phase 3 相关文档建议同步清扫（属文档漂移，非代码错）。

---

## 6. Phase 2 遗留闭环确认

Phase 2 审查提出的应修项**本次已全部落地**，review→修复闭环有效：

- **S2+S3（theme 暗色系统回归）**：`theme.go` 已改存 `systemScheme` 字段、`currentScheme()` 按 scheme 推断（L180-185），`Watch` 推送并即时 `Current()`（L190-206）；`theme_test` 已含暗色系统清除覆盖回归 ✅
- **S4（today 跨午夜）**：`calendar.go RefreshToday`（L193-196）+ `app.go` 30min ticker（L159-175）已接 ✅
- **S5（SEED 真实数据）**：`2026.json` 已换真实 holiday-cn 数据（详见 Phase 2 报告）；`joinDate` 校验已加 ✅
- **S6（Builtin 误标）**：`buildTheme` 已据来源正确设 `Builtin` ✅
- **S7（闰月 golden）**：已补闰月 golden 测试 ✅
- **S1（文档 drift）**：`8a39c9f` 已对齐 5 份 Phase 2 设计文档 §9 到实现代码 ✅

> 注：Phase 0 的 **B1（`RenderModeHostManaged` 死 token）** 已于 `74c7262` 清除文档残留，治理修订（gogpu vs 路径 D）随 ADR-08 转正已自然消解——不再是 blocker。

---

## 7. ADR-08 降级评估（对照决策与代价）

| 决策点 | 落地情况 | 评价 |
|---|---|---|
| 渲染后端 gg（CPU 光栅，实心不透明） | `ui/render.go` 用 gg 绘 `*image.RGBA`、`flattenAlpha` 强制不透明（L88-95） | ✅ 与「方角不透明」一致 |
| 窗口后端自拥普通弹窗（WS_POPUP+WS_EX_TOPMOST + DIBSection+WM_PAINT/BitBlt） | `win32/window_windows.go` 完整实现；无 layered/alpha 坑 | ✅ 比 POC 更轻（见 B1 待修） |
| 响应式 coregx/signals，不引 gogpu/ui | 仅 `state/signal.go` 复用类型别名，无 gogpu/ui 依赖 | ✅ |
| 托盘 + 设置走 systray 菜单 | `settings.BuildTrayMenu` 用声明式 `MenuItem`（AddCheckbox/Submenu 由 platform 落到 systray） | ✅ |
| WindowController 收敛 Show/Hide/AnchorAboveTray/Visible(+Present) | `win32.WindowController` 接口（window.go:19-32） | ✅ |
| 依赖图瘦身 / 零 CGO / 无 wgpu | `go list` + `go.mod` 实证：gg+systray 本地 replace，wgpu 未进 go.mod，CGO 构建通过 | ✅ 收益兑现 |
| 完全可逆（未来可换回 gogpu 表面） | 渲染层隔离为 `Render(model)→RGBA` 缓冲边界 | ✅ |

**代价侧**：窗口层维护责任已自转给我们（约 180 行 win32，`win32_windows.go`）；B1/S3/S5/S6 即这部分「自行保证正确性」的代价体现，需在真机烟测中夯实。整体权衡**符合 ADR-08 预期**。

---

## 8. 亮点（值得肯定）

- **降级真正落地、不打折**：没有「嘴上降级、代码还 import gogpu」的夹生饭；依赖图干净到 `go list` 一眼可证。
- **双循环铁律守住**：托盘 goroutine 只经 `SendCommand`(非阻塞 select) 发命令（app.go:137-142、platform 通道），窗口操作全经主循环 `lifecycle.Handle` → `WindowController` → `SendMessage` 派发到窗口线程。S1 的并发隐患是**唯一**越界处，且主题侧已正确加锁。
- **seam 注入贯穿始终**：`app.Options` 全部可注入（Window/Tray/Anchor/Startup/Theme/Calendar），`win32.fakeWindow`/`fakeTray`/`fakeStartup` 让 `app`(80%)、`shell`(100%)、`settings`(84%) 高覆盖且快。
- **shell 状态机严谨**：`StateBoot/Ready/Showing/Hiding/Quit` + 退出幂等（lifecycle.go:99-108、测试 `TestLifecycle_CmdQuitIsIdempotent`）—— 这种「退出后忽略一切命令」的纪律很多项目会漏。
- **gg 渲染层质量高**：实心不透明 `flattenAlpha` 与 BitBlt 忽略 alpha 的契约对齐；`computeLayout` 纯函数可单测；CJK 字体优雅降级（缺字框不崩）；`ui` 不反向依赖窗口/平台，单测独立。
- **Phase 2 遗留全部闭环**：见 §6，review 反馈被认真消化。

---

## 9. 结论与下一步

**评级 A-，1 个 🔴、6 个 🟡、3 个 💭。建议：修 B1 + S2 后立即上真机（Windows）烟测（ADR-08 行动项 #5），S1 改为单写者设计后再发版。**

优先序：
1. **🔴 B1** —— `AnchorAboveTray`/`Present` 补 `pendingRect`/`pendingBmp` 的 `Store`（约 2 行），并补一个真机集成测试（或 Windows CI）。否则首屏即空白。
2. **🟡 S2** —— 表头跟随 `WeekStart` 旋转，补对齐测试。用户首屏可见。
3. **🟡 S1** —— 共享 Config/Calendar 写收口主循环（或加锁）。零 CGO 下这是唯一的并发正确性兜底。（已修复：单写者设计）
4. **🟡 S3 / S5** —— S3 已修复（激活/失焦抢前台 + activated 守卫）；S5 随真机烟测一并验证（GDI 对象生命周期）。
5. **🟡 S4** —— 字体 `SetFontSize` 去重读（顺手优化）。
6. **🟡 S6** —— 多窗口类名（v1.3 设置窗）已在本轮修复（见 §10），v1.3 复用窗口工厂时无需再动。
7. **💭 N1 / N2 / N3** —— 退出时 `WM_QUIT`、cmd 集成测试、Phase 3 文档 sweep。

---

## 10. 修复与验证（2026-07-10 实施）

已完成 B1/S2 修复，并在一并做真机烟测时额外发现并修复了 S6（多窗口类名冲突）。

### 已修复项

| 编号 | 问题 | 修复摘要 | 验证方式 |
|---|---|---|---|
| **B1** | `AnchorAboveTray`/`Present` 跨线程数据通道未 `Store` → 真机首屏空白 | `window_windows.go` 中发送前 `pendingRect.Store`（堆拷贝，防 `&r` 悬垂）+ `pendingBmp.Store`；`lparam` 改为 0，wndProc 仍读 atomic | 真机集成测试 `TestWin32Window_AnchorAndPresentReachWindowThread`（真实窗口线程，非 fake）PASS |
| **S2** | 表头周日首与网格周一首错位一天 | `calendar.MonthGrid` 新增 `WeekStart` 字段；`ui.NewMonthModel` 按 `grid.WeekStart` 旋转 `WeekdayLabels`（一 二 三 四 五 六 日）| 新增 `TestNewMonthModel_HeaderFollowsWeekStart`：Sunday/Monday/Saturday 三值验证每列表头 == 该列首行格公历星期；渲染 PNG 目视确认表头为周一首 |
| **S6** | `RegisterClassExW` 硬编码类名 → 第二窗口消息误派发到首个窗口（已销毁 DIB）→ 崩溃 | 每个窗口使用唯一类名 `fmt.Sprintf("DeskCalendarWin32_%d", atomic.AddInt64(&classSeq, 1))` | 两个真实窗口测试同进程顺序运行均 PASS；否则第二个测试必崩 |
| **S1** | 托盘回调 / midnight goroutine 直接改共享 `Config` / `Calendar.today`，与 `render()` 主循环读构成无同步并发（零 CGO 下 `-race` 不可用） | **单写者**：托盘回调与后台 goroutine 只发 `TrayCommand`，所有 `Config` / `Calendar.today` 写 + `render()` 收口到主循环 `app.Run` 内 `applyConfigCommand`（顶层 `select`）。`TrayCommand` 扩充 `CmdToggleLunar / CmdToggleHoliday / CmdToggleStartup / CmdThemeLight / CmdThemeDark / CmdThemeSystem / CmdRefreshToday / CmdRender`，原 4 个 iota 值（CmdShow/CmdHide/CmdToggle/CmdQuit）保持不变 | `internal/settings` 5 测断言回调只发命令、不直改 `Config`；`TestRun_ConfigCommandsAppliedOnMainLoop`（S1 回归）验证「显示农历」「深色主题」「开机启动」均经主循环落地并触发重渲；`CGO_ENABLED=0 go test ./...` 全绿 |
| **S3** | `wmUserShow` 仅 `ShowWindow(swShow)` 未抢前台；`wmActivate` 收 `WA_INACTIVE` 即 `ShowWindow(swHide)`，导致 `WS_EX_TOPMOST` 弹窗显示后若未成前台、先收到 `WA_INACTIVE` 会**自己隐藏**（点托盘闪一下就没） | 双保险：(1) `wmUserShow` 增 `AllowSetForegroundWindow(ASFW_ANY)` + `SetForegroundWindow(hwnd)` 显式抢前台；(2) 新增 `activated atomic.Int32`，`wmUserShow` 复位为 0，`wmActivate` 仅当 `activated==1` 收到 `WA_INACTIVE` 才隐藏，`WA_ACTIVE/WA_CLICKACTIVE` 置 `activated=1` | `TestWin32Window_S3_ActivateGuard`（`//go:build windows`）直接驱动 `wndProc` 验证「未激活时 WA_INACTIVE 不隐藏 / 点开后再失焦才隐藏 / 重显复位」三态；`CGO_ENABLED=0 GOOS=windows go build/vet` 全绿（headless CI 下该测试按设计 Skip，真机交互验证属 ADR-08 #5 烟测） |
| — | 测试环境窗口 goroutine 未等待 | `win32Window` 新增 `done chan struct{}`，`run()` 在 `destroy()` 后关闭；测试 `defer` 中 post WM_QUIT + `<-wc.done` | 真实窗口测试干净退出，无 GDI 泄漏竞争 |

### 真机烟测结果

- `CGO_ENABLED=0 go build ./...` ✅ 零 CGO 离线构建成立
- `go vet ./...` ✅
- `go test ./...` ✅ 全包通过（含 `internal/platform/win32` 真实窗口测试，`GOOS=windows` 默认生效）
- `internal/platform/win32` 真实窗口测试：
  - `TestWin32Window_AnchorAndPresentReachWindowThread` — 验证 `AnchorAboveTray` 写入 `lastTray`、`Present` 写入 `lastBmp`。
  - `TestWin32Window_RenderAndPresentFullPipeline` — 用 `ui.Render` 渲染真实 2026-07 日历 → `Present` 到真实窗口 → 断言窗口 DIB 已被绘入（非中性灰底）。
- 原生二进制 `dist/deskcalendar.exe` 构建成功；头less 启动 6 秒无 panic、无 stderr。
- 渲染验收图 `dist/acceptance-2026-07-light.png` 目视确认表头为「一 二 三 四 五 六 日」，与 Monday 首网格对齐。

### 剩余待办（非 B1/S2/S1/S3 阻塞）

- **S4**：`applyFont` 改为 `LoadFontFace` 一次 + `SetFontSize` 多次，避免每帧重读 `msyh.ttc`。
- **S5**：`createDIB` 删除旧 bitmap 前先 `selectObject` 将其选出 `memDC`。
- **N1/N2/N3**：退出路径、cmd 集成测试、文档 sweep。

### 结论

本轮修复后，**B1/S2/S1/S3 已闭环**（S1 单写者收口主循环；S3 抢前台 + activated 守卫消除「闪一下就没了」），S6 作为烟测副产物被识别并修复。当前代码可进入 Windows 真机手测；发版前剩余 S4/S5/N1/N2/N3 待处理（S4/S5 为真机烟测同验项）。
