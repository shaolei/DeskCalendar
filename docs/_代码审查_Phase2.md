# DeskCalendar · Phase 2 代码审查报告

> 审查对象：Phase 2 功能层（4 个 slice：`internal/calendar` 域骨架 + 真实 Lunar（lunar-go）+ 真实 Holiday（go:embed）+ `internal/theme` MVP 主题模型/Provider/JSON/图标）
> 对照基线：Issue #146（开发顺序与依赖地图）+ `docs/50-Calendar/{Calendar,Lunar,Holiday,Month,Week}.md` + `docs/40-Theme/{Theme,ThemeJson,Icon}.md` + ADR-05（数据源）+ ADR-07a（依赖倒置）+ `docs/02-开发规范.md`（覆盖率目标）
> 审查日期：2026-07-09 ｜ 工具：Go 1.25、`go build/vet/test/cover` + `go list` 依赖校验 + `go tool cover -func`（per-function）

---

## 0. 总体结论

**质量评级：良好（A- 区间），无代码层 🔴 blocker，可直接进入 Phase 3（shell 装配）。**

- ✅ **硬性约束全部满足**：`go build ./...`、`go vet ./...`、`go test ./...` 全绿；`CGO_ENABLED=0 go build ./...` 通过（lunar-go 纯 Go + theme 用 `x/sys/windows/registry` 纯 Go，ADR-01/06 零 CGO 成立）。
- ✅ **依赖倒置铁律（ADR-07a）实证成立**：`calendar` 仅依赖 `state` + `lunar-go` + `stdlib` + `embed`；`theme` 仅依赖 `x/sys/windows/registry` + `stdlib` + `embed`。反向依赖校验确认 `feature/ui/plugin` 包尚不存在（Phase 3 前），无业务反向边。
- ✅ **seam/DI 落地扎实**：`LunarService` / `HolidayRepository` / `ThemeProvider` / `IconProvider` 全部接口/可注入，纯逻辑可单测，无需 lunar-go 网络或真实 Windows 桌面。
- ⚠️ **头号问题不是代码，是文档漂移**：5 个 Phase 2 设计文档的 §9 签名/方案与**已落地的更好代码**不符（Calendar/Lunar/Holiday/Theme/ThemeJson）。代码普遍优于文档，但照文档写的开发者会写出错的代码。
- ⚠️ **1 个真实逻辑 bug（已被掩盖，现已修复）**：`ThemeProvider.ClearOverride` 在暗色系统下清除覆盖后会回退到 light（丢失系统方案）；原测试只用 light 初始化，故绿灯掩盖。该 bug 已在 §10 修复（`systemScheme` 持久化 + `currentScheme()` 重写），并补暗色系统回归测试。
- 📊 **覆盖率**：整体 **70.6%**（Phase 1 为 57.8%，已越过 ≥60% 目标）；calendar `85.9%` / theme `72.1%`。核心 domain 达标。

---

## 1. 验证事实（工具产出，非推测）

| 项 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `CGO_ENABLED=0 go build ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go test ./...` | 全部 ok |
| `calendar` 导入 | `[container/list context embed encoding/json fmt lunar-go/calendar state strconv strings sync time]`（仅 `state` + lunar-go + stdlib/embed） |
| `theme` 导入 | `[context embed encoding/hex encoding/json fmt x/sys/windows/registry image/color os strconv strings sync time]`（仅 `x/sys` + stdlib/embed） |
| 反向依赖 | `ui/plugin/platform` 包尚不存在（Phase 3 前）；无业务反向边 |
| 覆盖率（整体） | **70.6%** |
| 覆盖率（分包） | calendar `85.9%` / theme `72.1%` / platform `40.8%`（较 Phase 1 的 31.9% 提升）/ 其余同 Phase 1 |
| per-func 低项 | `calendar`: `WithView` 0%、`NewDefaultCalendarService` 0%；`theme`: `iconAt` 42.9%、`currentScheme` 80%、`Watch` 81.8%、`resName` 0%、`Scheme.String` 0% |

---

## 🔴 Blockers（必须解决）

**Phase 2 代码层无 🔴 blocker。**

