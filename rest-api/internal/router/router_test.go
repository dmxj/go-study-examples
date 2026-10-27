package router_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/dmxj/go-study-examples/rest-api/internal/auth"
	"github.com/dmxj/go-study-examples/rest-api/internal/database"
	"github.com/dmxj/go-study-examples/rest-api/internal/model"
	"github.com/dmxj/go-study-examples/rest-api/internal/response"
	"github.com/dmxj/go-study-examples/rest-api/internal/router"
)

// testServer 封装一个基于 httptest.Server 的完整服务，每个测试使用独立的内存数据库。
type testServer struct {
	t   *testing.T
	srv *httptest.Server
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// 每个测试用不同名字的共享内存库，互不干扰；cache=shared 让连接池中的多个连接看到同一个库。
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := database.Open(dsn, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close(db) })

	engine := router.New(router.Deps{
		DB:         db,
		Tokens:     auth.NewTokenManager("test-secret-0123456789", time.Hour),
		Log:        log,
		BcryptCost: bcrypt.MinCost,
		GinMode:    "test",
	})
	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)
	return &testServer{t: t, srv: srv}
}

// do 发送请求并把响应解析为统一结构；data 非 nil 时把 Body.Data 解析进去。
func (s *testServer) do(method, path, token string, body any, data any) (int, response.Body) {
	s.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, s.srv.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.srv.Client().Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()

	var raw struct {
		response.Body
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		s.t.Fatalf("%s %s 响应不是合法 JSON: %v", method, path, err)
	}
	if data != nil && len(raw.Data) > 0 {
		if err := json.Unmarshal(raw.Data, data); err != nil {
			s.t.Fatal(err)
		}
	}
	return resp.StatusCode, raw.Body
}

// login 注册并登录，返回 token。
func (s *testServer) login(username string) string {
	s.t.Helper()
	cred := map[string]string{"username": username, "password": "password123"}
	if code, b := s.do("POST", "/api/v1/auth/register", "", cred, nil); code != http.StatusCreated {
		s.t.Fatalf("注册失败: %d %+v", code, b)
	}
	var tok model.TokenResponse
	if code, b := s.do("POST", "/api/v1/auth/login", "", cred, &tok); code != http.StatusOK {
		s.t.Fatalf("登录失败: %d %+v", code, b)
	}
	return tok.Token
}

func TestAuthFlow(t *testing.T) {
	s := newTestServer(t)
	token := s.login("alice")

	var me model.User
	if code, _ := s.do("GET", "/api/v1/users/me", token, nil, &me); code != 200 || me.Username != "alice" {
		t.Errorf("/users/me = %d %+v", code, me)
	}

	tests := []struct {
		name   string
		method string
		path   string
		token  string
		body   any
		want   int
	}{
		{"重复注册", "POST", "/api/v1/auth/register", "", map[string]string{"username": "alice", "password": "password123"}, 409},
		{"密码过短", "POST", "/api/v1/auth/register", "", map[string]string{"username": "bob", "password": "123"}, 400},
		{"密码超过 72 字节", "POST", "/api/v1/auth/register", "", map[string]string{"username": "bob", "password": strings.Repeat("密", 30)}, 400},
		{"密码错误", "POST", "/api/v1/auth/login", "", map[string]string{"username": "alice", "password": "wrong-password"}, 401},
		{"用户不存在", "POST", "/api/v1/auth/login", "", map[string]string{"username": "nobody", "password": "password123"}, 401},
		{"未带 token", "GET", "/api/v1/users/me", "", nil, 401},
		{"伪造 token", "GET", "/api/v1/users/me", token + "x", nil, 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, b := s.do(tt.method, tt.path, tt.token, tt.body, nil); code != tt.want {
				t.Errorf("状态码 = %d，期望 %d，响应 %+v", code, tt.want, b)
			}
		})
	}
}

