# ADR-07: 事件总线归属与 gogpu 入口 / 符号命名约定

**状态**：Accepted（已拍板 · 2026-07-08）
**日期**：2026-07-08
**作者**：Software Architect Agent
**关联**：ADR-02（gogpu/systray 双循环）、技术评估报告 v5、`docs/_交叉一致性审查报告.md`（F1 / F4 / F5 / F7）、`80-Plugin/Event.md`、`01-总体架构.md` §2 依赖方向

---

## Context（背景与约束）

`docs/` 设计文档在「交叉一致性审查」（`_交叉一致性审查报告.md`）中暴露出两类问题：一类是**分层正确性缺陷**（P0），一类是**术语 / 符号不一致**（P2）。二者都指向同一个根因——文档在「事件总线归谁所有」和「gogpu 主入口长什么样」上缺乏单一事实来源（single source of truth）。

### F1（P0 · 分层崩溃级）

`80-Plugin/Event.md` 原稿声明事件总线实现归属 `internal/plugin`，并画出反向依赖边：

```
internal/plugin  ◀─依赖──   internal/calendar   (emit DateChanged)
internal/plugin  ◀─依赖──   internal/ui         (emit PanelShown/Hidden)
internal/plugin  ◀─依赖──   internal/theme      (emit ThemeChanged)
internal/plugin  ◀─依赖──   internal/shell      (emit LifecycleChanged)
internal/plugin  ◀─依赖──   internal/app        (装配总线)
```

这**直接违反** `01-总体架构.md` §2 的依赖倒置铁律：

> `plugin` 以接口钩子反向依赖 `feature/ui`（依赖倒置，**不反向编译依赖**）。底层不依赖上层。

含义：如果 `calendar`/`ui`/`theme`/`shell`/`app` 在编译期 import `plugin` 来发事件，那么插件层就不再是「可插拔、可编译隔离」的可选模块——feature 反过来被 plugin 绑架，分层崩溃，feature 单测也必须带上 plugin 才能编译。

### F4 / F5 / F7（P2 · 术语分裂）

- **F4**：主入口写法分裂——部分文档写 `gogpu.App.Run()`，部分写 `desktop.Run(gogpuApp, uiApp)`。二者不等价：前者是误用的「App 自带 Run」，后者才是真实的双循环概念入口（`desktop.Run` 内部 `runtime.LockOSThread` 拉起主线程，再委托 gogpu/ui 帧循环）。
- **F5**：`RenderModeHostManaged` 在 `20-Platform/WindowStyle.md` 里被定义成**本地 iota 枚举**，却在 `10-Shell/App.md` 里被当作 **gogpu 导出符号**使用。同名的两种来源必然在读者脑中分裂。
- **F7**：`gogpu.NewApp` 调用形式不一致（位置参数 vs `gogpu.DefaultConfig().WithXxx()`），导致示例代码不可直接对拍。

**硬约束（继承已拍板决策）**：
1. **零 CGO**：gogpu 全系纯 Go，`CGO_ENABLED=0` 可编译（ADR-01 / ADR-06）。
2. **依赖方向单一**：`app → shell → platform/state → feature → theme`，`plugin` 永远是被倒置方，不成为上层编译依赖。
3. **可逆**：插件层作为可选模块，必须能从主二进制编译图中摘除而不影响 feature。

---

## 决策拆分

本 ADR 拆为两个相互独立的子决策：

- **ADR-07a**：事件总线实现下沉到 `internal/state`（依赖倒置铁律落地）
- **ADR-07b**：主入口与 gogpu 导出符号命名约定（消除术语分裂）

---

## ADR-07a: 事件总线实现归属 `internal/state` ✅ 已定

### 决策