代码编译、测试、零 CGO、依赖方向、seam 可测性、 lunar 黄金测试全部达标，未发现正确性/安全/竞态/数据丢失层面的硬伤。唯一的「发布前必须解决」项是 **Holiday SEED 数据是占位近似**（见 S5），属数据/流程项且文件内已自陈，非代码缺陷。

---

## 🟡 Suggestions（应修）

### S1 · 设计文档与实现严重漂移（5 个 Phase 2 模块）

**事实**：实现普遍**优于**文档，但文档 §9 会让照着写的开发者产出错误代码。逐模块：

- **`Calendar.md` §9**：文档 `NewCalendarService(lunar, holiday)`；代码为 `NewCalendarService(bus state.EventBus, lunar, holiday, opts...)`——代码**新增 `bus` 参数**以 emit `TopicDateChanged`（ADR-07a feature→state），且 `GetDayInfo` 用可注入 `today`（`WithToday`）而非 `time.Now()`。**代码更好**（可测、符合事件总线契约），但文档未同步。
- **`Lunar.md` §9**：文档用 `lunar.Solar.FromDate(date)`→`GetLunar()` 且闰月判定 `GetMonth() != GetMonthInChinese() && IsLeap()`；代码用 `lunar.NewLunarFromDate(date)` 且 `month < 0` 判闰。黄金测试（`大暑`/`正月`/`丙午`/`马`）通过，API 正确；文档自陈「方法名以 v1.4.6 实际 API 为准」，属可接受滞后。
- **`Holiday.md` §9（最该改）**：文档画的是 `data map[string]string` + **脆弱字符串 heuristic** `isMakeupWorkday(name)`（名称含"补班"/"班"）+ **运行时 HTTP 拉取 `raw.githubusercontent.com/NateScarlet/holiday-cn`** 的 `Refresh`。代码实际用 **`holidays`/`workdays` 两张独立 map**（无需脆弱 heuristic，补班天然不误判为节假日）+ **`Refresh` 仅重放 embed（零网络）**。**代码明显优于文档**，且去网络更契合项目「离线优先」(ADR-05)；但文档 §9 与 §10（HTTP 延后 v1.2）自相矛盾，且会误导实现者去写网络 + heuristic。
- **`Theme.md` §9**：文档 `ThemeProvider` 含 `schemeCh chan Scheme` 字段 + 导出 `SystemScheme(ctx)`；代码用未导出 `systemScheme()` + `watchSystem(ctx, onScheme)`。文档 §6 序列图写「`Watch` → `Resolve(newScheme)` → 重建 `current`」，代码**并未重建 `current`**（见 S3）。
- **`ThemeJson.md` §9**：文档 `Validate` 是**占位 stub**（仅校验范围，注释写「必填色角色齐全[实现中可借助 map 缺失检查补全严格性]」），还列了代码中不存在的 `LoadResult`/`ValidateOption` 类型；代码 `buildTheme` **实际强制校验 8 个必填色** + `#RRGGBBAA` 解析 + 范围。代码更严格。

**为什么是 🟡（非 🔴）**：代码本身正确/更优，漂移不导致运行故障；但 5 份文档会在 Phase 3/插件阶段误导协作者，属「单一事实来源」污染。

**建议**：做一轮 **doc-sweep**，把上述 §9 对齐到已落地的更好代码（尤其是 Holiday.md 删掉 HTTP+heuristic、Theme.md 修正 Watch 行为描述、ThemeJson.md 删掉不存在的类型与 stub 注释）。这与 Phase 1 的 S1（文档清扫）是同一类动作，可一并纳入「文档回写」纪律。

---

### S2 · `ThemeProvider.ClearOverride` 丢失系统方案（暗色系统回归，被测试掩盖）

**位置**：`internal/theme/theme.go:156-169`（`SetOverride`/`ClearOverride`）+ `:173-181`（`currentScheme`）。

