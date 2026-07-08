# DeskCalendar · Phase 0 代码审查报告

> 审查对象：Phase 0 基础设施（`internal/state` / `internal/calendar` / `internal/plugin` / `internal/platform/windowstyle` / `internal/infra/{log,config}`）
> 对照基线：Issue #146（开发顺序与依赖地图）+ `docs/30-State/*`、`docs/20-Platform/WindowStyle.md`、`docs/ADR-07-*`、`docs/02-开发规范.md`、`docs/03-项目目录规范.md`
> 审查日期：2026-07-08 ｜ 工具：Go 1.25.9，`go build/vet/test` + `go list` 依赖校验 + `go test -cover`

## 0. 总体结论

**质量评级：良好（B+），可进入 Phase 1，但必须先消掉 1 个架构治理冲突（🔴）。**

- ✅ 硬性约束全部满足：`go build ./...`、`go vet ./...`、`go test ./...` 全绿；`CGO_ENABLED=0 go build ./...` 通过（ADR-01/06 零 CGO 成立）。
- ✅ 依赖倒置铁律（ADR-07a）经 `go list` **实证成立**：`state` 仅依赖 `coregx/signals` + `infra/log`；`plugin`/`calendar` 仅依赖 `state`；无 `plugin→feature`、`state→feature` 反向边。
- ⚠️ **1 个 🔴**：代码与已 `Accepted` 的 ADR-07b（Q5）直接冲突 —— `windowstyle.go` 本地重定义了 `RenderMode` 枚举，而 ADR 明文禁止业务包本地重定义。设计文档 `WindowStyle.md §9` 还引用了本地 gogpu fork 根本不存在的 `gogpu.RenderModeHostManaged`。三份“事实来源”互相打架，Phase 3 shell 装配时必然踩坑。
- 🟡 若干质量改进：配置部分加载静默掉默认值、`signal.go` 最关键原语无单测、`Dispatcher` 满 channel 静默丢命令、Event.At 不自动盖章。
- 👍 亮点：EventBus 的 panic/error 隔离、双循环测试、`plugin.Host` 用编译期类型断言锁死“只能订阅不能 emit”都做得很好。

---

## 1. 验证事实（工具产出，非推测）

| 项 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `CGO_ENABLED=0 go build ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go test ./...` | 全部 ok |
| 外部依赖 | 仅 `github.com/coregx/signals v0.1.0`（零 CGO） |
| 覆盖率 | calendar `100%` / platform `100%` / state `81.8%` / plugin `66.7%` / config `58.3%` / log `0.0%` |
| 依赖图（`go list`） | `state → {coregx/signals, infra/log}`；`calendar/plugin → state`；`windowstyle/config/log → 仅 stdlib` |

---

## 🔴 Blockers（必须解决）

### B1 · ADR-07b（Q5）与 `windowstyle.go` 本地 `RenderMode` 枚举直接冲突

**位置**：`internal/platform/windowstyle/windowstyle.go:9-24` 对比 `docs/ADR-07-事件总线归属与入口约定.md` Q5 / B1，以及 `docs/20-Platform/WindowStyle.md §9`。

**事实冲突**：
- ADR-07b（2026-07-08 `Accepted`）明文：**“业务包（含 internal/platform）不得重新定义 iota 或本地枚举。需要引用时直接 `import "gogpu"` 后使用 `gogpu.RenderModeHostManaged`。”** 决策门 Q5 亦确认 `RenderModeHostManaged` 属 gogpu 导出符号。
- 代码 `windowstyle.go` 恰恰**本地定义**了 `type RenderMode int` 与 `RenderModeAuto/CPU/GPU`，并明确 `import` **不**引入 gogpu。其注释自陈：“早期设计稿引用了 `gogpu.RenderModeHostManaged`，但本地 gogpu fork 实际只导出 `RenderModeAuto/CPU/GPU`”。
- 设计文档 `WindowStyle.md §9` 仍写 `RenderMode: gogpu.RenderModeHostManaged` 与 `gogpu.NewApp(gogpu.Frameless, gogpu.RenderModeHostManaged)` —— 既与代码矛盾，也与 ADR 共同基于一个**对本地 fork 的错误前提**（fork 根本没有 `RenderModeHostManaged`）。