1. **事件总线（EventBus）的实现落在 `internal/state`**，而不是 `internal/plugin`。
2. **feature → state（emit）**：`calendar` / `ui` / `theme` / `shell` / `app` 通过 `state.Publish(topic, payload)` 发布领域事件；它们**只编译依赖 `state`，不依赖 `plugin`**。
3. **plugin → state（subscribe）**：插件经 `Host.Subscribe(topic, handler)` 订阅；`Host.Subscribe` 在内部**委托**给 `state` 的总线，插件本身不直接持有总线实现。即：

   ```
   feature ──emit──▶  state (EventBus 实现)
                              ▲
                          subscribe (经 Host 委托)
                              │
                           plugin
   ```

4. **删除所有 `plugin ◀─依赖── feature` 编译边**。原 `Event.md` 中的 `Bus` / `Event` / `EventTopic` / `EventHandler` / `Topic*` 等符号改由 `internal/state` 导出（或 `state` 再委托给一个极薄的 `internal/state/event` 子包，但**归属权在 state**，不在 plugin）。

### 候选方案

| 方案 | 含义 | 取舍 |
|------|------|------|
| **A1. 总线实现归 `internal/state`** ✅选定 | feature→state emit，plugin→state subscribe（经 Host 委托） | 复用既有 `feature → state` 分层，零新增包；依赖倒置铁律天然成立；plugin 可编译隔离 |
| A2. 新增 `internal/core/events` 独立包 | 把所有事件类型与总线抽到全新包 | 更「名正言顺」，但**多一个包**、多一层依赖（`feature→core/events`、`plugin→core/events`），对当前规模是过度设计 |
| A3. 维持原稿：总线归 `internal/plugin` | feature 反向依赖 plugin | ❌ 直接违反依赖倒置，分层崩溃，否决 |

### 决策：采用 A1

**理由**：
- 与 `01-总体架构.md` §2 的依赖方向**完全一致**——`state` 本就是 feature 的下游汇聚点（feature 早已依赖 `state` 的 Signal/Store），把事件总线并入 `state` 不引入任何新依赖边。
- plugin 经 `Host` 接口订阅，仍守住「feature/ui 不编译依赖 plugin」——插件可单独编译、可热拔插、可 mock，feature 单测无需 plugin。
- 比 A2 少一个包，符合「无架构宇航员」原则：在 MVP 规模下，`state` 承载事件总线足够，不必为「纯事件」单开一层。

**我们放弃了什么**：
- 没有为事件单独设立 `internal/core/events` 命名空间——若未来事件类型膨胀到需要独立包，再抽离即可（从 `state` 搬出，调用方仍是 `state.Publish`/`Host.Subscribe`，迁移成本可控）。决策可逆。

---

## ADR-07b: 主入口与 gogpu 符号命名约定 ✅ 已定

### 决策

1. **主入口唯一写法**：`desktop.Run(gogpuApp, uiApp)`。
   - 这是 gogpu 生态的**概念入口**（内部 `runtime.LockOSThread` 拉起主线程 → 委托 gogpu/ui 帧循环 + Win32 消息泵）。
   - **禁止**在任何文档/代码中出现 `gogpu.App.Run()` 这类误用写法——gogpu 没有「App 自带 Run」，那是把 `desktop.Run` 张冠李戴。
2. **窗口创建唯一写法**：`gogpu.NewApp(gogpu.Frameless, gogpu.RenderModeHostManaged)`（**位置参数**，不使用 `DefaultConfig().WithXxx()` 链式形式，避免样板噪音）。
3. **渲染模式符号归属 gogpu**：`RenderModeHostManaged` / `RenderMode` 是 **`gogpu` 导出的常量/类型**，业务包（含 `internal/platform`）**不得**重新定义 iota 或本地枚举。需要引用时直接 `import "gogpu"` 后使用 `gogpu.RenderModeHostManaged`。

### 候选方案

| 方案 | 含义 | 取舍 |
|------|------|------|
| **B1. 统一 `desktop.Run` + 位置参数 `gogpu.NewApp` + gogpu 导出 `RenderModeHostManaged`** ✅选定 | 单一事实来源，示例代码可直接对拍 | 与本地 gogpu fork 已 patch 的 API 一致；零 CGO 不受影响 |
| B2. 允许 `gogpu.App.Run()` 写法 | 部分旧文档习惯 | ❌ 不存在的 API，误导实现，否决 |
| B3. `RenderMode` 在 `platform` 本地定义 iota | 业务包自管枚举 | ❌ 与 gogpu 导出符号重名冲突，F5 之源，否决 |

