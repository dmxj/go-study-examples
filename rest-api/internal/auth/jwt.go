// Package auth 封装 JWT 的签发与校验。
package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 是写入 Token 的自定义声明。
// 嵌入 jwt.RegisteredClaims 获得 exp/iat/iss/sub 等标准字段。
type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// UserID 从 Subject 中解析用户 ID。
func (c *Claims) UserID() (uint, error) {
	id, err := strconv.ParseUint(c.Subject, 10, 64)
	return uint(id), err
}

// TokenManager 负责签发与解析 Token。
type TokenManager struct {
	secret []byte
	ttl    time.Duration
	issuer string
	now    func() time.Time
}

// NewTokenManager 创建 TokenManager。
func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl, issuer: "book-api", now: time.Now}
}

// TTL 返回 Token 有效期。
func (m *TokenManager) TTL() time.Duration { return m.ttl }

// Generate 为用户签发 HS256 Token。
func (m *TokenManager) Generate(userID uint, username string) (string, error) {
	now := m.now()
	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(userID), 10),
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// ErrInvalidToken 表示 Token 无效（签名错误、过期、格式错误等）。
var ErrInvalidToken = errors.New("无效的 token")

// Parse 校验并解析 Token。
func (m *TokenManager) Parse(tokenString string) (*Claims, error) {
	var claims Claims
	_, err := jwt.ParseWithClaims(tokenString, &claims,
		func(t *jwt.Token) (any, error) { return m.secret, nil },
		// 显式限定算法，防御经典的 "alg=none" / 算法混淆攻击。
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return &claims, nil
}
