// Package config 从命令行参数和环境变量加载聊天室配置。
package config

import (
	"flag"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 是聊天室配置。
type Config struct {
	Addr            string
	AllowedOrigins  []string
	HistorySize     int
	SendBuffer      int
	ShutdownTimeout time.Duration
	Debug           bool
}

// Load 解析参数。优先级：命令行参数 > 环境变量 > 默认值。
// 做法是先用环境变量算出"默认值"，再交给 flag 覆盖。
func Load(args []string, stderr io.Writer) (*Config, error) {
	fs := flag.NewFlagSet("chatroom", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := &Config{}
	var origins string
	fs.StringVar(&cfg.Addr, "addr", env("CHAT_ADDR", ":8080"), "监听地址 [CHAT_ADDR]")
	fs.StringVar(&origins, "origins", env("CHAT_ALLOWED_ORIGINS", ""), "额外允许的跨域 Origin，逗号分隔 [CHAT_ALLOWED_ORIGINS]")
	fs.IntVar(&cfg.HistorySize, "history", envInt("CHAT_HISTORY", 50), "每个房间保留的历史消息数 [CHAT_HISTORY]")
	fs.IntVar(&cfg.SendBuffer, "send-buffer", envInt("CHAT_SEND_BUFFER", 64), "每个客户端的发送队列长度 [CHAT_SEND_BUFFER]")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", 5*time.Second, "优雅关闭超时")
	fs.BoolVar(&cfg.Debug, "debug", false, "调试日志")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}
