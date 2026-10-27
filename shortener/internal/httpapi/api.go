// Package httpapi 是短链接服务的 HTTP 层，使用 Go 1.22+ 标准库增强路由。
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/dmxj/go-study-examples/shortener/internal/ratelimit"
	"github.com/dmxj/go-study-examples/shortener/internal/service"
	"github.com/dmxj/go-study-examples/shortener/internal/store"
)

// API 持有 HTTP 层依赖。
type API struct {
	Svc        *service.Service
	BaseURL    string             // 短链前缀，如 http://localhost:8080
	Limiter    *ratelimit.Limiter // 仅作用于创建接口
	TrustProxy bool               // 是否信任 X-Forwarded-For
	Log        *slog.Logger
	Health     func(r *http.Request) error
}

// Handler 注册路由并套上中间件。
//
// Go 1.22 起 ServeMux 支持 "方法 路径" 与 {通配符}：
//   - "GET /{code}" 匹配 /abc，r.PathValue("code") 取值；
//   - "GET /{$}" 只匹配根路径 "/"（不加 {$} 的 "/" 会匹配所有路径）；
//   - 多个模式都匹配时，更具体的优先，所以 /healthz 不会被 /{code} 抢走。
//   - 方法不匹配时自动返回 405 并带 Allow 头。
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.index)
	mux.HandleFunc("GET /healthz", a.health)
	mux.Handle("POST /api/links", a.rateLimit(http.HandlerFunc(a.create)))
	mux.HandleFunc("GET /api/links/{code}", a.stats)
	mux.HandleFunc("GET /{code}", a.redirect)
	return a.recoverer(a.logging(mux))
}

type createRequest struct {
	URL  string `json:"url"`
	Code string `json:"code,omitempty"`
	TTL  string `json:"ttl,omitempty"` // 如 "24h"、"30m"
}

type linkResponse struct {
	Code         string     `json:"code"`
	ShortURL     string     `json:"short_url"`
	URL          string     `json:"url"`
	Clicks       int64      `json:"clicks"`
	CreatedAt    time.Time  `json:"created_at"`
	LastAccessAt *time.Time `json:"last_access_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

func (a *API) toResponse(l *store.Link) linkResponse {
	resp := linkResponse{
		Code: l.Code, ShortURL: strings.TrimRight(a.BaseURL, "/") + "/" + l.Code,
		URL: l.URL, Clicks: l.Clicks, CreatedAt: l.CreatedAt,
	}
	if !l.LastAccessAt.IsZero() {
		resp.LastAccessAt = &l.LastAccessAt
	}
	if !l.ExpiresAt.IsZero() {
		resp.ExpiresAt = &l.ExpiresAt
	}
	return resp
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	// 限制请求体大小，防止超大请求耗尽内存
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // 拼错字段名（如 "uri"）时报错，而不是静默忽略
	var req createRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法 JSON: "+err.Error())
		return
	}
	var ttl time.Duration
	if req.TTL != "" {
		d, err := time.ParseDuration(req.TTL)
		if err != nil {
			writeError(w, http.StatusBadRequest, service.ErrInvalidTTL.Error())
			return
		}
		ttl = d
	}
	link, err := a.Svc.Create(r.Context(), service.CreateInput{URL: req.URL, Code: req.Code, TTL: ttl})
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	resp := a.toResponse(link)
	w.Header().Set("Location", resp.ShortURL)
	writeJSON(w, http.StatusCreated, resp)
}

func (a *API) redirect(w http.ResponseWriter, r *http.Request) {
	target, err := a.Svc.Resolve(r.Context(), r.PathValue("code"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	// 用 302 而不是 301：301 会被浏览器永久缓存，之后的访问不再经过我们，统计就失真了。
	w.Header().Set("Cache-Control", "private, no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	link, err := a.Svc.Stats(r.Context(), r.PathValue("code"))
	if err != nil {
		a.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a.toResponse(link))
}

func (a *API) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("短链接服务\n\n" +
		"创建: curl -X POST " + a.BaseURL + "/api/links -d '{\"url\":\"https://go.dev\"}'\n" +
		"统计: curl " + a.BaseURL + "/api/links/<code>\n"))
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if a.Health != nil {
		if err := a.Health(r); err != nil {
			writeError(w, http.StatusServiceUnavailable, "unhealthy")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeServiceError 把业务错误映射为 HTTP 状态码。
func (a *API) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var maxErr *http.MaxBytesError
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, service.ErrNotFound.Error())
	case errors.Is(err, service.ErrExpired):
		writeError(w, http.StatusGone, err.Error()) // 410：资源曾经存在但已永久失效
	case errors.Is(err, service.ErrCodeTaken):
		writeError(w, http.StatusConflict, service.ErrCodeTaken.Error())
	case errors.Is(err, service.ErrInvalidURL), errors.Is(err, service.ErrInvalidCode),
		errors.Is(err, service.ErrInvalidTTL), errors.Is(err, service.ErrSelfLink):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.As(err, &maxErr):
		writeError(w, http.StatusRequestEntityTooLarge, "请求体过大")
	default:
		a.Log.ErrorContext(r.Context(), "内部错误", "err", err, "path", r.URL.Path)
		writeError(w, http.StatusInternalServerError, "服务器内部错误")
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
