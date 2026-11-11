package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"example.com/app/internal/bootstrap"
	"example.com/app/internal/config"
)

// sqliteDSN 给每个用例一份独立的库文件，用例之间不会互相看到对方的数据。
func sqliteDSN(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
		filepath.Join(t.TempDir(), "app.db"))
}

// devConfigYAML 是开发环境的等价配置：SQLite + 内存缓存 + 种子管理员。
func devConfigYAML(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`
app:
  name: app
  env: dev
  seed_admin_username: admin
  seed_admin_password: admin123
http:
  addr: ":0"
  mode: debug
database:
  driver: sqlite
  dsn: %q
  log_level: silent
  auto_migrate: true
  seed: true
cache:
  driver: memory
  prefix: "test:"
  memory:
    persist: false
log:
  level: error
  format: text
jwt:
  secret: "test-only-secret-0123456789abcdef"
  expire: 1
`, sqliteDSN(t))
}

// prodConfigYAML 是生产环境的等价配置：隐藏文档路由、不建种子账号。
func prodConfigYAML(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`
app:
  name: app
  env: prod
  seed_admin_username: ""
  seed_admin_password: ""
http:
  addr: ":0"
  mode: release
database:
  driver: sqlite
  dsn: %q
  log_level: silent
  auto_migrate: true
  seed: false
cache:
  driver: memory
  prefix: "test:"
  fallback_to_memory: false
  memory:
    persist: false
log:
  level: error
  format: text
jwt:
  secret: "prod-only-secret-0123456789abcdef"
  expire: 1
`, sqliteDSN(t))
}

// newTestApp 装配一个真实应用（SQLite + 内存缓存）并把 *bootstrap.App 交出来，
// 给需要直接改库的用例用（造普通账号、停用账号）。
func newTestApp(t *testing.T, cfgYAML string) (*httptest.Server, *bootstrap.App) {
	t.Helper()

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
		t.Fatalf("写测试配置: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("加载测试配置: %v", err)
	}

	app, err := bootstrap.New(cfg, "test")
	if err != nil {
		t.Fatalf("装配应用: %v", err)
	}
	t.Cleanup(app.Close)

	srv := httptest.NewServer(app.Server.Handler())
	t.Cleanup(srv.Close)
	return srv, app
}

// newTestServer 只要 HTTP 服务本身。
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, _ := newTestApp(t, devConfigYAML(t))
	return srv
}

// login 登录并返回令牌与账号 id。
func login(t *testing.T, srv *httptest.Server, username, password string) (string, float64) {
	t.Helper()

	status, _, doc := call(t, srv, http.MethodPost, "/api/v1/auth/login",
		fmt.Sprintf(`{"username":%q,"password":%q}`, username, password), "")
	if status != http.StatusOK {
		t.Fatalf("登录 %s 的状态码 = %d，期望 200（响应 %v）", username, status, doc)
	}

	data, _ := doc["data"].(map[string]any)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("登录 %s 没有拿到 token", username)
	}
	user, _ := data["user"].(map[string]any)
	id, _ := user["id"].(float64)
	return token, id
}
