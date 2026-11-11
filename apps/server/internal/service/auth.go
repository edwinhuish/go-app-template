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
	// loginFailResetAttempts 是登录成功后清理失败计数的尝试次数（含首次）。
	loginFailResetAttempts = 2
)

// errLoginTooManyFails 是「猜错太多次」的统一出口。
//
// 已知账号与不存在的账号必须回同一句：否则连续尝试几下就能从
// 「403 过多」与「401 密码错误」的差别里看出账号是否存在。
var errLoginTooManyFails = errs.New(errs.CodeForbidden, "登录失败次数过多，请 15 分钟后再试")

// errBadCredentials 是账号不存在与密码错误的统一文案，避免账号枚举。
var errBadCredentials = errs.New(errs.CodeUnauthorized, "用户名或密码错误")

// dummyPasswordHash 只用于「账号不存在」时也消耗一次 bcrypt 的时间。
//
// 少了它，两条失败路径的耗时差两个数量级（bcrypt cost 10 约几十毫秒，查不到
// 账号几乎立即返回），任何人都能按响应时间把用户名枚举出来 —— 文案统一了，
// 时间没统一，等于白做。启动时算一次，不放在热路径上。
var dummyPasswordHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("timing-equalizer"), bcrypt.DefaultCost)
	if err != nil {
		return nil
	}
	return h
}()

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
// 锁定在比对密码之前判：失败次数到上限就直接 403，不再继续尝试。上一版把计数
// 只当「事后提示」——密码正确时永远放行，密码错误时照旧逐个比对，于是爆破速率
// 一次都没被降下来，而文案却在承诺「15 分钟后再试」。
//
// 代价是任何人都能用 5 次错误把某个账号挡在门外。这是有意选择的取舍：要抗
// 分布式爆破应该在入口按 IP 限流，而不是靠放宽这里的判断（那样等于没有锁定）。
func (s *authService) Login(ctx context.Context, req dto.LoginRequest) (*dto.LoginResponse, error) {
	failKey := loginFailPrefix + req.Username

	if locked(ctx, s.sessions, failKey, s.cfg.MaxLoginFail) {
		return nil, errLoginTooManyFails
	}

	user, err := s.users.FindByUsername(ctx, req.Username)
	if err != nil {
		if !errors.Is(err, errs.ErrNotFound) {
			// 把「库挂了」折成「密码错」会让一次 500 变成满屏 401，运维只能看到
			// 一片登录失败，排查方向直接跑偏。
			return nil, errs.Wrap(err, errs.CodeInternal, "查询账号失败")
		}
		// 账号不存在也走一次 bcrypt，把两条失败路径的耗时拉平（见 dummyPasswordHash）。
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))
		s.markLoginFail(ctx, failKey)
		return nil, errBadCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		n := s.markLoginFail(ctx, failKey)
		if n >= int64(s.cfg.MaxLoginFail) {
			return nil, errLoginTooManyFails
		}
		return nil, errBadCredentials
	}

	// 状态判断放在密码校验之后：放在前面的话，手里没有正确密码的人也能靠
	// 「账号已被禁用」这句文案确认某个用户名存在。
	if !user.IsEnabled() {
		return nil, errs.New(errs.CodeForbidden, "账号已被禁用")
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
	s.resetLoginFail(ctx, failKey)

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

// Verify 校验令牌签名、会话存在性与账号当前状态，返回去掉了敏感字段的身份。
//
// 身份字段（角色、昵称）以数据库为准而不是令牌自述：令牌在有效期内不会变，
// 而账号可能刚被停用、刚被降级。只在令牌里取角色，等于把「改权限」变成
// 「等 12 小时自然生效」。
//
// 代价是每个请求多一次主键查询（走主键索引，单行）。真要省掉它，可以给账号
// 状态加一层几十秒 TTL 的缓存，但要接受「停用最多延迟一个 TTL 生效」；
// 在骨架阶段，正确性比这一次查询更值钱。
func (s *authService) Verify(ctx context.Context, token string) (*ctxs.Principal, error) {
	var claims tokenClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名算法: %v", t.Header["alg"])
		}
		return []byte(s.cfg.Secret), nil
	},
		// 默认只校验「算法族」，HS256/HS384/HS512 都能过；这里钉死签发时用的那一个。
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		// 没有 exp 的令牌在 v5 里默认不过期，等于一把永久钥匙。
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errs.ErrUnauthorized.WithMsg("登录已过期，请重新登录")
		}
		return nil, errs.ErrUnauthorized
	}
	if !parsed.Valid || claims.ID == "" {
		return nil, errs.ErrUnauthorized
	}

	sessionUserID, err := s.sessions.Get(ctx, sessionKeyPrefix+claims.ID)
	if err != nil {
		if errors.Is(err, cache.ErrMiss) {
			return nil, errs.ErrUnauthorized
		}
		return nil, errs.Wrap(err, errs.CodeInternal, "校验会话失败")
	}
	// 会话里记的是签发时的账号 id：必须与令牌自述一致，否则「拿着 A 的令牌
	// 配上 B 的会话」这种拼装也能通过。
	if sessionUserID != strconv.FormatUint(uint64(claims.UserID), 10) {
		return nil, errs.ErrUnauthorized
	}

	user, err := s.users.FindByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			// 账号被删（含软删）后令牌立即失效，不必等它过期。
			return nil, errs.ErrUnauthorized
		}
		return nil, errs.Wrap(err, errs.CodeInternal, "校验账号状态失败")
	}
	if !user.IsEnabled() {
		return nil, errs.New(errs.CodeForbidden, "账号已被禁用")
	}

	p := &ctxs.Principal{
		UserID:   user.ID,
		Username: user.Username,
		Nickname: user.Nickname,
		Role:     user.Role,
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

// resetLoginFail 清掉失败计数。清不掉不当成登录失败：令牌已经签出、会话已经落库，
// 为了一次缓存删除把「登录成功」翻成 500，用户只会再试一次，问题反而更糟。
//
// 但也不能静默吞掉。这一步失败的可见症状是「密码明明对了，可能立刻又被告知失败
// 次数过多」——用户无从解释，运维在日志里也找不到线索，只能等窗口自然过期。
// 所以重试一次（缓存抖动多为瞬时），仍失败就打一条 warn，带上 key 与错误。
func (s *authService) resetLoginFail(ctx context.Context, key string) {
	var err error
	for attempt := 0; attempt < loginFailResetAttempts; attempt++ {
		if err = s.sessions.Del(ctx, key); err == nil {
			return
		}
	}
	s.log.Warn("清理登录失败计数失败，该账号可能在窗口期内被误锁",
		"key", key, "attempts", loginFailResetAttempts, "err", err)
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
