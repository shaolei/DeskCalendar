# ADR-08：降级方案 —— 脱离 gogpu 上游阻塞，核心功能自立

## Status
Confirmed（已确认生效，2026-07-09）

## Context（根因，全部基于本地依赖库实代码核对）

我们的 Phase 3（Shell 装配）原方案（ADR-01 / ADR-03）在技术评估报告中被判为"风险清零"，但**那次验证用的是本地打过补丁的 gogpu**（通过 `go.mod replace` 指向本地克隆 + 本地 `ui` 仓库）。现在 gogpu 上游（commit `0578472`, v0.44.1）对补丁 issue **无回应**，必须重新审视"我们到底被上游卡住了什么"。

经逐文件核对本地仓库，得到三个硬事实：

**事实 1：`gogpu/ui` 不在 gogpu 主仓，是独立仓库。**
- gogpu 主仓（`D:\workspace\github\gogpu`）顶层目录只有 `assets/docs/examples/gmath/gpu/input/internal/scripts/sound/testdata`，**没有 `ui` 包**。
- `gogpu/ui` 是另一个仓库（`D:\workspace\github\ui`）。这意味着 ADR-01 写的"用 `gogpu/ui` 做响应式框架"依赖的是一个**独立、且我们需自行 vendored 的仓库**，而非 gogpu 主仓的一部分。
- 而我们实际已在用的响应式原语 `coregx/signals`（go.mod 直接依赖）就是 `gogpu/ui/state.Signal` 的类型别名来源——**UI 框架本身我们早已不真正依赖 gogpu/ui**。

**事实 2：Windows 窗口的 Hide / SetPosition / SetSize 只存在于我们的本地 stash，未进上游。**
- `git -C D:\workspace\github\gogpu stash list` → `stash@{0}: DeskCalendar ADR-03 windowing patches: WS_EX_LAYERED + CompositeAlphaModePremultiplied + Window.Hide/SetPosition/SetSize`。
- `internal/platform/platform_windows.go` 中 `win32Window` 当前方法集（实测）：`ID / GetHandle / ScaleFactor / PrepareFrame / ShouldClose / SetTitle / SetMinSize / SetMaxSize / SetCursor / SetFrameless / IsFrameless / SetHitTestCallback / Minimize / Maximize / IsMaximized / SetFullscreen / IsFullscreen / Close / Show / SyncFrame / SetCursorMode / CursorMode / SetModalFrameCallback`。
  - **缺：`Hide()`、`SetPosition()`、`SetSize()`、`SetBounds()`**（macOS 侧 `darwin/window.go` 仅有 `Show/Hide/SetSize`，也无 `SetPosition`）。
- `CreateWindowExW` 调用处 `dwExStyle = 0`（`platform_windows.go:655,662`），**无 `WS_EX_LAYERED`**。
- `renderer.go:293`：`AlphaMode: gputypes.CompositeAlphaModeOpaque`，**无每像素 alpha**。

**事实 3：已有零补丁、零 CGO 的替代 POC 且已能跑。**
- `poc/layered-window/` 提供完整证明：用 `github.com/gogpu/gg`（纯 Go 2D 光栅化）绘面板 → premultiplied RGBA 缓冲 → 自建 `WS_EX_LAYERED` 分层窗口推送。**全程未引用 gogpu 主仓、未打任何补丁、零 CGO。**
- gg 经核实：**零 CGO**（无 `import "C"`/`#cgo`），核心 `context.go/pixmap.go/device.go` 不引 `gpu`；`examples/cjk_text`（ADR-027）已验证中文渲染（农历/节气/节假日无碍）。

**事实 4（新）：`gogpu/systray` 菜单能力齐全，设置走托盘菜单零成本。**
- 本地 `D:\workspace\github\systray\internal\menu.go` 提供 `Add` / `AddCheckbox` / `AddSeparator` / `AddSubmenu`，菜单项类型含 `Normal/Checkbox/Separator/Submenu`。
- 说明设置项的"勾选/分组/子菜单"需求**完全可由托盘右键菜单承载**，无需自建设置窗。

