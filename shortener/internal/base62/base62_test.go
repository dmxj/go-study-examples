package base62

import (
	"math"
	"testing"
)

func TestEncodeKnown(t *testing.T) {
	tests := map[uint64]string{
		0:              "0",
		9:              "9",
		10:             "a",
		61:             "Z",
		62:             "10",
		3843:           "ZZ", // 62*62 - 1
		math.MaxUint64: "lYGhA16ahyf",
	}
	for n, want := range tests {
		if got := Encode(n); got != want {
			t.Errorf("Encode(%d) = %q，期望 %q", n, got, want)
		}
		if back, err := Decode(want); err != nil || back != n {
			t.Errorf("Decode(%q) = %d, %v，期望 %d", want, back, err, n)
		}
	}
}

func TestDecodeInvalid(t *testing.T) {
	for _, s := range []string{"", "a-b", "你好", "lYGhA16ahyg", "zzzzzzzzzzzz"} {
		if _, err := Decode(s); err == nil {
			t.Errorf("Decode(%q) 应返回错误", s)
		}
	}
}

func TestValid(t *testing.T) {
	if !Valid("abcXYZ019") || Valid("ab_c") || Valid("") || Valid("é") {
		t.Error("Valid 判断错误")
	}
}

// FuzzRoundTrip 是模糊测试：go test -fuzz=FuzzRoundTrip ./internal/base62
// 普通 go test 只会运行下面的种子语料。
func FuzzRoundTrip(f *testing.F) {
	for _, seed := range []uint64{0, 1, 61, 62, 1 << 40, math.MaxUint64} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, n uint64) {
		s := Encode(n)
		got, err := Decode(s)
		if err != nil || got != n {
			t.Fatalf("Decode(Encode(%d)) = %d, %v", n, got, err)
		}
	})
}

func BenchmarkEncode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Encode(uint64(i) + 1<<32)
	}
}
