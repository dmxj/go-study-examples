package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dmxj/go-study-examples/shortener/internal/cache"
	"github.com/dmxj/go-study-examples/shortener/internal/httpapi"
	"github.com/dmxj/go-study-examples/shortener/internal/ratelimit"
	"github.com/dmxj/go-study-examples/shortener/internal/service"
	"github.com/dmxj/go-study-examples/shortener/internal/stats"
	"github.com/dmxj/go-study-examples/shortener/internal/store"
)

type testEnv struct {
	srv    *httptest.Server
	client *http.Client
}

func newEnv(t *testing.T, burst int) *testEnv {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	rec := stats.NewRecorder(db, time.Hour, log)
	t.Cleanup(func() {
		// 注意不能用 t.Context()：它在 Cleanup 函数执行之前就已被取消
		rec.Close(context.Background())
		db.Close()
	})
	api := &httpapi.API{
		Svc:     service.New(db, cache.New[string, store.Link](100), rec, "http://s.test"),
		BaseURL: "http://s.test",
		Limiter: ratelimit.New(0.001, burst), // 几乎不补充令牌，便于测试 429
		Log:     log,
	}
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return &testEnv{
		srv: srv,
		// 不自动跟随重定向，这样才能断言 302 与 Location
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

func (e *testEnv) create(t *testing.T, body string) (int, map[string]any) {
	t.Helper()
	resp, err := e.client.Post(e.srv.URL+"/api/links", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var m map[string]any
	json.NewDecoder(resp.Body).Decode(&m)
	return resp.StatusCode, m
}

func (e *testEnv) get(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := e.client.Get(e.srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestCreateRedirectStats(t *testing.T) {
	e := newEnv(t, 10)
	code, body := e.create(t, `{"url":"https://go.dev/doc/"}`)
	if code != http.StatusCreated || body["code"] != "1" || body["short_url"] != "http://s.test/1" {
		t.Fatalf("创建 = %d %v", code, body)
	}

	for i := 0; i < 3; i++ {
		resp := e.get(t, "/1")
		if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "https://go.dev/doc/" {
			t.Fatalf("跳转 = %d %s", resp.StatusCode, resp.Header.Get("Location"))
		}
	}

	resp := e.get(t, "/api/links/1")
	var st map[string]any
	json.NewDecoder(resp.Body).Decode(&st)
	// 统计尚未落库（flush 间隔 1 小时），但接口应合并内存中的计数
	if st["clicks"] != float64(3) || st["last_access_at"] == nil {
		t.Errorf("统计 = %v，期望 clicks=3", st)
	}
}

func TestCustomCodeAndErrors(t *testing.T) {
	e := newEnv(t, 20)
	if code, _ := e.create(t, `{"url":"https://go.dev","code":"golang"}`); code != 201 {
		t.Fatalf("自定义短码创建失败 %d", code)
	}
	tests := []struct {
		name string
		body string
		want int
	}{
		{"短码重复", `{"url":"https://a.test","code":"golang"}`, 409},
		{"非 http 协议", `{"url":"javascript:alert(1)"}`, 400},
		{"自引用", `{"url":"http://s.test/1"}`, 400},
		{"短码过短", `{"url":"https://a.test","code":"ab"}`, 400},
		{"保留字", `{"url":"https://a.test","code":"healthz"}`, 400},
		{"未知字段", `{"uri":"https://a.test"}`, 400},
		{"非法 TTL", `{"url":"https://a.test","ttl":"1天"}`, 400},
		{"非 JSON", `url=https://a.test`, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, body := e.create(t, tt.body); code != tt.want {
				t.Errorf("状态码 = %d，期望 %d，响应 %v", code, tt.want, body)
			}
		})
	}
	if r := e.get(t, "/nope"); r.StatusCode != 404 {
		t.Errorf("不存在的短码应 404，实际 %d", r.StatusCode)
	}
	if r := e.get(t, "/healthz"); r.StatusCode != 200 {
		t.Errorf("/healthz 应 200（不能被 /{code} 抢走），实际 %d", r.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodDelete, e.srv.URL+"/golang", nil)
	resp, _ := e.client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("DELETE 应返回 405，实际 %d", resp.StatusCode)
	}
}

func TestExpiredLink(t *testing.T) {
	e := newEnv(t, 10)
	_, body := e.create(t, `{"url":"https://go.dev","ttl":"50ms"}`)
	code := body["code"].(string)
	if r := e.get(t, "/"+code); r.StatusCode != http.StatusFound {
		t.Fatalf("未过期时应跳转，实际 %d", r.StatusCode)
	}
	time.Sleep(80 * time.Millisecond)
	if r := e.get(t, "/"+code); r.StatusCode != http.StatusGone {
		t.Errorf("过期后应返回 410，实际 %d", r.StatusCode)
	}
}

func TestRateLimit(t *testing.T) {
	e := newEnv(t, 2)
	e.create(t, `{"url":"https://a.test"}`)
	e.create(t, `{"url":"https://b.test"}`)
	resp, err := e.client.Post(e.srv.URL+"/api/links", "application/json", strings.NewReader(`{"url":"https://c.test"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Errorf("超过突发上限应返回 429 + Retry-After，实际 %d %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	// 限流只作用于创建接口，跳转不受影响
	if r := e.get(t, "/1"); r.StatusCode != http.StatusFound {
		t.Errorf("跳转不应被限流，实际 %d", r.StatusCode)
	}
}