**产品需求裁定（用户 2026-07-09 补充）—— 进一步收窄 MVP：**
- **R1**：点击 Win11 dock 时间日期 → 日历只固定在右下角托盘上方弹出，与系统默认一致；**不移动窗口、不改变窗口大小**。→ 砍掉拖拽移动、运行时改尺寸。
- **R2**：透明窗口 + 圆角 **不是核心功能、优先级低、允许舍弃**。→ MVP 砍掉透明/圆角/阴影，降级为**普通不透明方角弹窗**；圆角后续可用 Win11 `DwmSetWindowAttribute(DWMWA_WINDOW_CORNER_PREFERENCE, DWMWCP_ROUND)` 零成本白嫖（不需要 gg 也不需分层窗口）。
- **R3**：日历预计有设置功能；设置表单可走**托盘右键菜单**（简单勾选项）或**独立弹窗**（复杂时），按复杂度取舍。→ MVP 优先菜单，独立窗仅作后备。

**结论**：原方案把"透明圆角 / 弹窗锚定 / 显隐 / 每像素 alpha / 拖拽"全部押在 gogpu 上游接受补丁上；而事实 3 已证明同一组特性用 gg + 自拥 win32 即可 100% 实现且不依赖上游。结合 R1–R3，**MVP 甚至不需要分层窗口**——普通不透明弹窗（gg 绘实心背景 + DIBSection/WM_PAINT 推送）就是最简解，比 POC 还轻。既然上游无回应，继续等待就是阻塞核心功能。

## Decision（降级方案）

**将 Path D（gg + 自拥 win32 窗口）从"备选 fallback"提升为 MVP 唯一主线；正式放弃对 gogpu 主仓 + gogpu/ui 框架 + wgpu GPU 渲染的 Blocking 依赖。MVP 窗口进一步降级为「固定位置、不透明、普通弹窗」，舍弃移动/改尺寸/透明圆角/阴影/显隐动画（均为用户裁定可舍或需求不需要的润色）。**

具体落地：

1. **渲染后端**：弃用 gogpu/wgpu，改用 `github.com/gogpu/gg`（纯 Go CPU 光栅化）绘制面板到 `image.RGBA`。MVP 绘**实心背景**（不再绘透明圆角/阴影）。
2. **窗口后端**：弃用 gogpu 的 `Window`/`win32Window`，在 `internal/platform/win32`（新建）用 `x/sys/windows` 自建**普通弹窗**（`WS_POPUP` + `WS_EX_TOPMOST`），经 **DIBSection + `WM_PAINT`/`BitBlt`** 推送 gg 像素（比 POC 的 `UpdateLayeredWindow` 更简单、无 premultiplied-alpha 细节坑）。窗口**固定尺寸、初次 `SetWindowPos` 锚定托盘上方后即不再移动/缩放**。关闭：自拥 `WM_ACTIVATE`（`WA_INACTIVE`→Hide）、`Esc`→Hide。DPI：`.SetProcessDpiAwarenessContext(-4)` + `WM_DPICHANGED`。
3. **响应式**：继续用 `coregx/signals`（已是直接依赖），不引 `gogpu/ui`。
4. **托盘 + 设置**：**保留 `gogpu/systray`**（独立仓库、零 CGO、真机验证过 `Bounds()/OnClick/菜单/通知`，与本决策解耦）。设置项 MVP 走**托盘右键菜单**（`AddCheckbox`/`AddSubmenu`），如"显示农历/显示节假日/开机启动/主题"等布尔项直接勾选；仅当设置复杂到菜单放不下（如自定义主题色、字号、周起始日）时，才用同一窗口工厂开**第二 gg 弹窗**，成本极低。
5. **接口收敛**：`WindowController` 精简为 `Show / Hide / AnchorAboveTray(rect) / Visible`（去掉 `Move`/`Resize` 这类 MVP 不需要的方法）；底层实现从 `*gogpu.Window` 换成自拥弹窗。`internal/ui` 视图改为"渲染到 RGBA 缓冲"而非产出 `gogpuui.Node`。状态层 / 域层 / 插件层（event bus）完全不受影响。
6. **ADR-03 修订**：ADR-03 原定的"无边框 + 每像素 alpha 透明圆角 + DWM 阴影"在 MVP 中**降级为可选项**——MVP 用不透明方角弹窗；透明圆角/阴影后续视视觉优先级再决定（届时走 `WS_EX_LAYERED` 或 DWM corner preference，与上游无关）。

## 特性降级矩阵

