package todo

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// fixedNow 是所有测试共用的"当前时间"：2026-10-27 10:00 (UTC+8)。
var fixedNow = time.Date(2026, 10, 27, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))

func TestParsePriority(t *testing.T) {
	tests := []struct {
		in      string
		want    Priority
		wantErr bool
	}{
		{"high", PriorityHigh, false},
		{"H", PriorityHigh, false},
		{" 2 ", PriorityMedium, false},
		{"low", PriorityLow, false},
		{"urgent", PriorityNone, true},
	}
	for _, tt := range tests {
		got, err := ParsePriority(tt.in)
		if (err != nil) != tt.wantErr {
			t.Fatalf("ParsePriority(%q) err = %v，期望出错 %v", tt.in, err, tt.wantErr)
		}
		if err != nil && !errors.Is(err, ErrInvalidPriority) {
			t.Errorf("ParsePriority(%q) 错误应包装 ErrInvalidPriority，实际 %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("ParsePriority(%q) = %v，期望 %v", tt.in, got, tt.want)
		}
	}
}

func TestTaskJSON(t *testing.T) {
	task := Task{ID: 1, Title: "写代码", Priority: PriorityHigh, CreatedAt: fixedNow}
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"priority":"high"`) {
		t.Errorf("优先级应序列化为字符串，实际 %s", s)
	}
	// omitzero：零值的 due/completed_at 不应出现在 JSON 中
	if strings.Contains(s, "due") || strings.Contains(s, "completed_at") {
		t.Errorf("零值时间字段应被省略，实际 %s", s)
	}
	var back Task
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back.Priority != PriorityHigh || back.Title != task.Title {
		t.Errorf("反序列化结果 %+v 与原值不一致", back)
	}
}

func TestOverdue(t *testing.T) {
	today := startOfDay(fixedNow)
	tests := []struct {
		name string
		task Task
		want bool
	}{
		{"无截止日期", Task{}, false},
		{"昨天截止", Task{Due: today.AddDate(0, 0, -1)}, true},
		{"今天截止不算逾期", Task{Due: today}, false},
		{"已完成不算逾期", Task{Due: today.AddDate(0, 0, -5), Done: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.task.Overdue(fixedNow); got != tt.want {
				t.Errorf("Overdue() = %v，期望 %v", got, tt.want)
			}
		})
	}
}

func TestParseDue(t *testing.T) {
	today := startOfDay(fixedNow)
	tests := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"", time.Time{}, false},
		{"today", today, false},
		{"tomorrow", today.AddDate(0, 0, 1), false},
		{"+3d", today.AddDate(0, 0, 3), false},
		{"+2w", today.AddDate(0, 0, 14), false},
		{"2026-11-01", time.Date(2026, 11, 1, 0, 0, 0, 0, fixedNow.Location()), false},
		{"+3m", time.Time{}, true},
		{"2026/11/01", time.Time{}, true},
	}
	for _, tt := range tests {
		got, err := ParseDue(tt.in, fixedNow)
		if (err != nil) != tt.wantErr {
			t.Fatalf("ParseDue(%q) err = %v，期望出错 %v", tt.in, err, tt.wantErr)
		}
		if !got.Equal(tt.want) {
			t.Errorf("ParseDue(%q) = %v，期望 %v", tt.in, got, tt.want)
		}
	}
}

func TestSortTasks(t *testing.T) {
	today := startOfDay(fixedNow)
	tasks := []Task{
		{ID: 1, Priority: PriorityLow, Due: today.AddDate(0, 0, 2)},
		{ID: 2, Priority: PriorityHigh},
		{ID: 3, Priority: PriorityHigh, Due: today},
		{ID: 4, Priority: PriorityMedium, Due: today.AddDate(0, 0, 1)},
	}
	ids := func(ts []Task) []int {
		var out []int
		for _, t := range ts {
			out = append(out, t.ID)
		}
		return out
	}
	SortTasks(tasks, SortByPriority)
	if got, want := ids(tasks), []int{2, 3, 4, 1}; !slices.Equal(got, want) {
		t.Errorf("按优先级排序 = %v，期望 %v", got, want)
	}
	SortTasks(tasks, SortByDue)
	if got, want := ids(tasks), []int{3, 4, 1, 2}; !slices.Equal(got, want) {
		t.Errorf("按截止日期排序 = %v，期望 %v（无截止日期排最后）", got, want)
	}
}

func TestFilterMatch(t *testing.T) {
	task := Task{ID: 1, Title: "Write Weekly Report", Priority: PriorityMedium, Tags: []string{"Work"}}
	tests := []struct {
		name string
		f    Filter
		want bool
	}{
		{"默认只看未完成", Filter{}, true},
		{"只看已完成", Filter{Status: StatusDone}, false},
		{"最低优先级 high", Filter{MinPriority: PriorityHigh}, false},
		{"最低优先级 low", Filter{MinPriority: PriorityLow}, true},
		{"标签大小写不敏感", Filter{Tag: "work"}, true},
		{"关键字", Filter{Keyword: "report"}, true},
		{"关键字不匹配", Filter{Keyword: "牛奶"}, false},
		{"只看逾期", Filter{Overdue: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.Match(task, fixedNow); got != tt.want {
				t.Errorf("Match() = %v，期望 %v", got, tt.want)
			}
		})
	}
}
