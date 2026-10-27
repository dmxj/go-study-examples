package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmxj/go-study-examples/todo-cli/internal/todo"
)

// runCLI 在临时数据文件上执行一条命令，返回标准输出。
// 这就是 CLI 的"集成测试"：走完整的 cobra 解析 → Service → JSON 文件流程。
func runCLI(t *testing.T, file string, args ...string) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	clock := func() time.Time { return time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC) }
	err := Execute(context.Background(), append([]string{"--file", file}, args...),
		Options{Out: &out, Err: &errOut, Clock: clock})
	return out.String(), err
}

func TestCLIWorkflow(t *testing.T) {
	file := filepath.Join(t.TempDir(), "todo.json")

	if _, err := runCLI(t, file, "add", "写周报", "-p", "high", "-d", "2026-10-25", "-t", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, file, "add", "买", "牛奶", "-p", "low"); err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, file, "list", "--sort", "priority")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "写周报") || !strings.Contains(out, "买 牛奶") || !strings.Contains(out, "(逾期)") {
		t.Errorf("list 输出不完整:\n%s", out)
	}
	if strings.Index(out, "写周报") > strings.Index(out, "买 牛奶") {
		t.Errorf("按优先级排序时 high 应排在 low 前面:\n%s", out)
	}

	if out, err = runCLI(t, file, "done", "1"); err != nil || !strings.Contains(out, "已完成 #1") {
		t.Fatalf("done 输出 %q, err=%v", out, err)
	}
	out, _ = runCLI(t, file, "ls")
	if strings.Contains(out, "写周报") {
		t.Errorf("默认 list 不应显示已完成任务:\n%s", out)
	}
	out, _ = runCLI(t, file, "ls", "--done")
	if !strings.Contains(out, "写周报") {
		t.Errorf("--done 应显示已完成任务:\n%s", out)
	}

	if _, err = runCLI(t, file, "edit", "2", "--title", "买酸奶", "--due", "+1d"); err != nil {
		t.Fatal(err)
	}
	out, _ = runCLI(t, file, "ls", "--search", "酸奶")
	if !strings.Contains(out, "2026-10-28") {
		t.Errorf("edit 后截止日期应为 2026-10-28:\n%s", out)
	}

	if out, _ = runCLI(t, file, "clear"); !strings.Contains(out, "已清理 1 个") {
		t.Errorf("clear 输出 %q", out)
	}
}

func TestCLIErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "todo.json")
	tests := []struct {
		name string
		args []string
	}{
		{"缺少标题", []string{"add"}},
		{"非法优先级", []string{"add", "x", "-p", "urgent"}},
		{"非法日期", []string{"add", "x", "-d", "next-year"}},
		{"非法 ID", []string{"done", "abc"}},
		{"edit 无字段", []string{"edit", "1"}},
		{"互斥参数", []string{"ls", "--all", "--done"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := runCLI(t, file, tt.args...); err == nil {
				t.Errorf("todo %v 应返回错误", tt.args)
			}
		})
	}
}

func TestCLIPartialFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "todo.json")
	runCLI(t, file, "add", "a")
	out, err := runCLI(t, file, "done", "1", "42")
	// #1 成功、#42 失败：成功的部分要输出，错误要能用 errors.Is 识别
	if !strings.Contains(out, "已完成 #1") {
		t.Errorf("成功的 ID 应正常输出，实际 %q", out)
	}
	if !errors.Is(err, todo.ErrNotFound) {
		t.Errorf("errors.Join 后仍应能用 errors.Is 识别 ErrNotFound，实际 %v", err)
	}
}

func TestCLICanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	err := Execute(ctx, []string{"--file", filepath.Join(t.TempDir(), "x.json"), "ls"}, Options{Out: &out, Err: &out})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("已取消的 context 应返回 context.Canceled，实际 %v", err)
	}
}