| # | 产品特性 | 原方案（依赖上游补丁） | 降级后（Path D / MVP） | 是否砍 | 备注 |
|---|---------|----------------------|----------------------|--------|------|
| F1 | 透明圆角面板 + 柔和阴影 | gogpu/ui + 补丁 `WS_EX_LAYERED` | **MVP 舍弃透明/圆角/阴影**，改用 gg 绘**实心不透明方角面板**；圆角后续可用 `DwmSetWindowAttribute` 零成本白嫖 | ✅ MVP 主动舍弃 | 用户裁定：低优先级、可舍 |
| F2 | 点击托盘弹窗 + 锚定右下 | gogpu.Window + 补丁 `Hide/SetPosition` | 自拥 `ShowWindow` + 一次性 `SetWindowPos` 锚定托盘上方 | ❌ 保住 | 窗口固定不动，仅初次定位 |
| F3 | DPI Per-Monitor V2 | gogpu 内置 | 自拥 `SetProcessDpiAwarenessContext(-4)` + `WM_DPICHANGED` | ❌ 保住 | 标准 win32 |
| F4 | 拖拽移动窗口 | gogpu `SetHitTestCallback` | **舍弃**（需求 R1：日历不移动窗口） | ✅ 主动舍弃（需求裁定） | 不再需要 `WM_NCHITTEST`/`HTCAPTION` |
| F5 | 点击外部/失焦关闭 | gogpu/ui | 自拥：`WM_ACTIVATE` `WA_INACTIVE`→Hide；`Esc`→Hide；`WS_EX_TOPMOST` 弹窗 | ❌ 保住 | 比补丁方案更可控，无需鼠标钩子 |
| F6 | 显隐过渡动画（淡入+上移） | gogpu 帧循环 + alpha | **MVP 舍弃动画**（无分层窗口则无逐像素 alpha 淡入），改为瞬时显示；后续如需可走 layered+alpha 或 DWM 过渡 | ⚠️ MVP 降级为瞬时 | 纯视觉润色，不影响功能 |
| F7 | 多窗口（设置/Todo） | gogpu 多 Window | MVP 单窗；设置走**托盘右键菜单**（systray 支持 checkbox/submenu）；仅复杂时建第二 gg 弹窗 | ⚠️ 设置改菜单优先，独立窗后置 | 复用自拥窗口工厂，成本低 |
| F8 | Mica/Acrylic 毛玻璃 | 已 ADR-04 跳过 | 跳过（不变） | ❌ 已弃 | 自绘渐变替代 |
| F9 | **GPU 加速渲染（wgpu）** | gogpu wgpu | **gg CPU 光栅化** | ✅ 主动舍弃 | 360×480 面板 CPU 足够，去驱动/WebGPU 风险 |
| F10 | **gogpu/ui 部件树 / 布局 / 主题引擎** | `gogpu/ui` 框架 | 自拥轻量布局（固定日历网格）+ gg 绘制 | ✅ 主动舍弃框架依赖 | 非砍功能，是去掉外部框架 |

> 说明：被"舍弃"的有 **F9（GPU 渲染）**、**F10（gogpu/ui 框架依赖）** 两项技术栈，以及 **F1（透明圆角阴影，MVP）**、**F4（拖拽移动，需求裁定）**、**F6（显隐动画，MVP）** 三项视觉/交互润色。**所有核心功能（公历+农历+节假日日历展示、托盘锚定弹出、DPI 自适应、失焦关闭、设置）均保住**，仅实现机制从"等上游补丁"换成"自己掌控"，且 MVP 窗口形态已进一步简化为普通弹窗。

## 降级后的目标架构（MVP）

```
┌─ 主 goroutine ───────────────────────────────────────┐
│  自拥消息循环: GetMessage/Dispatch                    │
│    ├─ internal/ui: 视图渲染 → gg → image.RGBA        │
│    ├─ internal/platform/win32: 普通弹窗(DIBSection+WM_PAINT) │
│    │     · 固定尺寸 · 初次 SetWindowPos 锚定托盘上方    │
│    │     · WM_ACTIVATE/Esc → Hide                     │
│    └─ internal/state (coregx/signals): 响应式状态      │
└───────────────────┬──────────────────────────────────┘
                    │ channel (Toggle/Show/Hide/Anchor)
┌───────────────────┴──────────────────────────────────┐
│  systray goroutine (gogpu/systray, 独立, 零 CGO)      │
│    OnClick → ch<-Toggle; Bounds() 提供锚点             │
│    右键菜单: AddCheckbox/AddSubmenu → 设置项          │
└──────────────────────────────────────────────────────┘
依赖: coregx/signals · gogpu/systray(replace 本地) · gogpu/gg(replace 本地)
      · golang.org/x/sys · 6tail/lunar-go · holiday-cn
❌ 不再依赖(最终二进制层): gogpu 主仓 · gogpu/ui · wgpu(运行时不被编译进 DeskCalendar 二进制; CPU 光栅路径不触碰 GPU 后端, 见文末"确认与实证")
```