**根因**：`currentScheme()` 在无覆盖时用指针比较推断方案：
```go
if p.current == p.dark { return SchemeDark }
return SchemeLight
```
但 `p.current` 在清除覆盖的瞬间仍是「旧覆盖主题」（指针 ≠ `p.dark`），于是推断为 `SchemeLight` → `resolveLocked` 返回 `p.light`。**检测到的系统方案从未被存储**，清除后无法恢复。

**复现（暗色系统）**：init 探测系统=dark → `p.current = p.dark`。`SetOverride(自定义dark t)` → `p.current = t`。`ClearOverride()` → `p.override=nil` → `currentScheme()` 看 `p.current==p.dark`？此时 `p.current` 仍是 `t`（≠`p.dark`）→ 返回 `SchemeLight` → `p.current = p.light`。**系统明明是暗色，却被错误回退到亮色。**

**测试为何绿灯**：`theme_test.go:27` `TestProvider_SetOverride_ClearOverride` 用 `WithInitialScheme(SchemeLight)`，清除后断言 `SchemeLight`——恰好踩中 bug 的「假阳性」路径。若初始为 dark，该测试会失改。

**建议**：
1. `ThemeProvider` 增字段 `systemScheme Scheme`，`NewProvider` 探测后存之；`Watch` 检测到系统变化时同步更新 `systemScheme`。
2. `currentScheme()` 无覆盖时返回 `p.systemScheme`（而非指针比较）。
3. 补测试：`WithInitialScheme(SchemeDark)` + 暗色覆盖 + `ClearOverride` → 断言回到 `SchemeDark`。

---

### S3 · `Watch` 不更新 `Current()`（与文档 §6 契约不符）

**位置**：`internal/theme/theme.go:185-201`（`Watch`）+ `:149-153`（`Current`）。

**事实**：`Watch` 的 goroutine 在系统方案变化时只 `sendScheme(ch, s)` 把 `Scheme` 推到 channel，**不调用 `Resolve`、不写 `p.current`**。因此运行时用户切换 Windows 主题后，`Current()` 仍是初始化时的值（陈旧），只有显式消费 `Watch` 并经 `Resolve(scheme)` 取主题的 UI 才正确。

**文档冲突**：`Theme.md` §6 序列图明确「`Watch` → `Resolve(newScheme)` → 重建 `current` → 推送 UI」。代码未实现「重建 current」。

**建议（二选一，推荐前者）**：
- (a) `Watch` 内 `onScheme` 回调里：若 `p.override == nil` 则 `p.current = p.resolveLocked(s)` 并（可选）把新 scheme 推给已有订阅者——让 `Current()` 保持实时；
- (b) 若坚持「消费者自己 Resolve」，则在 `Theme.md` §6 与 `Watch` 注释中明确写清「`Current()` 仅反映初始化/覆盖态，实时跟随须用 `Watch` 返回的 scheme 调 `Resolve`」，避免误解。

---

### S4 · `calendarService.today` 跨午夜陈旧

**位置**：`internal/calendar/calendar.go:62`（字段）、`:80`（默认 `time.Now()`）、`:108-119`（`GetDayInfo` 用 `isSameDay(date, s.today)`）、`:77`（`WithToday`）。

**事实**：`today` 在构造时一次性捕获。若 `CalendarService` 由 shell 在启动期创建一次并跨午夜存活（弹窗复用同一 service），次日起 `IsToday` 会持续错误。对「每次弹窗重建 service」的用法无碍，但对长生命周期 service 是隐患。

**建议**：供给一个 `RefreshToday()`（shell 每日定时器调用），或 `GetDayInfo` 内对生产路径懒算 `isSameDay(date, time.Now())`、仅测试走 `WithToday` 注入值。二者都不破坏现有测试。

---

### S5 · Holiday SEED 2026.json 为占位近似（v1.0 发布门）+ 日期校验可收紧

**位置**：`internal/calendar/embed/holidays/2026.json:2`（自陈「SEED DATA — 近似草稿，发布前须以国务院通知 / holiday-cn 真实数据替换」）+ `internal/calendar/holiday_embed.go:97-111`（`joinDate`）。

