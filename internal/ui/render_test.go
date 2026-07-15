package ui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/shaolei/DeskCalendar/internal/calendar"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// testTheme 返回一个已知背景色的主题，便于断言像素。
func testTheme() *theme.Theme {
	return &theme.Theme{
		Name:    "test",
		Scheme:  theme.SchemeLight,
		Palette: testPalette(),
		Alpha:   1,
	}
}

func testPalette() theme.ColorPalette {
	return theme.ColorPalette{
		Background: color.RGBA{247, 247, 247, 255},
		Surface:    color.RGBA{255, 255, 255, 255},
		Foreground: color.RGBA{26, 26, 26, 255},
		Muted:      color.RGBA{154, 160, 166, 255},
		Accent:     color.RGBA{45, 127, 249, 255},
		HolidayRed: color.RGBA{229, 57, 53, 255},
		TodayBlue:  color.RGBA{45, 127, 249, 255},
		Border:     color.RGBA{224, 224, 224, 255},
	}
}

// sample 取像素 RGBA（忽略可能的边角越界，调用方保证在范围内）。
func sample(img *image.RGBA, x, y int) color.RGBA {
	i := img.PixOffset(x, y)
	return color.RGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
}

func TestNewMonthModel_MapsFields(t *testing.T) {
	grid := calendar.MonthGrid{Year: 2026, Month: time.July, WeekStart: time.Monday}
	// 周一为列首：2026-07-01 是周三 → 落在第 0 行第 3 列。
	grid.Weeks[0][3] = calendar.Cell{
		Date:           time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local),
		InCurrentMonth: true,
		IsToday:        true,
		IsSelected:     true,
		Lunar:          calendar.LunarInfo{DayStr: "初一"},
		Holiday:        calendar.HolidayInfo{Name: "建党节", IsHoliday: true},
	}
	// 农历优先级：节气 > 节日 > 日。
	grid.Weeks[1][0] = calendar.Cell{
		Date:  time.Date(2026, 7, 6, 0, 0, 0, 0, time.Local),
		Lunar: calendar.LunarInfo{SolarTerm: "小暑", Festival: "端午", DayStr: "十二"},
	}

	m := NewMonthModel(grid, true, true)

	if m.MonthLabel != "2026年7月" {
		t.Errorf("MonthLabel = %q, want 2026年7月", m.MonthLabel)
	}
	c := m.Weeks[0][3]
	if !c.IsToday || !c.IsSelected || !c.InMonth {
		t.Errorf("cell(0,3) flags = today=%v sel=%v inMonth=%v, want all true", c.IsToday, c.IsSelected, c.InMonth)
	}
	if c.Day != 1 {
		t.Errorf("cell(0,3).Day = %d, want 1", c.Day)
	}
	if c.Lunar != "初一" {
		t.Errorf("cell(0,3).Lunar = %q, want 初一", c.Lunar)
	}
	if !c.IsHoliday || c.Holiday != "建党节" {
		t.Errorf("cell(0,3) holiday = %q (isHoliday=%v), want 建党节/true", c.Holiday, c.IsHoliday)
	}
	// 节气优先于节日/日。
	if m.Weeks[1][0].Lunar != "小暑" {
		t.Errorf("cell(1,0).Lunar = %q, want 小暑 (节气优先级)", m.Weeks[1][0].Lunar)
	}
}

func TestNewMonthModel_HonorsDisplayFlags(t *testing.T) {
	grid := calendar.MonthGrid{Year: 2026, Month: time.July, WeekStart: time.Monday}
	grid.Weeks[0][0] = calendar.Cell{
		Date:    time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local),
		Lunar:   calendar.LunarInfo{DayStr: "初一"},
		Holiday: calendar.HolidayInfo{Name: "元旦", IsHoliday: true},
	}

	off := NewMonthModel(grid, false, false)
	if off.Weeks[0][0].Lunar != "" || off.Weeks[0][0].Holiday != "" || off.Weeks[0][0].IsHoliday {
		t.Errorf("with flags off, lunar/holiday should be empty: %+v", off.Weeks[0][0])
	}

	on := NewMonthModel(grid, true, true)
	if on.Weeks[0][0].Lunar != "初一" || on.Weeks[0][0].Holiday != "元旦" || !on.Weeks[0][0].IsHoliday {
		t.Errorf("with flags on, lunar/holiday should be populated: %+v", on.Weeks[0][0])
	}
}

