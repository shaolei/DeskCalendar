# Phase 3 任务重排计划（ADR-08 降级方案落地）

> 关联决策：[ADR-08-降级脱离gogpu上游阻塞.md](./ADR-08-降级脱离gogpu上游阻塞.md)（状态：Confirmed，2026-07-09）
> 范围：Phase 3（Shell 装配 + UI 渲染）的全部 8 个 epic 重排
> 作者：Senior Developer ｜ 最后更新：2026-07-09

---

## 1. 背景与核心变化

架构师给出 ADR-08 并经确认转正。Phase 3 不再依赖「等待 gogpu 上游合并补丁」（`Hide/SetPosition/SetSize`、`CompositeAlphaModePremultiplied`、`gogpu/ui` 部件树）。改为 **Path D 自立方案**：

| 维度 | 原方案（依赖上游补丁） | ADR-08 后（MVP 主线） |
|------|----------------------|----------------------|
| 渲染后端 | gogpu/wgpu GPU 合成 | `github.com/gogpu/gg` 纯 Go CPU 光栅 → `image.RGBA` |
| 窗口后端 | `*gogpu.Window`（依赖 stash 补丁） | 自拥 `internal/platform/win32` 普通弹窗（`WS_POPUP`+`WS_EX_TOPMOST`，DIBSection+`WM_PAINT`/`BitBlt`） |
| 窗口形态 | 透明圆角 + 每像素 alpha + DWM 阴影 | **不透明方角弹窗**（圆角后续 `DwmSetWindowAttribute` 零成本白嫖） |
| 显隐/锚定 | `Hide/SetPosition/SetSize` 持续可调 | 初次 `SetWindowPos` 锚定托盘上方后即固定；`Show/Hide`/`AnchorAboveTray(rect)` |
| 设置交付 | 独立设置窗 + SettingsView | **托盘右键菜单**（`AddCheckbox`/`AddSubmenu`）承载布尔项；独立窗仅作 v1.3 后备 |
| 动画 | 淡入+上移（依赖分层窗 alpha） | MVP **瞬时显示**；`Animator` 推到 v1.2 |
| 响应式 | `gogpu/ui` Signal | `coregx/signals`（已是直接依赖，不变） |

**不变（已建好的叶子包，全部原封不动）**：`internal/state`、`internal/plugin`、`internal/calendar`、`internal/theme`、`internal/platform`（DPI/MultiMonitor/Tray/Startup/Notification 在 Phase 1 已验证）。双循环架构（systray goroutine + 主窗口 goroutine + channel）不变，systray 保留。

### ⚠️ 一处必须写实的措辞精度（关于 wgpu）

ADR-08 原写「❌ 不再依赖 wgpu」。实测澄清：

- 本地 `gg`（`D:/workspace/github/gg`）根包 `context.go`/`context_image.go`/`accelerator.go` **仍 `import "github.com/gogpu/gpucontext"`**，其 `go.mod` 传递 require `wgpu`/`gpucontext`/`naga`/`go-webgpu/webgpu`。
- 但 **`CGO_ENABLED=0 go build -tags gg`（POC 实证）编译通过**，且 POC 已生成 `poc_gg.exe`/`panel.png`。说明 gg 的传递 GPU 依赖**不编译进 DeskCalendar 最终二进制**（CPU 光栅路径不触碰 GPU 后端），零 CGO 离线构建成立。
- 因此准确含义：**wgpu 从「最终二进制」层被甩掉；仍作为 gg 的传递依赖存在于「go.mod 模块图」层（离线缓存可解析，无需联网）**。若日后需从模块图彻底移除，可改用 gg 纯 CPU 子包或 fork 裁剪——属后续优化，非 MVP 阻塞。

该 nuance 已回填进 ADR-08 文末「确认与实证」段。

---

## 2. 受影响 Epic 清单（8 个，均 v1.0 MVP）

