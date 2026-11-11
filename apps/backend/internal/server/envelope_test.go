// 本文件是响应信封的线格式守卫。
//
// 为什么必须有它：信封的组装已经移出 handler 签名（见 envelope.go 的序列化钩子），
// 于是「改了信封字段」或「某个接口漏进了钩子」都不会有编译错误，
// handler 单测也测不到——它们断言的是业务结构体本身。
// 唯一能抓住这两类回归的地方就是真实响应体的形状，所以在这里把它钉住。
package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// successKeys 是成功响应的顶层字段集合。
//
// 用集合而不是「包含」判断：多出字段说明信封被改宽了，少了说明被改窄了，
// 两种都是对外接口变更，应该让测试失败而不是静默通过。
var (
	successKeys = []string{"code", "message", "data", "request_id"}
	errorKeys   = []string{"code", "message", "request_id"}
)

// call 发一次请求并解析响应体；body 为 nil 表示不带请求体。
func call(t *testing.T, srv *httptest.Server, method, path, body, token string) (int, http.Header, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("构造请求 %s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("请求 %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读响应 %s %s: %v", method, path, err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s %s 的响应不是 JSON 对象: %v\n原文: %s", method, path, err, raw)
	}
	return resp.StatusCode, resp.Header, doc
}

// assertKeys 断言顶层字段集合完全一致。
func assertKeys(t *testing.T, doc map[string]any, want []string) {
	t.Helper()

	got := make([]string, 0, len(doc))
	for k := range doc {
		got = append(got, k)
	}
	if len(got) != len(want) {
		t.Fatalf("顶层字段数 = %d %v，期望 %d %v", len(got), got, len(want), want)
	}
	for _, k := range want {
		if _, ok := doc[k]; !ok {
			t.Fatalf("缺少顶层字段 %q，实际 %v", k, got)
		}
	}
}

