package calendar

import (
	"testing"
	"time"
)

func TestGenMonthGrid_StructureMondayStart(t *testing.T) {
	// 2026-07：7 月 1 日是周三；WeekStart=Monday → 首格应为 2026-06-29（周一）。
	opts := GridOptions{WeekStart: time.Monday, Today: time.Date(2026, 7, 9, 0, 0, 0, 0, time.Local)}
	grid := GenMonthGrid(2026, time.July, opts)

	if grid.FirstCell.Weekday() != time.Monday {
		t.Errorf("FirstCell weekday = %v, want Monday", grid.FirstCell.Weekday())
	}
	if grid.LastCell.Weekday() != time.Sunday {
		t.Errorf("LastCell weekday = %v, want Sunday", grid.LastCell.Weekday())
	}
	// 当月格数应等于 7 月天数 31。
	var inMonth int
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			if grid.Weeks[r][c].InCurrentMonth {
				inMonth++
			}
		}
	}
	if inMonth != 31 {
		t.Errorf("InCurrentMonth cells = %d, want 31", inMonth)
	}
	// 选中/今日高亮应正确落位。
	sel := time.Date(2026, 7, 24, 0, 0, 0, 0, time.Local)
	opts2 := opts
	opts2.Selected = sel
	grid2 := GenMonthGrid(2026, time.July, opts2)
	found := false
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			if grid2.Weeks[r][c].Date.Equal(sel) && grid2.Weeks[r][c].IsSelected {
				found = true
			}
		}
	}
	if !found {
		t.Error("selected date cell not marked IsSelected")
	}
	if !grid.Weeks[1][3].IsToday { // 2026-07-09 是周四，首格 06-29 周一(0,0) → 第 2 行第 4 列(索引1,3)
		t.Error("today cell (2026-07-09) not marked IsToday")
	}
}

func TestGenMonthGrid_WeekStartSunday(t *testing.T) {
	opts := GridOptions{WeekStart: time.Sunday}
	grid := GenMonthGrid(2026, time.July, opts)
	if grid.FirstCell.Weekday() != time.Sunday {
		t.Errorf("FirstCell weekday = %v, want Sunday", grid.FirstCell.Weekday())
	}
	if grid.LastCell.Weekday() != time.Saturday {
		t.Errorf("LastCell weekday = %v, want Saturday", grid.LastCell.Weekday())
	}
}

func TestGenMonthGrid_LeapFebruary(t *testing.T) {
	// 2024-02 闰年有 29 天。
	grid := GenMonthGrid(2024, time.February, GridOptions{WeekStart: time.Monday})
	var inMonth int
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			if grid.Weeks[r][c].InCurrentMonth {
				inMonth++
			}
		}
	}
	if inMonth != 29 {
		t.Errorf("leap Feb InCurrentMonth = %d, want 29", inMonth)
	}
}

func TestGenMonthGrid_CrossYear(t *testing.T) {
	// 2025-12：末格应跨入 2026-01。
	grid := GenMonthGrid(2025, time.December, GridOptions{WeekStart: time.Monday})
	crossed := false
	for r := 0; r < 6; r++ {
		for c := 0; c < 7; c++ {
			cell := grid.Weeks[r][c]
			if cell.Date.Year() == 2026 && cell.Date.Month() == time.January {
				if cell.InCurrentMonth {
					t.Error("Jan 2026 cell must not be InCurrentMonth for Dec 2025 grid")
				}
				crossed = true
			}
		}
	}
	if !crossed {
		t.Error("expected Dec 2025 grid to include Jan 2026 padding cells")
	}
}

func TestGenMonthGrid_AnnotationFromServices(t *testing.T) {
	// 验证 GridOptions 注入 Lunar/Holiday 后，每格正确填充（标注优先级在 UI 层，这里只验证数据到位）。
	termDate := time.Date(2026, 7, 24, 0, 0, 0, 0, time.Local) // 大暑（抽样，真实值由 lunar-go 切片验证）
	holiDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)  // 元旦
	lunar := &fakeLunar{byDate: map[string]LunarInfo{
		termDate.Format("2006-01-02"): {SolarTerm: "大暑", DayStr: "初十"},
	}}
	holiday := &fakeHoliday{
		holidays: map[string]bool{holiDate.Format("2006-01-02"): true},
		names:    map[string]string{holiDate.Format("2006-01-02"): "元旦"},
	}
	opts := GridOptions{WeekStart: time.Monday, Lunar: lunar, Holiday: holiday}
	grid := GenMonthGrid(2026, time.July, opts)

	gotTerm := grid.Weeks[3][4] // 2026-07-24 周四，首格 06-29 周一 → 第 4 行第 5 列(索引3,4)
	if !gotTerm.Date.Equal(termDate) {
		t.Fatalf("cell at (3,4) = %v, want %v", gotTerm.Date, termDate)
	}
	if gotTerm.Lunar.SolarTerm != "大暑" {
		t.Errorf("cell Lunar.SolarTerm = %q, want 大暑", gotTerm.Lunar.SolarTerm)
	}
	// 元旦不在 7 月网格，但 Holiday 注入应覆盖全网格（无值则空）。验证非节假日的格 Holiday 为空。
	if grid.Weeks[0][0].Holiday.Name != "" {
		t.Error("non-holiday cell should have empty Holiday.Name")
	}
}