**关键复用**：`WindowController` 收敛接口、`state` 事件总线、`plugin` Host 边界、`calendar` 域、`theme` 模型——全部原封不动。改动集中在「渲染+窗口」这一层，且已有 POC 兜底（POC 的分层窗口代码保留作参考，MVP 走更简单的普通弹窗路径）。

**为什么 MVP 窗口比 POC 还简单**：POC 为了透明圆角用了 `WS_EX_LAYERED` + `UpdateLayeredWindow` + premultiplied alpha（有字节序/alpha 预乘的细节坑）。既然 R2 允许舍弃透明，我们改用 `WS_POPUP` 普通弹窗 + DIBSection + `WM_PAINT`/`BitBlt`——这是最经典的 GDI 推送方式，gg 只负责产出像素，稳定性最高、代码量更小。

## 重新划分的 MVP 范围（建议）

- **v1.0（核心，自立）**：单窗**不透明方角**日历面板（公历+农历+节气+节假日+调休）、托盘点击锚定右下弹出、DPI 自适应、失焦/Esc 关闭、**托盘右键菜单承载设置项**（显示农历/节假日/开机启动/主题等勾选）、系统托盘+通知+暗色图标、开机自启。→ 完全不依赖上游。
- **v1.1（可选视觉润色，按优先级）**：Win11 `DwmSetWindowAttribute` 圆角（零成本）；如需再补柔和阴影。
- **v1.2**：显隐动画（若产品认为必要，走 layered+alpha 或 DWM 过渡，自绘 `Animator` 已在 Animation.md 设计且无 gogpu 依赖）。
- **v1.3**：复杂设置页——若右键菜单放不下，用同窗口工厂开**第二 gg 弹窗**（同机制，仅多一个窗口实例）；皮肤/字体（theme 已就绪）。
- **v1.4+**：多屏/DPI 锚定换算、插件只读可见性、自动更新等（原计划不变）。

> 注：v1.1/v1.2 的润色项（圆角/阴影/动画）均为**可独立追加、互不影响**的增量，不影响 v1.0 核心交付，且都**不依赖 gogpu 上游**。

## Consequences（权衡 —— 我们放弃了什么）

**得到的（收益）**
- 不再被无回应的上游阻塞；MVP 100% 自拥有、可离线 `CGO_ENABLED=0` 构建。
- 依赖图大幅瘦身、风险面缩小：去掉 wgpu/WebGPU 驱动与 GPU HAL 不确定性。
- 窗口实现比 POC 更轻：普通弹窗 + DIBSection 推送，无分层/alpha 细节坑。
- 设置走托盘菜单，零额外窗口/UI 负担（systray 菜单 API 已验证齐全）。
- 完全可逆：`WindowController` 抽象已隔离窗口实现，未来若上游合并补丁并想用 GPU，可在不改 UI 代码的前提下换回 gogpu 后端（gg 输出本就是 RGBA 缓冲，理论上也能喂给 gogpu 表面）。

**付出的（代价）**
- **窗口层维护责任转移给自己**：约 100~180 行 win32 代码（已有 POC 原型可改），需自行保证正确性（DPI、失焦、多屏）。但这是稳定、一次性的平台代码，且已脱敏为纯 `x/sys/windows`，无 CGO 编译负担。
- **失去 gogpu/ui 声明式部件树**：日历面板布局需自写（固定网格，复杂度低）；若未来 UI 变复杂（大量动态控件），自拥布局成本上升——但 MVP + 设置菜单阶段远未到临界点，且复杂设置页可复用同一窗口工厂。
- **MVP 视觉降级**：方角不透明面板（无圆角/阴影/淡入）。这是用户明确裁定可舍的低优先级项；圆角等可后续零成本追加。
- **CPU 渲染**：对 360×480 静态面板性能无感；若以后要做大面积实时特效，需重新评估——届时再引入 GPU 后端即可。

## 开发难度评估（gg 直接绘制 vs gogpu/ui 部件树）

