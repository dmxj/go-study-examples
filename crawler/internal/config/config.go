// Package config 解析爬虫的命令行参数。
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/dmxj/go-study-examples/crawler/internal/crawler"
)

// Config 是命令行解析结果。
type Config struct {
	Seeds    []*url.URL
	Crawler  crawler.Options
	Output   string // 输出文件，"-" 表示标准输出
	Format   string // json / jsonl
	Verbose  bool
	Deadline time.Duration // 整体运行时长上限，0 不限
}

// Parse 解析参数。使用独立的 FlagSet（而不是全局 flag.CommandLine），便于测试。
func Parse(args []string, stderr io.Writer) (*Config, error) {
	fs := flag.NewFlagSet("crawler", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: crawler [选项] <种子URL>...")
		fmt.Fprintln(stderr, "示例: crawler -depth 2 -workers 8 -rate 5 -o result.json https://go.dev/")
		fs.PrintDefaults()
	}

	cfg := &Config{}
	o := &cfg.Crawler
	fs.IntVar(&o.Workers, "workers", 4, "并发 worker 数")
	fs.IntVar(&o.MaxDepth, "depth", 2, "最大抓取深度（种子为 0）")
	fs.IntVar(&o.MaxPages, "max-pages", 100, "最多抓取页面数，0 表示不限")
	fs.Float64Var(&o.Rate, "rate", 2, "全局限速：每秒请求数，0 表示不限")
	fs.IntVar(&o.Burst, "burst", 1, "限速令牌桶容量（允许的瞬时突发请求数）")
	fs.BoolVar(&o.SameHost, "same-host", true, "只抓取与种子相同主机的链接")
	fs.BoolVar(&o.RespectRobots, "robots", true, "遵守 robots.txt")
	fs.StringVar(&o.UserAgent, "ua", "go-study-crawler/1.0 (+https://github.com/dmxj/go-study-examples)", "User-Agent")
	fs.DurationVar(&o.Timeout, "timeout", 10*time.Second, "单个请求超时")
	fs.Int64Var(&o.MaxBodyBytes, "max-body", 2<<20, "单页最大读取字节数")
	fs.StringVar(&cfg.Output, "o", "-", "输出文件，- 表示标准输出")
	fs.StringVar(&cfg.Format, "format", "json", "输出格式 json/jsonl")
	fs.DurationVar(&cfg.Deadline, "deadline", 0, "整体运行时长上限，如 30s，0 表示不限")
	fs.BoolVar(&cfg.Verbose, "v", false, "输出调试日志")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return nil, errors.New("至少需要一个种子 URL")
	}
	for _, raw := range fs.Args() {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("无效的种子 URL %q：需要 http(s)://host/ 形式", raw)
		}
		cfg.Seeds = append(cfg.Seeds, u)
	}

	var errs []error
	if o.Workers < 1 || o.Workers > 256 {
		errs = append(errs, errors.New("-workers 取值范围 1~256"))
	}
	if o.MaxDepth < 0 {
		errs = append(errs, errors.New("-depth 不能为负数"))
	}
	if o.Rate < 0 {
		errs = append(errs, errors.New("-rate 不能为负数"))
	}
	cfg.Format = strings.ToLower(cfg.Format)
	return cfg, errors.Join(errs...)
}
