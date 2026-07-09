# DeskCalendar POC — 透明圆角验证

验证 **gogpu/ui 在打过 patch 的 gogpu 上能否输出每像素 alpha**，让圆角面板之外的区域透出桌面。
这是 360 小清新日历复刻的核心观感前提，也是架构评估报告里的最后一个技术风险点（ADR-03）。

---

## 1. 已应用的 Patch（位于 `D:\workspace\github\gogpu`）

本 POC 依赖对 gogpu 的两处修改（已应用到本地克隆，可用 `git -C D:\workspace\github\gogpu checkout -- .` 还原）。

### Patch A — swapchain 改为 Premultiplied alpha
`renderer.go`（约 311 行，`RenderTarget.configure`）：

```diff
 	return ws.surface.Configure(device, &wgpu.SurfaceConfiguration{
 		Format:      ws.format,
 		Usage:       gputypes.TextureUsageRenderAttachment,
 		Width:       ws.width,
 		Height:      ws.height,
-		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
+		AlphaMode:   gputypes.CompositeAlphaModePremultiplied,
 		PresentMode: presentMode,
 	})
```

### Patch B — 窗口加 `WS_EX_LAYERED | WS_EX_NOREDIRECTIONBITMAP`
`internal/platform/platform_windows.go`：

```diff
 	// Frameless window constants
 	wsPopup            = 0x80000000 // WS_POPUP
 	wsThickFrame       = 0x00040000 // WS_THICKFRAME (for resize in frameless)
 	wsCaption          = 0x00C00000 // WS_CAPTION (title bar)
+	// POC patch: enable per-pixel alpha compositing with the desktop.
+	wsExLayered             = 0x00080000 // WS_EX_LAYERED
+	wsExNoRedirectionBitmap = 0x00200000 // WS_EX_NOREDIRECTIONBITMAP
```

```diff
 	// Create window with pre-scaled outer dimensions.
 	hwnd, _, _ := procCreateWindowExW.Call(
-		0,
+		uintptr(wsExLayered | wsExNoRedirectionBitmap), // POC patch
 		uintptr(unsafe.Pointer(className)),
```

> 为什么是这两个：`CompositeAlphaModeOpaque` 把每像素 alpha 压成不透明（根因）；
> 没有 `WS_EX_LAYERED` 时 DWM 不会按 alpha 与桌面合成。两者叠加才具备透明窗口能力。
> 另：`WS_EX_NOREDIRECTIONBITMAP` 让 DWM 直接用 swapchain 合成、避免首帧黑闪；
> 若该标志导致窗口异常，可仅保留 `WS_EX_LAYERED`（见排错）。

---

## 2. 运行环境要求

- **Windows**（本 POC 强依赖 Win32 + WGPU）。
- **Go 1.25+**（gogpu/go.mod 要求 `go 1.25.0`）。
- **CGO + C 工具链**：wgpu 走 FFI，编译需 MinGW-w64 或 MSVC，并带 `wgpu-native`。
  这是 gogpu 的固有要求，与是否透明无关。
- 网络（首次需从模块代理拉取 `wgpu` / `gputypes` / `gg` 等传递依赖）。

---

## 3. 运行

```powershell
cd D:\workspace\aicoding\DeskCalendar\poc\transparent-window
$env:CGO_ENABLED = "1"
go run .
```

窗口弹出后观察结果（见下「判读标准」）。

---

## 4. 判读标准

| 现象 | 结论 |
|------|------|
| 蓝色圆角面板悬浮在桌面之上，面板四角外的区域能看到后面的桌面 | ✅ 透明圆角成立，ADR-03 通过 |
| 整个窗口是**实心黑/白矩形**（无透明） | ❌ patch 未生效或需微调（见排错） |
| 窗口完全不可见 / 一闪即逝 | ⚠️ 多为 `WS_EX_NOREDIRECTIONBITMAP` 在首帧前未呈现，见排错 |

> 机制说明：gogpu/ui 默认采用宿主托管式渲染模式，其 `DrawTo` **不**清不透明主题背景；
> 根 boundary 会用 `ThemeBackground()` 填整窗，但 `desktop.go` 的 `flushBoundaryToTexture`
> 用 `cc.SetRGBA(..., bg.A)` 尊重 alpha。因此把 `th.Colors.Background` 设为
> `RGBA8(0,0,0,0)` 后，根填充即为透明，圆角面板之外自然透出桌面。

---

## 5. 排错

- **白/黑实心矩形**：先确认 patch 已应用到本地 gogpu 且 `go run` 确实用了本地副本
  （`go list -m github.com/gogpu/gogpu` 应指向 `D:\workspace\github\gogpu`）。
  若仍不透明，检查 `th.Colors.Background` 是否真的被设为透明（打印 `th.Colors.Background` 确认）。
- **改为仅 `WS_EX_LAYERED`**：编辑 `platform_windows.go`，把
  `wsExLayered | wsExNoRedirectionBitmap` 改为 `wsExLayered`，重跑。
  某些驱动/Win 版本对 `WS_EX_NOREDIRECTIONBITMAP` 支持不一致。
- **窗口不可见**：同上，去掉 `WS_EX_NOREDIRECTIONBITMAP`；并确保渲染循环在窗口显示后
  立即呈现首帧（gogpu 默认每帧呈现，通常无需额外处理）。
- **编译报 CGO / wgpu 相关错误**：确认 C 工具链与 `wgpu-native` 可用，并设置 `CGO_ENABLED=1`。

---

## 6. 还原 Patch

```powershell
git -C D:\workspace\github\gogpu checkout -- .
```

---

## 7. 对正式架构的含义

- 若 POC 通过：ADR-03 关闭，`Shell` 层用 gogpu/ui 创建 frameless 透明窗口即可，
  「小清新」圆角面板 + 桌面透出可直接落地；Mica 毛玻璃非必需（自绘渐变圆角即可高度还原）。
- 若 POC 失败：需降级方案 —— 不透明窗口 + 内部圆角面板（视觉接近，但窗体外缘是矩形），
  或改用 DComposition 路径（复杂度显著上升）。该结论会反向修订评估报告的 ADR-03。