**事实**：
- 内嵌 `2026.json` 明确是自述的「近似草稿」。若 v1.0 直接发布，节假日/调休会是错的（如春节区间是否 02-15..02-22 取决于当年国务院安排）。ADR-05 要求「每年构建期烘焙」真实数据——这是**发布前必须完成的步骤**，否则功能正确性不保。
- `joinDate` 仅校验 `m∈[1,12]`、`d∈[1,31]`，允许 `2026-02-30` 这类无效日期 → 生成永不命中的死键（`time.Time.Format` 不会产出它），静默而非报错。

**建议**：
- 🟡（发布门）：在 v1.0 前用真实 `holiday-cn`/国务院 2026（及 2027）数据替换 SEED，并加一个「SEED 被替换」的 CI 门禁或清单项，防止误发占位数据。
- 💭：收紧 `joinDate`——构造 key 后 `time.Parse("2006-01-02", key)` 并校验 round-trip 相等，捕获 `02-30` 这类坏数据。

---

### S6 · `buildTheme` 始终 `Builtin: true`（用户主题会被误标）

**位置**：`internal/theme/themejson.go:153`（`Builtin: true` 硬编码）。

**事实**：`ParseFile`（v1.3 用户主题）与 `LoadEmbedded`（内置）走同一 `buildTheme`，都标 `Builtin: true`。用户主题应标 `Builtin: false`（供 UI/设置区分「内置/自定义」）。

**建议**：`ParseFile` 路径显式置 `Builtin: false`；或 `buildTheme` 增 `builtin bool` 参数，内置调用传 `true`、用户调用传 `false`。属 v1.3 范畴，但接口已存在，现在改成本最低。

---

### S7 · Lunar 闰月分支缺 golden 测试

**位置**：`internal/calendar/lunar_lunargolang.go:19-43`（闰月分支 `leap := month < 0; absMonth = -month; LeapMonth: leap`）。

**事实**：现有 `lunar_lunargolang_test.go` 仅覆盖**非闰**的春节（正月初一）。闰月路径（`month<0` → 取绝对值 + `LeapMonth=true`）**无 golden 测试**。闰月约每 2–3 年一次，且第三方 `lunar-go` 升级可能改变 `GetMonth()` 负值的语义约定——缺测试即无回归护栏。

**建议**：补一个已知闰月日期的 golden 测试（如 2023-03-22 闰二月初一，或 2025 闰六月），断言 `LeapMonth==true`、`LunarMonth==abs`、`MonthStr=="闰X月"`。低成本、高价值（锁第三方行为）。

---

## 💭 Nits（可选）

- **N1 · `buildTheme` 冗余 `switch key`**（`themejson.go:111-145`）：8 个 case 都只调 `assign(key, &palette.X)`，可改为 `key→*color.RGBA` 的小映射表，减少样板。低优。
- **N2 · `icon.resName` default 分支 0% 覆盖**（`:20`）：未知分辨率回退 "16" 未测；补一行或删掉死 default。低优。
- **N3 · `Scheme.String()` 0% 覆盖**（`:23`）：一行断言即可。
- **N4 · `calendar.WithView` / `NewDefaultCalendarService` 0% 覆盖**：补 `SetView(ViewWeek)` + `VisibleRange` 周分支测试，及 `NewDefaultCalendarService` 加载真实 repo 的成功/失败路径。低优。
- **N5 · `joinDate` 无效日期**（S5 已含）：round-trip 校验。见 S5。
- **N6 · `Theme.md` §1 称 theme 依赖 `internal/infra/log`**，代码未引入——文档漂移，顺手改。

---

## 👍 做得好的地方（应当保留）

