package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

// env 把 map 转成 lookup 函数，避免测试修改真实环境变量。
func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.JWTTTL != 24*time.Hour || cfg.LogLevel != slog.LevelInfo || cfg.Mode != "debug" {
		t.Errorf("默认配置不正确: %+v", cfg)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"APP_ADDR": ":9000", "APP_JWT_TTL": "30m", "APP_LOG_LEVEL": "debug",
		"APP_MODE": "release", "APP_JWT_SECRET": "a-very-long-production-secret",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9000" || cfg.JWTTTL != 30*time.Minute || cfg.LogLevel != slog.LevelDebug {
		t.Errorf("覆盖配置不正确: %+v", cfg)
	}
}

func TestLoadErrorsJoined(t *testing.T) {
	_, err := Load(env(map[string]string{"APP_JWT_TTL": "1 day", "APP_MODE": "release"}))
	if err == nil {
		t.Fatal("期望返回错误")
	}
	// errors.Join 后所有错误一次性返回
	for _, want := range []string{"APP_JWT_TTL", "release 模式必须"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误信息应包含 %q，实际:\n%v", want, err)
		}
	}
}
