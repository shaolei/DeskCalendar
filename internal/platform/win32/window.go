// Package win32 实现 DeskCalendar 的窗口层（路径 D：自拥普通弹窗）。
//
// 设计原则（ADR-08 / Phase3-重排计划 §3.2）：纯 Go·零 CGO，仅依赖
// golang.org/x/sys/windows（LazyDLL）与项目内 platform 包（DPI / 多屏锚定）。
// 窗口为普通 WS_POPUP + WS_EX_TOPMOST 弹窗，经 DIBSection + WM_PAINT/BitBlt
// 推送 gg（internal/ui）产出的像素——比 POC 的 layered 窗口更简单，无
// premultiplied-alpha 细节坑。窗口固定尺寸、初次锚定托盘上方后即不再移动/缩放。
//
// 铁律（ADR-02）：Show/Hide/AnchorAboveTray 仅允许在主线程（窗口线程）执行；
// 托盘 goroutine 永不直调窗口，必须经 channel 命令交由窗口线程消费。
package win32

import (
	"image"
)

// WindowController 窗口操作接口（主线程安全）。
// 业务/状态机（shell.Lifecycle、app）只依赖此接口，便于单测用 fake 替换。
type WindowController interface {
	// Show 显示窗口（若隐藏则弹出于上次锚定位置）。
	Show()
	// Hide 隐藏窗口（失焦/Esc/关闭均走此路径，而非销毁）。
	Hide()
	// AnchorAboveTray 将窗口锚定到托盘图标正上方居中（初次定位即固定）。
	// rect 为托盘图标的屏幕坐标矩形（物理像素），通常来自 tray.Bounds()。
	AnchorAboveTray(rect image.Rectangle)
	// Visible 返回当前可见状态（由窗口线程维护，供 Lifecycle 决策 toggle 方向）。
	Visible() bool
	// Present 推送最新像素缓冲（straight RGBA）并触发重绘。
	// 由 90-UI 渲染层 internal/ui.Render 每帧调用。
	//
	// 所有权契约（💭#2）：调用方在 Present 返回后、下次 Present 之前，不得复用、
	// 修改或释放该 *image.RGBA——窗口线程会持有其指针（lastBmp）用于 DPI 变化时
	// 重绘，直至下一次 Present 覆盖。ui.Render 每次返回全新缓冲，满足此契约。
	Present(bmp *image.RGBA)
	// Quit 请求窗口退出其消息泵 goroutine（销毁窗口 + 释放 GDI）。调用会阻塞至
	// 该 goroutine 完全退出，确保 quit 路径无 goroutine 泄漏（代码审查 N1）。
	Quit()
}

// Options 构造窗口的选项。
type Options struct {
	// Width/Height 逻辑设计尺寸（96 DPI 基准，如 360×480）。
	// 实际物理像素由 DPI 缩放得出。
	Width  int
	Height int
	// Margin 锚定到托盘上方时的留白（物理像素）。
	Margin int
}

// NewWindow 构造默认实现。Windows 下为自拥普通弹窗；
// 非 Windows（CI / 测试）回退为内存 fake，保证跨平台可编译与单测。
func NewWindow(opts Options) WindowController {
	return newNativeWindow(opts)
}

// fakeWindow 测试/非 Windows 下的内存实现，记录调用以便断言。
type fakeWindow struct {
	visible    bool
	anchorRect image.Rectangle
	presents   []*image.RGBA
	showCalls  int
	hideCalls  int
}

func (w *fakeWindow) Show()                             { w.showCalls++; w.visible = true }
func (w *fakeWindow) Hide()                             { w.hideCalls++; w.visible = false }
func (w *fakeWindow) Visible() bool                    { return w.visible }
func (w *fakeWindow) AnchorAboveTray(r image.Rectangle) { w.anchorRect = r }
func (w *fakeWindow) Present(b *image.RGBA)             { w.presents = append(w.presents, b) }
func (w *fakeWindow) Quit()                             {}

// compile-time 接口满足性校验（不实例化，避免测试期副作用）。
var _ WindowController = (*fakeWindow)(nil)

// blitScaled 将 src（straight RGBA）经最近邻缩放写入 DIB 的 BGRA 位（bits）。
// bits 长度须 ≥ dibW*dibH*4。GDI DIB 字节序为 BGRA，故 R/B 互换；alpha 对非分层
// 普通窗口被 BitBlt 忽略，原样拷贝。纯函数，易单测。
func blitScaled(bits []byte, dibW, dibH int, src *image.RGBA) {
	if src == nil || len(bits) < dibW*dibH*4 {
		return
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return
	}
	for y := 0; y < dibH; y++ {
		sy := y * sh / dibH
		if sy >= sh {
			sy = sh - 1
		}
		dbase := y * dibW * 4
		sbase := (sy-b.Min.Y)*src.Stride + b.Min.X*4
		for x := 0; x < dibW; x++ {
			sx := x * sw / dibW
			if sx >= sw {
				sx = sw - 1
			}
			s := sbase + sx*4
			d := dbase + x*4
			bits[d+0] = src.Pix[s+2] // B
			bits[d+1] = src.Pix[s+1] // G
			bits[d+2] = src.Pix[s+0] // R
			bits[d+3] = src.Pix[s+3] // A
		}
	}
}
