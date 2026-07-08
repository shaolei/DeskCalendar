# ADR-03: 无边框窗口样式（路径 D · gg + 原生分层窗口，完整形态）

**状态**：Revised（路径 D · gg + 原生分层窗口）· 2026-07-08（二次修订，推翻同日「路径 B 降级」）
**原始状态**：Accepted（POC 验证通过，但基于本地 patch 的 gogpu）
**日期**：2026-07-08
**作者**：Software Architect Agent
**关联**：`docs/20-Platform/WindowStyle.md`、`docs/DeskCalendar_技术评估报告_gogpu-ui-Windows.md` §8（POC，历史快照）、ADR-01（gogpu/ui）、ADR-07（依赖方向）、`internal/platform/windowstyle`

---

## Context（背景与约束）

ADR-03 的原始目标是让日历弹窗在观感上对齐 360 小清新日历：**无边框 + 每像素 alpha 透明圆角 + DWM 阴影**，悬浮于任务栏时钟之上。

### 原始 POC 的真相（重要）

技术评估报告 v4/v5 声称「ADR-03 POC 验证通过」，但那次验证依赖的是 **本地 patch 过的 gogpu**，而非上游原版。为让 POC 成立，对 `D:\workspace\github\gogpu` 打了三处补丁：

| 文件 | 补丁内容 | 用途 |
|---|---|---|
| `internal/platform/platform_windows.go` | `WS_EX_LAYERED \| WS_EX_NOREDIRECTIONBITMAP`；新增 `Window.Hide/SetPosition/SetSize` | 每像素 alpha 透明合成 + 显隐/定位 |
| `renderer.go` | `CompositeAlphaModeOpaque` → `CompositeAlphaModePremultiplied` | 透明交换链（圆角透出桌面） |
| `window_manager.go` | `positionablePlatform` 接口 + `Window.Hide/SetPosition/SetSize` | 对外暴露显隐/移动/缩放 |

**也就是说，圆角 / 每像素 alpha / 程序化显隐定位这三样能力，上游 gogpu 本来就没有，是补丁给的。**

### 2026-07-08 决策：恢复上游 gogpu

用户明确：**不希望修改依赖库，使用其原本功能**。因此 `D:\workspace\github\gogpu` 已 `git stash` 掉上述补丁，恢复到上游 `c73c0e5`（零本地分歧）。补丁备份在 `stash@{0}`，可随时 `git -C D:/workspace/github/gogpu stash pop` 取回。

### 上游 gogpu（c73c0e5）能力矩阵（实测）

| ADR-03 需求 | 上游 gogpu | 证据 |
|---|---|---|
| 无边框 | ✅ 原生 | `Config.Frameless` + `App.SetFrameless(bool)` + `WithFrameless()` |
| DWM 阴影 | ✅ 原生（frameless 时自动开） | `platform_windows.go` `if config.Frameless { 启用 DWM 阴影 }` |
| 每像素 alpha 透明 | ❌ 不支持 | `renderer.go` 硬编码 `CompositeAlphaModeOpaque`；全仓零 `WS_EX_LAYERED` |
| 圆角 | ❌ 不支持 | 全仓零 `DwmSetWindowAttribute` / `DWMWA_WINDOW_CORNER_PREFERENCE` |
| 程序化 Show/Hide | ❌ 公共 API 无 | `App`/`Window` 无 Show/Hide；内部 `PlatformWindow` 有 Show 无 Hide |
| 程序化 SetPosition/SetSize | ❌ 公共 API 无 | 仅构造期 `Config` 写死尺寸；运行期无法定位/缩放 |
| 外部 HWND / 嵌入钩子 | ❌ 无 | `Config` 无 Parent/Owner/Handle；`GetHandle()` 未对 DeskCalendar 暴露 |

### gogpu/gg 库评估（2026-07-08 · 决定路径 D 的关键）

`github.com/gogpu/gg`（已 `git clone` 到 `D:/workspace/github/gg`，HEAD `f0b4f54`）是 gogpu 生态的**纯 Go 零 CGO 2D 图形库**（Canvas/Context/Surface，GPU 加速 + CPU 回退），**不是窗口库**。全仓无 `WS_EX_LAYERED` / DWM 圆角 / Show-Hide-SetPosition / HWND；也**不依赖 gogpu/ui 或 gogpu/gogpu**（仅依赖 `gpucontext`/`gputypes`/`naga`/`wgpu`）。

关键事实（已读源码确认）：
- `Pixmap.data` 即 **premultiplied RGBA / 4 bytes per pixel**（`gg.NewContextForPixmap` + `gg.NewImageSurface` 纯 CPU 渲染、无需窗口）。
- `DrawRoundedRectangle` + 圆角裁剪可画圆角内容；角部 alpha=0 即透明圆角。
- premultiplied 位图正是 Win32 `UpdateLayeredWindow`（`AC_SRC_OVER`+`AC_SRC_ALPHA`，BGRA 仅需字节换位）所需。

