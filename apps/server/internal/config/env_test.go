package config

import (
	"strings"
	"testing"
)

// 本文件守住「环境变量能覆盖任意配置项」这条承诺。
//
// 旧实现是一张手写的对照表，只有最常变的十几项能覆盖：新加的字段（database.port、
// cache.memory.save_interval…）配在环境变量里毫无反应，也不报错——部署时最常见的
// 表现是「明明注入了变量，服务还在连旧地址」。现在变量名由 yaml 路径推导，这里的
// 用例逐类型钉住转换结果。

const envFixture = `
app:
  env: dev
http:
  addr: ":9000"
  shutdown_timeout: 10
database:
  driver: sqlite
  seed: true
  params:
    sslmode: disable
cache:
  driver: memory
  memory:
    persist: true
jwt:
  secret: from-file
`

func TestEnvOverridesEveryType(t *testing.T) {
	path := writeConfig(t, envFixture)

	t.Setenv("APP_HTTP_ADDR", ":18080")                                      // string
	t.Setenv("APP_HTTP_SHUTDOWN_TIMEOUT", "30")                              // int
	t.Setenv("APP_DATABASE_SEED", "false")                                   // bool
	t.Setenv("APP_CACHE_MEMORY_PERSIST", "false")                            // *bool（三态开关）
	t.Setenv("APP_DATABASE_PORT", "5433")                                    // 配置文件里没有的键
	t.Setenv("APP_HTTP_CORS_ORIGINS", " https://a.test , https://b.test , ") // []string

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}

	if cfg.HTTP.Addr != ":18080" {
		t.Errorf("http.addr = %q，期望 :18080", cfg.HTTP.Addr)
	}
	if cfg.HTTP.ShutdownTimeout != 30 {
		t.Errorf("http.shutdown_timeout = %d，期望 30", cfg.HTTP.ShutdownTimeout)
	}
	if cfg.Database.Seed {
		t.Error("database.seed 仍为 true，期望被 APP_DATABASE_SEED=false 覆盖")
	}
	if cfg.Cache.Memory.Persist == nil || *cfg.Cache.Memory.Persist {
		t.Errorf("cache.memory.persist = %v，期望被覆盖成 false", cfg.Cache.Memory.Persist)
	}
	if cfg.Database.Port != 5433 {
		t.Errorf("database.port = %d，期望 5433（文件里没有这个键也要能覆盖）", cfg.Database.Port)
	}
	// 跨域白名单是精确匹配：带空格或者整串当一个元素，都会静默失效。
	if want := []string{"https://a.test", "https://b.test"}; len(cfg.HTTP.CORSOrigins) != 2 ||
		cfg.HTTP.CORSOrigins[0] != want[0] || cfg.HTTP.CORSOrigins[1] != want[1] {
		t.Errorf("cors_origins = %v，期望 %v", cfg.HTTP.CORSOrigins, want)
	}
	// 配置文件的其余键不受影响。
	if cfg.JWT.Secret != "from-file" {
		t.Errorf("jwt.secret = %q，期望仍为文件里的 from-file", cfg.JWT.Secret)
	}
}

// TestEnvEmptyValueKeepsFileValue：容器编排里 `VAR=${VAR}` 展开成空串很常见，
// 那种情况必须保持配置文件的值，不能把生产密钥清成空。
func TestEnvEmptyValueKeepsFileValue(t *testing.T) {
	path := writeConfig(t, envFixture)
	t.Setenv("APP_HTTP_ADDR", "")
	t.Setenv("APP_JWT_SECRET", "")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}
	if cfg.HTTP.Addr != ":9000" {
		t.Errorf("http.addr = %q，期望保持文件里的 :9000", cfg.HTTP.Addr)
	}
	if cfg.JWT.Secret != "from-file" {
		t.Errorf("jwt.secret = %q，期望保持文件里的 from-file", cfg.JWT.Secret)
	}
}

// TestEnvInvalidValueFailsFast：取值非法要拒绝启动并点名是哪个变量。
//
// 旧实现遇到 Atoi 失败会当成「没设置」直接跳过，于是 APP_DATABASE_PORT=abc 这种
// 手滑会安静地留在默认值上；换成拒绝启动是刻意的行为改变。
func TestEnvInvalidValueFailsFast(t *testing.T) {
	path := writeConfig(t, envFixture)
	t.Setenv("APP_HTTP_SHUTDOWN_TIMEOUT", "abc")

	_, err := Load(path)
	if err == nil {
		t.Fatal("非法取值仍然加载成功，期望报错")
	}
	if !strings.Contains(err.Error(), "APP_HTTP_SHUTDOWN_TIMEOUT") {
		t.Errorf("err = %v，期望点名出错的变量", err)
	}
}

