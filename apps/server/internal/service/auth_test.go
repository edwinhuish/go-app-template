// 本文件覆盖 Verify 里那几条「只有构造出异常输入才走得到」的分支。
//
// 它们都做安全判断：会话与令牌串号、令牌没有 exp、账号被停用、角色在库里被改过。
// 走 HTTP 很难构造（要伪造签名、要绕过中间件），但漏掉任何一条的后果都是
// 「停用的账号还能用」「过期令牌永不过期」这类问题，所以放在 service 层测。
package service

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"example.com/app/internal/dto"
	"example.com/app/internal/infra/cache"
	"example.com/app/internal/model"
	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/role"
)

const testSecret = "test-secret-0123456789abcdef"

// fakeUserRepo 是只够用完这四个方法的账号仓储。
type fakeUserRepo struct {
	byID   map[uint]*model.User
	byName map[string]*model.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[uint]*model.User{}, byName: map[string]*model.User{}}
}

func (f *fakeUserRepo) add(u *model.User) {
	f.byID[u.ID] = u
	f.byName[u.Username] = u
}

func (f *fakeUserRepo) Create(_ context.Context, u *model.User) error {
	f.add(u)
	return nil
}

func (f *fakeUserRepo) FindByID(_ context.Context, id uint) (*model.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, errs.ErrNotFound
}

func (f *fakeUserRepo) FindByUsername(_ context.Context, name string) (*model.User, error) {
	if u, ok := f.byName[name]; ok {
		return u, nil
	}
	return nil, errs.ErrNotFound
}

func (f *fakeUserRepo) List(context.Context) ([]model.User, error) { return nil, nil }

// fakeCache 复刻 cache.Cache 的语义（未命中返回 ErrMiss），只实现用例需要的部分。
type fakeCache struct {
	mu   sync.Mutex
	vals map[string]string
	// delErr 非 nil 时 Del 一律失败，用来构造「缓存清不掉」的分支；delCalls 记录尝试次数。
	delErr   error
	delCalls int
}

func newFakeCache() *fakeCache { return &fakeCache{vals: map[string]string{}} }

func (c *fakeCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.vals[key]
	if !ok {
		return "", cache.ErrMiss
	}
	return v, nil
}

func (c *fakeCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vals[key] = value
	return nil
}

func (c *fakeCache) Del(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delCalls++
	if c.delErr != nil {
		return c.delErr
	}
	for _, k := range keys {
		delete(c.vals, k)
	}
	return nil
}

func (c *fakeCache) Incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, _ := strconv.ParseInt(c.vals[key], 10, 64)
	n++
	c.vals[key] = strconv.FormatInt(n, 10)
	return n, nil
}

func (c *fakeCache) Ping(context.Context) error { return nil }
func (c *fakeCache) Driver() string             { return "fake" }
func (c *fakeCache) Close() error               { return nil }

// newAuthFixture 装配一个用假仓储与假缓存驱动的鉴权服务。
func newAuthFixture() (AuthService, *fakeUserRepo, *fakeCache) {
	repo := newFakeUserRepo()
	c := newFakeCache()
	svc := NewAuthService(repo, c, AuthConfig{Secret: testSecret, Expire: time.Hour, MaxLoginFail: 5})
	return svc, repo, c
}

// enabledUser 造一个可用账号；密码摘要用不到真 bcrypt（这些用例都走不到密码比对）。
func enabledUser(id uint) *model.User {
	return &model.User{
		Base:     model.Base{ID: id},
		Username: "user-" + strconv.FormatUint(uint64(id), 10),
		Password: "not-a-real-hash",
		Role:     role.User,
		Status:   model.EnableStatusOn,
	}
}

func signToken(t *testing.T, claims tokenClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("签发测试令牌: %v", err)
	}
	return signed
}

