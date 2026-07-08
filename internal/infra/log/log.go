// Package log 提供极简结构化日志接口与实现。
//
// 设计约束（ADR-01 / ADR-06）：纯 Go、零 CGO，无任何第三方依赖，
// 可被 internal/state 等底层包安全引用而不污染依赖图。
package log

import (
	"fmt"
	"io"
	"os"
)

// Logger 结构化日志接口。参数以 key/value 平铺传递（args ...any）。
// 仅暴露四个级别，足够内部诊断使用，避免引入 slog/zerolog 等重依赖。
type Logger interface {
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
	Info(msg string, args ...any)
	Debug(msg string, args ...any)
}

// nopLogger 丢弃所有日志，便于测试与静默路径（如事件总线 panic 隔离时）。
type nopLogger struct{}

func (nopLogger) Error(string, ...any) {}
func (nopLogger) Warn(string, ...any)  {}
func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Debug(string, ...any) {}

// Nop 返回丢弃型 Logger。
func Nop() Logger { return nopLogger{} }

// stdLogger 基于标准库 io.Writer 的输出型实现。
type stdLogger struct {
	w io.Writer
}

func (l stdLogger) logf(level, msg string, args ...any) {
	fmt.Fprintf(l.w, "[%s] %s %v\n", level, msg, args)
}

func (l stdLogger) Error(msg string, args ...any) { l.logf("ERROR", msg, args...) }
func (l stdLogger) Warn(msg string, args ...any)  { l.logf("WARN", msg, args...) }
func (l stdLogger) Info(msg string, args ...any)  { l.logf("INFO", msg, args...) }
func (l stdLogger) Debug(msg string, args ...any) { l.logf("DEBUG", msg, args...) }

// New 返回输出到 stderr 的 Logger。
func New() Logger { return stdLogger{w: os.Stderr} }
