package assets

import (
	"encoding/json"
	"testing"
)

func TestIcon_Readable(t *testing.T) {
	b, err := Icon()
	if err != nil {
		t.Fatalf("Icon() error: %v", err)
	}
	if len(b) < 22 {
		t.Fatalf("icon too small: %d bytes", len(b))
	}
	// ICONDIR: reserved(0,0) + type(1,0)
	if b[0] != 0 || b[1] != 0 || b[2] != 1 || b[3] != 0 {
		t.Fatalf("not a valid ICO (bad magic): %x", b[:4])
	}
}

func TestDefaultThemeJSON_Valid(t *testing.T) {
	b, err := DefaultThemeJSON()
	if err != nil {
		t.Fatalf("DefaultThemeJSON() error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("default theme JSON invalid: %v", err)
	}
	if _, ok := m["scheme"]; !ok {
		t.Fatal("default theme JSON missing 'scheme'")
	}
}
