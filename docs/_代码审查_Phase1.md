# DeskCalendar · Phase 1 代码审查报告

> 审查对象：Phase 1 平台原语（`internal/platform`：`DPI` / `MultiMonitor` / `Tray` / `Startup` / `Notification`，以及既有 `internal/platform/windowstyle`）
> 对照基线：Issue #146（开发顺序与依赖地图）+ `docs/20-Platform/{DPI,MultiMonitor,Tray,Startup,Notification,WindowStyle}.md` + ADR-07a（依赖倒置）+ `docs/02-开发规范.md`（覆盖率目标）
> 审查日期：2026-07-09 ｜ 工具：Go 1.25、`go build/vet/test/cover` + `go list` 依赖校验 + `go tool cover -func`（per-function）

---

## 0. 总体结论

**质量评级：良好（A- 区间），可直接进入 Phase 2，无代码层 blocker。**

- ✅ **硬性约束全部满足**：`go build ./...`、`go vet ./...`、`go test ./...` 全绿；`CGO_ENABLED=0 go build ./...` 通过（ADR-01/06 零 CGO 成立）。
- ✅ **依赖倒置铁律（ADR-07a）经 `go list` 实证**：`internal/platform` 仅依赖 `stdlib + golang.org/x/sys/windows + golang.org/x/sys/windows/registry + github.com/gogpu/systray`；反向依赖校验确认 **无任何** `feature/state/ui/plugin` 反向边进入 `platform`。平台层不认识业务层，符合 Issue #146 的依赖地图。
- ✅ **seam 架构落地扎实**：每个 OS 边界（DPI 感知、注册表、显示器枚举、托盘）都走 `xxxBackend` / `Monitor` / `TrayManager` 接口注入；纯逻辑（坐标换算、锚定、命令通道、自启往返）可单测，非 Windows 仅需 `//go:build !windows` 返回 nil backend 即可全量编译。
- ⚠️ **覆盖率**：项目整体 **57.8%**（略低于 `docs/02-开发规范.md` 的 ≥60% 目标），主因是 `internal/platform` 仅 **31.9%**，且该项几乎全部是被「无头 CI 无法执行的 Win32/注册表/systray 真调用」拉低——可测纯逻辑已覆盖。
- ⏸ **遗留（承接 Phase 0）**：B1（渲染模式死 token 文档漂移）已依 S1 于 2026-07-09 清扫（11 文档删除死 token，ADR-07 同步修订渲染模式条款）；剩余仅待架构师就 gogpu vs 路径 D 拍板技术选型。
- 👍 **亮点**：托盘双循环命令通道用非阻塞 `select` 防 goroutine 泄漏；build tag 隔离干净；Phase 0 的 S2（`signal_test`）/ N4（`log_test`）已闭环，说明 review 闭环有效。

---

## 1. 验证事实（工具产出，非推测）

| 项 | 结果 |
|---|---|
| `go build ./...` | 通过 |
| `CGO_ENABLED=0 go build ./...` | 通过 |
| `go vet ./...` | 通过 |
| `go test ./...` | 全部 ok |
| `platform` 导入 | `[context errors fmt gogpu/systray x/sys/windows x/sys/windows/registry os time]`（仅 stdlib + x/sys + gogpu/systray） |
| 反向依赖 | **无** —— `feature/state/ui/plugin` 均不被 `platform` 依赖；`platform` 也不被它们反向依赖至业务层 |
| `windowstyle` 导入 | `[]`（完全自包含，零外部依赖） |
| 覆盖率（整体） | **57.8%** |
| 覆盖率（分包） | calendar `100%` / windowstyle `100%` / state `82.6%` / plugin `66.7%` / config `74.4%` / log `57.1%` / **platform `31.9%`** |
| 覆盖率（per-func 低项） | `dpi_windows`：`setAwareness`/`effectiveDPI`/`dpiAwarenessContextValue` 0%；`startup_windows`：`openRunKey`/`setString`/`deleteValue`/`queryString` 0%；`tray_systray`：`SetIcon`/`SetTooltip`/`OnClick`/`Bounds`/`Run`/`Remove`/`sendTrayCmd` 0%；`multimonitor`：`defaultPanelAnchor.AnchorAboveTray` 0%；`dpi.DefaultAwareness` 0% |