| Epic | 标题 | 受 ADR-08 影响程度 | 主要变更 |
|------|------|------------------|----------|
| #1 | [10] App 模块实现 | 🔴 高 | 自拥主消息循环替代 `desktop.Run(gogpuApp,uiApp)` |
| #9 | [10] Window 模块实现 | 🔴 高 | 自拥 win32 弹窗替代 `*gogpu.Window`；`WindowController` 收敛 |
| #17 | [10] Layout 模块实现 | 🟡 中 | 固定尺寸方角面板；移除运行时改尺寸相关设计 |
| #24 | [10] Lifecycle 模块实现 | 🟡 中 | `AnchorAboveTray` 替代 `SetPosition` 调用 |
| #105 | [90] MainWindow 模块实现 | 🔴 高 | `Render→RGBA` 替代 `Root()*gogpuui.Node` 部件树 |
| #110 | [90] CalendarView 模块实现 | 🔴 高 | `Draw(ctx,rect,model,theme)` 替代 `Build()*gogpuui.Node` |
| #115 | [90] Settings 模块实现 | 🔴 高（范围大幅缩小） | MVP 改走托盘菜单；独立设置窗降级 v1.3 后备 |
| #120 | [90] Animation 模块实现 | 🟡 中（范围缩小） | MVP 瞬时显示；`Animator` 推 v1.2 |

> 说明：上述 epic 的现有子任务 issue（实际归属：#2–#8 属 #1、#10–#16 属 #9、#18–#23 属 #17、#25–#31 属 #24、#106–#109 属 #105、#111–#114 属 #110、#116–#119 属 #115、#121–#123 属 #120，共 42 个）原按 gogpu API 撰写，**已于 2026-07-09 按本计划第 3 节全部重写**（正文对齐 Path D：gg + 自拥 win32 弹窗 + 托盘菜单设置），各 epic 下仍保留「ADR-08 影响」评论供回溯。

---

## 3. 逐 Epic 变更点与子任务重排

### 3.1 #1 App（自拥主循环）
- **文档修订**：`docs/10-Shell/App.md` §1/§9——`desktop.Run(gogpuApp, uiApp)` → 自拥 `main`：`systray.Run(func(){...})` 启动 systray goroutine + 主 goroutine `GetMessage/Dispatch` 自拥消息循环；`gogpu.App.OnUpdate` 渲染循环 → 自建 `Tick`（驱动 gg 重渲 + `Animator` 推进，MVP 瞬时）。
- **子任务重排**：
  - T1：自拥主消息循环 `GetMessage`/`Dispatch`（替代 gogpu App.Run）。
  - T2：双循环接线——systray goroutine 经 channel 发 `Toggle/Show/Hide/Quit`；主 goroutine 消费命令。
  - T3：装配 `internal/ui.Render` 输出 → `internal/platform/win32` 推送。
  - T4：退出路径（托盘退出菜单 / `WM_CLOSE` / `WM_QUIT`）。

### 3.2 #9 Window（自拥 win32 弹窗 + 收敛接口）
- **文档修订**：`docs/10-Shell/Window.md` 全文——删 `positionablePlatform` 断言接口与 `SetPosition/SetSize`；`NewWindow(*gogpu.Window)` → `NewWindow(opts)` 自拥实现；§2/§3/§6/§8 的 `gogpu.Window`/`OnUpdate` 全部改写；ASCII 原型里「圆角透明」→「方角不透明」。
- **WindowController 新签名**：
  ```go
  type WindowController interface {
      Show()
      Hide()
      AnchorAboveTray(rect image.Rectangle) // 初次定位即固定，不再 Move/Resize
      Visible() bool
  }
  ```
- **子任务重排**：
  - T1：`internal/platform/win32` 普通弹窗（`WS_POPUP`+`WS_EX_TOPMOST`，DIBSection + `WM_PAINT`/`BitBlt` 推 gg 像素）。
  - T2：`WindowController` 收敛实现（删 `SetPosition/SetSize`/`positionablePlatform`）。
  - T3：失焦关闭——`WM_ACTIVATE`(`WA_INACTIVE`)→Hide、`Esc`→Hide。
  - T4：DPI——`SetProcessDpiAwarenessContext(-4)` + `WM_DPICHANGED` 重设 DIB 尺寸。
  - T5：fake backend 单测（Show/Hide/Visible/Anchor 调用断言）。

### 3.3 #17 Layout（基本不变，去圆角）
- **文档修订**：`docs/10-Shell/Layout.md`——面板固定 `360×480` 方角；移除「运行时改尺寸」「圆角半径」相关设计；保留 `Layout(w,h)→[]CellRect` 纯函数（已可单测）。
- **子任务重排**：
  - T1：固定面板布局纯函数 `Layout(w,h)→[]CellRect`（一次性，覆盖 6×7 网格 + 头部）。
  - T2：周起始日（`time.Monday`/`Sunday`）可配，布局参数化。
  - T3：移除原「`SetSize` 同步」子任务（窗口固定，不再需要）。

