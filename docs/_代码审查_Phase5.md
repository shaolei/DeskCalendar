# 代码审查报告 — Phase 5（Release / 发布工程）

> 审查对象：`github.com/shaolei/DeskCalendar` Phase 5（v1.0 MVP 发布）
> 对照基线：Issue #146 发布计划、`docs/100-Release/{Build,Package,CI,AutoUpdate}.md`、ADR-06（零 CGO）
> 审查方式：逐文件读码 + `go build`/`CGO_ENABLED=0 go build`/`go vet`/`go test` 工具实证 + 依赖方向校验
> 日期：2026-07-12

---

## 0. 概览

**质量评级：B（发布工具链代码质量高、测试扎实；但 CI/Release 流水线存在 2 个 🔴 阻断项，导致在干净 runner 上无法自动化发布。另有 1 个跨模块契约 🟡 必须在发布前修。）**

Phase 5 交付了三块发布工程：`build`（版本注入 + 全局资源嵌入，`build/`、`build/assets`、`build/packaging`）、`Package`（NSIS 安装器 + 便携 zip，`build/nsis/installer.nsi` + `cmd/packager`）、`CI`（`.github/workflows/ci.yml` + `Makefile` + `scripts/ci-build.sh` + `scripts/package.sh` + `.golangci.yml`）。

**代码本身质量很好**：版本管理单一可信源、打包层接口清晰可测、节假日刷新 fail-soft 契合离线优先。但**发布流水线的可运行性**有两个硬伤——都源于「本地开发便利」与「干净环境可复现」的冲突——这恰恰是 Phase 5 这种「release」阶段最该被审查出来的问题。

---

## 1. 验证事实（工具实证，非推测）

| 项 | 结果 |
|---|---|
| `go build ./...` | ✅ BUILD_OK |
| `CGO_ENABLED=0 go build ./...` | ✅ CGO_OK（零 CGO 硬约束守住） |
| `go vet ./...` | ✅ VET_OK |
| `go test ./...` | ✅ 全绿（20 包，Phase 4 的 B1 红测试已修复，见 §6） |
| 依赖方向（ADR-07a） | ✅ `build`/`build/assets`/`build/packaging`/`cmd/*` 均为叶子，无任何 `internal/feature` 反向依赖（`go list` 实测：`build => []`，`build/packaging => [stdlib only]`，`cmd/packager => [build/packaging]`） |
| 覆盖率 | `build` **100%** / `build/assets` **100%** / `build/packaging` **46.4%** / `cmd/deskcalendar` **50.0%**（发布工具链，不在「核心 domain ≥60%」目标内，可接受）；整体约 **74%**，与 Phase 4 基线持平 |
| 资产嵌入 | ✅ `build/assets` 仅嵌入 `icon/app.ico` + `theme/default.json`（字体按路线图推迟 v1.3，代码注释明确，与 Build.md 实际一致） |
| 节假日数据 | ✅ `2026.json` 已是**真实生产数据**（33 节假日 / 6 补班，逐日核对 holiday-cn master@2026-07-11，零偏差）——Phase 2 遗留 S5 已闭环（见 §6） |
| 二进制入库 | ✅ `.gitignore` 正确忽略 `/dist/`、`*.exe`；`git ls-files` 确认无二进制被追踪 |

---

## 2. 🔴 Blockers（发布前必须修）

### 🔴 B1 — `go.mod` 本地绝对路径 `replace` 让干净 CI runner 无法解析模块

`go.mod` 当前含：

```go
replace github.com/gogpu/systray => D:/workspace/github/systray
replace github.com/gogpu/gg      => D:/workspace/github/gg
```

**Why：** 这两个是 Windows 绝对路径的本地 replace。`.github/workflows/ci.yml` 在 `ubuntu-latest` 上 `actions/checkout` 后执行 `go build` / `go test` / `go run ./cmd/packager` / `golangci-lint`，而 `D:/workspace/github/...` 在 Linux runner 上**根本不存在** → 所有 go 命令报 `directory does not exist` → **lint / test / build / release 四个 job 全红**。

**Why（续）：** 工程师在提交 `2ed5995 feat(release): #129 …（暂留本地，待推送）` 已自知 CI 未接线。但「Phase 5 完成」的判定标准应是「`git push` 后打 tag 能自动出 Release」，而当前的流水线做不到。这正是 release 阶段审查必须抓住的头号问题。