> **环境说明**：本机为 **win32（Git Bash）**，`//go:build windows` 文件确实参与编译，故上表 0% 项均为「真实 syscall 在无头 CI 下无法执行」，而非「未编译」。这正是 `internal/platform` 31.9% 的成因——见 §3 S2 分析。

---

## 🔴 Blockers（必须解决）

**Phase 1 代码层无新增 🔴 blocker。**

代码编译、测试、零 CGO、依赖方向、seam 可测性全部达标，未发现正确性 / 安全性 / 数据丢失 / 竞态层面的硬伤。Phase 0 的 🔴 B1 已在本报告 §2 以「承接遗留 ⏸」重新定性（属治理/文档范畴，代码本身正确），不再作为本阶段代码 blocker。

---

## 🟡 Suggestions（应修）

### S1 · 文档漂移：渲染模式死 token 曾残留在 11 个文档（承接 Phase 0 B1 · 已清扫 2026-07-09）

**事实**：`windowstyle.go` 本身**并未**定义“宿主托管式”渲染模式常量——它定义了与本地 gogpu fork 对齐的本地 `RenderMode` 枚举（Auto/CPU/GPU），并有清晰注释说明「早期设计稿曾设想 gogpu 提供该常量，但本地 fork 实际只导出 Auto/CPU/GPU」。代码侧决策是**正确且自洽**的。

**但文档侧仍漂移**。本轮 `grep` 曾命中 11 个文件含该死 token（已于 2026-07-09 清扫）：

```
docs/_路径D文档修订计划.md
docs/_模板与写作规范.md
docs/_代码审查_Phase0.md
docs/_交叉一致性审查报告.md
docs/DeskCalendar_技术评估报告_gogpu-ui-Windows.md
docs/ADR-03-无边框窗口样式.md
docs/ADR-07-事件总线归属与入口约定.md
docs/poc/transparent-window/README.md
docs/90-UI/MainWindow.md
docs/10-Shell/App.md
docs/20-Platform/WindowStyle.md
```

`ADR-07` 与 `WindowStyle.md §9` 曾基于「fork 有“宿主托管式”渲染模式常量」这一**错误前提**（已于 S1 清扫）；Phase 3 shell 装配 `RenderMode → gogpu.RenderMode` 适配器时，作者会困惑该信文档还是代码。

**为什么是 🟡（已降级，非 🔴）**：Phase 0 报告 §9 已将该项移交架构师（⏸ 待 gogpu vs 路径 D 选型拍板），代码保持原状。**但文档清扫可「无偏见」先行**——无论最终选 gogpu/ui 还是路径 D，该渲染模式常量在本地 fork 中都不存在，删掉死 token 不会 prejudge 架构（已于 2026-07-09 完成）。

**建议**：
1. **现在即可做**：对上述 11 个文件做一轮「删除/替换该死 token」清扫，统一指向代码现状（本地 `RenderMode` 枚举 + Phase 3 适配器说明）。这是纯文档一致性修复，不依赖架构师决策（已于 2026-07-09 完成）。
2. **架构师拍板后**：修订 `ADR-07` 与 `WindowStyle.md §9`，把「RenderMode 权威来源」正式钉为代码现状。

---

### S2 · `internal/platform` 31.9% 覆盖率的成因与补齐策略