**结论**：gg **不能单独**提供 ADR-03 的窗口级能力（圆角 OS 窗口 / 真透明合成 / 显隐 / 定位），但它是**路径 D（独立原生分层窗口）的理想内容/光栅化后端**——「gg 画圆角半透明面板（含阴影）→ 取 premultiplied 位图 → 自建一小段 Win32 syscall 分层窗口并 `UpdateLayeredWindow`」即可**零依赖 patch 拿到完整 ADR-03**，且平台代码是**自己的 `internal/platform/win32`** 而非改依赖库，符合「不改依赖」的意愿。

| ADR-03 缺失能力 | gg 能否直接提供 | 路径 D + gg 下如何补全 |
|---|---|---|
| M1 圆角（OS 窗口） | ❌ 不能直接 | gg 画圆角内容（角部 alpha=0）+ `WS_EX_LAYERED` 分层窗口 → 真圆角透明 |
| M2 每像素 alpha 透明 | ⚠️ 间接 | gg 产 premultiplied 位图 → `UpdateLayeredWindow` 贴到分层窗口 → 真·透出桌面 |
| M3 Show/Hide | ❌ 不能 | 自建分层窗口自行 `ShowWindow`/`SetWindowLong` |
| M4 SetPosition（贴时钟） | ❌ 不能 | 自有 HWND → `SetWindowPos` 定位到时钟下方 |
| M5 外部 HWND | ✅ 自己建窗 | 分层窗口 HWND 本就是自己创建，无反射依赖 |

---

## Decision（决策 · 路径 D：gg 渲染 + 原生分层窗口，恢复完整 ADR-03）

**采用路径 D：托盘弹窗用「gg 绘制内容 + 自建 `WS_EX_LAYERED` 原生分层窗口」实现，不再依赖 gogpu 的窗口/swapchain 透明度能力，也不 patch gogpu。**

- 渲染层：`gg`（纯 Go 零 CGO）在 `gg.ImageSurface` / `gg.NewContextForPixmap` 上绘制圆角半透明日历面板（含 DWM 风格阴影），产出 **premultiplied RGBA** 位图。
- 窗口层：新增平台专属包 `internal/platform/win32`（Windows 实现），用 `golang.org/x/sys/windows` 调用 `CreateWindowExW(WS_EX_LAYERED | WS_EX_NOACTIVATE | WS_EX_TOOLWINDOW)` + `UpdateLayeredWindow` 把 gg 位图上屏；`ShowWindow`/`SetWindowPos` 实现显隐与贴时钟定位。该包为**本项目自有代码**，不修改任何依赖库。
- 由此 **M1–M5 全部可达成**：真圆角 + 真每像素 alpha 透明 + 自由 Show/Hide + 贴时钟定位，且 `WindowStyle` 的 `Layered`/`PerPixelAlpha`/`CornerRadius` 字段从「死字段」变为**真实生效**。
- `gogpu/ui`（ADR-01）对**托盘弹窗**不再需要；gogpu 仅保留给需要 GPU 渲染的部件（如主窗口/设置面板，若后续需要）。详见下方「受影响文档」对 ADR-01 的影响。
- `WindowStyle.RenderMode` 本地枚举（Auto/CPU/GPU）保留：`gg` 侧可映射为 `gg` 的 GPU/CPU 渲染选择（`gg.NewContext` 默认 GPU 加速、可回退 CPU），与 gogpu 解耦不变。

### 路径 D 下的实际观感

- 弹窗 = **圆角、半透明（透出桌面）、带柔和阴影**的悬浮面板，观感对齐 360 小清新日历。
- 显隐 = 自建分层窗口 `ShowWindow(SW_HIDE/SHOW)` 原位切换，无重建开销。
- 位置 = `SetWindowPos` 精确定位到任务栏时钟正上方，点击外部自动隐藏。
- 内容仍由 gg 高质量 GPU/CPU 光栅化（日历网格、农历、渐变背景）。

---

## 缺失功能清单（路径 D + gg 下均已可达成）

| # | 能力 | 路径 D + gg 实现方式 | 负责里程碑 | 当前状态 |
|---|---|---|---|---|
| M1 | 圆角 | gg 圆角内容 + `WS_EX_LAYERED` 分层窗口（角部 alpha=0 即真圆角） | v1.0（Phase 3 shell） | 待实现（gg 已具备；窗口层待建） |
| M2 | 每像素 alpha 透明 | gg premultiplied 位图 → `UpdateLayeredWindow` | v1.0（Phase 3 shell） | 待实现 |
| M3 | 程序化 Show/Hide | 自有分层窗口 `ShowWindow` | v1.0（Phase 3 shell） | 待实现 |
| M4 | SetPosition（贴时钟） | 自有 HWND `SetWindowPos` 到时钟下方 | v1.0（Phase 3 shell） | 待实现 |
| M5 | 外部 HWND | 分层窗口本就自建，无反射依赖 | v1.0（Phase 3 shell） | 待实现 |