**为什么是 blocker（而非普通 nit）**：三份“单一事实来源”彼此不一致。代码侧的理由（Phase 0 解耦 gogpu、fork 实际只有 Auto/CPU/GPU）在工程上更站得住，但 ADR 是“已拍板”的治理文件、被 Issue #146 和各模块文档当作契约引用。Phase 3 shell 装配 `RenderMode → gogpu.RenderMode` 适配器时，作者无从判断该信 ADR、设计文档还是代码。**必须先收敛，否则依赖地图断裂。**

**建议（文档/治理修复，非改代码）**：
1. 修订 ADR-07b Q5/B1，记录 Phase 0 的真实决策：`platform` 用本地 `RenderMode`(Auto/CPU/GPU) 枚举以避免把 gogpu/wgpu 引入基础层；`RenderModeHostManaged` 在本地 fork 不存在，原 ADR 前提有误；Phase 3 shell 提供 `RenderMode → gogpu.RenderMode` 的自由函数适配器。
2. 同步修正 `docs/20-Platform/WindowStyle.md §9`：删除 `gogpu.RenderModeHostManaged` / `gogpu.NewApp(gogpu.Frameless, ...)` 等不符 fork 的引用，改为与代码一致的本地枚举 + Phase 3 适配器说明。
3. 代码本身**无需回退**（它已对齐真实 fork 与零 CGO 目标），但应补一行注释引用新 ADR 结论，避免后人“按 ADR 改回 gogpu 导入”造成回归。

---

## 🟡 Suggestions（应修）

### S1 · `config.Load` 部分配置静默偏离默认值，且缺枚举校验
**位置**：`internal/infra/config/config.go:72-88`。

一个合法但**部分**的 JSON（如 `{"version":1}`）反序列化后，`Theme{Acent:""}`、`Window{CornerRadius:0}` 等会被**零值**填充，而非 `Default()` 的值（`"#4C8DFF"`、`16`）。UI 会拿到空字符串强调色 / 0 圆角，且无任何报错。`Mode ∈ {system,light,dark}`、`Accent` 十六进制格式也完全没有校验。

**建议**：
- 反序列化后做枚举校验（非法 `Mode` 回退 `system`，非法 `Accent` 回退默认）；
- 可选：将 partial 配置 merge 到 `Default()` 之上，缺字段自动补齐（避免升级新增字段后旧配置全零）。

### S2 · 最关键原语 `signal.go` 无单测，`NewSignalWithEqual` 路径零覆盖
**位置**：`internal/state/signal.go:25-34`（无 `signal_test.go`）。

`Signal` 是“主线程唯一写入”的基石，目前仅经 store/dispatcher 间接覆盖。`NewSignalWithEqual`（自定义 `Equal` 判定）这条 `signals.NewWithOptions` 路径**从未被任何测试触达**。Phase 4 UI 将直接消费这些 Signal，一旦 `coregx/signals` 升级/替换，没有单测兜底。

**建议**：新增 `internal/state/signal_test.go`：
- `NewSignal` 初始值正确；
- `NewSignalWithEqual`：新值“相等”时不通知订阅者、不等时通知；
- `Subscribe(ctx, fn)` 在 `Set` 后触发回调。
（符合 `docs/02-开发规范.md §4` “核心 domain ≥ 80%”目标，state 当前 81.8% 主要靠间接覆盖，补此更稳。）

### S3 · `Dispatcher.Enqueue` 满 channel 时静默丢弃命令
**位置**：`internal/state/dispatcher.go:30-36`。

缓冲 64，满则 `default` 分支 `logger.Warn(... dropped)` 后丢弃。属于“已知取舍”，但在 60fps 主循环 Pump 下实践中几乎不可达。风险点：被丢弃的是 `CmdShow`/`CmdSelectDate` 这类**用户输入意图**，静默丢失体验上比报错更隐蔽。

**建议**：至少把“丢弃”做成**有意识的设计决策**并写进注释（说明为何 64 足够、为何选择丢而非阻塞），并考虑加一个 `droppedTotal` 计数指标便于观测；若担心极端情况，可适度放大缓冲。

### S4 · EventBus 不自动盖章 `Event.At`
**位置**：`internal/state/event.go:125-147`。

