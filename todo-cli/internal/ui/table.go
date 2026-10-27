package ui

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/dmxj/go-study-examples/todo-cli/internal/todo"
)

// cell 是表格中的一个单元格：text 用于计算宽度，styled 是带颜色的最终输出。
// 必须分开保存——ANSI 转义码不占显示宽度，若直接对 styled 计算宽度会导致错位。
type cell struct {
	text   string
	styled string
}

// Table 把任务渲染为对齐的表格。
//
// 为什么不用 text/tabwriter？tabwriter 按字节/rune 计数，
// 既不认识 ANSI 颜色码，也不知道中文字符在终端占 2 列，表格会错位。
// 这里用 go-runewidth 计算"显示宽度"，手动补空格。
type Table struct {
	Palette Palette
	Now     time.Time
}

var headers = []string{"ID", "状态", "优先级", "标题", "截止日期", "标签"}

// Render 输出表格；任务为空时输出提示语。
func (t Table) Render(w io.Writer, tasks []todo.Task) error {
	if len(tasks) == 0 {
		_, err := fmt.Fprintln(w, t.Palette.Dim("（没有符合条件的任务）"))
		return err
	}
	rows := make([][]cell, 0, len(tasks)+1)
	head := make([]cell, len(headers))
	for i, h := range headers {
		head[i] = cell{text: h, styled: t.Palette.Bold(h)}
	}
	rows = append(rows, head)
	for _, task := range tasks {
		rows = append(rows, t.row(task))
	}

	// 计算每列最大显示宽度
	widths := make([]int, len(headers))
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], runewidth.StringWidth(c.text))
		}
	}

	var b strings.Builder
	for ri, r := range rows {
		for i, c := range r {
			b.WriteString(c.styled)
			if i < len(r)-1 { // 最后一列不补空格，避免行尾多余空白
				pad := widths[i] - runewidth.StringWidth(c.text) + 2
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		b.WriteByte('\n')
		if ri == 0 { // 表头下的分隔线
			total := 0
			for _, wd := range widths {
				total += wd + 2
			}
			b.WriteString(t.Palette.Dim(strings.Repeat("─", total-2)))
			b.WriteByte('\n')
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func (t Table) row(task todo.Task) []cell {
	p := t.Palette
	status := cell{text: "[ ]", styled: "[ ]"}
	if task.Done {
		status = cell{text: "[x]", styled: p.Green("[x]")}
	}

	prio := task.Priority.String()
	var prioStyled string
	switch task.Priority {
	case todo.PriorityHigh:
		prioStyled = p.Red(prio)
	case todo.PriorityMedium:
		prioStyled = p.Yellow(prio)
	default:
		prioStyled = p.Green(prio)
	}

	title := runewidth.Truncate(task.Title, 40, "…")
	titleStyled := title
	if task.Done {
		titleStyled = p.Strike(title)
	}

	due := cell{text: "-", styled: p.Dim("-")}
	if task.HasDue() {
		d := task.Due.Format(todo.DateLayout)
		switch {
		case task.Overdue(t.Now):
			d += " (逾期)"
			due = cell{text: d, styled: p.Bold(p.Red(d))}
		default:
			due = cell{text: d, styled: d}
		}
	}

	tags := strings.Join(task.Tags, ",")
	return []cell{
		{text: strconv.Itoa(task.ID), styled: p.Cyan(strconv.Itoa(task.ID))},
		status,
		{text: prio, styled: prioStyled},
		{text: title, styled: titleStyled},
		due,
		{text: tags, styled: p.Dim(tags)},
	}
}
