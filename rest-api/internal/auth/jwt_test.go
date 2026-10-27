package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenRoundTrip(t *testing.T) {
	m := NewTokenManager("unit-test-secret-123", time.Hour)
	tok, err := m.Generate(42, "alice")
	if err != nil {
		t.Fatal(err)
	}
	c, err := m.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := c.UserID(); id != 42 || c.Username != "alice" {
		t.Errorf("解析结果 id=%d username=%s，期望 42/alice", id, c.Username)
	}
}

func TestTokenExpired(t *testing.T) {
	m := NewTokenManager("unit-test-secret-123", time.Minute)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return base }
	tok, _ := m.Generate(1, "a")

	m.now = func() time.Time { return base.Add(2 * time.Minute) }
	_, err := m.Parse(tok)
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("过期 token 应返回 ErrInvalidToken，实际 %v", err)
	}
}

func TestTokenWrongSecretOrAlg(t *testing.T) {
	m := NewTokenManager("unit-test-secret-123", time.Hour)
	other := NewTokenManager("another-secret-4567", time.Hour)
	tok, _ := other.Generate(1, "a")
	if _, err := m.Parse(tok); err == nil {
		t.Error("其他密钥签名的 token 应校验失败")
	}

	// alg=none 攻击：构造一个无签名的 token
	none := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: "1", Issuer: "book-api", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}})
	s, _ := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := m.Parse(s); err == nil {
		t.Error("alg=none 的 token 必须被拒绝")
	}
}
