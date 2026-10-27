package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDataFilePriority(t *testing.T) {
	// t.Setenv 会在测试结束后自动恢复环境变量
	t.Setenv(EnvFile, "/tmp/from-env.json")

	if got, _ := DataFile("/tmp/from-flag.json"); got != "/tmp/from-flag.json" {
		t.Errorf("命令行参数优先级最高，实际 %s", got)
	}
	if got, _ := DataFile(""); got != "/tmp/from-env.json" {
		t.Errorf("未传参数时应使用环境变量，实际 %s", got)
	}

	t.Setenv(EnvFile, "")
	t.Setenv("HOME", t.TempDir())
	got, err := DataFile("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, filepath.Join(".todo", "todo.json")) {
		t.Errorf("默认路径应位于 ~/.todo/todo.json，实际 %s", got)
	}
}
