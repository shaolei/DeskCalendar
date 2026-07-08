package state_test

import (
	"testing"
	"time"

	"github.com/shaolei/DeskCalendar/internal/state"
)

func TestNewCalendarState(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.Local)
	cs := state.NewCalendarState(now)
	if !cs.SelectedDate().Get().Equal(now) {
		t.Fatal("selectedDate should equal now")
	}
	if cs.DisplayedMonth().Get().Month() != time.July {
		t.Fatal("displayedMonth should be July")
	}
	if cs.ViewMode().Get() != state.ViewMonth {
		t.Fatal("viewMode should be ViewMonth")
	}
	if cs.Snapshot() == nil {
		t.Fatal("Snapshot should not be nil")
	}
}

func TestNewThemeState(t *testing.T) {
	ts := state.NewThemeState()
	if ts.Mode().Get() != state.ThemeSystem {
		t.Fatal("theme mode should be ThemeSystem")
	}
	if ts.Snapshot() == nil {
		t.Fatal("Snapshot should not be nil")
	}
}

func TestNewUIState(t *testing.T) {
	us := state.NewUIState()
	if us.Visible().Get() {
		t.Fatal("ui visible should default false")
	}
	if us.Size().Get().W != 320 || us.Size().Get().H != 420 {
		t.Fatal("ui size should be 320x420")
	}
	if us.Snapshot() == nil {
		t.Fatal("Snapshot should not be nil")
	}
}