`Event.At` 完全依赖调用方手动 `At: time.Now()`（`calendar.EmitDateChanged` 有做，但总线本身不保证）。若某 feature 忘记设置，下游按时间排序/去重的订阅者会拿到零值时间。

**建议**：在 `Publish` 内若 `e.At.IsZero()` 则补 `e.At = time.Now()`，让时间戳成为总线契约的一部分。

---

## 💭 Nits（可选）

- **N1 · 包名 `platform` 与目录 `windowstyle` 不符**（`internal/platform/windowstyle` 声明 `package platform`）。与全仓 `state`/`calendar`/`plugin`/`config`/`log` 对齐目录名的惯例不一致，`goimports` 与读者都会困惑。文档称“有意为之”，但建议要么包改名 `windowstyle`，要么把文件上移到 `internal/platform`（若后续还有同包窗口原语）。低优。
- **N2 · `docs/30-State/Signal.md §9` 接口签名与真实 `coregx/signals` API 漂移**：文档写 `Subscribe(fn func(value T))` + `Observe(old,new)`，真实 API 是 `Subscribe(ctx, fn)` 且无 `Observe`（已在本项目 working memory 记录）。Phase 4 UI 作者照文档写会编不过。应把文档改回真实 API。
- **N3 · `ThemeState.accent` 无对应命令**：`Store.md §6` 表提到 `CmdSetAccent`，但 `command.go` 未定义、`ThemeState.Apply`（store.go:157-161）也未处理 accent。属 v1.3 范畴，但文档/代码漂移需登记，避免 v1.3 误以为已具备。
- **N4 · `infra/log` 0% 覆盖、`stdLogger` 格式未测**（log.go:38-45）：补一个“捕获 buffer 断言 `[LEVEL] msg [k v]` 形状”的小测试即可，成本低。
- **N5 · `config` 58.3% 覆盖**：`DefaultPath()` 与 `Load` 的读错误/解析错误分支未测，可补错误路径用例。
- **N6 · `docs/02-开发规范.md §2` 称日志“封装 log/slog”，实现是 `fmt.Fprintf` 极简版**（log.go 注释解释为避免重依赖）。实现取舍合理，但与规范文字不符 —— 要么规范改“极简自实现”，要么承认偏离。
- **N7 · `docs/03-项目目录规范.md §2` 要求“每个包一个 doc.go”**，现有包把包注释写在主文件（signal.go/store.go/event.go 等）而非 `doc.go`。属轻微规范偏离，Go 惯用法也接受包注释内联，建议明确采纳其一。

---

## 👍 做得好的地方（应当保留）

- **依赖倒置被工具实证**：`go list` 证明 `state` 不反向依赖任何 feature；`plugin`/`calendar` 只认 `state`。这正是 Issue #146 与 ADR-07a 的核心诉求，不是口头声明。
- **EventBus 健壮性到位**：`event_test.go` 覆盖了订阅/退订、handler 错误隔离、panic 隔离（`Publish` 内 `recover`）、异步派发、nil handler 拒绝 —— 对“会跑第三方插件 handler”的总线来说，隔离是正确优先级。
- **双循环契约有真测试**：`dispatcher_test.go` 验证 `Enqueue→Pump→Signal.Set`、满 channel 不阻塞、未知命令不 panic、Signal 订阅在 Pump 后收到通知。这是 Phase 3 装配的可靠地基。
- **`plugin.Host` 用类型断言锁死铁律**：`host_test.go` 的 `TestHostHasNoPublish` 用编译期 `h.(publisher)` 断言 Host 没有 `Publish` —— 把“插件只能订阅不能 emit”钉进类型系统，非常巧妙。
- **feature→state emit 端到端可测**：`calendar.Service → 订阅者` 链路在 `calendar_test.go` 跑通。
- **零 CGO、单外部依赖、导入图干净**：符合 ADR-01/06 与 `docs/02` 的“仅纯 Go 依赖”。
- **代码注释讲“为什么”**：`windowstyle.go`、`signal.go`、`dispatcher.go` 的包/函数注释清楚解释了 gogpu 解耦、主线程唯一写入等决策背景，可维护性强。

---

## 建议的后续动作（按优先级）