func validClaims(userID uint, tokenID string) tokenClaims {
	return tokenClaims{
		UserID: userID,
		Role:   role.User,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        tokenID,
			Subject:   strconv.FormatUint(uint64(userID), 10),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

// TestVerifyRejectsSessionMismatch 守住「令牌与会话必须指向同一个账号」。
//
// 少了这条比对，一个签发给 A 的令牌只要配上 B 的会话 id 就能读 B 的数据。
func TestVerifyRejectsSessionMismatch(t *testing.T) {
	svc, repo, c := newAuthFixture()
	repo.add(enabledUser(7))

	token := signToken(t, validClaims(7, "jti-mismatch"))
	if err := c.Set(context.Background(), "session:jti-mismatch", "8", time.Hour); err != nil {
		t.Fatalf("写入会话: %v", err)
	}

	if _, err := svc.Verify(context.Background(), token); !errors.Is(err, errs.ErrUnauthorized) {
		t.Errorf("err = %v，期望 401（会话里是另一个账号）", err)
	}
}

// TestVerifyRejectsTokenWithoutExpiry 守住「没有 exp 的令牌不算有效」。
//
// jwt/v5 默认不要求 exp，漏掉 WithExpirationRequired 就等于允许永不过期的令牌。
func TestVerifyRejectsTokenWithoutExpiry(t *testing.T) {
	svc, repo, c := newAuthFixture()
	repo.add(enabledUser(7))

	claims := tokenClaims{
		UserID: 7,
		Role:   role.User,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:       "jti-no-exp",
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}
	token := signToken(t, claims)
	if err := c.Set(context.Background(), "session:jti-no-exp", "7", time.Hour); err != nil {
		t.Fatalf("写入会话: %v", err)
	}

	if _, err := svc.Verify(context.Background(), token); !errors.Is(err, errs.ErrUnauthorized) {
		t.Errorf("err = %v，期望 401（缺少 exp）", err)
	}
}

// TestVerifyExpiredTokenMessage 守住过期令牌单独一句文案：用户看到「登录已过期」
// 才会去重新登录，看到「未登录」只会以为是自己没点登录。
func TestVerifyExpiredTokenMessage(t *testing.T) {
	svc, repo, c := newAuthFixture()
	repo.add(enabledUser(7))

	claims := validClaims(7, "jti-expired")
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	token := signToken(t, claims)
	if err := c.Set(context.Background(), "session:jti-expired", "7", time.Hour); err != nil {
		t.Fatalf("写入会话: %v", err)
	}

	_, err := svc.Verify(context.Background(), token)
	e, ok := errs.From(err)
	if !ok || e.Code != errs.CodeUnauthorized {
		t.Fatalf("err = %v，期望 401 业务错误", err)
	}
	if e.Msg == errs.ErrUnauthorized.Msg {
		t.Errorf("文案 = %q，期望与「未登录」区分开的过期提示", e.Msg)
	}
}

// TestVerifyRejectsDisabledUser 守住「停用账号的存量令牌立刻失效」。
func TestVerifyRejectsDisabledUser(t *testing.T) {
	svc, repo, c := newAuthFixture()
	u := enabledUser(7)
	u.Status = model.EnableStatusOff
	repo.add(u)

	token := signToken(t, validClaims(7, "jti-disabled"))
	if err := c.Set(context.Background(), "session:jti-disabled", "7", time.Hour); err != nil {
		t.Fatalf("写入会话: %v", err)
	}

	_, err := svc.Verify(context.Background(), token)
	e, ok := errs.From(err)
	if !ok || e.Code != errs.CodeForbidden {
		t.Fatalf("err = %v，期望 403（账号已停用）", err)
	}
}

// TestVerifyTakesRoleFromDB 守住「角色以数据库为准」。
//
// 只在令牌里取角色的话，把某人降级要等令牌自然过期才生效。
func TestVerifyTakesRoleFromDB(t *testing.T) {
	svc, repo, c := newAuthFixture()
	u := enabledUser(7)
	u.Role = role.Admin
	repo.add(u)

	token := signToken(t, validClaims(7, "jti-role")) // 令牌里写的是 user
	if err := c.Set(context.Background(), "session:jti-role", "7", time.Hour); err != nil {
		t.Fatalf("写入会话: %v", err)
	}

	p, err := svc.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if p.Role != role.Admin {
		t.Errorf("角色 = %q，期望以库里的 %q 为准", p.Role, role.Admin)
	}
}

// TestLoginRejectsBeforePasswordCheck 守住「到上限就在比对密码之前拒绝」。
//
// 如果先比对密码再判计数，爆破速率不会下降——这正是上一版的问题。
func TestLoginRejectsBeforePasswordCheck(t *testing.T) {
	svc, repo, c := newAuthFixture()
	u := enabledUser(7)
	repo.add(u)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := c.Incr(ctx, loginFailPrefix+u.Username, loginFailWindow); err != nil {
			t.Fatalf("预置失败次数: %v", err)
		}
	}

	// 密码摘要是假的，只要代码真的去比对就会报「密码错误」而不是「次数过多」。
	_, err := svc.Login(ctx, dto.LoginRequest{Username: u.Username, Password: "whatever123"})
	e, ok := errs.From(err)
	if !ok || e.Code != errs.CodeForbidden {
		t.Fatalf("err = %v，期望 403（次数过多）", err)
	}
}

// TestDummyHashFailureIsVisible 守住「时序对齐用的摘要生成失败必须留信号」。
//
// 这条分支在生产里几乎不可能触发，但触发时的后果是静默的：账号不存在那条路变回
// 「立刻返回」，用户名又能按响应时间被枚举出来，而文案统一、状态码一致，从外面
// 看不出任何异常。所以注入一个非法的 cost，要求日志里出现 error，并且登录本身
// 仍然可用（降级而不是瘫痪）。
func TestDummyHashFailureIsVisible(t *testing.T) {
	old := dummyHashCost
	dummyHashCost = bcrypt.MaxCost + 1 // 越界才报错：bcrypt 会把过小的 cost 悄悄夹回默认值
	t.Cleanup(func() { dummyHashCost = old })

	var logs bytes.Buffer
	svc := NewAuthService(newFakeUserRepo(), newFakeCache(),
		AuthConfig{Secret: testSecret, Expire: time.Hour, MaxLoginFail: 5},
		WithAuthLogger(slog.New(slog.NewTextHandler(&logs, nil))),
	)

	if !strings.Contains(logs.String(), "时序对齐") {
		t.Errorf("生成失败没有留下日志：%q", logs.String())
	}

	// 降级而非瘫痪：不存在的账号仍然是 401（只是比对那一步立刻返回）。
	_, err := svc.Login(context.Background(), dto.LoginRequest{Username: "ghost", Password: "whatever"})
	if e, ok := errs.From(err); !ok || e.Code != errs.CodeUnauthorized {
		t.Fatalf("err = %v，期望 401（降级后仍按凭据错误处理）", err)
	}
}

// TestLoginSurvivesFailCounterResetError 守住「清理失败计数失败不推翻本次登录」。
//
// 这一步失败的可见症状很隐蔽：密码对了、令牌也签出来了，但下一次登录可能立刻被告知
// 「失败次数过多」——用户无从解释，运维在日志里也没有线索，只能等窗口自然过期。
// 所以两条都要钉住：不返回错误，且日志里看得见。
func TestLoginSurvivesFailCounterResetError(t *testing.T) {
	repo := newFakeUserRepo()
	u := enabledUser(11)
	repo.add(u)

	c := newFakeCache()
	c.delErr = errors.New("缓存连接断开")

	var logs bytes.Buffer
	svc := NewAuthService(repo, c,
		AuthConfig{Secret: testSecret, Expire: time.Hour, MaxLoginFail: 5},
		WithAuthLogger(slog.New(slog.NewTextHandler(&logs, nil))),
	)

	hash, err := svc.HashPassword("pw-123456")
	if err != nil {
		t.Fatalf("生成密码摘要: %v", err)
	}
	u.Password = hash

	resp, err := svc.Login(context.Background(),
		dto.LoginRequest{Username: u.Username, Password: "pw-123456"})
	if err != nil || resp == nil || resp.Token == "" {
		t.Fatalf("Login = %+v, %v；清理失败不该把登录变成错误", resp, err)
	}

	if c.delCalls != 2 {
		t.Errorf("Del 尝试 %d 次，期望 2（首次 + 重试一次）", c.delCalls)
	}
	if !strings.Contains(logs.String(), "清理登录失败计数失败") {
		t.Errorf("日志里没有清理失败的记录：%q", logs.String())
	}
}
