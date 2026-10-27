package web

import (
	"io/fs"
	"strings"
	"testing"
)

func TestStaticEmbedded(t *testing.T) {
	b, err := fs.ReadFile(Static(), "index.html")
	if err != nil {
		t.Fatalf("index.html 应被 embed 进二进制: %v", err)
	}
	if !strings.Contains(string(b), "new WebSocket") {
		t.Error("index.html 内容不正确")
	}
}
