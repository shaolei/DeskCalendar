package todo

import (
	infralog "github.com/shaolei/DeskCalendar/internal/infra/log"
	"path/filepath"
	"sync/atomic"
)

// log 包级结构化日志（内部诊断用，无副作用失败路径）。
var log = infralog.New()

// idFallback 是 newID 降级路径的进程内计数器（保证降级 ID 唯一）。
var idFallback int64

// dirOf 返回 path 的父目录（用于确保 JSON 仓库父目录存在）。
func dirOf(path string) string {
	return filepath.Dir(path)
}

// newID 生成 UUID v4 字符串（RFC 4122），不引入任何第三方依赖。
// 以 crypto/rand 取 16 字节，置版本(4)/变体(10xx)位。极端情况下随机源不可用
// （几乎不可能），降级为基于时间戳+计数的值，保证非空唯一。
func newID() string {
	b := make([]byte, 16)
	if _, err := randRead(b); err == nil {
		b[6] = (b[6] & 0x0f) | 0x40 // version 4
		b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
		return formatUUID(b)
	}
	// 降级路径（crypto/rand 失败）：时间戳纳秒 + 进程内计数器。
	seq := atomic.AddInt64(&idFallback, 1)
	t := timeNow().UnixNano()
	return formatUUIDFromInts(uint32(t>>32), uint16(t>>16), uint16(t), uint16(uint32(seq)>>16), seq)
}