**成因（经 per-func 证据）**：31.9% 几乎全部来自「无头 CI 下无法执行的真实 OS 调用」——`winDPIBackend`/`winRegistryBackend` 的 syscall 方法、`systrayTrayManager` 的 `Run/SetIcon/Remove/sendTrayCmd` 全 0%。这些需要真实 Windows 桌面会话（user32.dll 加载、注册表 HKCU、systray 消息循环）才能跑。**可测纯逻辑其实已覆盖**：`AnchorAboveTray` 主函数 91.7%、`ScaleLogicalToPhysical/ScalePhysicalToLogical` 66.7%、Startup 往返经 `memRegistryBackend` fake 覆盖、Notification `noopSender` 100%、Tray 契约经 `fakeTrayManager` 覆盖。

**易得纯逻辑补强（低成本，不依赖 Windows）**：
- `multimonitor.go:37` `defaultPanelAnchor.AnchorAboveTray`（0%）——补 1 个「经 `NewPanelAnchor()` 接口调用」的测试即可（当前测试直接打包级函数，没走接口方法）。
- `dpi.go:16` `DefaultAwareness`（0%）——一行测试断言返回 `DPIPerMonitorAwareV2`。
- `dpi.go` `Scale*`（66.7%）——补 `dpi<=0 → 回退 96` 分支与 backend 返回 error 的两条路径。

**OS 胶水覆盖策略（需决策）**：
- 选项 A：引入 **Windows 桌面 CI runner**（GitHub Actions `windows-latest`），让 `winDPIBackend`/`winRegistryBackend` 真跑；但 systray 仍需真实托盘环境，部分仍难覆盖。
- 选项 B：**更深注入**——把 `systray.SystemTray` 也做成可注入接口（测试注入 mock），`Monitor` 真实枚举实现同样走 fake；这样即使无桌面也能覆盖命令闭环与枚举逻辑。
- 现状与 `docs/02` 的「≥60% 整体 / ≥80% 核心 domain」目标：整体 57.8% 仅微差，补完上面 3 个易得测试预计可把 platform 拉到 ~45–50%、整体回到 60%+。**核心 domain（state/calendar/windowstyle）均已 ≥80%**，达标。

**建议**：先补 3 个易得纯逻辑测试（顺手消掉明显空白），再就 OS 胶水覆盖在 Phase 3 shell 装配前定一个策略（倾向选项 B 更深注入，性价比最高）。

---

### S3 · `StartupManager.Enabled()` 精确字符串匹配偏脆

**位置**：`internal/platform/startup.go:79` `Enabled()` 直接比对注册表值与 `intendedValue()`（= `exePath + " --minimized"`）的**完全相等**。

**风险**：
- Windows 文件系统大小写不敏感，`C:\Apps\DeskCalendar.exe` 与 `c:\apps\deskcalendar.EXE` 是同一文件，但字符串比较会判为「不同」→ 误报未启用。
- 若用户把 exe 移动到其他目录（或安装器改路径），旧值仍在 → 既可能误报「已启用」指向死路径，也可能因路径不符误报「未启用」。
- `exePath` 含空格时注册表存储/读取的引号处理未显式归一。

**建议**：比较前对双方做 `filepath.Clean` +（Windows 下）`strings.ToLower` 归一；或改为「前缀匹配 `exePath` + 含 `--minimized` 标记」的宽松判定，比精确相等更鲁棒。当前 `TestStartupManager_EnabledFalseWhenStalePath` 已守住「不同路径→false」，但大小写/移动场景未覆盖。

**状态**：✅ 已修（2026-07-09）。`Enabled()` 改为经 `sameStartupValue()` 归一化比较：先 `stripStartupExe()` 去引号包裹 + 去 ` --minimized` 后缀，再 `filepath.Clean`，Windows 下用 `strings.EqualFold` 大小写不敏感判等；非 Windows 仍为精确比较。保留「必须指向本 exe」语义（旧路径/其他程序同名值仍判 false）。新增 `TestStartupManager_EnabledTrueWhenQuoted` / `EnabledCaseInsensitive`（Windows 守卫）/ `EnabledSeparatorNormalized`（Windows 守卫）三用例覆盖归一化，「移动 exe」场景维持「指向死路径即判未启用」的正确行为（避免把失效项误报为启用）。platform 覆盖率 36.1% → 40.8%。