- **seam/DI 贯穿功能层**：`LunarService`/`HolidayRepository`/`ThemeProvider`/`IconProvider` 全接口化、可注入，纯逻辑用 fake 单测，**完全不依赖 lunar-go 网络或真实 Windows 桌面**。
- **Holiday 用「两张独立 map」而非文档的脆弱 heuristic**：`holidays`/`workdays` 分离，补班日天然不会误判为节假日；比 `Holiday.md` §9 的 `isMakeupWorkday(name)`（"班"后缀匹配）更正确、更稳。
- **Lunar 黄金测试钉死第三方输出**：`大暑`/`正月`/`丙午`/`马` 抽样，是捕获 `lunar-go` 版本漂移的最佳护栏。
- **`Refresh` 去网络、仅重放 embed**：比 `Holiday.md` §9 的运行时 GitHub HTTP 拉取更契合「离线优先」(ADR-05)，MVP 安全且不触网。
- **ThemeJson 校验严格**：8 必填色 + `#RRGGBBAA` 解析 + `CornerRadius/Alpha/Shadow.Opacity` 范围，优于文档的 stub `Validate`。
- **icon.go 防御性到位**：`<scheme>_<res>.png` 解析、缺失分辨率回退、明暗缺失即报错，资源加载稳健。
- **零 CGO 守住**：lunar-go 纯 Go、theme 用 `x/sys/windows/registry` 纯 Go；`CGO_ENABLED=0` 通过。
- **依赖方向干净（ADR-07a）**：`calendar`→`state`；`theme`→`x/sys`（leaf，自探系统方案，不反向依赖 platform）；无业务反向边。
- **embed 路径正确、数据可加载**：`embed/holidays/*.json`、`embedded/themes/*.json`、`embedded/icons/*.png` 均按 `//go:embed` 正确固化。
- **编译期接口断言**：`var _ HolidayRepository = (*embedHolidayRepo)(nil)` 把「实现满足接口」钉进类型系统。

---

## Phase 1 遗留项处理情况（本次确认）

| Phase 1 项 | 级别 | 本次确认 | 状态 |
|-----------|------|----------|------|
| S1（RenderModeHostManaged 文档清扫） | 🟡 | 提交 `74c7262 docs(platform): clean RenderModeHostManaged dead token (S1)` 已落地 | ✅ 已修 |
| S2（补 3 个纯逻辑测试） | 🟡 | 提交 `74c7262` 含 3 测试；platform 覆盖 31.9%→**40.8%** | ✅ 已修 |
| S3（Startup.Enabled 路径归一化） | 🟡 | 提交 `a906415 fix(platform): normalize StartupManager.Enabled comparison (S3)` | ✅ 已修 |
| B1（RenderModeHostManaged 治理） | ⏸ | 仍待架构师就 gogpu vs 路径 D 拍板；Phase 2 代码未触碰，仍正确 | ⏸ 待架构师 |

**结论**：Phase 1 的全部 🟡 已在本次 Phase 2 前闭环，review→修复闭环有效；B1 仍属治理待办，不影响进度。

---

## 建议后续动作（按优先级）

1. **🟡 S2 + S3（theme 正确性）**：修 `ClearOverride` 系统方案丢失 + 让 `Watch` 更新 `Current()`；补暗色系统测试。这是 Phase 2 唯一的真实逻辑 bug，建议进 Phase 3 前修（不阻塞，但 v1.3 换肤依赖它）。
2. **🟡 S5（发布门）**：v1.0 前用真实 holiday-cn 数据替换 SEED 2026/2027。
3. **🟡 S1（文档回写）**：一次性 doc-sweep 对齐 5 份 Phase 2 文档到已落地代码（尤其 Holiday.md 删 HTTP+heuristic、Theme.md 修正 Watch 契约、ThemeJson.md 删不存在类型与 stub 注释）。
4. **🟡 S4 / S6 / S7**：`today` 跨午夜刷新、`Builtin` 用户主题标记、闰月 golden 测试——均可小步补。
5. **💭 N1–N6**：顺手清理。

> 备注：Phase 2 代码层无 🔴 blocker；所有 🟡 均为质量增强/文档一致性/发布数据准备，不阻塞 Phase 3 启动。若需要，我可直接执行 **S2+S3（theme 修复 + 暗色测试）** 与 **S1 文档 sweep**。

---

## 9. 执行记录（本次 / 2026-07-09）