**Suggestion：**
- 把 `gg` / `systray` 变成 CI 可达的版本化模块：打 tag 后去掉本地 replace，或在 `go.mod` 用可访问的版本（`require github.com/gogpu/gg vX.Y.Z`）；
- 或 `go mod vendor` + 提交 `vendor/`（注意 `.gitignore` 当前忽略 `vendor/`，需放开）+ CI 加 `-mod=vendor`；
- 或 `go.work` + submodule，但本地绝对路径仍不可移植，不推荐。
- 无论哪种，修完必须在**独立干净容器**里跑一次 `go build ./...` 验证（本机有 `D:/workspace` 能过，掩盖了问题）。

---

### 🔴 B2 — CI `test` job 用 `go test -race`，但 `-race` 强制要求 CGO，与 ADR-06 零 CGO 硬约束冲突且必然编译失败

`.github/workflows/ci.yml:42`：

```yaml
run: go test -race -coverprofile=coverage.out -covermode=atomic ./...
```

**Why（实测）：** 在本机直接执行得到：

```
go: -race requires cgo; enable cgo by setting CGO_ENABLED=1
```

即在零 CGO 环境下 `-race` 直接报错。在 Linux runner 上 CGO 默认开启，`-race` 会尝试以 CGO 编译**全部**传递依赖的测试二进制——而模块图含 `goffi`（wgpu 绑定的 cgo 包），需要系统安装 `libwebgpu` 开发头文件，标准 `ubuntu-latest` 无此依赖 → **test job 编译失败变红**。

**Why（续）：** 更深一层是设计冲突——ADR-06 明确「永不启用 CGO」，而本项目的并发模型（Phase 3 S1 单写者 + 通道）是**靠设计规避竞态**，不是靠 `-race` 检测。CI 文档（`CI.md` §9/§10）与代码都写了 `-race`，与 ADR-06 自相矛盾（同类问题已在前几轮反复出现，属 doc/code 漂移）。

**Suggestion：**
- 删除 `-race`，改为 `go test -coverprofile=coverage.out -covermode=atomic ./...`（保留覆盖率）。
- 同步修订 `docs/100-Release/CI.md` §9/§10，去掉 `-race` 表述，并显式注明「零 CGO 下 `-race` 不可用，并发安全靠单写者+通道设计保证」。
- 若未来确需竞态检测，应另起一个**仅编译不含 cgo 依赖的纯逻辑包**的 race job，而非全量 `./...`。

---

## 3. 🟡 Suggestions（应在发布前修）

### 🟡 S1 — NSIS 安装器的自启注册值缺 `--minimized`，与 app `startup.intendedValue()` 契约不符

`build/nsis/installer.nsi:99`：

