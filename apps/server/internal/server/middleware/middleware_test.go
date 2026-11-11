// 本文件守住两条只在异常路径上才暴露的行为：panic 怎么收尾、访问日志会不会丢。
//
// 这两条都没有编译期信号：panic 之后多写一段 JSON 是「响应看起来还挺正常」，
// 访问日志丢一条是「有个请求没记录」——都要等真出事故才会有人发现。
package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/pkg/response"
)

// TestRecoveryWritesEnvelopeWhenNothingWritten 验证「还没写出任何东西」时补一个标准错误信封。
func TestRecoveryWritesEnvelopeWhenNothingWritten(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	h := Recovery(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("状态码 = %d，期望 500", rec.Code)
	}

	var body response.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体不是 JSON 信封: %v（原文 %q）", err, rec.Body.String())
	}
	if body.Code != int(errs.CodeInternal) {
		t.Errorf("code = %d，期望 %d", body.Code, errs.CodeInternal)
	}
	if !strings.Contains(logs.String(), "boom") {
		t.Error("panic 原因没有进日志：线上只能靠复现去猜")
	}
}

// TestRecoveryAbortsAfterPartialWrite 验证「已经写过响应」时不再追加第二个信封。
//
// 追加的结果是 `{"partial":true{"code":50001,...}`——客户端可能把前半段当合法响应，
// 也可能在解析到一半时崩溃；两种都比「连接被中断」更难查。
func TestRecoveryAbortsAfterPartialWrite(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	const partial = `{"partial":true`
	h := Recovery(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(partial))
		panic("boom after write")
	}))

	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if rec2 := recover(); rec2 != http.ErrAbortHandler {
				t.Errorf("panic = %v，期望 http.ErrAbortHandler（中断连接）", rec2)
			}
		}()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil))
	}()

	if got := rec.Body.String(); got != partial {
		t.Errorf("响应体 = %q，期望保持 %q（不能再追加错误信封）", got, partial)
	}
}

// TestAccessLogKeepsEntryAfterPanic 验证 panic 的请求照样留下访问日志。
func TestAccessLogKeepsEntryAfterPanic(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))

	// 与生产的链序一致：AccessLog 在外，Recovery 在内。
	h := AccessLog(log)(Recovery(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil))

	out := logs.String()
	if !strings.Contains(out, "http 请求") {
		t.Fatalf("panic 的请求没有留下访问日志: %s", out)
	}
	if !strings.Contains(out, "status=500") {
		t.Errorf("访问日志里的状态码不是 500: %s", out)
	}
}

// TestExtractToken 守住 Bearer 头的解析：scheme 大小写不敏感，多余空格要容忍。
func TestExtractToken(t *testing.T) {
	cases := []struct{ name, header, want string }{
		{name: "标准写法", header: "Bearer abc", want: "abc"},
		{name: "小写 scheme", header: "bearer abc", want: "abc"},
		{name: "多余空格", header: "Bearer   abc  ", want: "abc"},
		{name: "缺令牌", header: "Bearer ", want: ""},
		{name: "其他 scheme", header: "Basic abc", want: ""},
		{name: "没有该头", header: "", want: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			if got := extractToken(req); got != c.want {
				t.Errorf("extractToken(%q) = %q，期望 %q", c.header, got, c.want)
			}
		})
	}
}

// fakeLimiter 是最小化的限流计数桩：只数次数，不关心窗口。
type fakeLimiter struct {
	counts map[string]int64
	err    error
}

func (f *fakeLimiter) Incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.counts == nil {
		f.counts = make(map[string]int64)
	}
	f.counts[key]++
	return f.counts[key], nil
}

// TestRateLimitBlocksOverLimit 守住「超过阈值回 429」以及分桶键里带 IP。
func TestRateLimitBlocksOverLimit(t *testing.T) {
	limiter := &fakeLimiter{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	h := RateLimit(log, limiter, "login", 2, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for i := 1; i <= 2; i++ {
		if got := do().Code; got != http.StatusOK {
			t.Fatalf("第 %d 次 = %d，期望 200", i, got)
		}
	}

	rec := do()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("超限那次 = %d，期望 429", rec.Code)
	}

	var body response.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("429 的响应体不是 JSON: %v", err)
	}
	if body.Code != int(errs.CodeTooManyRequests) {
		t.Errorf("429 的 code = %d，期望 %d", body.Code, errs.CodeTooManyRequests)
	}

	if _, ok := limiter.counts["ratelimit:login:192.0.2.1"]; !ok {
		t.Errorf("分桶键里没有 IP：实际 %v", limiter.counts)
	}
}

// TestRateLimitFailsOpen 守住「缓存抖动时放行」：限流是为了保护自己，
// 不该因为计数不可用把所有人挡在门外。
func TestRateLimitFailsOpen(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := RateLimit(log, &fakeLimiter{err: errors.New("cache down")}, "login", 1, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("计数失败时 = %d，期望放行（200）", rec.Code)
	}
}

// TestBodyLimitRejectsByContentLength 守住「Content-Length 阶段就拒」这条捷径：
// 超限的请求不该被读进内存再丢掉。
func TestBodyLimitRejectsByContentLength(t *testing.T) {
	h := BodyLimit(8)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/notes",
		strings.NewReader("0123456789")))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413", rec.Code)
	}

	var body response.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("413 的响应体不是 JSON: %v", err)
	}
	if body.Code != int(errs.CodeInvalidParam) {
		t.Errorf("413 的 code = %d，期望 %d", body.Code, errs.CodeInvalidParam)
	}
}
