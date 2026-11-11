// 本文件守住「谁能看到什么」与「登录态什么时候失效」这两类规则。
//
// 它们都不是编译期错误：范围写漏了只表现为别人的数据出现在列表里，回表校验
// 写漏了只表现为停用的账号还能用到令牌过期。只有真实请求能抓住。
package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"example.com/app/internal/model"
)

// TestNoteDataIsolation 验证普通用户只能拿到自己的记录，越权一律 404。
func TestNoteDataIsolation(t *testing.T) {
	srv, app := newTestApp(t, devConfigYAML(t))

	adminToken, adminID := login(t, srv, "admin", "admin123")

	status, _, doc := call(t, srv, http.MethodPost, "/api/v1/notes",
		`{"title":"admin-note","content":"secret"}`, adminToken)
	if status != http.StatusCreated {
		t.Fatalf("管理员建记录的状态码 = %d，期望 201（响应 %v）", status, doc)
	}
	created, _ := doc["data"].(map[string]any)
	noteID, _ := created["id"].(float64)
	if noteID == 0 {
		t.Fatal("新建响应里没有 id")
	}

	// 普通账号直接写库：本服务没有开放注册接口，测试也不该为它开一个。
	hash, err := bcrypt.GenerateFromPassword([]byte("alice-pass-123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("生成密码摘要: %v", err)
	}
	alice := &model.User{
		Username: "alice",
		Password: string(hash),
		Nickname: "Alice",
		Role:     model.RoleUser,
		Status:   model.EnableStatusOn,
	}
	if err := app.DB.Create(alice).Error; err != nil {
		t.Fatalf("创建普通账号: %v", err)
	}
	aliceToken, aliceID := login(t, srv, "alice", "alice-pass-123")

	// 列表里不该出现别人的记录。
	status, _, doc = call(t, srv, http.MethodGet, "/api/v1/notes", "", aliceToken)
	if status != http.StatusOK {
		t.Fatalf("列表的状态码 = %d，期望 200", status)
	}
	page, _ := doc["data"].(map[string]any)
	if page["total"] != float64(0) {
		t.Errorf("普通用户的列表 total = %v，期望 0", page["total"])
	}

	// 显式传别人的 owner_id 也不能把范围放开：范围由身份决定，不由 query 决定。
	status, _, doc = call(t, srv, http.MethodGet,
		fmt.Sprintf("/api/v1/notes?owner_id=%.0f", adminID), "", aliceToken)
	if status != http.StatusOK {
		t.Fatalf("带 owner_id 的列表状态码 = %d，期望 200", status)
	}
	page, _ = doc["data"].(map[string]any)
	if page["total"] != float64(0) {
		t.Errorf("按别人的 owner_id 过滤后的 total = %v，期望 0（不应越权）", page["total"])
	}

	notePath := fmt.Sprintf("/api/v1/notes/%.0f", noteID)

	// 读、改、删都该当「不存在」处理：403 等于确认这条 id 存在。
	status, _, doc = call(t, srv, http.MethodGet, notePath, "", aliceToken)
	if status != http.StatusNotFound || doc["code"] != float64(40401) {
		t.Errorf("越权读 = %d / code %v，期望 404 / 40401", status, doc["code"])
	}
	status, _, doc = call(t, srv, http.MethodPut, notePath, `{"title":"hijacked"}`, aliceToken)
	if status != http.StatusNotFound || doc["code"] != float64(40401) {
		t.Errorf("越权改 = %d / code %v，期望 404 / 40401", status, doc["code"])
	}
	status, _, doc = call(t, srv, http.MethodDelete, notePath, "", aliceToken)
	if status != http.StatusNotFound || doc["code"] != float64(40401) {
		t.Errorf("越权删 = %d / code %v，期望 404 / 40401", status, doc["code"])
	}

	// 自己建的那条必须能看到，否则上面几条可能是「全挂了」而不是「隔离生效」。
	status, _, doc = call(t, srv, http.MethodPost, "/api/v1/notes",
		`{"title":"alice-note","content":"mine"}`, aliceToken)
	if status != http.StatusCreated {
		t.Fatalf("普通用户建记录的状态码 = %d，期望 201（响应 %v）", status, doc)
	}
	created, _ = doc["data"].(map[string]any)
	if created["owner_id"] != aliceID {
		t.Errorf("owner_id = %v，期望取自令牌的 %v", created["owner_id"], aliceID)
	}

	status, _, doc = call(t, srv, http.MethodGet, "/api/v1/notes", "", aliceToken)
	page, _ = doc["data"].(map[string]any)
	if page["total"] != float64(1) {
		t.Errorf("普通用户自己的列表 total = %v，期望 1", page["total"])
	}

	// 管理员仍然看全部：限制只对非管理员生效。
	status, _, doc = call(t, srv, http.MethodGet, "/api/v1/notes", "", adminToken)
	if status != http.StatusOK {
		t.Fatalf("管理员列表的状态码 = %d，期望 200", status)
	}
	page, _ = doc["data"].(map[string]any)
	if page["total"] != float64(2) {
		t.Errorf("管理员的列表 total = %v，期望 2", page["total"])
	}
}

// TestPaginationLenient 验证「非法值归默认」这条对外承诺，包括非数字。
func TestPaginationLenient(t *testing.T) {
	srv := newTestServer(t)
	token, _ := login(t, srv, "admin", "admin123")

	status, _, doc := call(t, srv, http.MethodGet, "/api/v1/notes?page=abc&page_size=-5", "", token)
	if status != http.StatusOK {
		t.Fatalf("非数字分页参数的状态码 = %d，期望 200（响应 %v）", status, doc)
	}
	page, _ := doc["data"].(map[string]any)
	if page["page"] != float64(1) || page["page_size"] != float64(20) {
		t.Errorf("page/page_size = %v/%v，期望 1/20", page["page"], page["page_size"])
	}

	status, _, doc = call(t, srv, http.MethodGet, "/api/v1/notes?page=2&page_size=9999", "", token)
	if status != http.StatusOK {
		t.Fatalf("超上限分页参数的状态码 = %d，期望 200", status)
	}
	page, _ = doc["data"].(map[string]any)
	if page["page"] != float64(2) || page["page_size"] != float64(200) {
		t.Errorf("page/page_size = %v/%v，期望 2/200（超上限截断）", page["page"], page["page_size"])
	}
}

// TestDuplicateTitleConflict 验证重名预检真的命中（Exists 换实现后不能变成恒 false），
// 以及并发下靠唯一索引兜底的返回码一致。
func TestDuplicateTitleConflict(t *testing.T) {
	srv := newTestServer(t)
	token, _ := login(t, srv, "admin", "admin123")

	status, _, doc := call(t, srv, http.MethodPost, "/api/v1/notes", `{"title":"dup"}`, token)
	if status != http.StatusCreated {
		t.Fatalf("首次新建的状态码 = %d，期望 201（响应 %v）", status, doc)
	}

	status, _, doc = call(t, srv, http.MethodPost, "/api/v1/notes", `{"title":"dup"}`, token)
	if status != http.StatusConflict || doc["code"] != float64(40901) {
		t.Errorf("重名新建 = %d / code %v，期望 409 / 40901", status, doc["code"])
	}
}

// TestLoginLockout 验证失败计数是真的拦住了继续尝试，而不是只换一句文案。
func TestLoginLockout(t *testing.T) {
	srv := newTestServer(t)

	for i := 1; i <= 4; i++ {
		status, _, doc := call(t, srv, http.MethodPost, "/api/v1/auth/login",
			`{"username":"admin","password":"wrong-password"}`, "")
		if status != http.StatusUnauthorized {
			t.Fatalf("第 %d 次失败登录的状态码 = %d，期望 401（响应 %v）", i, status, doc)
		}
	}

	status, _, doc := call(t, srv, http.MethodPost, "/api/v1/auth/login",
		`{"username":"admin","password":"wrong-password"}`, "")
	if status != http.StatusForbidden || doc["code"] != float64(40301) {
		t.Errorf("达到上限的那次 = %d / code %v，期望 403 / 40301", status, doc["code"])
	}

	// 锁定期间即使密码正确也拒绝：否则锁定只是文案，爆破速率一点没降。
	status, _, doc = call(t, srv, http.MethodPost, "/api/v1/auth/login",
		`{"username":"admin","password":"admin123"}`, "")
	if status != http.StatusForbidden {
		t.Errorf("锁定期间用正确密码登录 = %d，期望 403（响应 %v）", status, doc)
	}
}

// TestDisabledAccountTokenRejected 验证停用账号后，已签发的令牌立刻失效。
func TestDisabledAccountTokenRejected(t *testing.T) {
	srv, app := newTestApp(t, devConfigYAML(t))
	token, _ := login(t, srv, "admin", "admin123")

	if err := app.DB.Model(&model.User{}).Where("username = ?", "admin").
		Update("status", int8(model.EnableStatusOff)).Error; err != nil {
		t.Fatalf("停用账号: %v", err)
	}

	status, _, doc := call(t, srv, http.MethodGet, "/api/v1/notes", "", token)
	if status != http.StatusForbidden || doc["code"] != float64(40301) {
		t.Errorf("停用账号的令牌 = %d / code %v，期望 403 / 40301", status, doc["code"])
	}
}

// TestDocsRoutesFollowEnv 验证文档路由只在非生产暴露。
//
// Fuego 的 OpenAPIConfig.Disabled 由 RegisterOpenAPIRoutes 内部短路，但这条
// 「生产不注册」的承诺只写在文档里，没有任何东西会在误配时报警。
func TestDocsRoutesFollowEnv(t *testing.T) {
	dev := newTestServer(t)
	for _, path := range []string{"/openapi.json", "/docs"} {
		if status, _ := rawGet(t, dev, path); status != http.StatusOK {
			t.Errorf("开发环境 GET %s = %d，期望 200", path, status)
		}
	}

	prod, _ := newTestApp(t, prodConfigYAML(t))
	for _, path := range []string{"/openapi.json", "/docs"} {
		if status, _ := rawGet(t, prod, path); status != http.StatusNotFound {
			t.Errorf("生产环境 GET %s = %d，期望 404", path, status)
		}
	}
}

// TestOpenAPIErrorSchema 验证文档里所有失败响应都指向统一信封。
//
// 这条是 sanitizeErrorResponses 的事后修补，属于「改了不会报错、只会让文档失真」
// 的那一类：Fuego 默认挂的 {title,status,detail} 一旦漏改，照着文档写的前端
// 就会读错字段，而接口本身完全正常。
func TestOpenAPIErrorSchema(t *testing.T) {
	srv := newTestServer(t)

	status, raw := rawGet(t, srv, "/openapi.json")
	if status != http.StatusOK {
		t.Fatalf("GET /openapi.json = %d，期望 200", status)
	}

	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("解析 OpenAPI 文档: %v", err)
	}

	if _, ok := spec.Components.Schemas["ErrorBody"]; !ok {
		t.Fatal("文档里没有 ErrorBody 组件：失败响应的形状会退回 Fuego 默认的 {title,status,detail}")
	}

	const wantRef = "#/components/schemas/ErrorBody"
	checked := 0
	for path, ops := range spec.Paths {
		for method, op := range ops {
			for code, resp := range op.Responses {
				n, err := strconv.Atoi(code)
				if err != nil || n < 400 {
					continue
				}
				checked++
				if got := resp.Content["application/json"].Schema.Ref; got != wantRef {
					t.Errorf("%s %s %s 的失败响应 schema = %q，期望 %q", method, path, code, got, wantRef)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("文档里一条失败响应声明都没有：说明 errorResponses 的调用漏了")
	}
}

// rawGet 不做 JSON 解析：文档与 404 的响应体都不是信封。
func rawGet(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()

	resp, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatalf("请求 %s: %v", path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读响应 %s: %v", path, err)
	}
	return resp.StatusCode, string(raw)
}
