package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dmxj/go-study-examples/todo-cli/internal/todo"
	"github.com/dmxj/go-study-examples/todo-cli/internal/ui"
)

func (a *app) newAddCmd() *cobra.Command {
	var priority, due string
	var tags []string
	cmd := &cobra.Command{
		Use:   "add <标题>",
		Short: "新增任务",
		Example: `  todo add 买牛奶 -p low -t 生活
  todo add "完成 Go 作业" -p high -d +3d`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := todo.AddInput{Title: strings.Join(args, " "), Tags: tags}
			var err error
			if priority != "" {
				if in.Priority, err = todo.ParsePriority(priority); err != nil {
					return err
				}
			}
			if in.Due, err = todo.ParseDue(due, a.svc.Now()); err != nil {
				return err
			}
			t, err := a.svc.Add(in)
			if err != nil {
				return err
			}
			a.log.Debug("任务已新增", "id", t.ID)
			a.printf("%s 已新增任务 #%d：%s\n", a.palette.Green("✔"), t.ID, t.Title)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&priority, "priority", "p", "", "优先级 high/medium/low（默认 medium）")
	f.StringVarP(&due, "due", "d", "", "截止日期：YYYY-MM-DD、today、tomorrow、+3d、+1w")
	f.StringSliceVarP(&tags, "tags", "t", nil, "标签，逗号分隔，如 -t work,urgent")
	return cmd
}

func (a *app) newListCmd() *cobra.Command {
	var (
		all, done, overdue bool
		priority, sortBy   string
		filter             todo.Filter
	)
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "列出任务（默认只显示未完成）",
		Example: `  todo list --all
  todo ls -p high --sort due
  todo ls --tag work --search 周报`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch {
			case all:
				filter.Status = todo.StatusAll
			case done:
				filter.Status = todo.StatusDone
			default:
				filter.Status = todo.StatusPending
			}
			filter.Overdue = overdue
			if priority != "" {
				p, err := todo.ParsePriority(priority)
				if err != nil {
					return err
				}
				filter.MinPriority = p
			}
			by, err := todo.ParseSortBy(sortBy)
			if err != nil {
				return err
			}
			tasks, err := a.svc.List(filter, by)
			if err != nil {
				return err
			}
			return ui.Table{Palette: a.palette, Now: a.svc.Now()}.Render(a.opts.Out, tasks)
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&all, "all", "a", false, "显示全部任务（含已完成）")
	f.BoolVar(&done, "done", false, "只显示已完成任务")
	f.BoolVar(&overdue, "overdue", false, "只显示已逾期任务")
	f.StringVarP(&priority, "priority", "p", "", "最低优先级过滤，如 -p medium 显示 medium 和 high")
	f.StringVar(&filter.Tag, "tag", "", "按标签过滤")
	f.StringVarP(&filter.Keyword, "search", "s", "", "按标题关键字过滤")
	f.StringVar(&sortBy, "sort", "id", "排序字段 id/priority/due")
	cmd.MarkFlagsMutuallyExclusive("all", "done")
	return cmd
}

// newDoneCmd 同时生成 done 与 undo 两个命令，它们只差一个布尔参数。
func (a *app) newDoneCmd(done bool) *cobra.Command {
	use, short, verb := "done <id>...", "标记任务完成", "已完成"
	if !done {
		use, short, verb = "undo <id>...", "把任务恢复为未完成", "已恢复"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			// 逐个处理，某个 ID 失败不影响其他 ID；最后用 errors.Join 汇总所有错误。
			var errs []error
			for _, id := range ids {
				t, err := a.svc.Complete(id, done)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				a.printf("%s %s #%d：%s\n", a.palette.Green("✔"), verb, t.ID, t.Title)
			}
			return errors.Join(errs...)
		},
	}
}

func (a *app) newEditCmd() *cobra.Command {
	var title, priority, due string
	var tags []string
	cmd := &cobra.Command{
		Use:     "edit <id>",
		Short:   "修改任务",
		Example: "  todo edit 3 --title 新标题 -p high --due none",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			var in todo.UpdateInput
			// Flags().Changed 区分"没传"与"传了空值"——这正是指针字段想表达的语义。
			if cmd.Flags().Changed("title") {
				in.Title = &title
			}
			if cmd.Flags().Changed("priority") {
				p, err := todo.ParsePriority(priority)
				if err != nil {
					return err
				}
				in.Priority = &p
			}
			if cmd.Flags().Changed("due") {
				var d time.Time
				if due != "none" {
					if d, err = todo.ParseDue(due, a.svc.Now()); err != nil {
						return err
					}
				}
				in.Due = &d
			}
			if cmd.Flags().Changed("tags") {
				in.Tags = &tags
			}
			if in == (todo.UpdateInput{}) {
				return fmt.Errorf("没有要修改的字段，请至少指定 --title/--priority/--due/--tags 之一")
			}
			t, err := a.svc.Update(ids[0], in)
			if err != nil {
				return err
			}
			a.printf("%s 已更新 #%d：%s\n", a.palette.Green("✔"), t.ID, t.Title)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "新标题")
	f.StringVarP(&priority, "priority", "p", "", "新优先级")
	f.StringVarP(&due, "due", "d", "", "新截止日期，none 表示清除")
	f.StringSliceVarP(&tags, "tags", "t", nil, "新标签（整体替换）")
	return cmd
}

func (a *app) newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rm <id>...",
		Aliases: []string{"delete"},
		Short:   "删除任务",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs(args)
			if err != nil {
				return err
			}
			var errs []error
			for _, id := range ids {
				t, err := a.svc.Remove(id)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				a.printf("%s 已删除 #%d：%s\n", a.palette.Yellow("✘"), t.ID, t.Title)
			}
			return errors.Join(errs...)
		},
	}
}

func (a *app) newClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "清理所有已完成任务",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n, err := a.svc.ClearDone()
			if err != nil {
				return err
			}
			a.printf("已清理 %d 个已完成任务\n", n)
			return nil
		},
	}
}
