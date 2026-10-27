package config

import (
	"io"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("SHORT_CACHE_SIZE", "42")
	c, err := Load([]string{"-rate", "3", "-base-url", "https://s.example.com"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.CacheSize != 42 || c.RatePerSec != 3 || c.BaseURL != "https://s.example.com" {
		t.Errorf("配置解析错误: %+v", c)
	}
}

func TestLoadInvalid(t *testing.T) {
	for _, args := range [][]string{
		{"-base-url", "s.example.com"},
		{"-rate", "0"},
		{"-burst", "0"},
	} {
		if _, err := Load(args, io.Discard); err == nil {
			t.Errorf("Load(%v) 应返回错误", args)
		}
	}
}
