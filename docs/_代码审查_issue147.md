# 代码审查报告 · Issue #147（v1.1 视觉润色：Win11 DWM 圆角 + 轻量阴影）

> 审查日期：2026-07-15 ｜ 审查对象：提交 `8f1cd22`（`feat(win32): #147 v1.1 视觉润色——Win11 DWM 圆角 + 轻量阴影`）
> 对照基线：Issue #147 验收条款 ＋ ADR-08（降级脱离 gogpu 上游阻塞）
> 审查方法：工具实证（build / vet / 零 CGO 交叉编译 / 单测）+ 源码逐行 diff 审查（CodeReviewExpert 优先级 🔴/🟡/💭）

---

## 0. 总评

**质量评级：A（发布级，无代码层 blocker）。**

#147 是一次教科书级的「增量视觉润色」：改动严格限定在 `internal/platform/win32` 的 DWM 调用与窗口类样式 + `internal/platform/windowstyle` 的文档声明，**零回归**既有 DPI / 失焦 / Esc / 托盘锚定逻辑。全程零 CGO、纯 DWM 合成、不引入 `WS_EX_LAYERED` / 每像素 alpha，与 ADR-08 决策完全一致。测试用与生产代码同款的 `func 变量 seam` 手法注入 `DwmSetWindowAttribute`，跨平台零真实窗口即可确定性跑通，规避了 `#113` 早期遇到的 `unsafeptr` vet 告警。

**结论：可直接合入（commit 已落 main）；工作区另有 1 处未提交文档改动 `docs/ADR-08-降级脱离gogpu上游阻塞.md`（仅里程碑映射，非阻塞，见 N4）。**

---

## 1. 验证事实（工具实证，非推测）

| 检查项 | 命令 | 结果 |
|---|---|---|
| 全量编译 | `go build ./...` | ✅ BUILD_OK |
| 零 CGO 编译 | `CGO_ENABLED=0 go build ./...` | ✅ CGO_OK |
| **真实目标交叉编译**（Windows） | `GOOS=windows CGO_ENABLED=0 go build ./...` | ✅ WIN_BUILD_OK |
| 静态检查 | `go vet ./...` | ✅ VET_OK |
| #147 单测 | `go test -run TestApplyVisualPolish_DWMRoundCorner ./internal/platform/win32/` | ✅ PASS（0.00s）|
| 依赖方向（铁律） | `go list -f '{{.ImportPath}} => {{.Imports}}' ./internal/platform/win32` | ✅ 仅 `[context fmt platform x/sys/windows image sync/atomic unsafe]`，**零 gogpu/wgpu/gg 反向依赖** |
| ADR-08 干净度 | `grep -rEn "gogpu|wgpu|gpucontext" --include=*.go internal cmd`（排除 `_test.go`） | ✅ 仅注释与合法 `gogpu/gg`（CPU 光栅）、`gogpu/systray`（托盘）本地 replace 库；**无 `wgpu`/`gpucontext` 源码引用** |

> ⚠️ **关于全量 `go test ./internal/platform/win32/` 在本会话耗时**：该包含需在交互式窗口站创建真实窗口的 smoke 测试（`TestWin32Window_RenderAndPresentFullPipeline` 等），在本无头/非交互会话下会阻塞于消息泵（与 Phase 3/4 复核时同样的现象，非 #147 回归）。#147 专属测试已单独通过；真实窗口套件建议在交互式 Windows runner 上跑（见 N3）。

---

## 2. 改动逐项核对（对照 Issue #147 验收条款）

### 2.1 圆角（必做，零成本）— ✅ 正确落地
- `window_windows.go` 新增 `dwmapi` LazyDLL 与 `dwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute").Call`。
- `run()` 在 `CreateWindowExW` 拿到 `hwnd` 后调用 `applyVisualPolish(hwnd)`：
  ```go
  pref := uint32(dwmwcpRound)            // DWMWCP_ROUND = 2
  dwmSetWindowAttribute(hwnd, dwmwaWindowCornerPreference /*33*/, uintptr(unsafe.Pointer(&pref)), 4)
  ```