// TestEnvelopeShape 覆盖成功、分页、创建、错误、鉴权五类响应，锁定线格式。
func TestEnvelopeShape(t *testing.T) {
	srv := newTestServer(t)

	// ---- 鉴权：错误码与 request_id ----
	status, hdr, doc := call(t, srv, http.MethodPost, "/api/v1/auth/login",
		`{"username":"admin","password":"wrong-password"}`, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("登录失败的状态码 = %d，期望 401", status)
	}
	assertKeys(t, doc, errorKeys)
	if _, ok := doc["data"]; ok {
		t.Error("失败响应不该带 data 字段")
	}
	if doc["code"] != float64(40101) {
		t.Errorf("code = %v，期望 40101", doc["code"])
	}
	assertRequestIDConsistent(t, hdr, doc)

	// ---- 登录成功 ----
	status, hdr, doc = call(t, srv, http.MethodPost, "/api/v1/auth/login",
		`{"username":"admin","password":"admin123"}`, "")
	if status != http.StatusOK {
		t.Fatalf("登录成功的状态码 = %d，期望 200", status)
	}
	assertKeys(t, doc, successKeys)
	if doc["code"] != float64(0) || doc["message"] != "ok" {
		t.Errorf("成功信封 = code %v / message %v，期望 0 / ok", doc["code"], doc["message"])
	}
	assertRequestIDConsistent(t, hdr, doc)

	data, _ := doc["data"].(map[string]any)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatal("登录响应里没有 token")
	}
	if _, ok := data["expires_at"]; !ok {
		t.Error("登录响应里没有 expires_at")
	}

	// ---- 新建：201，但 message 与其它成功响应一致，仍是 ok ----
	status, _, doc = call(t, srv, http.MethodPost, "/api/v1/notes",
		`{"title":"envelope-guard","content":"hello"}`, token)
	if status != http.StatusCreated {
		t.Fatalf("新建的状态码 = %d，期望 201（由 OptionDefaultStatusCode 决定）", status)
	}
	assertKeys(t, doc, successKeys)
	if doc["message"] != "ok" {
		t.Errorf("新建的 message = %v，期望 ok（成功文案只有一种，状态码由路由选项表达）", doc["message"])
	}
	created, _ := doc["data"].(map[string]any)
	noteID, _ := created["id"].(float64)
	if noteID == 0 {
		t.Fatal("新建响应里没有 id")
	}

	// ---- 分页：list 必须是数组而不是 null ----
	status, _, doc = call(t, srv, http.MethodGet, "/api/v1/notes", "", token)
	if status != http.StatusOK {
		t.Fatalf("列表的状态码 = %d，期望 200", status)
	}
	assertKeys(t, doc, successKeys)
	page, _ := doc["data"].(map[string]any)
	if _, ok := page["list"].([]any); !ok {
		t.Fatalf("分页的 list 不是数组: %#v", page["list"])
	}
	for _, k := range []string{"total", "page", "page_size"} {
		if _, ok := page[k]; !ok {
			t.Errorf("分页缺少字段 %q", k)
		}
	}

	// ---- 详情：data 是业务结构体本身，不是再套一层信封 ----
	path := fmt.Sprintf("/api/v1/notes/%.0f", noteID)
	status, _, doc = call(t, srv, http.MethodGet, path, "", token)
	if status != http.StatusOK {
		t.Fatalf("详情的状态码 = %d，期望 200", status)
	}
	assertKeys(t, doc, successKeys)
	item, _ := doc["data"].(map[string]any)
	if item["title"] != "envelope-guard" {
		t.Errorf("data.title = %v，期望 envelope-guard", item["title"])
	}
	if _, nested := item["data"]; nested {
		t.Error("data 里又套了一层 data：说明被包裹了两次")
	}

	// ---- 404：错误信封不带 data ----
	status, _, doc = call(t, srv, http.MethodGet, "/api/v1/notes/999999", "", token)
	if status != http.StatusNotFound {
		t.Fatalf("查询不存在的记录的状态码 = %d，期望 404", status)
	}
	assertKeys(t, doc, errorKeys)

	// ---- 缺令牌 ----
	status, _, doc = call(t, srv, http.MethodGet, "/api/v1/notes", "", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("无令牌的状态码 = %d，期望 401", status)
	}
	assertKeys(t, doc, errorKeys)

	// ---- 管理端：单一管理员，data 必须是数组 ----
	status, _, doc = call(t, srv, http.MethodGet, "/api/v1/users", "", token)
	if status != http.StatusOK {
		t.Fatalf("账号目录的状态码 = %d，期望 200", status)
	}
	if _, ok := doc["data"].([]any); !ok {
		t.Fatalf("账号目录的 data 不是数组: %#v", doc["data"])
	}

	// ---- 删除：data 是具名结构体 ----
	status, _, doc = call(t, srv, http.MethodDelete, path, "", token)
	if status != http.StatusOK {
		t.Fatalf("删除的状态码 = %d，期望 200", status)
	}
	deleted, _ := doc["data"].(map[string]any)
	if deleted["deleted"] != true {
		t.Errorf("data.deleted = %v，期望 true", deleted["deleted"])
	}
}

// assertRequestIDConsistent 守住「响应头与响应体的 request_id 一致」这条对外承诺。
func assertRequestIDConsistent(t *testing.T, hdr http.Header, doc map[string]any) {
	t.Helper()

	fromHeader := hdr.Get("X-Request-ID")
	fromBody, _ := doc["request_id"].(string)
	if fromHeader == "" {
		t.Fatal("响应头缺 X-Request-ID")
	}
	if fromHeader != fromBody {
		t.Fatalf("X-Request-ID(%q) 与响应体 request_id(%q) 不一致", fromHeader, fromBody)
	}
}

// TestUnknownRouteIsPlainText 守住兜底 404 的形状：
// 它不走信封（不是业务接口），改动信封逻辑时不应被连带改掉。
func TestUnknownRouteIsPlainText(t *testing.T) {
	srv := newTestServer(t)

	resp, err := srv.Client().Get(srv.URL + "/api/v1/nope")
	if err != nil {
		t.Fatalf("请求未知路由: %v", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("状态码 = %d，期望 404", resp.StatusCode)
	}
	if !bytes.Equal(raw, []byte("404 page not found")) {
		t.Errorf("404 正文 = %q，期望 %q", raw, "404 page not found")
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type = %q，期望 text/plain", ct)
	}
}
