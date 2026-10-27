// Package fetcher 负责下载网页。
package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"
)

// Response 是一次抓取的结果。
type Response struct {
	URL         *url.URL // 跟随重定向后的最终地址
	StatusCode  int
	ContentType string
	Body        []byte // 仅当内容是 HTML 时才读取
	IsHTML      bool
}

// Fetcher 是对 http.Client 的薄封装：统一 UA、限制响应大小、只读取 HTML。
type Fetcher struct {
	Client    *http.Client
	UserAgent string
	MaxBytes  int64 // 单页最大读取字节数
}

// New 创建 Fetcher。
//
// 注意：生产中不要用 http.DefaultClient——它没有超时，一个卡住的服务器就能让 goroutine 永远挂起。
func New(userAgent string, timeout time.Duration, maxBytes int64) *Fetcher {
	return &Fetcher{
		Client: &http.Client{
			Timeout: timeout,
			// 最多跟随 5 次重定向
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("重定向次数过多")
				}
				return nil
			},
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment, // 尊重 HTTP(S)_PROXY 环境变量
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     30 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		UserAgent: userAgent,
		MaxBytes:  maxBytes,
	}
}

// Fetch 下载 u。ctx 取消时请求立即中止。
func (f *Fetcher) Fetch(ctx context.Context, u *url.URL) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")

	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	// 必须关闭 Body，否则底层 TCP 连接无法复用，还会泄漏文件描述符。
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)
	out := &Response{
		URL:         resp.Request.URL,
		StatusCode:  resp.StatusCode,
		ContentType: mediaType,
		IsHTML:      mediaType == "text/html" || mediaType == "application/xhtml+xml",
	}
	if resp.StatusCode != http.StatusOK || !out.IsHTML {
		// 不需要内容时，丢弃少量剩余数据，帮助连接复用
		_, _ = io.CopyN(io.Discard, resp.Body, 4<<10)
		return out, nil
	}
	// LimitReader 防止超大页面（或恶意的无限响应）耗尽内存
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.MaxBytes))
	if err != nil {
		return nil, fmt.Errorf("读取响应: %w", err)
	}
	out.Body = body
	return out, nil
}
