// Package config 从环境变量加载服务配置。
//
// 采用 12-Factor 风格：所有配置都来自环境变量，并提供适合本地开发的默认值。
// 这样同一个二进制 / 镜像可以不改代码地运行在开发、测试、生产环境。
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Config 是服务的全部配置项。
type Config struct {
	Addr            string        // 监听地址，APP_ADDR，默认 :8080
	DSN             string        // SQLite 数据库文件，APP_DB，默认 data/books.db
	JWTSecret       string        // JWT 签名密钥，APP_JWT_SECRET（生产环境必填）
	JWTTTL          time.Duration // Token 有效期，APP_JWT_TTL，默认 24h
	Mode            string        // 运行模式 debug/release/test，APP_MODE，默认 debug
	LogLevel        slog.Level    // 日志级别，APP_LOG_LEVEL，默认 info
	ShutdownTimeout time.Duration // 优雅关闭超时，APP_SHUTDOWN_TIMEOUT，默认 10s
}

// devSecret 仅用于本地开发。release 模式下禁止使用。
const devSecret = "dev-secret-change-me"

// Load 读取环境变量并校验。lookup 通常传 os.LookupEnv，测试时可替换。
func Load(lookup func(string) (string, bool)) (*Config, error) {
	get := func(key, def string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}
	cfg := &Config{
		Addr:      get("APP_ADDR", ":8080"),
		DSN:       get("APP_DB", "data/books.db"),
		JWTSecret: get("APP_JWT_SECRET", devSecret),
		Mode:      get("APP_MODE", "debug"),
	}

	var errs []error
	var err error
	if cfg.JWTTTL, err = time.ParseDuration(get("APP_JWT_TTL", "24h")); err != nil {
		errs = append(errs, fmt.Errorf("APP_JWT_TTL: %w", err))
	}
	if cfg.ShutdownTimeout, err = time.ParseDuration(get("APP_SHUTDOWN_TIMEOUT", "10s")); err != nil {
		errs = append(errs, fmt.Errorf("APP_SHUTDOWN_TIMEOUT: %w", err))
	}
	// slog.Level 实现了 encoding.TextUnmarshaler，可以直接解析 "debug"/"info"/"warn"/"error"。
	if err := cfg.LogLevel.UnmarshalText([]byte(get("APP_LOG_LEVEL", "info"))); err != nil {
		errs = append(errs, fmt.Errorf("APP_LOG_LEVEL: %w", err))
	}
	switch cfg.Mode {
	case "debug", "release", "test":
	default:
		errs = append(errs, fmt.Errorf("APP_MODE 必须是 debug/release/test，实际 %q", cfg.Mode))
	}
	if cfg.Mode == "release" && cfg.JWTSecret == devSecret {
		errs = append(errs, errors.New("release 模式必须通过 APP_JWT_SECRET 设置 JWT 密钥"))
	}
	if len(cfg.JWTSecret) < 16 {
		errs = append(errs, errors.New("APP_JWT_SECRET 长度至少 16 个字符"))
	}
	// 一次性返回所有配置错误，而不是改一个报一个。
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return cfg, nil
}

// FromEnv 是 Load(os.LookupEnv) 的便捷写法。
func FromEnv() (*Config, error) { return Load(os.LookupEnv) }