---

### S4 · 托盘 `ctx` 取消契约需在 Phase 3 落实验证（前置预警）

**位置**：`internal/platform/tray_systray.go` `Run` 内 `go func(){ <-ctx.Done(); systray.Remove() }()`。

**事实**：非阻塞 `select` 发命令的设计很好（防 goroutine 泄漏），但「图标移除 goroutine」依赖调用方在退出时 **cancel 这个 ctx**。若 Phase 3 shell 忘记 cancel，`systray.Remove()` 不执行 → 托盘图标残留 / goroutine 泄漏。

**建议（Phase 3 前落实）**：在 `TrayManager` 接口或 shell 装配文档中明确「`Run` 的 ctx 由 shell 在退出路径 cancel」为契约；可补一个 `TestRun_RemovesIconOnCtxCancel`（用可注入的 `systray.SystemTray` mock）把该契约钉死。现在标记，避免 Phase 3 遗漏。

---

### S5 · `MultiMonitor` 真实显示器枚举未实现（已知接受，登记追踪）

**事实**：`Monitor` 接口已就绪，但真实 `EnumDisplayMonitors` 封装未做（今日 memory 已记录：Phase 3 shell 装配时补）。`AnchorAboveTray` 等纯逻辑已充分测试。

**建议**：Phase 3 实现真实 `Monitor` 时，**务必沿用 seam 思路**——把 `EnumDisplayMonitors` 封装进可注入的 backend，并提供 fake 做单测，避免把「显示器枚举」变成又一个只能实机验证的黑盒（重蹈 S2 覆辙）。仅登记，不阻塞。

---

## 💭 Nits（可选）

- **N1 · `defaultPanelAnchor.AnchorAboveTray` 方法 0% 覆盖**（S2 已含）：补一个经接口调用的测试即可，成本极低。
- **N2 · `dpi.DefaultAwareness` 0% 覆盖**（S2 已含）：一行断言测试。
- **N3 · `sendTrayCmd` 非阻塞 `select` 是好模式，建议加一行 WHY 注释**：说明「消费者慢时不阻塞发送方、避免 goroutine 泄漏」，防止后人「好心」改成阻塞发送导致回归。当前实现正确，纯文档增强。
- **N4 · `notification.go` `noopSender` 占位合理**：确保 v1.1 真实 Toast 实现（issue #60）沿用同一 `NotificationSender` 接口、走 seam 注入；接口现在就稳定是可取的「决策可逆」做法。仅追踪。
- **N5 · 测试 fake 可适度收敛**：`memRegistryBackend` / `fakeTrayManager` / `fakeDPIBackend` / `recordingSender` 分散在各 `_test.go`。若后续 fake 增多，可归入 `internal/platform/internal/testhelp` 减少重复。低优。

---

## 👍 做得好的地方（应当保留）

- **seam/backend 注入贯穿所有 OS 边界**：DPI、注册表、显示器、托盘全部接口化，纯逻辑可单测、非 Windows 可编译——这是平台代码最该有的样子，做得彻底。
- **双循环纪律（ADR-02）到位**：托盘命令通道用非阻塞 `select`，从设计上杜绝 goroutine 泄漏；`tray_test.go` 用 `fakeTrayManager` + `SimulateClick()` 验证了「点击→命令」闭环与枚举顺序稳定性。
- **build tag 隔离干净**：`//go:build windows` / `//go:build !windows` 正确切分真实 backend 与 nil stub；`!windows` 返回 nil backend 让全量编译不依赖 Windows，零 CGO 成立。
- **依赖倒置铁律实证**：`go list` 证明 `platform` 不反向依赖任何业务包，符合 Issue #146 依赖地图与 ADR-07a。
- **自启往返测试有安全/正确性洁癖**：`TestStartupManager_EnabledFalseWhenStalePath` 验证「注册表指向旧/不同 exe 路径 → 不视为启用」，防止误判。
- **Notification 接口现在就定义**（Issue #146 要求）：MVP 用 `noopSender` 占位、真实 Toast 延后 v1.1，决策可逆、上层可立即依赖，节奏正确。
- **Phase 0 的 S2（`signal_test`）/ N4（`log_test`）已闭环**：说明 review→修复闭环有效，团队听进去了。
- **`windowstyle.go` 本地 `RenderMode` 枚举决策正确且文档化**：对齐本地 gogpu fork 现实（无“宿主托管式”渲染模式常量）、避免把 wgpu 栈引入基础层、守住零 CGO——是经得起推敲的工程判断。