> 路径说明（历史候选，已决）：
> - **路径 A（恢复 patch）**：需 `git stash pop` 改 gogpu，**已否决**（违反「不改依赖」意愿）。
> - **路径 B（降级）**：仅 frameless + DWM 阴影，方角不透明、无法贴时钟——**已被本次决策推翻**。
> - **路径 C（反射取 hwnd）**：依赖未导出字段、真透明仍做不到——**已否决**。
> - **✅ 路径 D（gg + 原生分层窗口）**：零依赖 patch 拿完整 ADR-03，本次选定。

---

## Consequences（后果）

- ✅ **不修改任何依赖库**：gg 当原样用；分层窗口是本项目自有 `internal/platform/win32` 代码，符合「不改依赖」意愿。`gogpu` 已恢复原版不必再碰。
- ✅ **完整 ADR-03 可达**：真圆角 + 真每像素 alpha 透明 + 自由 Show/Hide + 贴时钟定位；`WindowStyle.Layered/PerPixelAlpha/CornerRadius` 从声明字段变为真实生效。
- ✅ 纯 Go 零 CGO（ADR-06 不变）；gg 与 `golang.org/x/sys` 均零 CGO。
- ⚠️ **需重架构**：ADR-01（UI 层）受牵连——托盘弹窗渲染从 `gogpu/ui` 切到 `gg + 分层窗口`，需改写 ADR-01 与 `90-UI/*` 相关描述；新增 `internal/platform/win32` 平台专属代码（仅 Windows 编译，用 build tag）。
- ⚠️ 分层窗口的显隐/定位/消息循环需自行维护（点击外部关闭、DPI 缩放、双击穿透等），是新增工作量；`gg` 仅负责绘制。
- ⚠️ 既有文档（见下）需同步修订以反映路径 D。

---

## 受影响文档（待同步修订，非本 ADR 范围）

以下文档需按「路径 D + gg 分层窗口」修订（仍按旧的「gogpu/ui 渲染 + patch 版透明」或「路径 B 降级」描述，与本次决策冲突）：

- **`docs/ADR-01-*.md`（UI 层决策）** — ⚠️ 受牵连最大：托盘弹窗渲染从 `gogpu/ui` 改为 `gg + 原生分层窗口`，需改写 ADR-01 结论与「UI 用 gogpu/ui」的表述。
- `docs/10-Shell/App.md:110` — 移除 `gogpu.NewApp(gogpu.Frameless, ...)` + 每像素 alpha 的弹窗装配，改为分层窗口 + gg。
- `docs/10-Shell/Layout.md` — 根容器背景改为 gg 绘制；`ClipRRect` 圆角由 gg `DrawRoundedRectangle` 实现。
- `docs/90-UI/MainWindow.md:239` — 移除 `gogpu.Frameless + RenderModeHostManaged + ThemeBackground()`，改 gg + 分层窗口。
- `docs/90-UI/Animation.md:4` — 关联「透明圆角」（仍成立，但实现层改为 gg）。
- `docs/40-Theme/Theme.md:316` — 面板观感「圆角透明 + DWM 阴影」（成立，但背景由 gg 绘制）。
- `docs/20-Platform/Acrylic.md:16` / `Mica.md:16` — 自绘渐变 + 每像素 alpha 替代（仍成立）。
- `docs/20-Platform/WindowStyle.md` — 本 ADR 已修订；其 §9 本地 `platform.RenderMode` 保留，新增「gg 渲染模式映射」说明。
- `docs/DeskCalendar_技术评估报告_gogpu-ui-Windows.md` §8 — POC 段落为**历史快照**，基于已 stash 的 patch，应标注「仅适用于 patch 版 gogpu，已被路径 D 取代」。

---

## Reversibility（可回退性）

本决策**非破坏性、可逆**，且比其他路径更稳：

- **退回路径 B（降级）**：不实现 `internal/platform/win32` 分层窗口，弹窗退回 gogpu frameless + DWM 阴影（方角不透明）。gg 渲染层可保留（仅作内容绘制）或移除。
- **改走路径 A（恢复 patch）**：若日后接受改依赖库，`git -C D:/workspace/github/gogpu stash pop` 取回补丁，可并行或替代分层窗口方案；`WindowStyle` 字段兼容。
- gg 与原生分层窗口代码均为本项目自有，随时可启用/停用，无外部锁定。
