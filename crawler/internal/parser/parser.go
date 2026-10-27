// Package parser 从 HTML 文档中提取标题与链接。
package parser

import (
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Page 是解析结果。
type Page struct {
	Title string
	Links []*url.URL // 已解析为绝对地址、去除 fragment 的链接（未去重）
}

// Parse 用 golang.org/x/net/html 的分词器（Tokenizer）流式解析 HTML。
//
// 为什么不用正则提取 <a href>？HTML 允许单引号、无引号、大小写混写、注释、
// 换行等各种写法，正则很难写对；Tokenizer 是符合 HTML5 规范的解析器，
// 而且是流式的，不需要先把整棵 DOM 树建出来，内存占用更小。
//
// base 是页面自身的 URL，用于把相对链接解析为绝对链接；
// 若页面中有 <base href>，以它为准。
func Parse(r io.Reader, base *url.URL) (*Page, error) {
	z := html.NewTokenizer(r)
	page := &Page{}
	inTitle := false
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			if z.Err() == io.EOF {
				return page, nil
			}
			return page, z.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			switch string(name) {
			case "title":
				inTitle = tt == html.StartTagToken
			case "base":
				if href := attr(z, hasAttr, "href"); href != "" {
					if u, err := base.Parse(href); err == nil {
						base = u
					}
				}
			case "a", "area":
				if href := attr(z, hasAttr, "href"); href != "" {
					if u := resolve(base, href); u != nil {
						page.Links = append(page.Links, u)
					}
				}
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "title" {
				inTitle = false
			}
		case html.TextToken:
			if inTitle && page.Title == "" {
				page.Title = strings.Join(strings.Fields(string(z.Text())), " ")
			}
		}
	}
}

// attr 读取当前标签的指定属性。
func attr(z *html.Tokenizer, hasAttr bool, want string) string {
	for hasAttr {
		var k, v []byte
		k, v, hasAttr = z.TagAttr()
		if string(k) == want {
			return strings.TrimSpace(string(v))
		}
	}
	return ""
}

// resolve 把 href 解析为绝对 URL；不是 http(s) 的链接（mailto:、javascript: 等）返回 nil。
func resolve(base *url.URL, href string) *url.URL {
	u, err := base.Parse(href)
	if err != nil {
		return nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil
	}
	return Normalize(u)
}

// Normalize 规范化 URL，使"看起来不同、实际相同"的地址得到同一个字符串，
// 这是去重的前提：
//   - 去掉 #fragment（同一页面的不同锚点）
//   - scheme/host 转小写，去掉默认端口（:80 / :443）
//   - 空路径补成 "/"
func Normalize(u *url.URL) *url.URL {
	n := *u // 复制一份，不修改调用方的对象
	n.Fragment = ""
	n.RawFragment = ""
	n.Scheme = strings.ToLower(n.Scheme)
	host := strings.ToLower(n.Host)
	if (n.Scheme == "http" && strings.HasSuffix(host, ":80")) ||
		(n.Scheme == "https" && strings.HasSuffix(host, ":443")) {
		host = host[:strings.LastIndexByte(host, ':')]
	}
	n.Host = host
	if n.Path == "" {
		n.Path = "/"
	}
	return &n
}