| 项 | 级别 | 本次判定 | 状态 |
|----|------|----------|------|
| Phase 2 代码 🔴 | — | 无代码层 blocker | ✅ 通过 |
| S1（文档漂移） | 🟡 | 5 份 Phase 2 文档 §9 与更好代码不符；代码优于文档 | ✅ 已修（见 §11） |
| S2（ClearOverride bug） | 🟡 | 暗色系统清除覆盖回退 light；测试仅 light init 掩盖 | ✅ 已修（见 §10） |
| S3（Watch 不更新 Current） | 🟡 | 与 Theme.md §6 契约不符 | ✅ 已修（见 §10） |
| S4（today 跨午夜） | 🟡 | 长生命周期 service 隐患 | ✅ 已修（见 §12） |
| S5（Holiday SEED） | 🟡 | joinDate 校验已收紧；SEED 真实数据替换仍属发布门 | 🟡 joinDate 已修 / 数据待发布前替换 |
| S6（Builtin 误标） | 🟡 | 用户主题 v1.3 误标 Builtin | ✅ 已修（见 §12） |
| S7（闰月测试） | 🟡 | 闰月分支无 golden | ✅ 已补（见 §12） |
| Phase 1 S1/S2/S3 | 🟡 | 提交 `74c7262`/`a906415` 已闭环 | ✅ 已修 |
| B1 | ⏸ | 待架构师 | ⏸ |

**验证**：`go build ./...` / `CGO_ENABLED=0 go build ./...` / `go vet ./...` / `go test ./...` 全绿；整体覆盖 **70.6%**（calendar 85.9% / theme 72.1% / platform 40.8%）；`go list` 确认 `calendar`→`state`+lunar-go、`theme`→`x/sys`+embed，无业务反向依赖。

**结论**：Phase 2 质量达标、无代码 blocker，可进入 Phase 3（shell 装配 calendar/theme/platform）。遗留项均为文档一致性、theme 逻辑小修、与发布数据准备，不阻塞进度。

---

## 10. S2 + S3 修复执行记录（2026-07-09 续）

**S2 + S3 已闭环**，改动集中在 `internal/theme/theme.go` + `theme_test.go`，TDD 红→绿。

### 改动要点
- **新增 `systemScheme Scheme` 字段**：`NewProvider` 探测后写入；`WithInitialScheme` 同步写入（测试用）。系统方案自此被持久化，不再靠指针比较推断。
- **`currentScheme()` 重写**：无覆盖时返回 `p.systemScheme`，删除 `p.current == p.dark` 指针比较（S2 根因）。
- **`Watch` 抽出 `onSystemSchemeChanged(ch, s)`**：系统方案变化时（持锁）更新 `systemScheme`，且 `override == nil` 时重建 `p.current`，使 `Current()` 实时跟随；随后推送 channel（S3，与 `Theme.md` §6 契约对齐）。该函数跨平台可单测，不依赖真实系统主题事件。
- **测试补强**：
  - `TestProvider_SetOverride_ClearOverride_DarkSystem`：S2 回归——暗色系统 + 暗色覆盖 + `ClearOverride` → 断言回到 `SchemeDark`（原 bug 会错误回退 light）。
  - `TestProvider_Watch_UpdatesCurrentOnSystemChange`：S3 验证——直接调 `onSystemSchemeChanged(ch, dark)` 断言 `Current()` 变 dark；并验证有覆盖时系统变化不破坏用户选择。

### 验证
- `go build ./...` / `CGO_ENABLED=0 go build ./...` / `go vet ./...` / `go test ./...` 全绿。
- theme 覆盖率 **72.1% → 72.8%**（S2/S3 路径补覆盖）。
- 既有 `TestProvider_SetOverride_ClearOverride`（light init）仍绿，确认未引入回归。

### 状态
| 项 | 级别 | 状态 |
|----|------|------|
| S1（文档漂移） | 🟡 | ✅ 已修（见 §11，5 份 Phase 2 文档 §9 + Theme.md §1/§6 + Icon/Skin 关联引用已回写对齐代码） |
| S2（ClearOverride bug） | 🟡 | ✅ 已修 |
| S3（Watch 不更新 Current） | 🟡 | ✅ 已修 |

> 注：本次仅动 `internal/theme`（leaf 包，零 CGO），未触碰 calendar/theme 依赖方向；ADR-07a 仍成立。S1 文档 sweep、S4/S5/S6/S7 仍按原优先级待办，不阻塞 Phase 3。

