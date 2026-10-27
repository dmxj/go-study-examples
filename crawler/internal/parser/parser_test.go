package parser

import (
	"net/url"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	const doc = `<!DOCTYPE html>
<html><head><title>
   Go 学习   站点
</title></head>
<body>
  <a href="/a">相对路径</a>
  <a href='b.html#sec'>单引号 + fragment</a>
  <A HREF=https://Example.COM:443/x>大写标签、无引号、默认端口</A>
  <a href="mailto:me@example.com">邮件</a>
  <a href="javascript:void(0)">JS</a>
  <a>无 href</a>
  <!-- <a href="/commented">注释里的链接不应被提取</a> -->
</body></html>`
	base, _ := url.Parse("https://site.test/dir/index.html")
	p, err := Parse(strings.NewReader(doc), base)
	if err != nil {
		t.Fatal(err)
	}
	if p.Title != "Go 学习 站点" {
		t.Errorf("Title = %q，期望空白被规范化为 %q", p.Title, "Go 学习 站点")
	}
	var got []string
	for _, l := range p.Links {
		got = append(got, l.String())
	}
	want := []string{
		"https://site.test/a",
		"https://site.test/dir/b.html",
		"https://example.com/x",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("Links =\n  %v\n期望\n  %v", got, want)
	}
}

func TestParseBaseTag(t *testing.T) {
	base, _ := url.Parse("https://site.test/page")
	p, _ := Parse(strings.NewReader(`<base href="https://cdn.test/root/"><a href="x">x</a>`), base)
	if len(p.Links) != 1 || p.Links[0].String() != "https://cdn.test/root/x" {
		t.Errorf("应以 <base href> 解析相对链接，实际 %v", p.Links)
	}
}

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"HTTP://Example.com:80":         "http://example.com/",
		"https://a.test:443/p?q=1#frag": "https://a.test/p?q=1",
		"https://a.test:8443/":          "https://a.test:8443/",
	}
	for in, want := range tests {
		u, _ := url.Parse(in)
		if got := Normalize(u).String(); got != want {
			t.Errorf("Normalize(%q) = %q，期望 %q", in, got, want)
		}
	}
}
