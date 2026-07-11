package build

import "testing"

func TestInfo_CGODisabled(t *testing.T) {
	info := Info()
	if info.CGOEnabled {
		t.Fatal("CGOEnabled must be false (zero-CGO hard constraint, ADR-01/06)")
	}
}

func TestInfo_Defaults(t *testing.T) {
	info := Info()
	if info.Version == "" || info.Commit == "" || info.BuildTime == "" {
		t.Fatalf("unexpected empty version metadata: %+v", info)
	}
	if info.TargetOS != "windows" {
		t.Fatalf("default TargetOS = %q, want windows", info.TargetOS)
	}
	if info.TargetArch == "" {
		t.Fatal("TargetArch must not be empty")
	}
}