---

## 11. S1 文档回写执行记录（2026-07-09 续）

**S1 已闭环**：把 5 份 Phase 2 设计文档的 §9（及关联章节）对齐到已落地的更好代码，消除「单一事实来源」污染。纯文档改动，无代码变更。

### 逐文档对齐要点
- **`Calendar.md`**：`NewCalendarService(lunar, holiday)` → `NewCalendarService(bus state.EventBus, lunar, holiday, opts...)`（新增 `bus` 以 emit `TopicDateChanged`，ADR-07a）；新增 `WithSelected`/`WithView`/`WithToday` Option，`today` 经 `WithToday` 可注入；`SetSelectedDate` 经事件总线广播（非独立函数）；§1/§2/§6/§8 同步更新（bus 字段、emit TopicDateChanged、Init 签名）。
- **`Lunar.md`**：`lunar.Solar.FromDate`+`IsLeap` 闰月判定 → `lunar.NewLunarFromDate(date)` + `GetMonth() < 0` 判闰（月份存绝对值）；`MonthStr` 补「月」；`GetFestivals/GetDayYi/GetDayJi` 返回 `*list.List` 经 `listToStrings` 转换；§3 数据流图同步。
- **`Holiday.md`（最彻底）**：**删除**运行时 HTTP 拉取 `raw.githubusercontent.com/NateScarlet/holiday-cn` 与 `isMakeupWorkday(name)` 脆弱 heuristic；改为 `holidays`/`workdays` 两张独立 map（补班天然不误判节假日）+ `Refresh` 仅重放 embed（零网络，契合 ADR-05 离线优先）；`NewHolidayRepository() (*embedHolidayRepo, error)`；§1/§2/§3/§6/§10 全部同步，§9 与 §10 自矛消除。
- **`Theme.md`**：删除错误的 `schemeCh` 字段与导出 `SystemScheme(ctx)`（用 `windows.OpenKey` 不存在的 API）；改为未导出 `systemScheme()`（经 `golang.org/x/sys/windows/registry`）+ `watchSystem(ctx, onScheme)`（GOOS 编译隔离）；`ThemeProvider` 新增 `systemScheme` 字段；`NewProvider` 返回 `(*ThemeProvider, error)`；§6 序列图已与 S3 修复一致（`Watch` 内 `onSystemSchemeChanged` 重建 `Current()`）；§1 删除对 `internal/infra/log` 的错误依赖声明（N6）。
- **`ThemeJson.md`**：**删除**不存在的 `LoadResult`/`ValidateOption` 类型；`Validate` 占位 stub → 实际由 `buildTheme` 强制校验 8 必填色 + `#RRGGBB(AA)` 解析，再交 `Validate` 做范围校验；`LoadEmbedded` 用正斜杠字符串拼接读取 embed（避免 Windows 上 `filepath.Join` 反斜杠破坏 `go:embed`）。

### 关联文档一致性
- `Icon.md` §6 序列图 `schemeCh <-` → `Watch() <-`；`Skin.md` 状态图 `Watch → schemeCh` → `Watch → channel`。两处引用已随 `schemeCh` 字段移除同步修正。

### 验证
- 文档自查：`docs/` 下 grep `schemeCh`/`SystemScheme(`/`LoadResult`/`ValidateOption`/`isMakeupWorkday`/`holiday-cn/master`/`net/http`（Holiday 上下文）均仅在说明性注释或无关模块（Weather/AutoUpdate 合法使用 net/http）出现，5 份目标文档及关联引用已无残留。
- 无代码改动，构建/测试不受影响；本任务仅消除文档漂移。

### 状态
| 项 | 级别 | 状态 |
|----|------|------|
| S1（文档漂移） | 🟡 | ✅ 已修 |

---

## 12. S4–S7 小项收尾执行记录（2026-07-09 续）

**S4 / S5（joinDate）/ S6 / S7 已闭环**，改动集中在 `internal/calendar` 与 `internal/theme`，TDD 红→绿。

