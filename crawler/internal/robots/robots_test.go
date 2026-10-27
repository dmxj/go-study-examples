package robots

import (
	"strings"
	"testing"
	"time"
)

const sample = `
# 示例 robots.txt
User-agent: *
Disallow: /private
Disallow: /*.pdf$
Allow: /private/public
Crawl-delay: 1.5

User-agent: badbot
User-agent: evilbot
Disallow: /
`

func TestParseAndAllowed(t *testing.T) {
	r := Parse(strings.NewReader(sample), "go-study-crawler/1.0")
	tests := []struct {
		path string
		want bool
	}{
		{"/", true},
		{"/about", true},
		{"/private", false},
		{"/private/data", false},
		{"/private/public/x", true}, // Allow 路径更长，优先
		{"/docs/a.pdf", false},      // 通配符 + $ 锚定
		{"/docs/a.pdf?x=1", true},   // $ 要求以 .pdf 结尾
		{"/docs/a.pdfx", true},
	}
	for _, tt := range tests {
		if got := r.Allowed(tt.path); got != tt.want {
			t.Errorf("Allowed(%q) = %v，期望 %v", tt.path, got, tt.want)
		}
	}
	if r.CrawlDelay != 1500*time.Millisecond {
		t.Errorf("CrawlDelay = %v，期望 1.5s", r.CrawlDelay)
	}
}

func TestSpecificAgentGroup(t *testing.T) {
	// 连续两行 User-agent 属于同一组
	for _, ua := range []string{"EvilBot/2.0", "badbot"} {
		r := Parse(strings.NewReader(sample), ua)
		if r.Allowed("/anything") {
			t.Errorf("%s 应被完全禁止", ua)
		}
	}
}

func TestEmptyAndNil(t *testing.T) {
	if !Parse(strings.NewReader(""), "x").Allowed("/a") {
		t.Error("空 robots.txt 应全部允许")
	}
	if !Parse(strings.NewReader("User-agent: *\nDisallow:\n"), "x").Allowed("/a") {
		t.Error("Disallow 空值应全部允许")
	}
	var r *Rules
	if !r.Allowed("/a") {
		t.Error("nil Rules 应全部允许")
	}
}