- 常量值正确：`DWMWA_WINDOW_CORNER_PREFERENCE=33`、`DWMWCP_ROUND=2`、`cbAttribute=4`（`sizeof DWORD`），与 Windows SDK 完全一致。
- **Win10 优雅降级**：`DwmSetWindowAttribute` 在 Win10 返回 `E_INVALIDARG`（该 attribute 为 Win11 新增），生产代码**忽略返回值**，窗口仍是可用方角弹窗 —— 与验收「无回归」一致。
- **DPI PMv2 自适应**：圆角由 DWM 在 Per-Monitor V2 下自动随缩放合成，无需额外处理 —— 验收条款明确认可。

### 2.2 阴影（可选）— ✅ 决策正确、落地正确
- 窗口类 `wcex.Style` 新增 `csDropShadow = 0x00020000`（`CS_DROPSHADOW`）。
- 关键正确性判断：DWM 默认阴影在 `WS_EX_TOOLWINDOW` 窗口上被系统抑制；而 `CS_DROPSHADOW`（经典非 DWM 投影）**恰好在 tool window 上显示**，因此本弹窗既得柔和阴影、**又保留 Alt-Tab 隐藏**（未触碰 `WS_EX_TOOLWINDOW` 标志）、**不引入分层窗** —— 精准命中 Issue #147「评估 DWM 默认阴影行为；若缺失引入轻量方案、不引入分层窗为优先」的结论。
- 该样式为窗口**类**样式、在 `RegisterClassEx` 时设置；因本弹窗每实例用唯一类名（`classSeq`，Phase 3 S6 修复），作用域隔离、无交叉污染。

### 2.3 不引入项（与 ADR-08 一致）— ✅ 确认未引入
- 全仓 **无** `WS_EX_LAYERED`、`UpdateLayeredWindow`、每像素 alpha、透明 gg 绘制；`go.mod` 与依赖图均无 `wgpu`/`gpucontext` 回归。