针对"改用 gg 直接绘制是否会加大开发难度"的疑问，给出诚实对比。**核心判断：对 DeskCalendar 这种固定布局的日历面板，gg 直接绘制难度持平甚至更低；仅在 UI 演化为大量动态表单/复杂响应式布局时，手绘布局才会成为税负。结合 R1–R3，MVP 窗口进一步简化为「固定不透明弹窗」，渲染层比最初设想更轻。**

| 维度 | gogpu/ui 部件树（若上游可用） | gg 直接绘制（Path D） |
|------|------------------------------|----------------------|
| 初始接入 | 学框架 API + vendored 独立仓库 | 写一个 `Render(model, theme) → RGBA` 函数 |
| 布局 | 框架内置（弹性/自动尺寸） | 自算网格矩形（固定面板 = 一次性纯函数，~30 行） |
| 文字/CJK | 框架处理 | gg `LoadFontFace` + 已验证中文（`examples/cjk_text`） |
| 命中测试/点击 | 框架事件 | 鼠标坐标 → 格子矩形（简单矩形命中） |
| 主题 | 框架 Theme | `internal/theme`（Phase 2 已建）直接喂 gg 颜色/字体 |
| 状态→重绘 | Signal 绑定 | `coregx/signals` 变更 → 重渲缓冲 → 推窗口（已有） |
| 构建/依赖风险 | 依赖无回应上游 | 零（gg 本地、零 CGO） |
| 大量动态控件/表单 | ✅ 框架擅长 | ⚠️ 需逐控件手绘布局（未来若是此类 UI 才显痛） |

**为什么本产品不痛**：日历面板是**固定静态布局**——表头 + 7×N 网格，变化的只是格子里的数字/农历/节假日标记。布局计算是一次性的纯函数 `Layout(w,h) → []CellRect`（可单测）；渲染是 `(model, layout, theme) → buffer` 的纯函数。这与"需要弹性布局 + 几十个可交互控件的设置页/聊天页"完全不同，后者才是手绘布局的痛点——而我们的设置页走的是托盘菜单，恰好避开了这个痛点。

**控制难度的架构做法（建议）**：
1. 不裸调 gg 全局画。建一层薄渲染层 `internal/ui`：拆 `drawHeader/drawCell/drawLunar` 等小助手，面板 = 这些助手的组合。这层就是我们自拥的"极简部件抽象"，只覆盖所需。
2. 保留 `WindowController` + `state` Signal 边界：UI 只响应 Signal 并触发重渲，不写意大利面。
3. 复用已建的 `theme` 包（颜色/字体/SystemScheme），不重造主题。
4. 重绘成本：每次状态变更整面板重光栅（360×480）——CPU 光栅亚毫秒到几毫秒，非性能问题。
5. 设置优先托盘菜单（`AddCheckbox` 等），避免为简单布尔项开独立窗。

**净权衡**：MVP 下 gg 直接绘制比"接入并等待无回应上游框架"更简单（无框架要学、无上游要等、布局是平凡网格、窗口是普通弹窗）。我们付出的真实代价是**未来复杂 UI 的可扩展性**——但这是可逆赌注：渲染层被缓冲边界隔离，未来可在 gg 之上加一层 retained widget，或上游合并补丁后换回 gogpu 表面，UI 业务代码不受影响。

## 行动项（已确认，Phase 3 开工执行）

1. `go.mod` 调整：移除对 gogpu 主仓的一切 `replace`/`require`（Phase 3 前本就不该有）；新增 `replace github.com/gogpu/gg => D:/workspace/github/gg`、`replace github.com/gogpu/systray => D:/workspace/github/systray`；确认 `go-webgpu/goffi` 的 v0.5.6 replace 仍保留（gg 模块图需要解析）。
2. 将 `poc/layered-window/draw_gg.go` 提升为 `internal/ui` 渲染层（gg 绘实心面板）；POC 的 `layered_windows.go` 保留作参考，**MVP 新建 `internal/platform/win32` 为普通弹窗**（DIBSection + `WM_PAINT`/`BitBlt`，非 layered）。
3. 新建 `WindowController` 自拥后端：方法收敛为 `Show / Hide / AnchorAboveTray(rect) / Visible`；替换 `Window.md` 里对 `*gogpu.Window` 的包装。
4. 托盘右键菜单：用 `gogpu/systray` 的 `AddCheckbox`/`AddSubmenu` 实现 v1.0 设置项（显示农历/节假日/开机启动/主题），绑定到 `state` Signal。
5. 真机 Windows 跑一遍普通弹窗（像素对比法）确认锚定/实心面板/失焦关闭/CJK 渲染。
6. 中文渲染验收：gg `LoadFontFace` 加载系统中文字体（如 `msyh.ttc`）或内嵌 TTF，确认农历/节气显示正常。
7. 文档修订：`Window.md` / `90-UI/*` 中 `gogpuui.Node`、`gogpu.App.OnUpdate` 等表述改为自拥渲染；ADR-01 状态更新为"降级：弃用 gogpu/ui，改用 gg + 自拥弹窗"；ADR-03 标注透明圆角/阴影在 MVP 降级为可选项。

