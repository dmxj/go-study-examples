package httpapi

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// statusRecorder 包装 ResponseWriter 以记录状态码，供访问日志使用。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap 让 http.ResponseController 能找到底层 ResponseWriter（Go 1.20+）。
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// logging 记录每个请求的方法、路径、状态码与耗时。
func (a *API) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		a.Log.Info("http",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"latency", time.Since(start).String(), "ip", a.clientIP(r))
	})
}

// recoverer 捕获 panic，返回 500。
//
// net/http 自身也会 recover handler 的 panic，但它的处理方式是直接断开连接并打印堆栈，
// 客户端只会看到"连接被重置"。自己 recover 可以返回规范的 JSON 错误并记录结构化日志。
func (a *API) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler { // 约定用于主动中止的 panic，按原样抛出
					panic(v)
				}
				a.Log.Error("panic", "value", v, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "服务器内部错误")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// rateLimit 按客户端 IP 限流，超限返回 429 与 Retry-After 头。
func (a *API) rateLimit(next http.Handler) http.Handler {
	if a.Limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, wait := a.Limiter.Allow(a.clientIP(r))
		if !ok {
			secs := int(math.Ceil(wait.Seconds()))
			w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
			writeError(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP 获取客户端 IP。
//
// X-Forwarded-For 可以被客户端随意伪造，只有部署在可信反向代理（Nginx、SLB）之后
// 才应该信任它，否则攻击者每次换一个伪造 IP 就能绕过限流。
func (a *API) clientIP(r *http.Request) string {
	if a.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(first)
		}
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
