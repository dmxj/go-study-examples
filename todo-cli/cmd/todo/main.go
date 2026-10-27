// Command todo 是命令行 Todo 工具的入口。
//
// main 只做三件事：创建可被信号取消的 context、调用 cli.Execute、把错误转换为退出码。
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dmxj/go-study-examples/todo-cli/internal/cli"
)

func main() {
	os.Exit(run())
}

// run 返回进程退出码。把逻辑放在 run 里而不是 main 里，
// 是为了让 defer（如 stop()）在 os.Exit 之前正常执行。
func run() int {
	// Ctrl+C（SIGINT）或 kill（SIGTERM）时 ctx 被取消。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := cli.Execute(ctx, os.Args[1:], cli.Options{Out: os.Stdout, Err: os.Stderr})
	switch {
	case err == nil:
		return 0
	case errors.Is(err, context.Canceled):
		fmt.Fprintln(os.Stderr, "已取消")
		return 130 // 约定：被 SIGINT 中断的进程退出码为 128+2
	default:
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 1
	}
}
