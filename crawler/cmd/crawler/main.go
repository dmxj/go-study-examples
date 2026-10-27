// Command crawler 是一个并发网页爬虫。
//
//	crawler -depth 2 -workers 8 -rate 5 -o result.json https://go.dev/
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dmxj/go-study-examples/crawler/internal/config"
	"github.com/dmxj/go-study-examples/crawler/internal/crawler"
	"github.com/dmxj/go-study-examples/crawler/internal/output"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cfg, err := config.Parse(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 2
	}

	level := slog.LevelInfo
	if cfg.Verbose {
		level = slog.LevelDebug
	}
	// 日志写 stderr，结果写 stdout/文件：两者分离，管道 `crawler ... | jq` 才不会被日志污染。
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	var out io.Writer = os.Stdout
	if cfg.Output != "-" {
		f, err := os.Create(cfg.Output)
		if err != nil {
			log.Error("创建输出文件失败", "err", err)
			return 1
		}
		defer f.Close()
		out = f
	}
	w, err := output.New(out, cfg.Format)
	if err != nil {
		log.Error("初始化输出失败", "err", err)
		return 2
	}

	// Ctrl+C 时取消 ctx：正在进行的请求被中止，已抓到的结果仍会完整写出（JSON 数组正确闭合）。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.Deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Deadline)
		defer cancel()
	}

	c := crawler.New(cfg.Crawler, log)
	var writeErr error
	stats, err := c.Run(ctx, cfg.Seeds, func(r crawler.Result) {
		if writeErr == nil {
			writeErr = w.Write(r)
		}
	})
	if cerr := w.Close(); writeErr == nil {
		writeErr = cerr
	}

	log.Info("抓取结束", "pages", stats.Pages, "errors", stats.Errors,
		"skipped", stats.Skipped, "elapsed", stats.Elapsed.Round(time.Millisecond).String())
	switch {
	case writeErr != nil:
		log.Error("写出结果失败", "err", writeErr)
		return 1
	case errors.Is(err, context.Canceled):
		log.Warn("被用户中断，已输出部分结果")
		return 130
	case errors.Is(err, context.DeadlineExceeded):
		log.Warn("达到 -deadline 时长上限，已输出部分结果")
		return 0
	}
	return 0
}
