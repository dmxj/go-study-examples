// Package service 实现短链接业务：创建、解析（带缓存）、统计。
package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/dmxj/go-study-examples/shortener/internal/cache"
	"github.com/dmxj/go-study-examples/shortener/internal/stats"
	"github.com/dmxj/go-study-examples/shortener/internal/store"
)

// 业务错误。
var (
	ErrInvalidURL  = errors.New("URL 无效：需要 http(s) 协议且长度不超过 2048")
	ErrInvalidCode = errors.New("自定义短码只能包含字母、数字、- 和 _，长度 4~32")
	ErrInvalidTTL  = errors.New("有效期无效：需为正数时长，如 24h，最长 8760h")
	ErrSelfLink    = errors.New("不能为本服务的短链接再生成短链接")
	ErrNotFound    = store.ErrNotFound
	ErrCodeTaken   = store.ErrCodeTaken
	ErrExpired     = errors.New("短链接已过期")
)

// Store 是 service 需要的存储能力。
type Store interface {
	Create(ctx context.Context, url, code string, now, expiresAt time.Time) (*store.Link, error)
	Get(ctx context.Context, code string) (*store.Link, error)
}

// codeRe 同时用于校验自定义短码与路由中的短码（生成的短码是 base62，必然匹配）。
var codeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// reserved 是与路由冲突的保留字，不能用作自定义短码（否则 /healthz 会被"抢走"）。
var reserved = map[string]bool{"api": true, "healthz": true, "admin": true, "static": true}

// ValidCode 报告 code 是否可能是合法短码。用于在查库前快速拒绝明显非法的请求。
func ValidCode(code string) bool { return codeRe.MatchString(code) }

// Service 组合存储、缓存与统计。
type Service struct {
	store    Store
	cache    *cache.LRU[string, store.Link]
	recorder *stats.Recorder
	selfHost string
	now      func() time.Time
}

// New 创建 Service。baseURL 是对外的短链前缀（如 https://s.example.com），用于拒绝自引用。
func New(st Store, c *cache.LRU[string, store.Link], rec *stats.Recorder, baseURL string) *Service {
	host := ""
	if u, err := url.Parse(baseURL); err == nil {
		host = strings.ToLower(u.Host)
	}
	return &Service{store: st, cache: c, recorder: rec, selfHost: host, now: time.Now}
}

// CreateInput 是创建短链接的参数。
type CreateInput struct {
	URL  string
	Code string        // 可选：自定义短码
	TTL  time.Duration // 可选：有效期，0 表示永久
}

// Create 校验参数并创建短链接。
func (s *Service) Create(ctx context.Context, in CreateInput) (*store.Link, error) {
	target, err := s.validateURL(in.URL)
	if err != nil {
		return nil, err
	}
	if in.Code != "" && (len(in.Code) < 4 || !codeRe.MatchString(in.Code) || reserved[strings.ToLower(in.Code)]) {
		return nil, ErrInvalidCode
	}
	if in.TTL < 0 || in.TTL > 365*24*time.Hour {
		return nil, ErrInvalidTTL
	}
	now := s.now()
	var expires time.Time
	if in.TTL > 0 {
		expires = now.Add(in.TTL)
	}
	link, err := s.store.Create(ctx, target, in.Code, now, expires)
	if err != nil {
		return nil, fmt.Errorf("创建短链接: %w", err)
	}
	s.cache.Add(link.Code, *link) // 写入后预热缓存：新链接往往马上就会被访问
	return link, nil
}

// Resolve 解析短码并记录一次访问。这是最热的路径：先查 LRU，未命中再查库。
func (s *Service) Resolve(ctx context.Context, code string) (string, error) {
	if !ValidCode(code) {
		return "", ErrNotFound
	}
	link, ok := s.cache.Get(code)
	if !ok {
		l, err := s.store.Get(ctx, code)
		if err != nil {
			return "", err
		}
		link = *l
		s.cache.Add(code, link)
	}
	now := s.now()
	if link.Expired(now) {
		return "", ErrExpired
	}
	s.recorder.Record(code, now)
	return link.URL, nil
}

// Stats 返回链接详情，访问次数 = 已落库次数 + 内存中待写入次数。
func (s *Service) Stats(ctx context.Context, code string) (*store.Link, error) {
	if !ValidCode(code) {
		return nil, ErrNotFound
	}
	l, err := s.store.Get(ctx, code)
	if err != nil {
		return nil, err
	}
	p := s.recorder.Pending(code)
	l.Clicks += p.Count
	if p.LastAccess.After(l.LastAccessAt) {
		l.LastAccessAt = p.LastAccess
	}
	return l, nil
}

func (s *Service) validateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", ErrInvalidURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", ErrInvalidURL
	}
	if s.selfHost != "" && strings.EqualFold(u.Host, s.selfHost) {
		return "", ErrSelfLink
	}
	return u.String(), nil
}
