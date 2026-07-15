package ui

import (
	"image"
	"image/color"

	"github.com/gogpu/gg"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// 默认逻辑设计尺寸（96 DPI 基准）；窗口固定尺寸，DPI 缩放由 Render(Scale) 在
// 光栅化期处理（#41）：Render 按物理像素创建 gg 上下文并 dc.Scale(Scale,Scale)，
// 使面板在高 DPI 下清晰；win32 窗口/DIB 尺寸仍为物理像素，blitScaled 退化为 1:1。
// app 经 ui.DefaultWidth 反算 Scale = 物理宽 / DefaultWidth（见 app.go）。
const (
	DefaultWidth  = 360
	DefaultHeight = 480
	// DefaultWeatherBandH 顶部天气带默认高度（逻辑像素）；app 经 RenderOptions
	// 传入以保持 Render 与 HitTest 共用同一偏移（#149）。
	DefaultWeatherBandH = 64
	// DefaultTabStripH 待办 Tab 条默认高度（逻辑像素）；仅在注入待办服务
	// （opts.Todo != nil）时由 app 传入 >0，否则布局与旧版完全一致（向后兼容）。
	DefaultTabStripH = 36
)

// RenderOptions 渲染参数（逻辑设计尺寸）。
type RenderOptions struct {
	Width  int // 逻辑宽（如 360）；≤0 → 360
	Height int // 逻辑高（如 480）；≤0 → 480
	// WeatherBandH 顶部天气带高度；>0 时日历区整体下移该高度（#149）。
	// 与 HitTest 共用，确保点击坐标与绘制对齐。0 表示无天气带。
	WeatherBandH int
	// TabStripH 待办/日历切换 Tab 条高度；>0 时内容区再下移该高度并在顶部绘制
	// 两个 Tab（#148）。仅当注入待办服务时由 app 设为 DefaultTabStripH；0 表示
	// 无 Tab 条（旧版布局，完全向后兼容）。
	TabStripH int
	// ViewMode 当前视图（#148）；HitTest 据此把内容区派发到日历或待办命中逻辑。
	// 必须与 Render 读取的 Model.ViewMode 保持一致（由 app 同步设置）。
	ViewMode ViewMode
	// TodoCount 当前待办条数（#148）；HitTest 判定待办行命中时用，避免越界。
	// 与 Model.Todos 长度一致（由 app 同步设置）。
	TodoCount int
	// Scale 渲染缩放比 = 物理 DPI / 96（#41）。>0 时 Render 按物理尺寸
	// （round(Width*Scale) × round(Height*Scale)）创建 gg 上下文并 dc.Scale(Scale,Scale)，
	// 之后所有 Draw/HitTest 仍用逻辑坐标（96-DPI 基准），从而在高 DPI 下产出清晰像素，
	// 且窗口 DIB 与位图 1:1（blitScaled 退化为无缩放直拷）。≤0 或 0 表示 1.0（逻辑分辨率）。
	// 注意：HitTest 永远在逻辑坐标工作，不感知 Scale；app 计算 Scale 与 Render 共用同
	// 一逻辑宽，确保几何一致（画在哪、点哪对齐）。
	Scale float64
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
		w = DefaultWidth
	}
	if h <= 0 {
		h = DefaultHeight
	}
	// 物理缩放比：>0 时按物理像素创建上下文（#41 高 DPI 清晰）；≤0 退化为 1.0。
	scale := opts.Scale
	if scale <= 0 {
		scale = 1
	}
	// 物理像素尺寸（与 win32 DIB 同尺寸，blitScaled 退化为 1:1 直拷，无缩放模糊）。
	// round 与 win32.scaleLogical 一致（半值进位）；app 侧 Scale 由 物理宽/逻辑宽 反算，
	// 此处 round(逻辑*Scale)≈物理宽，确保位图与 DIB 恰好 1:1。
	pw := int(float64(w)*scale + 0.5)
	ph := int(float64(h)*scale + 0.5)
	if pw <= 0 {
		pw = w
	}
	if ph <= 0 {
		ph = h
	}
	// 自建上下文（gg 内部 Pixmap 持有真实像素缓冲）；绘制后经 dc.Image() 取回
	// 其 *image.RGBA。注意：不可回填传入的 image.NewRGBA——NewContextForImage 会
	// 把图像拷入独立 Pixmap，原始 img 不会被写入（实测全黑）。
	dc := gg.NewContext(pw, ph)
	// CTM 缩放：之后所有 Draw 调用以逻辑坐标（96-DPI 基准）表达，gg 自动映射
	// 到物理像素（含文字——gg#2434 后 DrawString 尊重完整 CTM，按 deviceSize=
	// faceSize*Scale 栅格化，故高 DPI 下文字同样清晰）。
	dc.Scale(scale, scale)

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

	// Tab 条（#148）：仅当 TabStripH>0（注入待办服务）时绘制，内容区再下移。
	tabH := opts.TabStripH
	if tabH < 0 {
		tabH = 0
	}
	if tabH > 0 {
		drawTabStrip(dc, image.Rect(0, bandH, w, bandH+tabH), m, th)
	}

	// 子视图组合：内容区自天气带+Tab 条之下开始；按 ViewMode 派发到
	// CalendarView 或 TodoView（#148）。
	contentTop := bandH + tabH
	if m.ViewMode == ViewTodo {
		var tdv TodoView
		tdv.Draw(dc, image.Rect(0, contentTop, w, h), m, th)
	} else {
		var cv CalendarView
		cv.Draw(dc, image.Rect(0, contentTop, w, h), m, th)
	}

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

// drawTabStrip 在 rect（宽=面板宽、高=TabStripH）内绘制「日历 | 待办」两个 Tab，
// 当前 ViewMode 对应的 Tab 以 Surface 底色 + Accent 下划线高亮（#148）。
// rect 由 Render 填好不透明背景，本函数仅覆写该条。
func drawTabStrip(dc *gg.Context, rect image.Rectangle, m Model, th *theme.Theme) {
	w := float64(rect.Dx())
	x0, y0 := float64(rect.Min.X), float64(rect.Min.Y)
	tabH := float64(rect.Dy())

	// Tab 条底色（与天气带一致的 Surface，和日历内容区分）。
	dc.SetColor(opaque(th.Palette.Surface))
	dc.DrawRectangle(x0, y0, w, tabH)
	_ = dc.Fill()

	// 底部分隔线。
	dc.SetColor(opaque(th.Palette.Border))
	dc.SetLineWidth(1)
	dc.DrawLine(x0, y0+tabH-0.5, x0+w, y0+tabH-0.5)
	_ = dc.Stroke()

	labels := []struct {
		text string
		mode ViewMode
	}{
		{"日历", ViewCalendar},
		{"待办", ViewTodo},
	}
	half := w / 2.0
	for i, tab := range labels {
		tx := x0 + half*float64(i) + half/2.0
		active := m.ViewMode == tab.mode
		col := th.Palette.Muted
		if active {
			col = th.Palette.Accent
		}
		applyFont(dc, 14)
		dc.SetColor(opaque(col))
		dc.DrawStringAnchored(tab.text, tx, y0+tabH/2, 0.5, 0.5)
		// 激活 Tab 底部 Accent 短下划线。
		if active {
			dc.SetColor(opaque(th.Palette.Accent))
			dc.SetLineWidth(2)
			dc.DrawLine(tx-14, y0+tabH-2, tx+14, y0+tabH-2)
			_ = dc.Stroke()
		}
	}
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