func TestRender_DimensionsOpaqueAndBackground(t *testing.T) {
	m := NewMonthModel(calendar.MonthGrid{Year: 2026, Month: time.July, WeekStart: time.Monday}, true, true)
	img := Render(m, RenderOptions{Width: 360, Height: 480}, testTheme())

	if img.Bounds() != image.Rect(0, 0, 360, 480) {
		t.Fatalf("bounds = %v, want 0,0,360,480", img.Bounds())
	}
	// 整图必须不透明（普通弹窗 BitBlt 忽略 alpha）。
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.Pix[img.PixOffset(x, y)+3] != 255 {
				t.Fatalf("pixel (%d,%d) alpha = %d, want 255 (must be opaque)", x, y, img.Pix[img.PixOffset(x, y)+3])
			}
		}
	}
	// 左上角背景像素应为主题背景色。
	bg := testPalette().Background
	if got := sample(img, 3, 3); got.R != bg.R || got.G != bg.G || got.B != bg.B {
		t.Errorf("corner pixel = %v, want background %v", got, bg)
	}
}

func TestRender_TodayTintAltersGridPixel(t *testing.T) {
	grid := calendar.MonthGrid{Year: 2026, Month: time.July, WeekStart: time.Monday}
	// 全部置为今日 → 整片网格底色被 todayBlue 浅染。
	allToday := grid
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			allToday.Weeks[r][c] = calendar.Cell{
				Date:           time.Date(2026, 7, r*7+c+1, 0, 0, 0, 0, time.Local),
				InCurrentMonth: true,
				IsToday:        true,
			}
		}
	}
	noneToday := grid // 无今日标记

	imgToday := Render(NewMonthModel(allToday, true, true), RenderOptions{Width: 360, Height: 480}, testTheme())
	imgNone := Render(NewMonthModel(noneToday, true, true), RenderOptions{Width: 360, Height: 480}, testTheme())

	// 网格中部某点：全今日应被浅蓝染色，无今日则为纯背景。
	const px, py = 180, 315
	withToday := sample(imgToday, px, py)
	without := sample(imgNone, px, py)
	if withToday == without {
		t.Errorf("today tint did not change pixel at (%d,%d): both %v", px, py, withToday)
	}
	if without != testPalette().Background {
		t.Errorf("pixel without today = %v, want plain background %v", without, testPalette().Background)
	}
}

func TestComputeLayout(t *testing.T) {
	lay := computeLayout(360, 480)
	if lay.headerH != 56 || lay.weekH != 28 {
		t.Errorf("headerH/weekH = %v/%v, want 56/28", lay.headerH, lay.weekH)
	}
	if got := lay.colW * 7; abs(got-360) > 1e-6 {
		t.Errorf("colW*7 = %v, want 360", got)
	}
	if got := lay.gridTop + lay.rowH*6; abs(got-480) > 1e-6 {
		t.Errorf("gridTop+rowH*6 = %v, want 480", got)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// TestRender_WeatherBandReservesTopRegion 验证含天气卡片时顶部预留天气带
// （Surface 底色，与背景区分），且整体尺寸/不透明不变（#149）。
func TestRender_WeatherBandReservesTopRegion(t *testing.T) {
	grid := calendar.MonthGrid{Year: 2026, Month: time.July, WeekStart: time.Monday}
	base := NewMonthModel(grid, true, true)

	with := base
	with.Weather = &WeatherCard{
		Status:  WeatherReady,
		Current: &WeatherItem{TempC: 28, ConditionText: "晴", Icon: "晴"},
		Forecast: []*WeatherItem{
			{TempC: 28, LowC: 20, ConditionText: "晴", Icon: "晴"},
			{TempC: 30, LowC: 22, ConditionText: "多云", Icon: "云"},
			{TempC: 31, LowC: 23, ConditionText: "雨", Icon: "雨"},
		},
	}

	imgWith := Render(with, RenderOptions{Width: 360, Height: 480, WeatherBandH: 64}, testTheme())
	imgNone := Render(base, RenderOptions{Width: 360, Height: 480}, testTheme())

	if imgWith.Bounds() != image.Rect(0, 0, 360, 480) {
		t.Fatalf("bounds = %v, want 0,0,360,480", imgWith.Bounds())
	}
	// 整图仍不透明。
	b := imgWith.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if imgWith.Pix[imgWith.PixOffset(x, y)+3] != 255 {
				t.Fatalf("pixel (%d,%d) alpha = %d, want 255", x, y, imgWith.Pix[imgWith.PixOffset(x, y)+3])
			}
		}
	}
	// 天气带区域（y=30 < 64）应为 Surface（白），而非背景（247）。
	surf := testPalette().Surface
	if got := sample(imgWith, 180, 30); got.R != surf.R || got.G != surf.G || got.B != surf.B {
		t.Errorf("weather band pixel = %v, want Surface %v", got, surf)
	}
	// 无天气时同位置为背景色。
	bg := testPalette().Background
	if got := sample(imgNone, 180, 30); got.R != bg.R || got.G != bg.G || got.B != bg.B {
		t.Errorf("no-weather pixel = %v, want Background %v", got, bg)
	}
	// 日历区下方仍渲染（y=400 应为背景，未因天气带崩溃）。
	if got := sample(imgWith, 180, 400); got.R != bg.R || got.G != bg.G || got.B != bg.B {
		t.Errorf("calendar area pixel = %v, want Background %v", got, bg)
	}
}