// TestEnvSkipsMapFields：map 类型的键（database.params）没有环境变量出口，
// 传进来也应当被忽略，而不是拼出半个连接串。
func TestEnvSkipsMapFields(t *testing.T) {
	path := writeConfig(t, envFixture)
	t.Setenv("APP_DATABASE_PARAMS_SSLMODE", "require")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}
	if got := cfg.Database.Params["sslmode"]; got != "disable" {
		t.Errorf("params.sslmode = %q，期望仍为文件里的 disable", got)
	}
}

// TestEnvAppEnvSwitchesToProd：APP_ENV 必须真的能切到生产校验。
//
// 这条线路决定「环境变量有没有生效」这件事本身：env 没切过去，后面每一条生产
// 校验都会静默跳过，看起来一切正常。
func TestEnvAppEnvSwitchesToProd(t *testing.T) {
	path := writeConfig(t, envFixture) // jwt.secret 只有 10 字符，生产校验会拒绝
	t.Setenv("APP_ENV", "prod")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "jwt.secret") {
		t.Fatalf("err = %v，期望按生产校验拒绝（jwt.secret 太短）", err)
	}
}

// TestEnvName 钉住命名规则：前缀 + yaml 路径，app 段并进前缀。
func TestEnvName(t *testing.T) {
	cases := map[string]string{
		"app.env":                    "APP_ENV",
		"app.name":                   "APP_NAME",
		"http.addr":                  "APP_HTTP_ADDR",
		"database.dsn":               "APP_DATABASE_DSN",
		"cache.redis.addr":           "APP_CACHE_REDIS_ADDR",
		"cache.memory.save_interval": "APP_CACHE_MEMORY_SAVE_INTERVAL",
	}
	for key, want := range cases {
		if got := envName("APP_", key); got != want {
			t.Errorf("envName(\"APP_\", %q) = %q，期望 %q", key, got, want)
		}
	}
}

// TestEnvPrefixFor 钉住前缀的推导：app.name 大写、非字母数字变下划线、末尾补一个 "_"。
func TestEnvPrefixFor(t *testing.T) {
	cases := map[string]string{
		"app":        "APP_",
		"":           "APP_", // 文件里没写 name 时落回默认名
		"app-server": "APP_SERVER_",
		"Orders":     "ORDERS_",
		"app.v2":     "APP_V2_",
		"订单中心":       "APP_", // 没有可用字母数字，退回默认前缀
	}
	for name, want := range cases {
		if got := envPrefixFor(name); got != want {
			t.Errorf("envPrefixFor(%q) = %q，期望 %q", name, got, want)
		}
	}
}

// TestEnvPrefixFollowsAppName：app.name 决定变量命名空间。
//
// 同一份配置文件被两套服务共用时，这一点是必须的：否则两边的 APP_JWT_SECRET
// 会互相覆盖，而症状是「一套服务用着另一套服务的密钥」。
func TestEnvPrefixFollowsAppName(t *testing.T) {
	path := writeConfig(t, "app:\n  name: orders\njwt:\n  secret: from-file\n")

	t.Setenv("ORDERS_JWT_SECRET", "from-orders")
	t.Setenv("APP_JWT_SECRET", "must-be-ignored") // 旧前缀不再生效

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}
	if cfg.JWT.Secret != "from-orders" {
		t.Errorf("jwt.secret = %q，期望被 ORDERS_JWT_SECRET 覆盖", cfg.JWT.Secret)
	}
}

// TestEnvAppNameNotOverridable：app.name 不能用环境变量改。
//
// 它决定前缀，再让它走环境变量就成了自指：改了名字前缀也跟着变，没法回答
// 「该用哪个前缀去读 name」。这里把行为钉住，免得以后有人「顺手支持一下」。
func TestEnvAppNameNotOverridable(t *testing.T) {
	path := writeConfig(t, envFixture) // app.name 没写，落默认 app
	t.Setenv("APP_NAME", "hijack")
	t.Setenv("HIJACK_HTTP_ADDR", ":19000")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加载配置: %v", err)
	}
	if cfg.App.Name != "app" {
		t.Errorf("app.name = %q，期望仍是默认的 app（该值不可被环境变量覆盖）", cfg.App.Name)
	}
	if cfg.HTTP.Addr != ":9000" {
		t.Errorf("http.addr = %q，期望仍是文件里的 :9000（前缀没被 HIJACK_ 抢走）", cfg.HTTP.Addr)
	}
}
