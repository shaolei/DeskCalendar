# 路径 D 文档修订计划（DeskCalendar）

> 状态：Draft（待用户评审）
> 关联：`ADR-03-无边框窗口样式.md`（已切路径 D）、`poc/layered-window/`（实机验证 `shot.png`/`shot_gg.png`）
> 日期：2026-07-08

## 0. 背景与目标

- **POC 已实机验证**：`gg`（纯 Go 零 CGO 2D 库）渲染圆角半透明面板 → `WS_EX_LAYERED` 原生分层窗口（`ShowWindow`/`SetWindowPos` 显隐定位），零依赖 patch、零 CGO，完整 ADR-03（真圆角 + 真每像素 alpha + 自由显隐 + 贴时钟）全部恢复。证据：`poc/screenshot/shot.png`（stdlib 版）、`shot_gg.png`（gg 版）。
- **现状冲突**：`docs/` 中多数文件仍描述两种过期现实——
  1. **路径 B 降级形态**（"方角不透明、圆角/每像素 alpha 待后续补全"）；或
  2. **gogpu/ui 渲染弹窗**（"`gogpu.NewApp(gogpu.Frameless, gogpu.RenderModeHostManaged)` + `ThemeBackground()` 透明根"）。
- **目标**：把设计文档对齐到路径 D，并厘清 gg-canvas 范式对 UI 模块的深层牵连。

---

## 1. 须先拍板的两个决策（阻塞 Tier 1/2 表述）

### D1 · ADR-01 存废
- **现状**：`ADR-03` 的「受影响文档」点名要改 `docs/ADR-01-*.md`，但仓库里**根本没有 ADR-01 文件**（仅 ADR-03/05/07）。属悬空引用。
- **建议**：新建 `ADR-01-UI渲染层与窗口方案.md`，正式记录：
  - 原 ADR-01 = "UI 用 gogpu/ui"；
  - **路径 D 修订**：托盘弹窗渲染从 gogpu/ui 切到 `gg + 原生分层窗口`；gogpu/ui 对托盘弹窗不再需要。
- 解决悬空引用的同时，把这次最大的架构翻转钉成正式 ADR。

### D2 · gogpu/ui 是否整体退役
- **选项 Y（推荐 · 全 gg）**：弹窗 + 设置 + 主窗口全部用 gg 画布统一渲染。
  - 收益：单一渲染栈、零 CGO、无 GPU 依赖（gg 默认 CPU 软件渲染器即可）、无部件树心智负担；
  - 代价：需自建轻量布局助手（`Rect`/`Insets`/`VBox`/`HBox` + 文本度量）。
- **选项 X（混合）**：弹窗 = gg + 分层窗口；设置/主窗口若需要交互控件，仍可用 gogpu/ui 独立窗口。
  - 收益：复用 gogpu/ui 部件树写设置页；
  - 代价：两套渲染栈并存、依赖 wgpu GPU 链、零 CGO 边界需额外看护。
- **影响**：决定 `90-UI/*` 是「gg 画布全重写」还是「仅弹窗改、其余保留 gogpu/ui」。

---

## 2. Tier 1 修订明细（直接矛盾，必改）

| 文件 | 位置 | 改动要点 | 验收 |
|---|---|---|---|
| `00-项目介绍.md` | L11/30/43/64/72 | "降级形态""圆角/每像素 alpha 待补全""ADR-03（降级）" → 改为"路径 D 已验证、完整形态可达"；UI 框架行（L62）按 D1/D2 调整 | grep 无"降级"残留 |
| `20-Platform/WindowStyle.md` | L4-5/50/54/68/177/185 | 删除"降级形态"表述；`Layered`/`PerPixelAlpha`/`CornerRadius` 从"死字段待补全"改为"路径 D 下真实生效"；M1/M2/M3/M4 里程碑改为"已实现 POC、Phase 3 接入" | 字段语义与 ADR-03 一致 |
| `10-Shell/App.md` | L84/L110 | 弹窗装配 `gogpu.NewApp(gogpu.Frameless, gogpu.RenderModeHostManaged)` + 每像素 alpha → 改为"建分层窗口 + gg 渲染位图"；`RenderModeHostManaged` 不再出现在弹窗路径 | 无 gogpu 弹窗装配代码 |
| `10-Shell/Window.md` | 整篇 | 当前基于 `gogpu.Window` + `positionablePlatform` 接口断言暴露 `SetPosition/SetSize`。路径 D 下窗口是**自有 win32 分层窗口**，`WindowController` 应包 `internal/platform/win32`，断言封装失效 | `WindowController` 契约指向 win32 实现 |
| `90-UI/MainWindow.md` | L239 T1 | `gogpu.Frameless + RenderModeHostManaged + ThemeBackground()` 搭透明圆角根 → 改为 gg 绘制 + 分层窗口 | T1 验收与 POC 一致 |

> ⚠️ `90-UI/MainWindow.md` 中"MainWindow"是否即"托盘弹窗"需在执行时确认（若是独立主窗口则按 D2 选项处理）。

---

## 3. Tier 2 修订明细（重定调 + 路径 D 前的旧不一致）