### 改动要点
- **S4 · `today` 跨午夜**：`calendarService` 新增 `todayFixed bool` 字段；`WithToday` 设置 `todayFixed=true`；新增 `todayDate()`（固定时返回注入值、否则实时 `time.Now()`）供 `GetDayInfo` 判定 `IsToday`；新增 `RefreshToday()`（写入接口）清除固定值并恢复实时——供 shell 每日定时器跨午夜后调用。生产路径自此不再陈旧，测试注入值仍被尊重。
- **S5 · `joinDate` round-trip**：`time.Date` 规整后 `Format("2006-01-02")` 必须回写等于原 key，否则拒绝（拦截 `02-30` 这类死键，不再静默生成永不命中项）。**注意**：SEED `2026.json` 真实数据替换仍属 **v1.0 发布门**（需真实 holiday-cn/国务院数据，当前构建环境无外网），本次仅完成代码层校验收紧，数据替换须发布前单独完成并加 CI 清单门禁。
- **S6 · `Builtin` 标记**：`buildTheme` 增 `builtin bool` 参数；抽出 `parseBytes(ctx, data, builtin)`；`ParseBytes`（内置）传 `true`、`ParseFile`（用户主题 v1.3）传 `false`。`LoadEmbedded` 主题断言 `Builtin=true`，用户主题 `Builtin=false`，UI/设置可据此区分。
- **S7 · 闰月 golden**：`lunar_test.go` 新增 `TestLunarService_LeapMonth_Golden`，以真实 `lunar-go` 对 **2023-03-22（闰二月初一）** 断言 `LeapMonth==true`、`LunarMonth==2`、`MonthStr=="闰二月"`、`DayStr=="初一"`，锁死第三方闰月语义、防升级漂移。

### 新增/修改测试
- `calendar_test.go`：`TestCalendarService_TodayLazyFresh`（未注入时实时 IsToday）、`TestCalendarService_RefreshToday`（RefreshToday 清除固定值并恢复实时）。
- `holiday_embed_test.go`：`TestJoinDate_RoundTrip`（拒 `02-30`/`13-01`/`02-32`/`0222`，接受 `02-14`）。
- `themejson_test.go`：`TestParseFile_NotBuiltin`（用户主题 `Builtin=false`）+ `TestLoadEmbedded` 增 `Builtin=true` 断言。
- `lunar_test.go`：`TestLunarService_LeapMonth_Golden`（闰月分支 golden）。

### 验证
- `go build ./...` / `CGO_ENABLED=0 go build ./...` / `go vet ./...` / `go test ./...` 全绿。
- 覆盖率：calendar **85.9% → 88.6%**（S4/S7 补强）；theme **72.8% → 74.2%**（S6 补覆盖，ParseFile 路径从 0% 起步）。整体仍远高于 ≥60% 线。
- 依赖方向未变：`calendar`→`state`+lunar-go、`theme`→`x/sys`+embed，ADR-07a 成立。

### 状态
| 项 | 级别 | 状态 |
|----|------|------|
| S4（today 跨午夜） | 🟡 | ✅ 已修 |
| S5（joinDate 校验） | 🟡 | ✅ 已修 |
| S5（SEED 真实数据） | 🟡 | ⏸ 发布门：v1.0 前替换 + CI 清单 |
| S6（Builtin 误标） | 🟡 | ✅ 已修 |
| S7（闰月测试） | 🟡 | ✅ 已补 |

> 注：S1–S7 中仅剩 **S5 的 SEED 真实数据替换**为发布流程项（代码层已全部收口）。Phase 2 审查的全部代码层 🟡 至此闭环，可干净进入 Phase 3（shell 装配）。Phase 1 遗留 S4/S5（托盘 ctx 取消契约、真实 Monitor 枚举）仍待 Phase 3。
| S4（today 跨午夜） | 🟡 | ✅ 已修 |
| S5（Holiday SEED） | 🟡 | joinDate 已修；SEED 真实数据待发布前替换（发布门） |
| S6（Builtin 误标） | 🟡 | ✅ 已修 |
| S7（闰月测试） | 🟡 | ✅ 已补 |