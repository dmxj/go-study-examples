// Package todo 定义 Todo 领域模型（任务、优先级、过滤、排序）与业务服务。
//
// 该包不依赖任何 I/O 细节：存储通过 Repository 接口注入，时间通过 Clock 注入，
// 因此可以在单元测试中完全确定性地运行。
package todo

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Priority 表示任务优先级，数值越大越紧急。
type Priority int

// 优先级常量。零值 PriorityNone 表示"未设置"，在 Add 时会被替换为默认的 Medium。
const (
	PriorityNone Priority = iota
	PriorityLow
	PriorityMedium
	PriorityHigh
)

// ErrInvalidPriority 表示无法识别的优先级字符串。
var ErrInvalidPriority = errors.New("无效的优先级")

// String 实现 fmt.Stringer，用于表格展示和 JSON 序列化。
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityMedium:
		return "medium"
	case PriorityHigh:
		return "high"
	default:
		return "none"
	}
}

// ParsePriority 解析用户输入的优先级，支持全称、首字母和数字：
// high/h/3、medium/m/2、low/l/1。
func ParsePriority(s string) (Priority, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "high", "h", "3":
		return PriorityHigh, nil
	case "medium", "med", "m", "2":
		return PriorityMedium, nil
	case "low", "l", "1":
		return PriorityLow, nil
	default:
		return PriorityNone, fmt.Errorf("%w: %q（可选 high/medium/low）", ErrInvalidPriority, s)
	}
}

// MarshalText 让 Priority 在 JSON 中以字符串形式存储（"high" 而不是 3），
// 这样数据文件对人类更友好，也不怕以后调整常量顺序。
func (p Priority) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText 是 MarshalText 的逆操作。encoding/json 会自动调用它。
func (p *Priority) UnmarshalText(b []byte) error {
	if string(b) == "none" || len(b) == 0 {
		*p = PriorityNone
		return nil
	}
	v, err := ParsePriority(string(b))
	if err != nil {
		return err
	}
	*p = v
	return nil
}

// Task 是一条待办事项。
//
// 注意 Due、CompletedAt 使用了 Go 1.24 新增的 `omitzero` 标签：
// 对 time.Time 这类结构体，`omitempty` 不生效，而 `omitzero` 会在零值时省略字段。
type Task struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Priority    Priority  `json:"priority"`
	Done        bool      `json:"done"`
	Tags        []string  `json:"tags,omitempty"`
	Due         time.Time `json:"due,omitzero"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitzero"`
}

// HasDue 报告任务是否设置了截止日期。
func (t Task) HasDue() bool { return !t.Due.IsZero() }

// Overdue 报告任务在 now 时刻是否已逾期：未完成、有截止日期且截止日期早于今天。
// 截止日期按"天"比较——截止到 11-01 的任务在 11-01 当天不算逾期。
func (t Task) Overdue(now time.Time) bool {
	if t.Done || !t.HasDue() {
		return false
	}
	return t.Due.Before(startOfDay(now))
}

// HasTag 报告任务是否带有指定标签（大小写不敏感）。
func (t Task) HasTag(tag string) bool {
	for _, x := range t.Tags {
		if strings.EqualFold(x, tag) {
			return true
		}
	}
	return false
}

// startOfDay 返回 t 所在日期的 00:00:00（保留时区）。
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