## 附录：代码证据清单

- `D:\workspace\github\gogpu` → `git stash list`：`stash@{0}` 含 `WS_EX_LAYERED + CompositeAlphaModePremultiplied + Window.Hide/SetPosition/SetSize`（即上游缺的能力）。
- `D:\workspace\github\gogpu\internal\platform\platform_windows.go:655,662`：`CreateWindowExW` 的 `dwExStyle = 0`；方法集无 `Hide/SetPosition/SetSize`。
- `D:\workspace\github\gogpu\renderer.go:293`：`AlphaMode: gputypes.CompositeAlphaModeOpaque`。
- `D:\workspace\github\gogpu` 顶层无 `ui/` 目录（gogpu/ui 为独立仓库）。
- `D:\workspace\github\systray\internal\menu.go`：`Add/AddCheckbox/AddSeparator/AddSubmenu` + `MenuItemNormal/Checkbox/Separator/Submenu`，菜单能力齐全。
- `D:\workspace\aicoding\DeskCalendar\poc\layered-window\draw_gg.go` + `layered_windows.go`：gg + 自拥分层窗口零补丁 POC（作参考，MVP 走更简单普通弹窗）。
- `D:\workspace\github\gg`：`grep -rln 'import "C"'` 无结果（零 CGO）；`examples/cjk_text` 验证中文渲染。
- `D:\workspace\aicoding\DeskCalendar\go.mod`：当前未引入 gogpu 主仓（Phase 3 才引入）。

## 确认与实证（2026-07-09）

**状态变更**：由 `Proposed` 转为 `Confirmed`，作为 Phase 3 唯一主线开工依据。

**关键实证（采纳前补测，澄清一处措辞）**：
- 本地 `gg`（`D:/workspace/github/gg`，HEAD f0b4f54）根包 `context.go` / `context_image.go` / `accelerator.go` 仍 `import "github.com/gogpu/gpucontext"`，其 `go.mod` 传递 require `wgpu` / `gpucontext` / `naga` / `go-webgpu/webgpu`。
- 但实测 `cd poc/layered-window && CGO_ENABLED=0 go build -tags gg .` **编译通过（EXIT=0）**，且 POC 已生成 `poc_gg.exe` / `panel.png`（headless 光栅成功）。说明 gg 的传递 GPU 依赖**不编译进 DeskCalendar 最终二进制**（CPU 光栅路径不触碰 GPU 后端），零 CGO 离线构建成立。
- 因此上文"`❌ 不再依赖 wgpu`"的准确含义是**二进制层**；`wgpu` 仍作为 gg 的传递依赖存在于模块图（离线模块缓存可解析，无需联网）。若日后需要真正从 go.mod 清单层移除 wgpu，可改用 gg 的纯 CPU 子包或 fork 裁剪——属后续优化，非 MVP 阻塞。

**go.mod 调整（已落地）**：
- 新增 `require github.com/gogpu/gg v0.0.0-00010101000000-000000000000` 与 `replace github.com/gogpu/gg => D:/workspace/github/gg`。
- 保留 `replace github.com/gogpu/systray => D:/workspace/github/systray` 与 `replace github.com/go-webgpu/goffi v0.5.5 => v0.5.6`。
- 注意：`gg` 在 `internal/ui` 实际 import 前仅为声明依赖；CI 验证用 `go build/vet/test` + `CGO_ENABLED=0 go build`（不跑 `go mod tidy`，避免 tidy 误删未引用 require）。

**Phase 3 任务重排**：受影响 epic 为 #1/#9/#17/#24（Shell）与 #105/#110/#115/#120（UI），重排计划见 `docs/Phase3-重排计划.md`。