1. **🔴 先修 B1**：修订 ADR-07b Q5/B1 + 修正 `WindowStyle.md §9`，让“RenderMode 权威来源”统一为代码现状（本地枚举 + Phase 3 适配器）。这是继续 Phase 1/3 的前置条件。
2. **🟡 修 S1/S2**：config 校验 + `signal_test.go`，二者都是“核心 domain”质量底线。
3. **🟡 考虑 S3/S4**：把丢命令、时间戳盖章做成有意识契约。
4. **💭 N1–N7**：文档回写（Signal API、accent、日志规范、doc.go 约定），可在 Phase 1 启动时顺手清。

> 备注：B1 为治理/文档修复，不涉及运行时代码改动；代码本身编译、测试、零 CGO 均达标。若需要，我可以直接帮你修订 ADR-07b 与 `WindowStyle.md §9` 并补上 `signal_test.go` / config 校验。

---

## 9. 二次执行记录（2026-07-08 晚）

技术选型（gogpu/ui vs 路径 D 等）已移交架构师，故本报告**仅处理与架构选型无关的代码质量项**。**交叉一致性审查报告（F1–F8）已全量修复，无遗留**。

| 缺陷 | 级别 | 本次处理 | 状态 |
|------|------|----------|------|
| B1 | 🔴 | RenderMode 冲突属技术选型范畴（gogpu vs 路径 D），移交架构师统一治理；代码本身保持原状 | ⏸ 待架构师 |
| S1 | 🟡 | config 部分 merge + Mode/PositionMode 校验此前已落地；本次补 **Accent `#RRGGBB` 格式校验**（normalizeAccent）+ 测试 `TestLoadInvalidAccentClampsToDefault` | ✅ 已修 |
| S2 | 🟡 | `signal_test.go` 此前已建，覆盖 `NewSignalWithEqual` 路径（`TestNewSignalWithEqualSkipsEqualValues`） | ✅ 已修（早前） |
| S3 | 🟡 | `dispatcher.go` 加 `droppedTotal atomic` 计数 + `DroppedTotal()` 观测方法；`Enqueue` 注释写明 64 缓冲/丢而非阻塞的设计决策 | ✅ 已修 |
| S4 | 🟡 | `event.go` `Publish` 内 `if e.At.IsZero() { e.At = time.Now() }` 自动补章 | ✅ 已修 |
| N4 | 💭 | 新建 `log_test.go`，断言 `[LEVEL] msg [k v]` 形状 + Nop 静默；log 覆盖 0%→57.1% | ✅ 已修 |
| N5 | 💭 | 补 `TestLoadMalformedJSONReturnsError`（Load 解析失败返回 error）；config 覆盖 58.3%→74.4% | ✅ 已修 |
| N1 | 💭 | `internal/platform/windowstyle` 包名 `platform` → `windowstyle`（与目录名一致，Go 惯用法）；同步改测试 import 别名 + 测试包名 `windowstyle_test` | ✅ 已修 |
| N2 | 💭 | `30-State/Signal.md §9` API 漂移（gogpu/ui 表述）→ 与技术选型（Signal 提供方）相关，留待架构师定稿后一并回写 | ⏸ 待架构师 |
| N3 | 💭 | `CmdSetAccent` 文档/代码漂移，属 v1.3 范畴，登记即可 | ⏸ v1.3 |
| N6 | 💭 | `02-开发规范.md §2` 日志"封装 log/slog"→ "极简自实现（fmt.Fprintf → stderr，零第三方依赖）"，对齐 log.go | ✅ 已修 |
| N7 | 💭 | `03-项目目录规范.md §2` "每个包一个 doc.go"→ 允许内联包注释或独立 doc.go（二选一不强制） | ✅ 已修 |

**验证**：`go build ./...` / `go vet ./...` / `go test ./...` 全绿；`CGO_ENABLED=0 go build ./...` 通过；state 覆盖 82.6%。

**剩余需解决**：仅 **B1**（待架构师拍板技术选型后做 ADR-07b 修订 + `WindowStyle.md §9` 回写）与 **N2/N3**（N2 与 Signal 提供方选型重叠、N3 属 v1.3 范畴）。**N1/N6/N7 已清零**，代码层 + 规范文字层缺陷全部清理完毕。
