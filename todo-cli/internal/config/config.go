// Package config 负责解析 todo 的数据文件位置。
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvFile 是覆盖数据文件路径的环境变量名。
const EnvFile = "TODO_FILE"

// DataFile 按优先级返回数据文件路径：
//  1. 命令行 --file 参数（flagValue 非空）
//  2. 环境变量 TODO_FILE
//  3. 用户主目录下的 ~/.todo/todo.json
func DataFile(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if v := os.Getenv(EnvFile); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户主目录失败（可用 --file 或 %s 指定数据文件）: %w", EnvFile, err)
	}
	return filepath.Join(home, ".todo", "todo.json"), nil
}
