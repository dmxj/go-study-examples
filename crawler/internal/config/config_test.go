package config

import (
	"io"
	"testing"
)

func TestParseOK(t *testing.T) {
	cfg, err := Parse([]string{"-workers", "8", "-depth", "1", "-format", "JSONL", "https://go.dev/"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Crawler.Workers != 8 || cfg.Crawler.MaxDepth != 1 || cfg.Format != "jsonl" || len(cfg.Seeds) != 1 {
		t.Errorf("解析结果不正确: %+v", cfg)
	}
	if !cfg.Crawler.RespectRobots || !cfg.Crawler.SameHost {
		t.Error("默认应遵守 robots 且只抓同主机")
	}
}

func TestParseErrors(t *testing.T) {
	cases := [][]string{
		{},                // 缺少种子
		{"ftp://x.test/"}, // 非 http(s)
		{"-workers", "0", "http://x.test/"},
		{"-unknown-flag", "http://x.test/"},
	}
	for _, args := range cases {
		if _, err := Parse(args, io.Discard); err == nil {
			t.Errorf("Parse(%v) 应返回错误", args)
		}
	}
}
