package todo

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Status 是列表过滤时的完成状态。
type Status string

// 可选状态。
const (
	StatusAll     Status = "all"
	StatusPending Status = "pending"
	StatusDone    Status = "done"
)

// Filter 描述 list 命令的过滤条件，零值表示"不过滤"。
type Filter struct {
	Status      Status   // 默认 pending
	MinPriority Priority // 只显示 >= 该优先级的任务
	Tag         string   // 只显示带该标签的任务
	Keyword     string   // 标题包含关键字（大小写不敏感）
	Overdue     bool     // 只显示已逾期任务
}

// Match 报告任务 t 是否满足过滤条件。
func (f Filter) Match(t Task, now time.Time) bool {
	switch f.Status {
	case StatusPending, "":
		if t.Done {
			return false
		}
	case StatusDone:
		if !t.Done {
			return false
		}
	}
	if f.MinPriority != PriorityNone && t.Priority < f.MinPriority {
		return false
	}
	if f.Tag != "" && !t.HasTag(f.Tag) {
		return false
	}
	if f.Keyword != "" && !strings.Contains(strings.ToLower(t.Title), strings.ToLower(f.Keyword)) {
		return false
	}
	if f.Overdue && !t.Overdue(now) {
		return false
	}
	return true
}

// SortBy 是列表排序字段。
type SortBy string

// 可选排序方式。
const (
	SortByID       SortBy = "id"
	SortByPriority SortBy = "priority"
	SortByDue      SortBy = "due"
)

// ParseSortBy 校验排序字段。
func ParseSortBy(s string) (SortBy, error) {
	switch v := SortBy(strings.ToLower(s)); v {
	case SortByID, SortByPriority, SortByDue:
		return v, nil
	case "":
		return SortByID, nil
	default:
		return "", fmt.Errorf("不支持的排序字段 %q（可选 id/priority/due）", s)
	}
}

// SortTasks 原地排序。使用 slices.SortStableFunc + cmp.Compare（Go 1.21+），
// 主键相同时按 ID 升序，保证输出稳定。
func SortTasks(tasks []Task, by SortBy) {
	slices.SortStableFunc(tasks, func(a, b Task) int {
		switch by {
		case SortByPriority:
			// 优先级高的排前面，所以反过来比较
			if c := cmp.Compare(b.Priority, a.Priority); c != 0 {
				return c
			}
		case SortByDue:
			// 没有截止日期的排最后
			if c := compareDue(a, b); c != 0 {
				return c
			}
		}
		return cmp.Compare(a.ID, b.ID)
	})
}

func compareDue(a, b Task) int {
	switch {
	case a.HasDue() && !b.HasDue():
		return -1
	case !a.HasDue() && b.HasDue():
		return 1
	case !a.HasDue() && !b.HasDue():
		return 0
	}
	return a.Due.Compare(b.Due)
}
