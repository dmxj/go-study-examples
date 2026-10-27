package service

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/dmxj/go-study-examples/rest-api/internal/apperr"
	"github.com/dmxj/go-study-examples/rest-api/internal/model"
	"github.com/dmxj/go-study-examples/rest-api/internal/repository"
)

// TokenIssuer 是 AuthService 需要的签发能力。用小接口而不是具体类型，测试更方便。
type TokenIssuer interface {
	Generate(userID uint, username string) (string, error)
}

// AuthService 处理注册与登录。
type AuthService struct {
	users  repository.UserRepository
	tokens TokenIssuer
	cost   int // bcrypt 成本因子，测试时调低以加快速度
}

// NewAuthService 创建 AuthService。
func NewAuthService(users repository.UserRepository, tokens TokenIssuer) *AuthService {
	return &AuthService{users: users, tokens: tokens, cost: bcrypt.DefaultCost}
}

// WithBcryptCost 设置 bcrypt 成本（测试用 bcrypt.MinCost）。
func (s *AuthService) WithBcryptCost(cost int) *AuthService {
	s.cost = cost
	return s
}

// Register 注册新用户。用户名已存在时返回 apperr.ErrConflict。
func (s *AuthService) Register(ctx context.Context, req model.RegisterRequest) (*model.User, error) {
	// binding 的 max=72 按字符（rune）计数，而 bcrypt 的上限是 72 字节：
	// 30 个汉字只有 30 个字符却占 90 字节，能通过 binding 校验，这里必须再按字节检查一次。
	if len(req.Password) > 72 {
		return nil, fmt.Errorf("%w: 密码不能超过 72 字节", apperr.ErrInvalidArgument)
	}
	// 永远不要明文存密码。bcrypt 自带随机盐，且计算故意很慢，能有效抵御彩虹表与暴力破解。
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.cost)
	if err != nil {
		return nil, fmt.Errorf("密码哈希: %w", err)
	}
	u := &model.User{Username: req.Username, PasswordHash: string(hash)}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("注册用户 %s: %w", req.Username, err)
	}
	return u, nil
}

// Login 校验用户名密码并签发 Token。
//
// 无论"用户不存在"还是"密码错误"都返回同一个 ErrInvalidCredentials，
// 避免攻击者借此枚举出哪些用户名已注册。
func (s *AuthService) Login(ctx context.Context, req model.LoginRequest) (string, error) {
	u, err := s.users.GetByUsername(ctx, req.Username)
	if errors.Is(err, apperr.ErrNotFound) {
		return "", apperr.ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return "", apperr.ErrInvalidCredentials
	}
	token, err := s.tokens.Generate(u.ID, u.Username)
	if err != nil {
		return "", fmt.Errorf("签发 token: %w", err)
	}
	return token, nil
}

// Me 返回当前登录用户信息。
func (s *AuthService) Me(ctx context.Context, userID uint) (*model.User, error) {
	return s.users.GetByID(ctx, userID)
}
