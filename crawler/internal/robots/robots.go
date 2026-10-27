// Package robots 实现 robots.txt 的解析与按主机缓存。
//
// 只实现最常用的子集：User-agent / Allow / Disallow / Crawl-delay，
// 匹配规则遵循 RFC 9309：选择与本爬虫 UA 最匹配的组；
// 组内按"最长匹配路径"决定 Allow 还是 Disallow，长度相同时 Allow 优先；
// 支持 * 通配符与 $ 结尾锚定。
package robots

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type rule struct {
	allow bool
	path  string         // 原始模式，其长度用于"最长匹配"
	re    *regexp.Regexp // 由 path 编译而来
}

// Rules 是对某个 User-Agent 生效的规则集合。零值表示"全部允许"。
type Rules struct {
	rules      []rule
	CrawlDelay time.Duration
}

// group 是 robots.txt 中一个 User-agent 组。
type group struct {
	agents []string
	rules  []rule
	delay  time.Duration
}

// Parse 解析 robots.txt，返回对 userAgent 生效的规则。
func Parse(r io.Reader, userAgent string) *Rules {
	var groups []*group
	var cur *group
	lastWasAgent := false

	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "user-agent":
			// 连续多行 User-agent 属于同一组
			if cur == nil || !lastWasAgent {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(val))
			lastWasAgent = true
			continue
		case "allow", "disallow":
			if cur != nil && val != "" { // "Disallow:" 空值表示全部允许，无需记录
				cur.rules = append(cur.rules, rule{allow: key == "allow", path: val, re: compile(val)})
			}
		case "crawl-delay":
			if cur != nil {
				if secs, err := strconv.ParseFloat(val, 64); err == nil && secs > 0 {
					cur.delay = time.Duration(secs * float64(time.Second))
				}
			}
		}
		lastWasAgent = false
	}

	g := pickGroup(groups, strings.ToLower(userAgent))
	if g == nil {
		return &Rules{}
	}
	return &Rules{rules: g.rules, CrawlDelay: g.delay}
}

// pickGroup 选择 UA 名称匹配最长的组；都不匹配则用 "*" 组。
func pickGroup(groups []*group, ua string) *group {
	var best, star *group
	bestLen := 0
	for _, g := range groups {
		for _, a := range g.agents {
			if a == "*" {
				if star == nil {
					star = g
				}
				continue
			}
			if strings.Contains(ua, a) && len(a) > bestLen {
				best, bestLen = g, len(a)
			}
		}
	}
	if best != nil {
		return best
	}
	return star
}

// Allowed 报告路径（含查询串）是否允许抓取。
func (r *Rules) Allowed(path string) bool {
	if r == nil {
		return true
	}
	if path == "" {
		path = "/"
	}
	matched, allow := -1, true
	for _, rl := range r.rules {
		if !rl.re.MatchString(path) {
			continue
		}
		n := len(rl.path)
		if n > matched || (n == matched && rl.allow) {
			matched, allow = n, rl.allow
		}
	}
	return allow
}

// compile 把 robots 路径模式转换为正则：* 匹配任意字符序列，结尾的 $ 表示必须匹配到路径末尾，
// 其余字符按字面匹配（QuoteMeta 转义），并且总是从路径开头匹配（前缀语义）。
func compile(pattern string) *regexp.Regexp {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	expr := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, ".*")
	if anchored {
		expr += "$"
	}
	return regexp.MustCompile(expr) // 输入已全部转义，不会编译失败
}