### 决策：采用 B1

**理由**：
- 与本地 gogpu fork（`D:\workspace\github\gogpu`，已 patch `CompositeAlphaModePremultiplied` + `WS_EX_LAYERED`）的公开 API 形态对齐，文档即实现契约。
- 全仓术语统一后，新贡献者照文档写代码不会再出现「到底用哪个 Run」的歧义。
- 位置参数 `NewApp` 更短，符合「无样板」原则；`RenderModeHostManaged` 作为 gogpu 常量引用，避免重复定义造成的语义漂移。

**我们放弃了什么**：
- 放弃 `DefaultConfig().WithXxx()` 的链式可读性——换取全仓一致；若未来配置项变多，可在 `state`/示例层封装一次性 helper，但**不准**在业务包重定义 `RenderMode`。

---

## Consequences（采纳后的整体影响）

**变得更容易**：
- 依赖倒置铁律有了**单一事实来源**：事件总线在 `state`，feature 永不编译依赖 plugin，分层不再有反向边。
- plugin 可编译隔离、可热拔插、可 mock——feature 单测无需 plugin 参与。
- 主入口与 gogpu 符号全仓统一，文档即契约，新实现零歧义。
- 零 CGO 链保持纯净（所有符号均来自纯 Go 的 gogpu）。

**变得更难 / 需承担**：
- `state` 包职责略增（既管 Signal/Store，又管事件总线）——但属同层汇聚，无新依赖边，可接受。
- 未来若事件类型爆炸需独立成 `internal/core/events`，需一次搬移——但调用方接口不变，迁移可逆、低成本。

---

## 决策门（已全部拍板 · 2026-07-08）

| # | 问题 | 决议 |
|---|------|------|
| Q1 | 事件总线实现归谁？ | ✅ **`internal/state`**；feature→state emit，plugin→state subscribe（经 `Host` 委托） |
| Q2 | feature 能否编译依赖 plugin？ | ✅ **禁止**——依赖倒置，`plugin ◀─依赖── feature` 边全部删除 |
| Q3 | 主入口怎么写？ | ✅ **`desktop.Run(gogpuApp, uiApp)`**，禁用 `gogpu.App.Run()` |
| Q4 | `gogpu.NewApp` 调用形式？ | ✅ **位置参数** `gogpu.NewApp(gogpu.Frameless, gogpu.RenderModeHostManaged)` |
| Q5 | `RenderModeHostManaged` 归属？ | ✅ **`gogpu` 导出符号**；业务包禁止本地重定义 iota |

> **ADR-07 状态：Accepted。** 本 ADR 将交叉一致性审查的 P0（F1）与 P2（F4/F5/F7）缺陷正式固化为架构决策；其余审查项（F2 包映射 / F3 import 前缀 / F6 plugin→todo 依赖 / F8 pkg/ 标注）为文档一致性修缮，已随审查同步写回对应文档，不另立 ADR。

### 关联修复落点（便于回溯）
- `80-Plugin/Event.md`：总线归属改为 `state`，删除反向依赖边（F1）。
- `80-Plugin/Plugin.md`：补 `plugin → todo` 依赖、修正 `EventBus` 引用、统一 `desktop.Run`（F6/F4）。
- `01-总体架构.md` / `02-开发规范.md` / `30-State/Signal.md` / `90-UI/MainWindow.md` / `80-Plugin/Lifecycle.md` / 技术评估报告：统一 `desktop.Run`（F4）。
- `20-Platform/WindowStyle.md`：删除本地 `RenderMode` iota，改引用 `gogpu.RenderModeHostManaged`（F5）。
- `10-Shell/App.md`：`gogpu.NewApp` 位置参数形式（F7）。
- `03-项目目录规范.md` / `40-Theme/Skin.md`：包映射与 import 前缀修缮（F2/F3/F8）。
