// Package calendar 是日历领域 feature（MVP 骨架）。
//
// 依赖方向（ADR-07a）：本包仅 import internal/state 来 emit 领域事件，
// 绝不 import internal/plugin。这保证 feature 不反向编译依赖插件层。
package calendar

import (
	"context"
	"time"

	"github.com/shaolei/DeskCalendar/internal/state"
)

// Service 日历领域服务。持有事件总线，仅在状态变更时经 state.Publish 广播。
type Service struct {
	bus state.EventBus
}

// NewService 构造日历服务。
func NewService(bus state.EventBus) *Service {
	return &Service{bus: bus}
}

// EmitDateChanged 在选中日期/月份变更时广播（feature → state）。
// 这是 ADR-07a 中 feature→state 方向的实体证据。
func (s *Service) EmitDateChanged(ctx context.Context, year, month, day int, isMonth bool) {
	s.bus.Publish(ctx, state.Event{
		Topic: state.TopicDateChanged,
		Payload: state.DateChangedPayload{
			Year:    year,
			Month:   month,
			Day:     day,
			IsMonth: isMonth,
		},
		At: time.Now(),
	})
}
