// Package output 把抓取结果写成 JSON。
package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/dmxj/go-study-examples/crawler/internal/crawler"
)

// Writer 是结果输出器。
type Writer interface {
	Write(r crawler.Result) error
	Close() error // 写入收尾内容（如 JSON 数组的 "]"），不关闭底层 io.Writer
}

// New 按格式创建输出器：json（一个 JSON 数组）或 jsonl（每行一个 JSON 对象）。
func New(w io.Writer, format string) (Writer, error) {
	switch format {
	case "json":
		return &arrayWriter{w: w}, nil
	case "jsonl", "ndjson":
		return &linesWriter{enc: json.NewEncoder(w)}, nil
	default:
		return nil, fmt.Errorf("不支持的输出格式 %q（可选 json/jsonl）", format)
	}
}

// linesWriter 输出 JSON Lines：每个结果一行，适合流式处理（jq、日志系统、大数据量）。
type linesWriter struct{ enc *json.Encoder }

func (l *linesWriter) Write(r crawler.Result) error { return l.enc.Encode(r) }
func (l *linesWriter) Close() error                 { return nil }

// arrayWriter 流式输出一个 JSON 数组：不把所有结果攒在内存里再 Marshal，
// 而是边抓边写 "[", 元素, ",", 元素, ..., "]"。
type arrayWriter struct {
	w     io.Writer
	count int
}

func (a *arrayWriter) Write(r crawler.Result) error {
	b, err := json.MarshalIndent(r, "  ", "  ")
	if err != nil {
		return err
	}
	prefix := ",\n  "
	if a.count == 0 {
		prefix = "[\n  "
	}
	a.count++
	_, err = fmt.Fprintf(a.w, "%s%s", prefix, b)
	return err
}

func (a *arrayWriter) Close() error {
	if a.count == 0 {
		_, err := io.WriteString(a.w, "[]\n")
		return err
	}
	_, err := io.WriteString(a.w, "\n]\n")
	return err
}
