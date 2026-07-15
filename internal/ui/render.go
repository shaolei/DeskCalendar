package ui

import (
	"image"
	"image/color"

	"github.com/gogpu/gg"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// 默认逻辑设计尺寸（96 DPI 基准）；窗口固定尺寸，DPI 缩放由 win32 在 Present 时处理。
const (
	defaultWidth  = 360
	defaultHeight = 480
	// DefaultWeatherBandH 顶部天气带默认高度（逻辑像素）；app 经 RenderOptions
	// 传入以保持 Render 与 HitTest 共用同一偏移（#149）。
	DefaultWeatherBandH = 64
)

// RenderOptions 渲染参数（逻辑设计尺寸）。
type RenderOptions struct {
	Width  int // 逻辑宽（如 360）；≤0 → 360
	Height int // 逻辑高（如 480）；≤0 → 480
	// WeatherBandH 顶部天气带高度；>0 时日历区整体下移该高度（#149）。
	// 与 HitTest 共用，确保点击坐标与绘制对齐。0 表示无天气带。
	WeatherBandH int
}

// View 是可挂载到面板的子视图（MVP 仅 CalendarView）。
// Draw 在给定矩形内绘制；OnShow/OnHide 在面板显隐时回调（MVP 多为空操作）。
type View interface {
	Draw(dc *gg.Context, rect image.Rectangle, m Model, th *theme.Theme)
	OnShow()
	OnHide()
}

// Render 用 gg 将完整面板光栅化为实心不透明 *image.RGBA（路径 D MVP：方角不透明）。
// 内容交由各子视图 Draw 组合（MVP 仅 CalendarView）。返回的缓冲 alpha 全部置 255，
// 与 win32 普通弹窗（DIBSection + BitBlt 忽略 alpha）兼容。
func Render(m Model, opts RenderOptions, th *theme.Theme) *image.RGBA {
	w, h := opts.Width, opts.Height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	// 自建上下文（gg 内部 Pixmap 持有真实像素缓冲）；绘制后经 dc.Image() 取回
	// 其 *image.RGBA。注意：不可回填传入的 image.NewRGBA——NewContextForImage 会
	// 把图像拷入独立 Pixmap，原始 img 不会被写入（实测全黑）。
	dc := gg.NewContext(w, h)

	// 背景：实心不透明面板（MVP 无圆角/阴影，后续 DwmSetWindowAttribute 白嫖）。
	dc.SetColor(opaque(th.Palette.Background))
	dc.DrawRectangle(0, 0, float64(w), float64(h))
	_ = dc.Fill()

	// 天气带：若 Model 含天气卡片，在顶部预留并绘制；日历区整体下移（#149）。
	bandH := 0
	if m.Weather != nil {
		bandH = opts.WeatherBandH
		if bandH <= 0 {
			bandH = DefaultWeatherBandH
		}
		var wv WeatherView
		wv.Draw(dc, image.Rect(0, 0, w, bandH), m, th)
	}

	// 子视图组合（MVP 仅日历；天气带之上、日历之下）。
	var cv CalendarView
	cv.Draw(dc, image.Rect(0, bandH, w, h), m, th)

	// 取回 gg 写入的 *image.RGBA（Pixmap.ToImage 保证返回 *image.RGBA）。
	img, ok := dc.Image().(*image.RGBA)
	if !ok {
		b := dc.Image().Bounds()
		img = image.NewRGBA(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				img.Set(x, y, dc.Image().At(x, y))
			}
		}
	}

	// 强制整图不透明：普通弹窗的 BitBlt 忽略 alpha，此步保证像素全不透明、
	// 与 blitScaled 的 straight-RGBA 假设一致（gg 写入的是 straight alpha，无歧义）。
	flattenAlpha(img)
	return img
}

// opaque 将带 alpha 的主题色转为全不透明（MVP 窗口不透明）。
func opaque(c color.RGBA) color.RGBA {
	c.A = 255
	return c
}

// withAlpha 返回给定色但指定 alpha（0..255），用于同帧内半透明叠绘（如今日底色）。
// 最终整图会被 flattenAlpha 置为不透明，故此处 alpha 仅影响同帧相对混合。
func withAlpha(c color.RGBA, a uint8) color.RGBA {
	c.A = a
	return c
}

// flattenAlpha 将整张缓冲 alpha 置 255。
func flattenAlpha(img *image.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.Pix[img.PixOffset(x, y)+3] = 255
		}
	}
}