### 3.4 #24 Lifecycle（命令→新接口）
- **文档修订**：`docs/10-Shell/Lifecycle.md`——`Handle(CmdToggle)` 调用 `controller.AnchorAboveTray(rect)` + `Show`（替代 `SetPosition`+`Show`）；状态机 `Hidden/Visible` 不变。
- **子任务重排**：
  - T1：`CmdToggle` → `AnchorAboveTray(rect)`+`Show` / `Hide`（用新接口）。
  - T2：状态机 `Hidden/Visible` 读取 `controller.Visible()` 决策下次 toggle 方向。
  - T3：接入 `calendar.RefreshToday()` 每日定时器（跨午夜修正，S4 已落地）。

### 3.5 #105 MainWindow（Render→RGBA）
- **文档修订**：`docs/90-UI/MainWindow.md` 全文——`Root() *gogpuui.Node` 删除；`Mount(child View)` 的 `View.Build()*gogpuui.Node` → 子视图作为「绘制函数组合」；透明圆角根容器 → 不透明方角缓冲；`gogpu.App.OnUpdate` → 自建主循环 `Tick`；§3/§6 的 `wgpu 合成→DWM 阴影` 删除。
- **新契约骨架**：
  ```go
  // internal/ui: 渲染层
  func Render(model Model, layout Layout, theme *theme.Theme) *image.RGBA // gg 绘实心面板
  type View interface {
      Draw(ctx *gg.Context, rect image.Rectangle, m Model, t *theme.Theme)
      OnShow(); OnHide()
  }
  ```
- **子任务重排**：
  - T1：`Render(model, layout, theme) → *image.RGBA`（gg 绘实心不透明方角面板，MVP 无圆角/阴影）。
  - T2：缓冲 → `WindowController` 推送（经 win32 DIBSection）。
  - T3：状态变更（signals）→ 重渲（每次整面板重光栅，360×480 亚毫秒~毫秒级）。
  - T4：`Mount` 改为子视图 `Draw` 组合（CalendarView/Settings-menu 入口），不再有 `gogpuui.Node` 树。

### 3.6 #110 CalendarView（gg 绘制）
- **文档修订**：`docs/90-UI/CalendarView.md` §1/§9——`Build()*gogpuui.Node` → `Draw(ctx *gg.Context, rect, model, theme)`；`DayCell`/`MonthModel` 模型保留；§3/§6「gogpu 组件树渲染」→「gg 缓冲绘制」。
- **子任务重排**：
  - T1：`Draw` 用 gg 画 6×7 网格（固定 42 格，上/下月补白）。
  - T2：农历/节气/节假日小字标注（消费 `50-Calendar` 经 Store 注入的 `MonthModel`）。
  - T3：点击命中测试（鼠标坐标 → 格子矩形），写 `Selected` Signal。
  - T4：今日/选中高亮（★ / 描边），订阅 `Today`/`Selected`/`Month` 重渲。

### 3.7 #115 Settings（改走托盘菜单，范围大缩）
- **文档修订**：`docs/90-UI/Settings.md`——MVP 不再有独立 `SettingsView` 窗口；改为 `gogpu/systray` 右键菜单（`AddCheckbox`/`AddSubmenu`）承载布尔项，绑定 `config` Store；原 §9 `SettingsView.Build` 降级为 v1.3 后备（仅当菜单放不下复杂项时启用第二 gg 弹窗）。
- **子任务重排（MVP 大幅缩小）**：
  - T1：托盘右键菜单 `AddCheckbox`（显示农历 / 显示节假日 / 开机启动 / 主题跟随系统）。
  - T2：主题切换 `AddSubmenu`（浅色/深色/跟随）。
  - T3：菜单项变更 → `config` Store → `infra/config` 落盘 + 副作用（注册表/主题/锚定）。
  - T4（v1.3 后备）：原 `SettingsView` 独立窗仅保留为复杂设置页入口（皮肤/字号/周起始日）。