```nsis
WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${APPNAME}" `"$INSTDIR\${EXE}`"
```

而应用侧 `internal/platform/startup.go`：

```go
const startupValueSuffix = " --minimized"
func (m *regStartupManager) intendedValue() string { return m.backend.setString(..., m.intendedValue()) } // = exe路径 + " --minimized"
func (m *regStartupManager) Enabled() bool { return sameStartupValue(v, m.intendedValue()), nil }          // 精确比较
```

**Why：** 安装时若勾选自启，注册表写入 `C:\...\deskcalendar-amd64.exe`（无 `--minimized`）。用户首次打开「设置」时，`Enabled()` 拿注册表值与 `exe --minimized` 精确比对 → **不等 → 设置里自启复选框显示为「关」**，但系统实际已自启。点一下「开」会被 app 覆盖成 `exe --minimized` 才正常。这是跨模块契约错配，会让 v1.0 的自启开关看起来「坏掉」。

**Suggestion：** NSIS 写入 `"$INSTDIR\${EXE}" --minimized`（与 `startup.intendedValue()` 对齐）；卸载段已 `DeleteRegValue` 一致，无需改。

---

### 🟡 S2 — `Build.md` §9 承诺的 `build/target.go`（Target/Builder/GoBuilder）未实现，交叉编译目标实际落在 shell

文档定义 `build/target.go` 含 `Target` / `AllTargets()` / `Builder` 接口 / `GoBuilder`，称「CI 的 `scripts/ci-build.sh` 按 `Target.Arch` 循环调用」。实际仓库**无 `build/target.go`**：目标矩阵由 `Makefile` + `scripts/ci-build.sh` 的 shell 循环（amd64/arm64）直接承担，`GoBuilder.Build` 的 Go 抽象被删去。

**Why：** 非 bug，但属反复出现的「doc 承诺 Go 抽象、代码用 shell/实测更简单」漂移。Release 模块的 §9 应改为如实描述 Makefile/shell 为交叉编译单一事实源。

**Suggestion：** 修订 `Build.md` §9，删除 `build/target.go` 虚构代码，写明「目标矩阵在 `Makefile` + `scripts/ci-build.sh` 维护」。

---

### 🟡 S3 — `installer.nsi` 在安装期写 HKCU Run，偏离 `Package.md` 设计边界，且是 S1 的根因

`Package.md` §1/§9 明确：「安装包不写注册表」「真正写注册表由 `internal/platform/startup` 在用户首次运行设置时完成」「`AutoStart` 仅控制 NSIS 页面是否默认勾选」。但 `installer.nsi` 实际在安装时直接 `WriteRegStr HKCU ...Run`（line 94-100），与文档边界冲突，也正是 S1 契约错配的来源。

**Why：** 功能上「安装即自启」体验更好，但文档与实现不一致，且造成了 S1 的 bug。要么改代码（移除安装期写注册表，回归文档边界），要么改文档（承认安装器负责首装自启）+ 配合 S1 修值。推荐后者（体验更好），但二者必须对齐。

**Suggestion：** 选一种并统一 doc 与代码；若保留安装期写注册表，需在 `Package.md` 显式改写边界，并同步 S1 的 `--minimized`。

---

### 🟡 S4 — `release` job 重复编译（`make package` 重建 amd64/arm64），`build` job 产物未被复用

`ci.yml` 中 `release` `needs: [build]`（已产出双架构 exe artifact），却又执行 `make package`，而 `Makefile` 的 `package` 依赖 `build-amd64 build-arm64` → **在 release runner 上重新交叉编译一遍**。且 `build` job 上传的 artifact 在 `release` 中完全没被 `download`。

**Why：** 浪费构建时间（在 CGO/网络受限环境下尤其明显），且 release 的版本号来自 `make package` 内的 `git describe`，与 `build` job 可能不一致（同一 commit 下一致，但属脆弱耦合）。

**Suggestion：** `release` 直接 `download-artifact` 复用 `build` job 的 exe，仅跑 `scripts/package.sh`（打包 + nsis + sha256），不要重编译。

---

### 🟡 S5 — 缺年度节假日自动刷新调度，ADR-05「每年构建期烘焙」不会自动发生

`ci.yml` `on:` 仅 `push`/`pull_request`/`tag`，**无 `schedule:`**。但 `ci-build.sh` 的节假日刷新（`gh api`）是「构建期拉取」。即：除非每年手动打 tag 触发 release（或有人 push），否则不会刷新。

**Why：** `2026.json` 当前数据是真实的（2026-07-11 核对），但若不做年度调度，2027 年的数据不会自动更新，违背 ADR-05「每年构建期烘焙」。fail-soft 设计保证断网/无 gh 时回落到已提交数据，所以 v1.0 可发布；但「自动年度刷新」这一承诺未落地。

**Suggestion：** 加一个 `schedule: - cron: '0 0 1 1 *'`（或每季度）的 workflow，仅跑 `ci-build.sh` 的节假日刷新并发 PR。v1.1+ 再做亦可，但应在 `CI.md` 标注「年度刷新尚未自动化」。

---

## 4. 💭 Nits（非阻塞，建议收尾时处理）

- **N1 供应链固定：** `actions/checkout@v4`、`setup-go@v5`、`golangci-lint-action@v6`、`action-gh-release@v2`、`upload-artifact@v4` 均只钉大版本 tag，未钉完整 commit SHA。release 工具链建议钉 SHA（或启用 Dependabot）以防 supply-chain 漂移。
- **N2 最小权限：** `permissions: contents: write` 设在 workflow 顶层，对 lint/test/build 三 job 过宽；应顶层 `permissions: {}` + 仅 `release` job 开 `contents: write`。
- **N3 覆盖率：** `cmd/deskcalendar` 50% / `build/packaging` 46.4%（均 release 工具链，不计入核心 domain ≥60% 目标，可接受）；`packaging` 的 NSIS 成功路径因需外部 `makensis` 未覆盖（测试已 Skip 非 Windows + 覆盖错误路径，合理）。
- **N4 模块缓存：** `setup-go` 未开 `cache`/未指定 `cache-dependency-path`；B1 修复后建议加上，加速 CI。
- **N5 真机烟测进 CI：** 与 Phase 3 💭#1 一致——`win32` 真实窗口测试在无交互窗口站的 runner 会 `Skip`，建议 release 流水线配交互式 Windows runner 跑 `internal/platform/win32` 的 `*_windows_test.go`，否则 B 类窗口线程缺陷仍靠本地发现。

---

## 5. 👍 亮点 / 值得肯定

- **版本单一可信源**：`build/version.go` 的 `Version/Commit/BuildTime/TargetOS/TargetArch` + `Info()` + `CGOEnabled:false` 干净，且 `version_test.go` 钉死「零 CGO」契约。
- **叶子包纪律**：`build`/`build/packaging`/`cmd/packager` 经 `go list` 证实零业务反向依赖，构建期工具不与运行时耦合（ADR-07a 延续成立）。
- **打包层可测性好**：`PortablePackager`（纯 zip，100% 路径覆盖）+ `NSISPackager` 错误路径 + `findNSISScript` 环境覆盖 + `InstallConfig.resolve()` 校验，结构清晰；`ctx` 取消、绝对路径归一、退出码友好都到位。
- **节假日刷新 fail-soft**：`scripts/ci-build.sh` 的 `gh api` 拉取 + Python 转换 + 异常回退静态兜底，严格契合 ADR-05 离线优先，且**绝不盲覆盖**嵌入文件（注释明确警告盲覆盖会写坏构建）。
- **`.gitignore` 正确**：`/dist/`、`*.exe`、`vendor/`、`go.work*` 全忽略，仓库零二进制膨胀。
- **S4/S5（Phase 4 遗留）已闭环**：退出死锁根因修复（`quitCh` 独立可靠通道，非缓冲缓解）、节假日数据经验证为真实——见 §6。
- **Phase 4 红测试已修复**：`TestRun_ClickNavigatesAndSelects` 现绿，全量 `go test` 通过。

---

## 6. Phase 4 遗留闭环确认

| Phase 4 项 | 状态 | 证据 |
|---|---|---|
| 🔴 B1（点击集成测试红） | ✅ 已修 | `5004be2 feat(phase4): …修复代码审查 B1/S1/N1/S2`；`go test ./...` 全绿 |
| 🟡 S1（atomic onClick 单写者） | ✅ 已修 | `5004be2`（与 Phase 3 单写者同模式） |
| 🟡 S2（Animation.md doc 漂移） | ✅ 已修 | `5004be2` |
| 🟡 S3（wmDpiChanged 未刷新 `w.dpi`） | ✅ 已修 | `window_windows.go:422` `w.dpi = newDPI`（注释标 N1） |
| 🟡 S4（退出死锁根因） | ✅ 已修 | `5841516 fix: S4 退出死锁根因`（新增 `quitCh`） |
| 🟡 S5（节假日 SEED 真实性） | ✅ 已修 | `5841516` + `2026.json` 改写 `_comment` 为真实数据证明；`holiday_embed` 测试守住 |

→ Phase 4 全部发现（B1/S1–S5）已闭环，连续五轮 review→修复 闭环健康。

---

## 7. 发布门清单（建议合入即执行）

1. **🔴 B1**：消除 `go.mod` 本地绝对路径 replace（版本化 / vendor / 代理），并在干净容器验证 `go build`。
2. **🔴 B2**：CI `test` job 去掉 `-race`，修订 `CI.md` 去 `-race` 表述。
3. **🟡 S1+S3**：NSIS 自启值改 `exe --minimized`，并统一 `Package.md` 边界描述。
4. **🟡 S2/S4/S5**：doc 对齐（Build.md §9 去 `target.go`；release 复用 build 产物；年度刷新调度）。
5. 可选 N1–N5 收尾。

> 注：工程师提交信息已标注「CI 暂留本地，待推送」「推送待确认」。B1/B2 属于「推送前必须解决」，否则 `git push` + 打 tag 后 GitHub Actions 必然全红。另注意用户侧约束（见长期记忆）：推送 `.github/workflows/*.yml` 需 `workflow` scope 的 classic PAT（gh device-code 端点被墙），且与 SSH 22 端口被墙一致，走 HTTPS+gh 凭据。

---

## 8. 结论

**代码质量 A-，发布流水线 B（被 2 个 🔴 阻断）。** Phase 5 的「代码」部分（build/packaging/cmd 工具链）扎实、可测、依赖方向干净、零 CGO 守住；但「能自动发布」这一 Phase 5 的核心交付物**当前不成立**——本地 replace 与 `-race` 两处会让干净 runner 上的 CI 全红。修掉 B1/B2（半天内），再顺手清 S1（跨模块自启契约，用户可见），即可宣布 Phase 5 / v1.0 发布就绪。

需要的话，我可以直接动手：（a）把 `go.mod` 改为可发布的版本化 replace 并验证；（b）改 `ci.yml` 去 `-race` + `Package.md`/`Build.md` doc 对齐；（c）修 `installer.nsi` 的 `--minimized` 自启值。要现在做吗？
