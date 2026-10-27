package config

import (
	"io"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("CHAT_ADDR", ":9999")
	t.Setenv("CHAT_HISTORY", "5")
	cfg, err := Load([]string{"-origins", "https://a.example, https://b.example"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9999" || cfg.HistorySize != 5 || len(cfg.AllowedOrigins) != 2 {
		t.Errorf("环境变量应作为默认值: %+v", cfg)
	}
	cfg, _ = Load([]string{"-addr", ":7000"}, io.Discard)
	if cfg.Addr != ":7000" {
		t.Errorf("命令行参数应覆盖环境变量，实际 %s", cfg.Addr)
	}
}