### 2.4 `windowstyle.go` 文档声明对齐 — ✅ 仅注释，无功能变更
- `CornerRadius` / `Shadow` 字段注释补充「v1.1(#147) 已通过 DWM / CS_DROPSHADOW 真正落地」；包文档同步。纯注释变更，不影响编译与运行。

---

## 3. 发现清单

### 🔴 Blockers：无
代码编译 / 零 CGO / 测试 / 依赖方向 / ADR-08 一致性全部达标，无正确性 / 安全 / 竞态硬伤。

### 🟡 应修（建议，非阻塞）
**S1 — `applyVisualPolish` 静默吞掉 DWM 返回值，缺诊断日志（可选增强）**
`window_windows.go:applyVisualPolish` 直接调用 `dwmSetWindowAttribute(...)` 而**不检查返回**。当前这是**有意设计**（Win10 下 `E_INVALIDARG` 即预期，方角无回归），本身没问题；但万一未来在 Win11 上因未知原因失败（如 DWM 服务异常），现场无任何可观测信号。
- **建议**：保留「忽略失败」的语义，但可 `log.Debugf("DwmSetWindowAttribute corner pref: %v", err)` 仅在 `err != nil` 时记录，便于一线排查。属可选增强，**不阻塞发布**。

**S2 — 测试 seam 的包级 `var` 注入非并发安全（文档化约束即可）**
`TestApplyVisualPolish_DWMRoundCorner` 通过 `dwmSetWindowAttribute = func(...){...}; defer 复原` 注入 seam。当前包内测试**串行执行**（无 `t.Parallel`），且 `applyVisualPolish` 仅在 `w.run()` 与测试内调用，故无竞态。但若日后给任何调用链加 `t.Parallel()`，该包级 `var` 的读写会成数据竞争。
- **建议**：在 seam 注入处加一行注释 `// 注意：dwmSetWindowAttribute 为包级变量，注入期间禁止并行测试`，与既有的 `deleteObject` seam 注释保持一致即可。🟡 中低风险。

### 💭 Nits（Nice to Have）
**N1 — 阴影可见性依赖系统设置（已在注释中部分覆盖，建议显式说明）**
`CS_DROPSHADOW` 的实际投影还取决于用户的「在窗口下显示阴影」系统设置（`SystemParametersInfo(SPI_SETDROPSHADOW)`）。默认开启，但部分精简/高对比度主题下可能不显示。这是「可选阴影」的合理边界，建议在 `windowstyle.go` 或 doc 中补一句「阴影受系统『显示阴影』设置影响」，避免用户误报为 bug。

**N2 — `WindowStyle.CornerRadius` / `Shadow` 仍为声明字段、未被 win32 消费（已知延后项）**
`#147` 把视觉效果**硬编码**在 win32（`DWMWCP_ROUND`、固定 `CS_DROPSHADOW`），并未从 `WindowStyle.CornerRadius` / `Shadow` 读取值驱动。当前单一样式（圆角固定系统值 + 阴影开）完全满足验收。但若未来 milestone `v1.3 #39 换肤联动圆角` 想运行时调节圆角半径，需要把 `WindowStyle` → win32 的通道真正接上（现在是声明与实现分离的「双轨」）。
- 这是 Phase 0/3 遗留的架构清晰度问题，非 #147 引入，且已在 ADR-08 / issue 标注为延后。**仅需知情**，无需本次修改。

**N3 — 真实窗口 smoke 测试需交互式 runner（CI 配置建议）**
`internal/platform/win32` 的 real-window 测试在本会话无头环境会阻塞于消息泵（见 §1 脚注）。建议在发布流水线配置一台交互式 Windows runner（`runs-on: windows-latest` + 开启桌面会话），让 B1/S3/S5/N1 等真实窗口回归在 CI 真正跑起来（此前 Phase 3 复核已数次提出，作为持续建议）。

**N4 — 工作区有 1 处未提交的文档改动（非阻塞）**
`docs/ADR-08-降级脱离gogpu上游阻塞.md` 相对 main 有未提交修改：仅补充了 GitHub 里程碑落地映射表（`v1.0~v1.5` 与各 epic 对应），属文档梳理，不影响代码。可一并提交或单独处理，不阻塞 #147 合入。

---

## 4. 亮点（值得肯定）

1. **零回归纪律**：改动精确收敛于「DWM 调用 + 窗口类样式 + 文档」，DPI PMv2、失焦/Esc 自隐藏（S3）、托盘锚定（Phase 3 B1）、单写者（Phase 3 S1）等核心路径**一行未动** —— 真正做到了增量润色。
2. **ADR-08 完整性守住**：纯 DWM 合成 ＋ 类样式，零 CGO，无 `WS_EX_LAYERED` / 每像素 alpha / 透明，无 `wgpu` 回潮；依赖方向 `go list` 实证仅 `platform + x/sys/windows + stdlib`。
3. **seam 测试手法一致且优雅**：`dwmSetWindowAttribute` 采用与 S5 `deleteObject` 同款的「`func 变量` 注入」模式；测试断言 `(hwnd, 33, 非空指针, 4)` 且 `hwnd==0` 不触发，**无需真实窗口即可跨平台确定性跑通**，并刻意不反向重建 `*uint32` 以规避 `go vet` 的 `unsafeptr` 告警 —— 细节处理到位。
4. **跨版本降级正确**：Win10 下 `E_INVALIDARG` 被静默忽略，方角窗口无回归，符合验收「无回归」硬条款。
5. **`hwnd==0` 防御性守卫** + `gofmt` 全量归一化，工程整洁度在线。

---

## 5. 历史闭环状态（连续 review→修复 链）

- Phase 0 → Phase 3 全部 🔴/🟡 已在既往轮次闭环（含 Phase 3 B1 跨线程 Store、S2 表头错位、S1 单写者、S3 抢前台、S4 字体缓存、S5 GDI 生命周期、S6 唯一类名；Phase 4 B1 红测试、S1/S3–S5；Phase 5 B1/B2/S3–S5）。
- **本次 #147：0 个新 🔴，0 个必须修 🟡**；S1/S2 为可选增强与约束文档化建议。review→修复→复核 闭环在增量润色类变更上继续保持「零回归」高标准。

---

## 6. 建议的下一步
1. 合入 #147 提交 `8f1cd22`（已落 main，无需额外动作）。
2. （可选）落实 S1 的 debug 日志、S2 的并行约束注释。
3. 顺手提交未暂存的 `docs/ADR-08-降级脱离gogpu上游阻塞.md` 里程碑映射（N4）。
4. 推进 `v1.2 #150 显隐动画`（里程碑已建，依赖自绘 `Animator`，无 gogpu 依赖）。