### 3.8 #120 Animation（MVP 瞬时，推后）
- **文档修订**：`docs/90-UI/Animation.md`——MVP 删除 `KindFadeSlideIn`/`KindFadeOut` 的 MainWindow 集成（无分层窗则无逐像素 alpha 淡入）；`Animator` 纯函数（Easing/Tick 不 sleep）保留但 v1.2 再接；§3/§6 `WindowController.SetPosition + 透明度` 帧推进改为 v1.2 范围。
- **子任务重排**：
  - T1：MVP `Show/Hide` 瞬时（无动画），焦点不抢（G1）。
  - T2：`Animator` 类型与 Easing 预设保留（可单测，零依赖），但不接入显隐。
  - T3（v1.1）：`DwmSetWindowAttribute(DWMWA_WINDOW_CORNER_PREFERENCE, DWMWCP_ROUND)` 圆角零成本。
  - T4（v1.2）：若产品认为必要，走 layered+alpha 或 DWM 过渡接回 `Animator`。

---

## 4. go.mod 调整（已完成）

- 新增 `require github.com/gogpu/gg v0.0.0-00010101000000-000000000000` 与 `replace github.com/gogpu/gg => D:/workspace/github/gg`。
- 保留 `replace github.com/gogpu/systray => D:/workspace/github/systray` 与 `replace github.com/go-webgpu/goffi v0.5.5 => v0.5.6`。
- 验证：`go mod download` / `go build ./...` / `CGO_ENABLED=0 go build ./...` / `go vet ./...` 全过。
- 注意：gg 在 `internal/ui` 实际 import 前仅为声明依赖，CI 用 `build/vet/test` + `CGO_ENABLED=0 build`（不跑 `go mod tidy`，避免误删未引用 require）。

---

## 5. 真机验证清单（MVP 完工门禁）

- [ ] 普通不透明方角面板在托盘图标正上方弹出，不越界（多屏/DPI 下由 platform 换算）。
- [ ] 失焦（`WM_ACTIVATE` WA_INACTIVE）/`Esc` 关闭；点击托盘 toggle。
- [ ] 公历+农历+节气+节假日+调休渲染正确（中文正常，gg `LoadFontFace` 加载中文字体如 `msyh.ttc`）。
- [ ] DPI 缩放（150%/175%）下面板尺寸与文字清晰，不模糊不溢出。
- [ ] 托盘右键菜单设置项（农历/节假日/开机启动/主题）即时生效并持久化到 `config.json`。
- [ ] 主题跟随系统切换（深/浅）面板配色实时更新（复用 Phase 2 `theme.Watch`）。
- [ ] `CGO_ENABLED=0 go build` 通过；离线可构建（模块缓存已含 gg 传递依赖）。
- [ ] 全量 `go build/vet/test ./...` 绿；覆盖率不低于各自基线。

---

## 6. 建议落地顺序

1. **#9 Window**（自拥 win32 弹窗 + 收敛 `WindowController`）—— 最底层，先打通「能弹出一个方角窗」。
2. **#105 MainWindow + #110 CalendarView**（gg 渲染层，从 POC `draw_gg.go` 提升为 `internal/ui`）—— 渲染实心面板 + 月历网格。
3. **#1 App + #24 Lifecycle**（双循环接线 + 命令状态机）—— 把窗口和渲染串成可运行程序。
4. **#115 Settings**（托盘菜单）—— 设置走菜单，低成本。
5. **#17 Layout / #120 Animation**（布局纯函数收尾；动画 MVP 瞬时，圆角/Animator 推 v1.1/v1.2）。
6. 真机验证清单全过 → v1.0 可发布（Holiday SEED 真实数据替换见 `_代码审查_Phase2.md` S5 发布门）。

---

## 7. GitHub Issue 动作

- 已对 8 个 Phase 3 epic（#1/#9/#17/#24/#105/#110/#115/#120）各加一条「ADR-08 影响」评论，链接本计划并简述变更（非破坏性，保留原正文供评审）。
- **遗留清理**：Phase 2 epic #101 关闭时漏关其子任务 #102/#103/#104（仍 open 孤儿），本次一并关闭。
- 子任务 issue 已于 2026-07-09 按第 3 节全部重写（共 42 个：#2–#8 / #10–#16 / #18–#23 / #25–#31 / #106–#109 / #111–#114 / #116–#119 / #121–#123），正文已对齐 Path D（gg + 自拥 win32 弹窗 + 托盘菜单设置），不再引用 gogpu 部件树 / RenderModeHostManaged / SetPosition/SetSize 等上游阻塞概念。

> 注：本计划为 Phase 3 的「规划文档」，不直接改代码。代码落地按第 6 节顺序，每个 epic 走 TDD（RED→GREEN）并在完成后更新对应设计文档 §9（doc-sweep，同 Phase 2 的 S1 纪律）。