// TestRender_ScaleProducesPhysicalBitmap 验证 #41：Scale>0 时 Render 产出物理
// 像素位图（逻辑尺寸 × Scale），且整图不透明、背景填充覆盖全画布（无因缩放导致
// 的透明条带/空洞）；逻辑坐标语义（HitTest）不受 Scale 影响。
func TestRender_ScaleProducesPhysicalBitmap(t *testing.T) {
	m := NewMonthModel(calendar.MonthGrid{Year: 2026, Month: time.July, WeekStart: time.Monday}, true, true)
	// Scale=1.5 → 物理尺寸 round(360*1.5) × round(480*1.5) = 540 × 720。
	img := Render(m, RenderOptions{Width: 360, Height: 480, Scale: 1.5}, testTheme())

	if got := img.Bounds(); got != image.Rect(0, 0, 540, 720) {
		t.Fatalf("scaled bounds = %v, want 0,0,540,720", got)
	}
	// 整图必须不透明（普通弹窗 BitBlt 忽略 alpha）。
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.Pix[img.PixOffset(x, y)+3] != 255 {
				t.Fatalf("pixel (%d,%d) alpha = %d, want 255", x, y, img.Pix[img.PixOffset(x, y)+3])
			}
		}
	}
	// 背景填充覆盖全画布：逻辑 (3,3) → 物理 (4.5,4.5) 附近应为背景色；底部逻辑
	// (359,479) → 物理 (~539,719) 也应为背景/内容（无透明条带），证明确实画满。
	bg := testPalette().Background
	if got := sample(img, 5, 5); got.R != bg.R || got.G != bg.G || got.B != bg.B {
		t.Errorf("scaled top-left pixel = %v, want background %v", got, bg)
	}
	if got := sample(img, 539, 719); got.A != 255 {
		t.Errorf("scaled bottom-right pixel alpha = %d, want 255 (canvas filled)", got.A)
	}
	// 逻辑坐标语义不变：Scale 字段不参与 HitTest，命中测试仍按 96-DPI 逻辑坐标工作。
	opts := RenderOptions{Width: 360, Height: 480, Scale: 1.5}
	if got := HitTest(282, 28, opts).Kind; got != HitPrevMonth {
		t.Errorf("with Scale set, HitTest(282,28) Kind = %v, want HitPrevMonth", got)
	}
}

// TestHitTest_WithWeatherBand 验证点击坐标在天气带偏移下仍正确命中（#149）。
func TestHitTest_WithWeatherBand(t *testing.T) {
	opts := RenderOptions{Width: 360, Height: 480, WeatherBandH: 64}
	// 天气带内（y=10）不命中。
	if got := HitTest(180, 10, opts).Kind; got != HitNone {
		t.Errorf("in-band click Kind = %v, want HitNone", got)
	}
	// 日历头部导航按钮：相对日历区 y=28 → 面板 y = 64+28 = 92。
	if got := HitTest(282, 92, opts).Kind; got != HitPrevMonth {
		t.Errorf("prev button Kind = %v, want HitPrevMonth", got)
	}
	// 网格格：日历区 row=2,col=3 在 calendar-y=249 → 面板 y = 64+249 = 313。
	res := HitTest(180, 313, opts)
	if res.Kind != HitCell || res.Row != 2 || res.Col != 3 {
		t.Errorf("cell = (%v,%d,%d), want (HitCell,2,3)", res.Kind, res.Row, res.Col)
	}
}

// TestNewMonthModel_HeaderFollowsWeekStart 验证表头与网格列对齐（S2 核心不变量）：
// 表头第 0 列必须 == 周首星期；且每一列表头必须 == 该列首行格的公历星期。
// 遍历周日/周一/周六三种周首，确保旋转逻辑对任意 WeekStart 成立。
func TestNewMonthModel_HeaderFollowsWeekStart(t *testing.T) {
	for _, ws := range []time.Weekday{time.Sunday, time.Monday, time.Saturday} {
		grid := calendar.GenMonthGrid(2026, time.July, calendar.GridOptions{
			WeekStart: ws,
			Today:     time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local),
			Selected:  time.Date(2026, 7, 10, 0, 0, 0, 0, time.Local),
		})
		m := NewMonthModel(grid, true, true)

		if got := m.Weekdays[0]; got != WeekdayLabels[int(ws)] {
			t.Errorf("WeekStart=%v: Weekdays[0]=%q, want %q", ws, got, WeekdayLabels[int(ws)])
		}
		for i := 0; i < 7; i++ {
			colWeekday := grid.Weeks[0][i].Date.Weekday()
			want := WeekdayLabels[int(colWeekday)]
			if m.Weekdays[i] != want {
				t.Errorf("WeekStart=%v col%d: header=%q, want %q (cell weekday=%v)",
					ws, i, m.Weekdays[i], want, colWeekday)
			}
		}
	}
}
