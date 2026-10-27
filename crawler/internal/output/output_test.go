package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmxj/go-study-examples/crawler/internal/crawler"
)

func TestArrayWriterProducesValidJSON(t *testing.T) {
	for _, n := range []int{0, 1, 3} {
		var buf bytes.Buffer
		w, _ := New(&buf, "json")
		for i := 0; i < n; i++ {
			w.Write(crawler.Result{URL: "https://a.test/", Depth: i})
		}
		w.Close()
		var got []crawler.Result
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("n=%d 输出不是合法 JSON: %v\n%s", n, err, buf.String())
		}
		if len(got) != n {
			t.Errorf("n=%d 解析出 %d 个元素", n, len(got))
		}
	}
}

func TestLinesWriter(t *testing.T) {
	var buf bytes.Buffer
	w, _ := New(&buf, "jsonl")
	w.Write(crawler.Result{URL: "a"})
	w.Write(crawler.Result{URL: "b"})
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], `"url":"b"`) {
		t.Errorf("JSON Lines 输出不正确:\n%s", buf.String())
	}
}

func TestUnknownFormat(t *testing.T) {
	if _, err := New(&bytes.Buffer{}, "xml"); err == nil {
		t.Error("未知格式应返回错误")
	}
}
