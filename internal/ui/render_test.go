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
	grid := calendar.MonthGrid{Year: 2026, Month: time.July}
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
	grid := calendar.MonthGrid{Year: 2026, Month: time.July}
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
	m := NewMonthModel(calendar.MonthGrid{Year: 2026, Month: time.July}, true, true)
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
	grid := calendar.MonthGrid{Year: 2026, Month: time.July}
	// 全部置为今日 → 整片网格底色被 todayBlue 浅染。
	allToday := grid
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			allToday.Weeks[r][c] = calendar.Cell{
				Date:        time.Date(2026, 7, r*7+c+1, 0, 0, 0, 0, time.Local),
				InCurrentMonth: true,
				IsToday:     true,
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
