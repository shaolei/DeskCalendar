package log

import (
	"bytes"
	"strings"
	"testing"
)

// N4 回归：stdLogger 输出应满足 "[LEVEL] msg [k v]" 形状。
func TestStdLoggerFormat(t *testing.T) {
	var buf bytes.Buffer
	l := stdLogger{w: &buf}

	l.Info("hello", "k", "v")
	line := strings.TrimRight(buf.String(), "\n")
	if !strings.HasPrefix(line, "[INFO] ") {
		t.Fatalf("prefix = %q, want [INFO]", line)
	}
	if !strings.Contains(line, "hello") {
		t.Errorf("msg missing: %q", line)
	}
	// args 经 fmt %v 打印为 [k v]
	if !strings.Contains(line, "[k v]") {
		t.Errorf("args shape wrong: %q, want [k v]", line)
	}

	buf.Reset()
	l.Warn("boom", "code", 42)
	line = strings.TrimRight(buf.String(), "\n")
	if !strings.HasPrefix(line, "[WARN] ") || !strings.Contains(line, "boom") || !strings.Contains(line, "[code 42]") {
		t.Errorf("warn line shape wrong: %q", line)
	}

	buf.Reset()
	l.Error("err", "e", "x")
	line = strings.TrimRight(buf.String(), "\n")
	if !strings.HasPrefix(line, "[ERROR] ") || !strings.Contains(line, "[e x]") {
		t.Errorf("error line shape wrong: %q", line)
	}
}

// N4 回归：Nop 不产出任何字节，且不 panic。
func TestNopLoggerSilent(t *testing.T) {
	var buf bytes.Buffer
	l := nopLogger{}
	// 强制走 stdLogger 同形状也行，这里直接验证 Nop 接口行为
	var _ Logger = l
	l.Error("should", "be", "silent")
	l.Warn("x")
	l.Info("y")
	l.Debug("z")
	_ = buf // Nop 不写任何 writer，仅确认无 panic
}