| 文件 | 位置 | 改动要点 |
|---|---|---|
| `30-State/Signal.md` | L1/7/9/11/123/147-152/201 | "Signal 由 gogpu/ui 提供" → 改为"由 `coregx/signals` 提供（Phase 0 已落地，`internal/state.Signal[T]` 直接 = `coregx/signals.Signal[T]`）" |
| `30-State/Store.md` | L8/10/53 | 同 Signal 源更正；`<<gogpu/ui>>` 依赖改为 `<<coregx/signals>>` |
| `30-State/DataFlow.md` | L10/56/128/183 | 同上；`gogpu/ui (Signal)` 标注改为 `coregx/signals` |
| `ADR-07-事件总线归属与入口约定.md` | F5/B1（L35/99/100/106/113/143-144/152） | 原规定"`RenderModeHostManaged` 是 gogpu 导出符号、业务包禁止本地枚举" → 翻转为"`platform` 用本地 `RenderMode`(Auto/CPU/GPU) 枚举（与 Phase 0 代码审查结论一致）；弹窗在路径 D 下根本不引用 gogpu 渲染模式" |
| `01-总体架构.md` | L105 | "ADR-01~07 全部拍板、无未决阻塞" → 加注"ADR-03 于 2026-07-08 二次修订为路径 D，详见 ADR-03" |
| `_模板与写作规范.md` | L71-73 | ADR-01/ADR-03 摘要改为路径 D 表述 |

---

## 4. Tier 3（历史快照，加状态横幅即可，不必逐句改）

| 文件 | 处理 |
|---|---|
| `DeskCalendar_技术评估报告_gogpu-ui-Windows.md` | 顶部加横幅："⚠️ 历史快照：基于 patch 版 gogpu，已被路径 D（gg+分层窗口）推翻。双循环 systray spike 章节仍有效。" |
| `_代码审查_Phase0.md` | 顶部加注：F5/F7/🔴RenderMode 条目在路径 D 下前提已变，结论以本计划 D1/D2 + ADR-01 为准 |
| `_交叉一致性审查报告.md` | F5/F7 条目加注"前提已随路径 D 变更" |

---

## 5. 90-UI/* 与 Layout 的 gg 画布范式重写（最大头，Phase 3 前置）

### 问题陈述
`90-UI/CalendarView` `WeatherView` `TodoView` `Settings` `Animation` 与 `10-Shell/Layout.md` 把弹窗当成 **gogpu/ui 部件树**（`Column`/`Row`/`Stack`/`ThemeBackground`/`ClipRRect`）来写。但 **gg 是 Canvas / 立即模式 2D 库，没有部件树**——弹窗位图是 gg 一笔笔画出来的，`UpdateLayeredWindow` 显示的就这一张位图，gogpu/ui 的 widget 拼不进去。因此这些文档**不是改词，而是重写绘制范式**。

### 建议范式（若 D2=Y）
1. **新建 `internal/ui`（项目自有，gg 之上）布局原语**：
   - `type Rect struct{ X, Y, W, H int }`、`Insets`、`VBox`/`HBox` 测量+布局 helper；
   - 文本度量：用 gg `MeasureString` / `FontMetrics` 计算基线。
2. **每个 90-UI 视图模块 = 纯绘制例程**：
   - 契约由 `func (v *XView) Paint(ctx *gg.Context, rect Rect, deps ...)` 取代"返回 gogpu/ui Widget"；
   - 不再 `root.Add(widget)`，改为 `xview.Paint(ctx, rect)`。
3. **根装配（Shell/App 或新 `internal/ui/compose.go`）**：
   - 建 `gg.Pixmap`（premultiplied RGBA）→ 各视图 `Paint` → 阴影/圆角合成 → `UpdateLayeredWindow`。
   - 与 POC `RenderPanel()` 输出格式一致。
4. **动画（`90-UI/Animation.md`）**：
   - 由"gogpu/ui 动画原语"改为"对分层窗口位图 alpha/位置做插值重绘"（定时重绘 + `SetWindowPos`），不依赖 gogpu/ui。

### 排期建议（分模块落地，每模块配最小 gg 绘制 stub 验证）
`CalendarView` → `Theme/Skin`（配色/圆角/阴影参数）→ `WeatherView`/`TodoView` → `Settings` → `Animation`。

---

## 6. 执行顺序与里程碑

- **Phase A（纯文档对齐，无代码）**：D1+D2 决策 → Tier 1（§2）→ Tier 2（§3）→ Tier 3 横幅（§4）。
- **Phase B（代码范式，Phase 3 前置）**：从 POC 提炼 `internal/platform/win32` 包 → 建 `internal/ui` 布局助手 → 按 §5 重写 `90-UI/*` + `Layout.md`。

---

## 7. 验收标准

- [ ] `docs/` 内 grep 无"降级形态 / 待后续补全 / ADR-03 降级"残留。
- [ ] 所有 `gogpu/ui` 引用经 D2 决策后一致（全退役 / 仅限非弹窗）。
- [ ] `RenderModeHostManaged` 不再出现在弹窗装配路径。
- [ ] `90-UI/*` 的 `Paint` 契约与 POC `RenderPanel()` 输出（premultiplied RGBA）一致。
- [ ] 新建/澄清的 ADR-01 消除 ADR-03 的悬空引用。

## 8. 开放问题

1. **D2**：设置面板是否仍需 gogpu/ui？（决定 90-UI 全重写还是部分保留）
2. **CJK 字形**：gg 文本渲染中文走系统字体还是 `go:embed`？（关联 `40-Theme/Font.md`，v1.3 路线图）
3. **MainWindow 指代**：`90-UI/MainWindow.md` 是否即托盘弹窗？（影响 §2 #5 的改法）
