// Package config 解析短链接服务的配置（命令行参数，可由环境变量提供默认值）。
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Config 是服务配置。
type Config struct {
	Addr            string
	DBPath          string
	BaseURL         string
	CacheSize       int
	RatePerSec      float64
	Burst           int
	FlushInterval   time.Duration
	TrustProxy      bool
	ShutdownTimeout time.Duration
	Debug           bool
}

// Load 解析参数。每个参数都可以用 SHORT_ 前缀的环境变量设置默认值。
func Load(args []string, stderr io.Writer) (*Config, error) {
	fs := flag.NewFlagSet("shortener", flag.ContinueOnError)
	fs.SetOutput(stderr)
	c := &Config{}
	fs.StringVar(&c.Addr, "addr", env("SHORT_ADDR", ":8080"), "监听地址 [SHORT_ADDR]")
	fs.StringVar(&c.DBPath, "db", env("SHORT_DB", "shortener.db"), "SQLite 文件路径 [SHORT_DB]")
	fs.StringVar(&c.BaseURL, "base-url", env("SHORT_BASE_URL", "http://localhost:8080"), "对外短链前缀 [SHORT_BASE_URL]")
	fs.IntVar(&c.CacheSize, "cache-size", envInt("SHORT_CACHE_SIZE", 10000), "LRU 缓存条目数 [SHORT_CACHE_SIZE]")
	fs.Float64Var(&c.RatePerSec, "rate", envFloat("SHORT_RATE", 1), "每个 IP 每秒可创建的短链数 [SHORT_RATE]")
	fs.IntVar(&c.Burst, "burst", envInt("SHORT_BURST", 5), "每个 IP 的突发上限 [SHORT_BURST]")
	fs.DurationVar(&c.FlushInterval, "flush-interval", 5*time.Second, "访问统计写库间隔")
	fs.BoolVar(&c.TrustProxy, "trust-proxy", os.Getenv("SHORT_TRUST_PROXY") == "true", "信任 X-Forwarded-For（仅在反向代理之后开启）")
	fs.DurationVar(&c.ShutdownTimeout, "shutdown-timeout", 10*time.Second, "优雅关闭超时")
	fs.BoolVar(&c.Debug, "debug", false, "调试日志")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	var errs []error
	if u, err := url.Parse(c.BaseURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("-base-url 无效: %q", c.BaseURL))
	}
	if c.RatePerSec <= 0 || c.Burst < 1 {
		errs = append(errs, errors.New("-rate 必须 > 0 且 -burst 必须 ≥ 1"))
	}
	if c.FlushInterval <= 0 {
		errs = append(errs, errors.New("-flush-interval 必须 > 0"))
	}
	return c, errors.Join(errs...)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return v
	}
	return def
}

func envFloat(k string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(k), 64); err == nil {
		return v
	}
	return def
}
