package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadDefaults 用最小配置覆盖「缺省值补齐」这条路径：
// 只写 jwt.secret，其余字段都应落到默认值上。
func TestLoadDefaults(t *testing.T) {
	path := writeConfig(t, "jwt:\n  secret: test-secret\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}

	if cfg.App.Name != "app" {
		t.Errorf("app.name 默认值 = %q，期望 app", cfg.App.Name)
	}
	if cfg.Database.Driver != DriverSQLite {
		t.Errorf("database.driver 默认值 = %q，期望 sqlite", cfg.Database.Driver)
	}
	if cfg.Cache.Driver != CacheMemory {
		t.Errorf("cache.driver 默认值 = %q，期望 memory", cfg.Cache.Driver)
	}
	if cfg.Cache.Prefix != "app:" {
		t.Errorf("cache.prefix 默认值 = %q，期望 app:", cfg.Cache.Prefix)
	}
	if cfg.JWT.ExpireDuration().Hours() != 24 {
		t.Errorf("jwt.expire 默认值 = %v，期望 24h", cfg.JWT.ExpireDuration())
	}
}

// TestLoadEnvOverride 验证环境变量优先级高于配置文件。
func TestLoadEnvOverride(t *testing.T) {
	path := writeConfig(t, "jwt:\n  secret: from-file\nhttp:\n  addr: \":9000\"\n")

	t.Setenv("APP_HTTP_ADDR", ":18080")
	t.Setenv("APP_LOG_LEVEL", "debug")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}

	if cfg.HTTP.Addr != ":18080" {
		t.Errorf("http.addr = %q，期望被环境变量覆盖成 :18080", cfg.HTTP.Addr)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("log.level = %q，期望 debug", cfg.Log.Level)
	}
}

// writeConfig 把内容写进临时目录，返回路径。
// 用 t.TempDir() 而不是手写清理：测试结束自动删除，失败时也不会留垃圾。
func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入临时配置: %v", err)
	}
	return path
}
