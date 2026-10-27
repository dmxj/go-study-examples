// Package cli 使用 cobra 组装 todo 的命令行界面。
//
// 设计要点：命令层只做"参数解析 + 调用 Service + 输出"，不写业务逻辑；
// 所有输出都写到注入的 io.Writer，便于测试时捕获。
package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/dmxj/go-study-examples/todo-cli/internal/config"
	"github.com/dmxj/go-study-examples/todo-cli/internal/storage"
	"github.com/dmxj/go-study-examples/todo-cli/internal/todo"
	"github.com/dmxj/go-study-examples/todo-cli/internal/ui"
)

// Version 由构建时 -ldflags "-X .../internal/cli.Version=v1.0.0" 注入。
var Version = "dev"

// Options 是创建根命令所需的外部依赖。
type Options struct {
	Out   io.Writer  // 标准输出
	Err   io.Writer  // 标准错误（日志写这里）
	Clock todo.Clock // 为 nil 时使用 time.Now
}

// app 保存所有子命令共享的状态，在 PersistentPreRunE 中初始化。
type app struct {
	opts    Options
	file    string
	noColor bool
	verbose bool

	svc     *todo.Service
	palette ui.Palette
	log     *slog.Logger
}

// NewRootCmd 构建完整的命令树。
func NewRootCmd(opts Options) *cobra.Command {
	a := &app{opts: opts}
	root := &cobra.Command{
		Use:   "todo",
		Short: "一个简洁的命令行 Todo 工具",
		Long: `todo 是一个用 Go 编写的命令行待办工具，数据保存在本地 JSON 文件。

示例:
  todo add "写周报" -p high -d tomorrow -t work
  todo list --sort priority
  todo done 3`,
		Version: Version,
		// 出错时不自动打印用法和错误，由 main 统一处理，避免重复输出。
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return a.init(cmd.Context())
		},
	}
	root.SetOut(opts.Out)
	root.SetErr(opts.Err)

	pf := root.PersistentFlags()
	pf.StringVarP(&a.file, "file", "f", "", "数据文件路径（默认 $TODO_FILE 或 ~/.todo/todo.json）")
	pf.BoolVar(&a.noColor, "no-color", false, "禁用彩色输出")
	pf.BoolVarP(&a.verbose, "verbose", "v", false, "输出调试日志")

	root.AddCommand(
		a.newAddCmd(),
		a.newListCmd(),
		a.newDoneCmd(true),
		a.newDoneCmd(false),
		a.newEditCmd(),
		a.newRemoveCmd(),
		a.newClearCmd(),
	)
	return root
}

// Execute 运行命令，是 main 的唯一入口。
func Execute(ctx context.Context, args []string, opts Options) error {
	cmd := NewRootCmd(opts)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

func (a *app) init(ctx context.Context) error {
	// 如果用户已经按了 Ctrl+C，就不要再开始任何读写。
	if err := ctx.Err(); err != nil {
		return err
	}
	level := slog.LevelWarn
	if a.verbose {
		level = slog.LevelDebug
	}
	a.log = slog.New(slog.NewTextHandler(a.opts.Err, &slog.HandlerOptions{Level: level}))

	path, err := config.DataFile(a.file)
	if err != nil {
		return err
	}
	a.log.Debug("使用数据文件", "path", path)
	a.svc = todo.NewService(storage.NewJSONStore(path), a.opts.Clock)
	a.palette = ui.Palette{Enabled: ui.ColorEnabled(a.opts.Out, a.noColor)}
	return nil
}

func (a *app) printf(format string, args ...any) {
	fmt.Fprintf(a.opts.Out, format, args...)
}

// parseIDs 把位置参数转换为 ID 列表。
func parseIDs(args []string) ([]int, error) {
	ids := make([]int, 0, len(args))
	for _, s := range args {
		id, err := strconv.Atoi(s)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("无效的任务 ID %q：需要正整数", s)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
