package calendar

import (
	"context"
	"testing"
	"time"
)

// TestEmbedHolidayRepository_Loads 验证内嵌种子数据可被构造并查询。
func TestEmbedHolidayRepository_Loads(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	if !repo.IsHoliday(d) {
		t.Error("IsHoliday(2026-01-01) = false, want true (元旦)")
	}
	if name := repo.Name(d); name != "元旦" {
		t.Errorf("Name(2026-01-01) = %q, want 元旦", name)
	}
}

// TestEmbedHolidayRepository_Workday 验证调休补班日识别（且不误判为节假日）。
func TestEmbedHolidayRepository_Workday(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	d := time.Date(2026, 2, 14, 0, 0, 0, 0, time.Local) // 春节补班（种子）
	if !repo.IsWorkday(d) {
		t.Error("IsWorkday(2026-02-14) = false, want true (春节补班)")
	}
	if repo.IsHoliday(d) {
		t.Error("IsWorkday day 2026-02-14 should NOT be IsHoliday")
	}
	if name := repo.Name(d); name != "春节补班" {
		t.Errorf("Name(2026-02-14) = %q, want 春节补班", name)
	}
}

// TestEmbedHolidayRepository_NonHoliday 验证普通日既非节假日也非补班。
func TestEmbedHolidayRepository_NonHoliday(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	d := time.Date(2026, 3, 10, 0, 0, 0, 0, time.Local)
	if repo.IsHoliday(d) {
		t.Error("IsHoliday(2026-03-10) = true, want false")
	}
	if repo.IsWorkday(d) {
		t.Error("IsWorkday(2026-03-10) = true, want false")
	}
	if name := repo.Name(d); name != "" {
		t.Errorf("Name(2026-03-10) = %q, want empty", name)
	}
}

// TestEmbedHolidayRepository_Refresh 验证 Refresh 重放内嵌数据不报错。
func TestEmbedHolidayRepository_Refresh(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	if err := repo.Refresh(context.Background()); err != nil {
		t.Errorf("Refresh = %v, want nil", err)
	}
	// 刷新后数据仍在（真实 holiday-cn 名称：国庆节）。
	d := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	if !repo.IsHoliday(d) || repo.Name(d) != "国庆节" {
		t.Errorf("after Refresh: 2026-10-01 holiday=%v name=%q, want 国庆节", repo.IsHoliday(d), repo.Name(d))
	}
}

// TestJoinDate_RoundTrip 验证 joinDate 拒绝非法日历日期（如 02-30），接受合法日期（S5）。
func TestJoinDate_RoundTrip(t *testing.T) {
	// 合法。
	if k, err := joinDate(2026, "02-14"); err != nil || k != "2026-02-14" {
		t.Errorf("joinDate(2026,02-14) = %q,%v; want 2026-02-14,nil", k, err)
	}
	// 非法：2 月没有 30 日，time.Date 会规整为 03-02，round-trip 不等 → 拒绝。
	if k, err := joinDate(2026, "02-30"); err == nil {
		t.Errorf("joinDate(2026,02-30) = %q, want error (dead key)", k)
	}
	// 非法：月份/日越界。
	if _, err := joinDate(2026, "13-01"); err == nil {
		t.Error("joinDate(2026,13-01) should error")
	}
	if _, err := joinDate(2026, "02-32"); err == nil {
		t.Error("joinDate(2026,02-32) should error")
	}
	// 非法：格式。
	if _, err := joinDate(2026, "0222"); err == nil {
		t.Error("joinDate(2026,0222) should error")
	}
}

// TestCalendarService_GetDayInfo_Holiday 验证聚合根经真实 HolidayRepository 组合出节假日信息。
func TestCalendarService_GetDayInfo_Holiday(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	// 用 fake lunar（不关心农历字段），真实 holiday repo。
	svc := NewCalendarService(nil, &fakeLunar{}, repo, WithToday(time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)))
	info := svc.GetDayInfo(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local))
	if !info.Holiday.IsHoliday || info.Holiday.Name != "元旦" {
		t.Errorf("GetDayInfo holiday = %+v, want IsHoliday + 元旦", info.Holiday)
	}
	if !info.IsToday {
		t.Error("GetDayInfo(2026-01-01) should be IsToday under WithToday(2026-01-01)")
	}
}

// TestEmbedHolidayRepository_Real2026Schedule 锁定 2026 真实节假日/调休排布，
// 与 ADR-05 来源（holiday-cn + 国务院通知）一致，防止被近似占位 SEED 误回退。
// 选取的关键鉴别点（占位草稿在这些点不同）：元旦放 3 天、春节放 9 天（含 02-23）、
// 元旦补班 01-04、国庆补班 09-20（占位稿误写为 09-26）。
func TestEmbedHolidayRepository_Real2026Schedule(t *testing.T) {
	repo, err := NewHolidayRepository()
	if err != nil {
		t.Fatalf("NewHolidayRepository: %v", err)
	}
	holiday := func(y, m, d int) bool {
		return repo.IsHoliday(time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.Local))
	}
	workday := func(y, m, d int) bool {
		return repo.IsWorkday(time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.Local))
	}

	// 元旦 3 天（占位稿仅 1 天）。
	for _, d := range []int{1, 2, 3} {
		if !holiday(2026, 1, d) {
			t.Errorf("2026-01-%02d should be 元旦 holiday (real=3 days)", d)
		}
	}
	// 春节 9 天（占位稿 8 天，止于 02-22）。
	for _, d := range []int{15, 16, 17, 18, 19, 20, 21, 22, 23} {
		if !holiday(2026, 2, d) {
			t.Errorf("2026-02-%02d should be 春节 holiday (real=9 days)", d)
		}
	}
	// 清明 / 端午 / 中秋 / 国庆 代表日。
	if !holiday(2026, 4, 4) {
		t.Error("2026-04-04 清明 should be holiday")
	}
	if !holiday(2026, 6, 19) {
		t.Error("2026-06-19 端午 should be holiday")
	}
	if !holiday(2026, 9, 25) {
		t.Error("2026-09-25 中秋 should be holiday")
	}
	for _, d := range []int{1, 2, 3, 4, 5, 6, 7} {
		if !holiday(2026, 10, d) {
			t.Errorf("2026-10-%02d should be 国庆 holiday", d)
		}
	}
	// 调休补班（占位稿缺 01-04、误写 09-26）。
	for _, tc := range []struct {
		m, d int
		name  string
	}{
		{1, 4, "元旦补班"}, {2, 14, "春节补班"}, {2, 28, "春节补班"},
		{5, 9, "劳动节补班"}, {9, 20, "国庆节补班"}, {10, 10, "国庆节补班"},
	} {
		if !workday(2026, tc.m, tc.d) {
			t.Errorf("2026-%02d-%02d should be 调休补班 %s", tc.m, tc.d, tc.name)
		}
		if repo.Name(time.Date(2026, time.Month(tc.m), tc.d, 0, 0, 0, 0, time.Local)) != tc.name {
			t.Errorf("Name(2026-%02d-%02d) != %s", tc.m, tc.d, tc.name)
		}
	}
	// 占位稿曾误将 09-26 标为中秋补班，真实并非补班日。
	if workday(2026, 9, 26) {
		t.Error("2026-09-26 should NOT be workday (placeholder wrongly had 中秋补班)")
	}
}