func TestBookCRUD(t *testing.T) {
	s := newTestServer(t)
	token := s.login("alice")

	newBook := map[string]any{"title": "Go 程序设计语言", "author": "Donovan", "isbn": "978-7-111-55842-2", "price": 7900, "year": 2016, "stock": 3}
	if code, _ := s.do("POST", "/api/v1/books", "", newBook, nil); code != 401 {
		t.Errorf("未登录创建应返回 401，实际 %d", code)
	}

	var created model.Book
	code, body := s.do("POST", "/api/v1/books", token, newBook, &created)
	if code != 201 || created.ID == 0 || created.ISBN != "9787111558422" {
		t.Fatalf("创建图书 = %d %+v %+v", code, body, created)
	}
	if code, _ := s.do("POST", "/api/v1/books", token, newBook, nil); code != 409 {
		t.Errorf("ISBN 重复应返回 409，实际 %d", code)
	}

	path := fmt.Sprintf("/api/v1/books/%d", created.ID)
	var got model.Book
	if code, _ := s.do("GET", path, "", nil, &got); code != 200 || got.Title != created.Title {
		t.Errorf("查询图书 = %d %+v", code, got)
	}

	// 部分更新：只改库存为 0（验证指针字段能区分"未传"和"零值"）
	var updated model.Book
	if code, _ := s.do("PATCH", path, token, map[string]any{"stock": 0}, &updated); code != 200 {
		t.Fatalf("更新失败 %d", code)
	}
	if updated.Stock != 0 || updated.Price != 7900 {
		t.Errorf("PATCH 只应修改 stock，实际 %+v", updated)
	}

	if code, _ := s.do("DELETE", path, token, nil, nil); code != 200 {
		t.Errorf("删除失败 %d", code)
	}
	if code, b := s.do("GET", path, "", nil, nil); code != 404 || b.Code != response.CodeNotFound {
		t.Errorf("删除后查询应 404，实际 %d %+v", code, b)
	}
	if code, _ := s.do("DELETE", path, token, nil, nil); code != 404 {
		t.Errorf("重复删除应 404，实际 %d", code)
	}
}

func TestBookValidation(t *testing.T) {
	s := newTestServer(t)
	token := s.login("alice")
	tests := []struct {
		name    string
		body    map[string]any
		wantMsg string
	}{
		{"缺少标题", map[string]any{"author": "a", "isbn": "9787111558422"}, "title 为必填字段"},
		{"非法 ISBN", map[string]any{"title": "t", "author": "a", "isbn": "123"}, "isbn 不是合法的 ISBN"},
		{"负价格", map[string]any{"title": "t", "author": "a", "isbn": "9787111558422", "price": -1}, "price 不能小于 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, b := s.do("POST", "/api/v1/books", token, tt.body, nil)
			if code != 400 || !strings.Contains(b.Message, tt.wantMsg) {
				t.Errorf("= %d %q，期望 400 且包含 %q", code, b.Message, tt.wantMsg)
			}
		})
	}
	if code, _ := s.do("GET", "/api/v1/books/abc", "", nil, nil); code != 400 {
		t.Errorf("非数字 id 应返回 400，实际 %d", code)
	}
	if code, _ := s.do("GET", "/api/v1/books?page_size=1000", "", nil, nil); code != 400 {
		t.Errorf("page_size 超过上限应返回 400，实际 %d", code)
	}
	if code, _ := s.do("GET", "/api/v1/books?sort=title%20DESC", "", nil, nil); code != 400 {
		t.Errorf("非白名单排序字段应返回 400，实际 %d", code)
	}
}

func TestBookListPagination(t *testing.T) {
	s := newTestServer(t)
	token := s.login("alice")
	isbns := []string{"9787111558422", "9787115480064", "9787121324260", "9787302423287", "9787508353937"}
	for i, isbn := range isbns {
		author := "张三"
		if i%2 == 1 {
			author = "李四"
		}
		b := map[string]any{"title": fmt.Sprintf("Go 实战 %d", i+1), "author": author, "isbn": isbn, "price": (i + 1) * 1000}
		if code, body := s.do("POST", "/api/v1/books", token, b, nil); code != 201 {
			t.Fatalf("创建第 %d 本失败: %d %+v", i+1, code, body)
		}
	}

	var page model.Page[model.Book]
	s.do("GET", "/api/v1/books?page=2&page_size=2&sort=price", "", nil, &page)
	if page.Total != 5 || len(page.Items) != 2 || page.Items[0].Price != 3000 {
		t.Errorf("第 2 页（按价格升序）= total %d, items %+v", page.Total, page.Items)
	}

	s.do("GET", "/api/v1/books?author="+url.QueryEscape("李四"), "", nil, &page)
	if page.Total != 2 {
		t.Errorf("按作者过滤 total = %d，期望 2", page.Total)
	}

	s.do("GET", "/api/v1/books?q=%25", "", nil, &page) // q=% 不应匹配全部
	if page.Total != 0 {
		t.Errorf("LIKE 通配符应被转义，q=%% 期望 0 条，实际 %d", page.Total)
	}
}

func TestHealthAndNoRoute(t *testing.T) {
	s := newTestServer(t)
	resp, err := http.Get(s.srv.URL + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz: %v %v", err, resp)
	}
	resp.Body.Close()
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("响应应包含 X-Request-ID")
	}
	if code, b := s.do("GET", "/nope", "", nil, nil); code != 404 || b.Code != response.CodeNotFound {
		t.Errorf("未知路由 = %d %+v", code, b)
	}
}