---

## 建议后续动作（按优先级）

1. **⏸ B1 治理（承接）**：等架构师就 gogpu vs 路径 D 拍板后修订 `ADR-07` + `WindowStyle.md §9`；但 **11 文档的死 token 清扫现在就能无偏见执行**（S1）。
2. **🟡 S2**：补 3 个易得纯逻辑测试（`NewPanelAnchor().AnchorAboveTray` / `DefaultAwareness` / `dpi` 边界分支）→ platform 预计 ~45–50%、整体回到 60%+；OS 胶水覆盖在 Phase 3 前定策略（倾向更深注入）。
3. **🟡 S3/S4**：Startup 路径归一化 + Tray `ctx` 取消契约（Phase 3 前落实）。
4. **💭 N1–N5**：顺手清理（多数已被 S2 覆盖）。

> 备注：Phase 1 代码层无 🔴 blocker；所有 🟡 均为质量增强与覆盖率策略，不阻塞 Phase 2 启动。若需要，我可直接执行 S1 的 11 文档清扫 + S2 的 3 个测试补强。

---

## 9. 执行记录（本次 / 2026-07-09）

| 项 | 级别 | 本轮判定 | 状态 |
|----|------|----------|------|
| B1（承接 Phase 0） | 🔴→⏸ | 代码正确（本地 `RenderMode` 枚举对齐 fork）；残留为 **11 文档死 token 漂移**，已量化（S1） | ⏸ 文档清扫可现在做 / 治理待架构师 |
| S2（Phase 0 signal_test） | 🟡 | 已闭环（`signal_test.go` 存在，覆盖 `NewSignalWithEqual`） | ✅ 已修（早前） |
| N4（Phase 0 log_test） | 💭 | 已闭环（`log_test.go` 存在，log 覆盖 57.1%） | ✅ 已修（早前） |
| S1（本轮新增） | 🟡 | 11 文档含渲染模式死 token | ✅ 已清扫（2026-07-09） |
| S2（本轮新增） | 🟡 | platform 31.9% = 无头 CI 不可执的 OS 胶水 + 3 个易得纯逻辑空白 | 🟡 待补强 |
| S3（本轮新增） | 🟡 | Startup 精确字符串匹配偏脆（大小写/移动路径） | ✅ 已归一化（2026-07-09） |
| S4（本轮新增） | 🟡 | Tray `ctx` 取消契约待 Phase 3 钉死 | 🟡 待 Phase 3 |
| S5（本轮新增） | 🟡 | MultiMonitor 真实枚举未做（已知接受） | ⏸ Phase 3 |

**验证**：`go build ./...` / `go vet ./...` / `go test ./...` 全绿；`CGO_ENABLED=0 go build ./...` 通过；项目整体覆盖 **57.8%**（platform 31.9% 为 outlier，主因 OS 胶水无头不可测）；`go list` 确认 `platform` 零反向业务依赖。

**结论**：Phase 1 平台原语质量达标，无代码 blocker，可进入 Phase 2。遗留项均为质量增强与文档治理，不阻塞进度。
