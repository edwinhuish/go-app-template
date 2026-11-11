package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"example.com/app/internal/dto"
	"example.com/app/internal/infra/cache"
	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/repository"
	"example.com/app/internal/server/ctxs"
)

const (
	sessionKeyPrefix = "session:"
	loginFailPrefix  = "login_fail:"
	// loginFailWindow 是登录失败计数的滑动窗口。
	loginFailWindow = 15 * time.Minute
)

// errLoginTooManyFails 是「猜错太多次」的统一出口。
//
// 已知账号与不存在的账号必须回同一句：否则连续尝试几下就能从
// 「403 过多」与「401 密码错误」的差别里看出账号是否存在。
var errLoginTooManyFails = errs.New(errs.CodeForbidden, "登录失败次数过多，请 15 分钟后再试")

// errBadCredentials 是账号不存在与密码错误的统一文案，避免账号枚举。
var errBadCredentials = errs.New(errs.CodeUnauthorized, "用户名或密码错误")

// AuthConfig 是鉴权相关的可调参数。
type AuthConfig struct {
	// Secret 是 JWT 签名密钥，生产必须从环境变量注入。
	Secret string
	// Expire 是令牌有效期。
	Expire time.Duration
	// MaxLoginFail 是触发锁定的连续失败次数，<=0 时取 5。
	MaxLoginFail int
}

// AuthService 定义登录、登出、令牌校验。
type AuthService interface {
	Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error)
	Logout(ctx context.Context, tokenID string) error
	Verify(ctx context.Context, token string) (*ctxs.Principal, error)
	HashPassword(plain string) (string, error)
}

// AuthOption 是鉴权服务的可选依赖。
type AuthOption = Option[authService]

// WithAuthLogger 注入 logger。
func WithAuthLogger(l *slog.Logger) AuthOption {
	return func(s *authService) { s.log = l }
}

type authService struct {
	users    repository.UserRepository
	sessions cache.Cache
	cfg      AuthConfig
	log      *slog.Logger
}

// NewAuthService 构造鉴权服务。
func NewAuthService(
	users repository.UserRepository, sessions cache.Cache, cfg AuthConfig, opts ...AuthOption,
) AuthService {
	if cfg.Expire <= 0 {
		cfg.Expire = 24 * time.Hour
	}
	if cfg.MaxLoginFail <= 0 {
		cfg.MaxLoginFail = 5
	}

	s := &authService{users: users, sessions: sessions, cfg: cfg, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *authService) HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("生成密码摘要: %w", err)
	}
	return string(h), nil
}

// Login 校验账号密码并签发令牌，同时把会话写进缓存（登出即失效）。
//
// 失败计数只决定「还能不能继续猜」，**不预先拒绝请求**：
// 先看计数再校验密码的写法，会让任何人连错 5 次就把某个账号锁在门外 15 分钟——
// 被锁的人手里握着正确密码，却什么也做不了。这里先校验密码，错了才计数。
func (s *authService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	failKey := loginFailPrefix + req.Username

	user, err := s.users.FindByUsername(ctx, req.Username)
	if err != nil {
		s.markLoginFail(ctx, failKey)
		if locked(ctx, s.sessions, failKey, s.cfg.MaxLoginFail) {
			return nil, errLoginTooManyFails
		}
		return nil, errBadCredentials
	}
	if !user.IsEnabled() {
		return nil, errs.New(errs.CodeForbidden, "账号已被禁用")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		n := s.markLoginFail(ctx, failKey)
		if n >= int64(s.cfg.MaxLoginFail) {
			return nil, errLoginTooManyFails
		}
		return nil, errBadCredentials
	}

	tokenID, err := newTokenID()
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "生成会话标识失败")
	}

	now := time.Now()
	expireAt := now.Add(s.cfg.Expire)

	claims := tokenClaims{
		UserID:   user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID,
			Subject:   strconv.FormatUint(uint64(user.ID), 10),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expireAt),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.Secret))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "签发令牌失败")
	}

	// 会话落缓存：签名有效还不够，登出后要能立刻失效。
	if err := s.sessions.Set(ctx, sessionKeyPrefix+tokenID,
		strconv.FormatUint(uint64(user.ID), 10), s.cfg.Expire); err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "写入会话失败")
	}
	_ = s.sessions.Del(ctx, failKey)

	s.log.Info("用户登录", "user_id", user.ID, "username", user.Username)

	return &dto.LoginResponse{
		Token:     signed,
		ExpiresAt: expireAt.Unix(),
		User: &dto.UserInfo{
			ID:       user.ID,
			Username: user.Username,
			Nickname: user.Nickname,
			Role:     user.Role,
		},
	}, nil
}

// Logout 删除会话，令牌立即失效。
func (s *authService) Logout(ctx context.Context, tokenID string) error {
	if tokenID == "" {
		return nil
	}
	return s.sessions.Del(ctx, sessionKeyPrefix+tokenID)
}

// Verify 校验令牌签名与会话存在性，返回去掉了敏感字段的身份。
func (s *authService) Verify(ctx context.Context, token string) (*ctxs.Principal, error) {
	var claims tokenClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名算法: %v", t.Header["alg"])
		}
		return []byte(s.cfg.Secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errs.ErrUnauthorized.WithMsg("登录已过期，请重新登录")
		}
		return nil, errs.ErrUnauthorized
	}
	if !parsed.Valid {
		return nil, errs.ErrUnauthorized
	}

	if _, err := s.sessions.Get(ctx, sessionKeyPrefix+claims.ID); err != nil {
		if errors.Is(err, cache.ErrMiss) {
			return nil, errs.ErrUnauthorized
		}
		return nil, errs.Wrap(err, errs.CodeInternal, "校验会话失败")
	}

	p := &ctxs.Principal{
		UserID:   claims.UserID,
		Username: claims.Username,
		Nickname: claims.Nickname,
		Role:     claims.Role,
		TokenID:  claims.ID,
	}
	if claims.ExpiresAt != nil {
		p.ExpiresAt = claims.ExpiresAt.Time
	}
	return p, nil
}

// markLoginFail 记一次失败并返回累计次数（读不出累计值时返回 0）。
func (s *authService) markLoginFail(ctx context.Context, key string) int64 {
	n, err := s.sessions.Incr(ctx, key, loginFailWindow)
	if err != nil {
		s.log.Warn("记录登录失败次数出错", "key", key, "err", err)
		return 0
	}
	return n
}

// locked 报告该账号是否已经用完猜密码的次数。缓存读失败时不锁定：
// 缓存抖动不该把所有人挡在门外。
func locked(ctx context.Context, c cache.Cache, key string, maxFail int) bool {
	v, err := c.Get(ctx, key)
	if err != nil {
		return false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return false
	}
	return n >= int64(maxFail)
}

type tokenClaims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"usr"`
	Nickname string `json:"nick"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func newTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
