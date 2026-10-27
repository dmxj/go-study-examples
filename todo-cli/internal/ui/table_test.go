package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/dmxj/go-study-examples/todo-cli/internal/todo"
)

var now = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

func TestTableRenderPlain(t *testing.T) {
	tasks := []todo.Task{
		{ID: 1, Title: "写周报", Priority: todo.PriorityHigh, Due: now.AddDate(0, 0, -2), Tags: []string{"work"}},
		{ID: 12, Title: "buy milk", Priority: todo.PriorityLow, Done: true},
	}
	var buf bytes.Buffer
	if err := (Table{Now: now}).Render(&buf, tasks); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Error("禁用颜色时不应输出 ANSI 转义码")
	}
	for _, want := range []string{"ID", "写周报", "(逾期)", "[x]", "work"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出缺少 %q:\n%s", want, out)
		}
	}
	// 中文与英文混排时，"标题"列之后的内容起始列应对齐。
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	col := func(line, sub string) int {
		i := strings.Index(line, sub)
		return runewidth.StringWidth(line[:i])
	}
	if a, b := col(lines[2], "2026-10-25"), col(lines[3], "-  "); a != b {
		t.Errorf("截止日期列未对齐: %d vs %d\n%s", a, b, out)
	}
}

func TestTableRenderColor(t *testing.T) {
	var buf bytes.Buffer
	tasks := []todo.Task{{ID: 1, Title: "x", Priority: todo.PriorityHigh}}
	(Table{Palette: Palette{Enabled: true}, Now: now}).Render(&buf, tasks)
	if !strings.Contains(buf.String(), red+"high"+reset) {
		t.Errorf("high 优先级应为红色，实际 %q", buf.String())
	}
}

func TestTableRenderEmpty(t *testing.T) {
	var buf bytes.Buffer
	(Table{Now: now}).Render(&buf, nil)
	if !strings.Contains(buf.String(), "没有符合条件的任务") {
		t.Errorf("空列表应输出提示，实际 %q", buf.String())
	}
}

func TestColorEnabled(t *testing.T) {
	// bytes.Buffer 不是终端，应该禁用颜色
	if ColorEnabled(&bytes.Buffer{}, false) {
		t.Error("非终端输出应禁用颜色")
	}
}
