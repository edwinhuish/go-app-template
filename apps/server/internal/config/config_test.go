package config

import (
	"os"
	"path/filepath"
	"strings"
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

// TestProdGuards 覆盖生产环境的安全默认值。
//
// 这些校验的价值在于「配置写错就拒绝启动」：env 少一个字母会让后面每一条生产
// 校验都静默跳过，而那种错误在流水线里看不出来，只能等到线上才发现。
func TestProdGuards(t *testing.T) {
	const base = `
app:
  env: prod
http:
  mode: release
  cors_origins:
    - https://app.example.com
database:
  driver: sqlite
cache:
  driver: memory
jwt:
  secret: "0123456789abcdef0123456789abcdef"
`

	cases := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "生产基线通过",
			yaml: base,
		},
		{
			// 写成 " Prod " 时必须仍按生产校验，而不是绕过。
			name: "env 大小写与空格归一化",
			yaml: strings.Replace(base, "env: prod", `env: " Prod "`, 1),
		},
		{
			name:    "未知 env 拒绝启动",
			yaml:    strings.Replace(base, "env: prod", "env: production", 1),
			wantErr: "app.env",
		},
		{
			name: "生产禁止静默降级为内存缓存",
			yaml: strings.Replace(base, "  driver: memory",
				"  driver: memory\n  fallback_to_memory: true", 1),
			wantErr: "fallback_to_memory",
		},
		{
			// 空名单在中间件里的语义是「允许任意来源」：生产必须显式写清楚。
			name:    "生产必须配置 CORS 白名单",
			yaml:    strings.Replace(base, "  cors_origins:\n    - https://app.example.com\n", "", 1),
			wantErr: "cors_origins",
		},
		{
			// 负值放过去会在 time.NewTicker 里 panic，报错信息与配置项毫无关系。
			name: "负数的落盘间隔拒绝启动",
			yaml: strings.Replace(base, "  driver: memory",
				"  driver: memory\n  memory:\n    save_interval: -1", 1),
			wantErr: "save_interval",
		},
		{
			// 同一类笔误的另外两项：0 是「用默认值」，只有负数要拒绝。
			name: "负数的连接寿命拒绝启动",
			yaml: strings.Replace(base, "  driver: sqlite",
				"  driver: sqlite\n  max_lifetime: -1", 1),
			wantErr: "max_lifetime",
		},
		{
			name: "负数的 Redis 超时拒绝启动",
			yaml: strings.Replace(base, "  driver: memory",
				"  driver: memory\n  redis:\n    dial_timeout: -1", 1),
			wantErr: "dial_timeout",
		},
		{
			name: "生产禁止种子账号",
			yaml: strings.Replace(base, "  driver: sqlite",
				"  driver: sqlite\n  seed: true", 1),
			wantErr: "database.seed",
		},
		{
			// 生产不允许继承「本地容器没有证书」的 disable 兜底。
			name: "Postgres 生产必须显式声明 sslmode",
			yaml: strings.Replace(base, "  driver: sqlite",
				"  driver: postgres\n  host: db\n  user: app\n  name: app", 1),
			wantErr: "sslmode",
		},
		{
			name: "生产密钥长度不足",
			yaml: strings.Replace(base, `secret: "0123456789abcdef0123456789abcdef"`,
				`secret: "short"`, 1),
			wantErr: "32",
		},
		{
			name:    "令牌有效期必须为正",
			yaml:    base + "  expire: -1\n",
			wantErr: "jwt.expire",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, c.yaml))
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("期望加载成功，实际报错: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("期望报错包含 %q，实际加载成功", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("错误信息 = %q，期望包含 %q", err, c.wantErr)
			}
		})
	}
}

// TestCORSOriginsNormalized 验证环境变量里的白名单会被去掉空格与空项。
//
// 白名单是精确匹配，带空格的项永远命不中，而现象只是浏览器里一句 CORS 报错。
func TestCORSOriginsNormalized(t *testing.T) {
	t.Setenv("APP_HTTP_CORS_ORIGINS", " https://a.example.com , ,https://b.example.com ")

	cfg, err := Load(writeConfig(t, "jwt:\n  secret: test-secret\n"))
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}

	want := []string{"https://a.example.com", "https://b.example.com"}
	if len(cfg.HTTP.CORSOrigins) != len(want) {
		t.Fatalf("cors_origins = %v，期望 %v", cfg.HTTP.CORSOrigins, want)
	}
	for i := range want {
		if cfg.HTTP.CORSOrigins[i] != want[i] {
			t.Errorf("cors_origins[%d] = %q，期望 %q", i, cfg.HTTP.CORSOrigins[i], want[i])
		}
	}
}

// TestConfigTemplatesLoad 守住仓库里的四份配置模板本身能通过校验。
//
// 模板不参与编译，也不会被别的用例读到：新增一条生产校验之后，最容易漏的就是它们。
// 漏了的表现是「代码全绿、拿 config.prod.yaml 部署时起不来」，而那通常是发版当下
// 才发生的事。
func TestConfigTemplatesLoad(t *testing.T) {
	// 生产模板刻意把密钥留在环境变量里（模板本身不含秘密），这里补一个合法值。
	t.Setenv("APP_JWT_SECRET", strings.Repeat("k", 32))

	cases := []struct {
		file   string
		driver string // 空表示不校验驱动
	}{
		{file: "config.yaml", driver: DriverSQLite},
		{file: "config.mysql.yaml", driver: DriverMySQL},
		{file: "config.postgres.yaml", driver: DriverPostgres},
		{file: "config.prod.yaml", driver: DriverPostgres},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			cfg, err := Load(filepath.Join("..", "..", "configs", c.file))
			if err != nil {
				t.Fatalf("模板未通过校验: %v", err)
			}
			// 复制模板改名时最容易漏改驱动：文件名叫 mysql 却连 sqlite 不会报错。
			if c.driver != "" && cfg.Database.Driver != c.driver {
				t.Errorf("database.driver = %q，与文件名期望的 %q 不符", cfg.Database.Driver, c.driver)
			}
		})
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
