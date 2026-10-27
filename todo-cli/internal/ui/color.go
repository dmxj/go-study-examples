// Package ui 负责终端输出：ANSI 颜色与对齐的表格。
package ui

import (
	"io"
	"os"
)

// ANSI 转义序列。终端看到 "\x1b[31m" 会把后续文字变成红色，"\x1b[0m" 恢复默认。
const (
	reset  = "\x1b[0m"
	bold   = "\x1b[1m"
	dim    = "\x1b[2m"
	strike = "\x1b[9m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
)

// Palette 根据是否启用颜色返回着色函数。禁用时所有函数原样返回字符串，
// 这样调用方无需到处写 if enabled。
type Palette struct {
	Enabled bool
}

func (p Palette) wrap(code, s string) string {
	if !p.Enabled || s == "" {
		return s
	}
	return code + s + reset
}

// Red 等方法给字符串着色。
func (p Palette) Red(s string) string    { return p.wrap(red, s) }
func (p Palette) Green(s string) string  { return p.wrap(green, s) }
func (p Palette) Yellow(s string) string { return p.wrap(yellow, s) }
func (p Palette) Cyan(s string) string   { return p.wrap(cyan, s) }
func (p Palette) Bold(s string) string   { return p.wrap(bold, s) }
func (p Palette) Dim(s string) string    { return p.wrap(dim, s) }
func (p Palette) Strike(s string) string { return p.wrap(dim+strike, s) }

// ColorEnabled 判断是否应该输出颜色：
//   - 用户显式 --no-color 时关闭；
//   - 遵循 https://no-color.org 约定，设置了 NO_COLOR 环境变量时关闭；
//   - 输出不是终端（被重定向到文件或管道）时关闭，避免把转义码写进文件。
func ColorEnabled(w io.Writer, noColorFlag bool) bool {
	if noColorFlag || os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
