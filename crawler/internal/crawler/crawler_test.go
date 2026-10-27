package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"
)

// site 是一个用 httptest 搭建的本地测试站点，记录每个路径被访问的次数。
type site struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

func newSite(t *testing.T, pages map[string]string, robotsTxt string) *site {
	t.Helper()
	s := &site{hits: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		if robotsTxt == "" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, robotsTxt)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		s.mu.Unlock()
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, body)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *site) url(path string) *url.URL {
	u, _ := url.Parse(s.srv.URL + path)
	return u
}

func page(title string, links ...string) string {
	h := "<html><head><title>" + title + "</title></head><body>"
	for _, l := range links {
		h += fmt.Sprintf(`<a href="%s">%s</a>`, l, l)
	}
	return h + "</body></html>"
}

func testOptions() Options {
	return Options{
		Workers: 4, MaxDepth: 2, SameHost: true, RespectRobots: true,
		UserAgent: "test-bot", Timeout: 2 * time.Second, MaxBodyBytes: 1 << 20,
	}
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func run(t *testing.T, c *Crawler, ctx context.Context, seeds ...*url.URL) (map[string]Result, Stats, error) {
	t.Helper()
	got := map[string]Result{}
	stats, err := c.Run(ctx, seeds, func(r Result) {
		u, _ := url.Parse(r.URL)
		got[u.Path] = r
	})
	return got, stats, err
}

func TestCrawlDepthDedupRobots(t *testing.T) {
	s := newSite(t, map[string]string{
		"/":          page("首页", "/a", "/b", "/a#top", "/private/x", "mailto:hi@example.com", "https://other.invalid/"),
		"/a":         page("A", "/b", "/c", "/"),
		"/b":         page("B", "/a"),
		"/c":         page("C", "/d"), // /c 在深度 2，/d 在深度 3，不应被抓取
		"/d":         page("D"),
		"/private/x": page("secret"),
	}, "User-agent: *\nDisallow: /private\n")

	got, stats, err := run(t, New(testOptions(), quietLogger()), context.Background(), s.url("/"))
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	for p := range got {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	want := []string{"/", "/a", "/b", "/c", "/private/x"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Errorf("访问过的路径 = %v，期望 %v", paths, want)
	}
	if got["/"].Title != "首页" || got["/c"].Depth != 2 {
		t.Errorf("结果字段不正确: %+v %+v", got["/"], got["/c"])
	}
	if got["/private/x"].Skipped != "robots" {
		t.Errorf("/private/x 应因 robots 被跳过，实际 %+v", got["/private/x"])
	}
	if stats.Pages != 4 || stats.Skipped != 1 {
		t.Errorf("统计 = %+v，期望 4 页、1 跳过", stats)
	}
	// 去重：每个页面只被请求一次；被 robots 禁止的页面一次都不请求
	s.mu.Lock()
	defer s.mu.Unlock()
	for p, n := range s.hits {
		if n != 1 {
			t.Errorf("%s 被请求了 %d 次，期望 1 次", p, n)
		}
	}
	if s.hits["/private/x"] != 0 || s.hits["/d"] != 0 {
		t.Errorf("不应请求 /private/x 与 /d，hits=%v", s.hits)
	}
}

func TestCrawlMaxPages(t *testing.T) {
	pages := map[string]string{"/": page("root", "/1", "/2", "/3", "/4", "/5")}
	for i := 1; i <= 5; i++ {
		pages[fmt.Sprintf("/%d", i)] = page("p")
	}
	s := newSite(t, pages, "")
	opts := testOptions()
	opts.MaxPages = 3
	got, _, _ := run(t, New(opts, quietLogger()), context.Background(), s.url("/"))
	if len(got) != 3 {
		t.Errorf("MaxPages=3 时应只抓 3 页，实际 %d", len(got))
	}
}

func TestCrawlRateLimit(t *testing.T) {
	pages := map[string]string{"/": page("root", "/1", "/2", "/3", "/4")}
	for i := 1; i <= 4; i++ {
		pages[fmt.Sprintf("/%d", i)] = page("p")
	}
	s := newSite(t, pages, "")
	opts := testOptions()
	opts.Rate, opts.Burst = 20, 1 // 每 50ms 一个令牌
	start := time.Now()
	got, _, _ := run(t, New(opts, quietLogger()), context.Background(), s.url("/"))
	elapsed := time.Since(start)
	// 5 个请求、桶容量 1：第 1 个立即放行，其余 4 个各等 50ms，总计至少约 200ms
	if len(got) != 5 || elapsed < 180*time.Millisecond {
		t.Errorf("抓取 %d 页用时 %v，限速未生效（期望 ≥ 200ms）", len(got), elapsed)
	}
}

func TestCrawlCancel(t *testing.T) {
	// 一个永远不返回的慢页面：只有客户端取消请求，handler 才会结束。
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	seed, _ := url.Parse(srv.URL + "/")

	opts := testOptions()
	opts.Timeout = time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := run(t, New(opts, quietLogger()), ctx, seed)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("期望返回 DeadlineExceeded，实际 %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("取消后应快速返回，实际用时 %v", time.Since(start))
	}
}

func TestCrawlErrorsReported(t *testing.T) {
	s := newSite(t, map[string]string{"/": page("root", "/missing")}, "")
	got, stats, err := run(t, New(testOptions(), quietLogger()), context.Background(), s.url("/"))
	if err != nil {
		t.Fatal(err)
	}
	if got["/missing"].Status != 404 || stats.Pages != 2 {
		t.Errorf("404 页面应记录状态码，got=%+v stats=%+v", got["/missing"], stats)
	}
}
