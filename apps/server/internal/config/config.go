// Package config 负责加载与校验应用配置。
//
// 优先级：配置文件 < 环境变量。变量前缀取自 app.name（见 envPrefixFor）：
// 生产环境的密码、连接串一律走环境变量，不进仓库。
//
// 变量名由字段的 yaml 路径推导，前缀取自配置里的 app.name（大写 + "_"）：
//
//	app.name=app 时：
//	app.env                    -> APP_ENV（前缀本身代表 app 段）
//	http.addr                  -> APP_HTTP_ADDR
//	database.dsn               -> APP_DATABASE_DSN
//	cache.memory.save_interval -> APP_CACHE_MEMORY_SAVE_INTERVAL
//
//	app.name=orders 时同一批配置变成 ORDERS_ENV、ORDERS_HTTP_ADDR……
//
// app.name 本身不能用环境变量改：它决定前缀，再让它走环境变量就成了自指。
//
// 加一个字段就自动多了对应的环境变量；取值非法（APP_DATABASE_PORT=abc）会拒绝启动，
// 而不是像以前那样当作没设置——那种静默忽略最贵的后果是「生产的密码没生效」。
//
// 存储相关的配置都遵循同一个思路：**用配置项切换实现，而不是改代码**。
// 数据库靠 database.driver（postgres | mysql | sqlite），
// 缓存靠 cache.driver（redis | memory），两者互不牵连，可以任意组合。
package config

import (
	"fmt"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"

	"example.com/app/internal/pkg/dialect"
)

// 支持的数据库驱动。取值定义在 pkg/dialect，配置校验与连接装配共用同一组字面量。
const (
	DriverPostgres = dialect.Postgres
	DriverMySQL    = dialect.MySQL
	DriverSQLite   = dialect.SQLite
)

// 支持的缓存驱动。
const (
	CacheRedis  = "redis"
	CacheMemory = "memory"
)

// defaultPorts 是各驱动的默认端口，未显式配置时使用。
var defaultPorts = map[string]int{
	DriverPostgres: 5432,
	DriverMySQL:    3306,
}

// Config 是应用配置的根。
type Config struct {
	App      App      `yaml:"app"`
	HTTP     HTTP     `yaml:"http"`
	Database Database `yaml:"database"`
	Cache    Cache    `yaml:"cache"`
	JWT      JWT      `yaml:"jwt"`
	Log      Log      `yaml:"log"`
}

// App 是应用级元信息。
type App struct {
	// Name 用于日志、缓存前缀等处的标识，换成自己的项目名即可。
	Name string `yaml:"name"`
	// Env 取 dev | prod，影响若干启动期安全校验。
	Env string `yaml:"env"`

	// SeedAdminUsername / SeedAdminPassword 仅在 database.seed=true 时生效，
	// 用于创建首个管理员账号。上线前务必改掉。
	SeedAdminUsername string `yaml:"seed_admin_username"`
	SeedAdminPassword string `yaml:"seed_admin_password"`
}

// HTTP 是 HTTP 服务配置。
type HTTP struct {
	Addr string `yaml:"addr"`
	// Mode 取 debug | release。release（或 app.env=prod）时不注册 /openapi.json
	// 与 /docs 这两条文档路由，生产对外接口集合因此与不带文档时一致。
	Mode        string   `yaml:"mode"`
	CORSOrigins []string `yaml:"cors_origins"`
	// ShutdownTimeout 是优雅退出的等待秒数。
	ShutdownTimeout int `yaml:"shutdown_timeout"`
}

// Database 描述数据库连接。
//
// 两种写法二选一：
//  1. 直接给 dsn —— 完全掌控连接串，适合云厂商托管的特殊参数；
//  2. 拆成 host/port/user/password/name + params —— 换驱动时只改 driver 一个字段，
//     其余键名不变（postgres 与 mysql 共用同一组键），避免换库就得重写连接串。
type Database struct {
	Driver   string `yaml:"driver"` // postgres | mysql | sqlite
	DSN      string `yaml:"dsn"`    // 显式连接串，非空时忽略下面的拆分字段
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
	// Params 是附加连接参数，语义按驱动解释：
	// postgres 拼成 key=value（sslmode、search_path…），mysql 拼成 query（charset、loc…）。
	Params map[string]string `yaml:"params"`

	MaxOpen     int  `yaml:"max_open"`
	MaxIdle     int  `yaml:"max_idle"`
	MaxLifetime int  `yaml:"max_lifetime"` // 秒
	AutoMigrate bool `yaml:"auto_migrate"`
	Seed        bool `yaml:"seed"`
	// LogLevel 取 silent | error | warn | info，默认 warn（只打慢查询与错误）。
	LogLevel string `yaml:"log_level"`

	// prod 由 Load 按 app.env 填入，不来自配置文件：它决定连接串里的安全默认值
	// （见 ResolvedDSN），不能交给使用方随手覆盖。
	prod bool
}

// Cache 描述缓存实现。
type Cache struct {
	Driver string `yaml:"driver"` // redis | memory
	// Prefix 统一加在业务 key 前，两套实现都生效，切换驱动后 key 语义不变。
	Prefix string `yaml:"prefix"`
	// FallbackToMemory 仅在 driver=redis 时有意义：连接失败是否退回内存实现。
	// 生产应设为 false —— 带病降级会让人误以为多实例共享还在生效。
	FallbackToMemory bool         `yaml:"fallback_to_memory"`
	Redis            RedisConfig  `yaml:"redis"`
	Memory           MemoryConfig `yaml:"memory"`
}

// RedisConfig 是 redis 连接参数。
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	// DialTimeout 是连接与读写超时（秒），默认 2。
	DialTimeout int `yaml:"dial_timeout"`
}

// MemoryConfig 控制进程内缓存的行为。
//
// 内存实现支持落盘：退出时把未过期数据写进快照，下次启动加载回来。
// 这样开发环境不用起 Redis，重启后登录态、限流计数也不会突然清零。
type MemoryConfig struct {
	// Persist 是否落盘，默认 true（仅 driver=memory 生效）。
	Persist *bool `yaml:"persist"`
	// Path 是快照文件路径，默认 ./data/cache.snapshot；目录不存在会自动创建。
	Path string `yaml:"path"`
	// SaveInterval 是定时落盘间隔（秒），默认 30。
	// 每次写操作只置脏标记，真正的磁盘 IO 由这个间隔合并。
	SaveInterval int `yaml:"save_interval"`
}

// JWT 是令牌签发参数。
type JWT struct {
	Secret string `yaml:"secret"`
	Expire int    `yaml:"expire"` // 小时
}

// Log 是日志配置。
type Log struct {
	Level  string `yaml:"level"`  // debug | info | warn | error
	Format string `yaml:"format"` // text | json
}

// Load 读取配置文件并应用默认值与环境变量覆盖。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}

	k := koanf.New(".")
	if err := k.Load(rawbytes.Provider(raw), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s: %w", path, err)
	}

	// 前缀先按文件里的 app.name 定下来，再覆盖其余键：环境变量命名空间跟着项目走，
	// 同一个配置文件换个 app.name 就是另一套变量。
	applied, err := applyEnv(k, envPrefixFor(k.String("app.name")))
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{
		Tag: "yaml",
		DecoderConfig: &mapstructure.DecoderConfig{
			TagName: "yaml",
			// 环境变量一律是字符串，int/bool/指针都要靠弱类型转换落地。
			WeaklyTypedInput: true,
			// 列表要单独处理：弱类型转换会把 "a,b" 整体塞成一个元素。
			DecodeHook: stringToSlice,
			Result:     &cfg,
		},
	}); err != nil {
		return nil, fmt.Errorf("解码配置失败（本次应用的环境变量：%s）: %w",
			strings.Join(applied, " "), err)
	}

	cfg.applyDefaults()

	// 环境可能在 APP_ENV 里被改写，安全默认值要等两轮覆盖都结束再定。
	cfg.Database.prod = cfg.IsProd()

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.App.Name == "" {
		c.App.Name = "app"
	}
	if c.App.Env == "" {
		c.App.Env = "dev"
	}
	// 归一化后再判断：写成 "Prod" / " prod" 会绕过全部生产校验（fail-open），
	// 而那种写法在环境变量里太容易出现了。
	c.App.Env = strings.ToLower(strings.TrimSpace(c.App.Env))
	if c.App.SeedAdminUsername == "" {
		c.App.SeedAdminUsername = "admin"
	}
	if c.App.SeedAdminPassword == "" {
		c.App.SeedAdminPassword = "admin123"
	}

	if c.HTTP.Addr == "" {
		c.HTTP.Addr = ":8080"
	}
	if c.HTTP.Mode == "" {
		c.HTTP.Mode = "debug"
	}
	if c.HTTP.ShutdownTimeout == 0 {
		c.HTTP.ShutdownTimeout = 10
	}
	c.HTTP.CORSOrigins = normalizeList(c.HTTP.CORSOrigins)

	if c.Database.Driver == "" {
		c.Database.Driver = DriverSQLite
	}
	if c.Database.Port == 0 {
		c.Database.Port = defaultPorts[c.Database.Driver]
	}
	if c.Database.MaxOpen == 0 {
		c.Database.MaxOpen = 50
	}
	if c.Database.MaxIdle == 0 {
		c.Database.MaxIdle = 10
	}
	if c.Database.MaxLifetime == 0 {
		c.Database.MaxLifetime = 3600
	}
	if c.Database.LogLevel == "" {
		c.Database.LogLevel = "warn"
	}

	if c.Cache.Driver == "" {
		c.Cache.Driver = CacheMemory
	}
	if c.Cache.Prefix == "" {
		c.Cache.Prefix = c.App.Name + ":"
	}
	if c.Cache.Redis.DialTimeout == 0 {
		c.Cache.Redis.DialTimeout = 2
	}
	if c.Cache.Memory.Persist == nil {
		c.Cache.Memory.Persist = boolPtr(true)
	}
	if c.Cache.Memory.Path == "" {
		c.Cache.Memory.Path = "data/cache.snapshot"
	}
	if c.Cache.Memory.SaveInterval == 0 {
		c.Cache.Memory.SaveInterval = 30
	}

	if c.JWT.Expire == 0 {
		c.JWT.Expire = 24
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "text"
	}
}

// envPrefixFor 由 app.name 推出环境变量前缀：转大写、非 ASCII 字母数字换成下划线，
// 末尾补一个 "_"。
//
//	app         -> APP_
//	app-server  -> APP_SERVER_
//	（未配置）  -> APP_
//
// 这样同一个模板里跑两套服务（app.name 不同）时，两套变量不会互相覆盖；
// 也意味着换个项目名就连带换了变量命名空间，不需要再改代码。
//
// 名字里没有任何可用的 ASCII 字母数字（纯中文之类）时退回 APP_：否则所有变量都会
// 变成 `_ENV` 这种既拼不出来、也看不出归属的名字。
func envPrefixFor(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "app"
	}

	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - 'a' + 'A')
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	prefix := strings.Trim(b.String(), "_")
	if prefix == "" {
		return "APP_"
	}
	return prefix + "_"
}

// applyEnv 把环境变量写进配置树，返回本次真正生效的变量名（用于报错时给线索）。
//
// 变量名不是手写的清单，而是从结构体推导：加一个字段就自动多一个环境变量，
// 不会出现「配置文件加了键、环境变量忘了加」这种只有部署时才发现的不一致。
//
// 空字符串当作「没有设置」：容器编排里 `VAR=${VAR}` 展开成空串很常见，那种情况
// 应当保持配置文件的值，而不是把生产密钥清成空。
//
// map 类型的键（database.params）不提供环境变量出口：它按驱动解释，硬塞一个字符串
// 会拼出非法连接串，需要改这类参数的场景请直接改配置文件的 params 段。
func applyEnv(k *koanf.Koanf, prefix string) ([]string, error) {
	keys := envKeys()

	seen := make(map[string]string, len(keys))
	applied := make([]string, 0, len(keys))

	for _, key := range keys {
		// app.name 决定前缀本身：再让它走环境变量就是自指——改了名字前缀也跟着变，
		// 于是「用哪个前缀去读 name」没有答案。重命名请直接改配置文件。
		if key == "app.name" {
			continue
		}

		name := envName(prefix, key)
		if prev, dup := seen[name]; dup {
			return nil, fmt.Errorf("配置项 %s 与 %s 推导出同一个环境变量名 %s，请改名",
				prev, key, name)
		}
		seen[name] = key

		val, ok := os.LookupEnv(name)
		if !ok || val == "" {
			continue
		}
		k.Set(key, val)
		applied = append(applied, name)
	}
	return applied, nil
}

// envKeys 用反射遍历配置结构体的 yaml tag，列出每个叶子字段的配置路径。
//
// 只认 yaml tag：它是配置文件里真实存在的键，也是文档表格里那一列。
// 未导出字段（database.prod 这类由程序填入的状态）跳过。
func envKeys() []string {
	var out []string

	var walk func(t reflect.Type, path string)
	walk = func(t reflect.Type, path string) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag, ok := f.Tag.Lookup("yaml")
			if !ok || tag == "" || tag == "-" {
				continue
			}

			key := tag
			if path != "" {
				key = path + "." + tag
			}

			typ := f.Type
			if typ.Kind() == reflect.Ptr {
				typ = typ.Elem()
			}
			switch typ.Kind() {
			case reflect.Struct:
				walk(typ, key)
			case reflect.Map:
				// 见 applyEnv 的说明：map 不做环境变量覆盖。
			default:
				out = append(out, key)
			}
		}
	}
	walk(reflect.TypeOf(Config{}), "")

	return out
}

// envName 把配置路径翻成环境变量名：app. 段并进前缀，层级与键名统一用下划线。
//
//	database.dsn               -> APP_DATABASE_DSN
//	cache.memory.save_interval -> APP_CACHE_MEMORY_SAVE_INTERVAL
//
// 名字是从路径「生成」的，不再反向解析回来，所以键名里的下划线（save_interval）
// 不会被误当成层级分隔。
func envName(prefix, key string) string {
	key = strings.TrimPrefix(key, "app.")
	return prefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// stringToSlice 让 `APP_HTTP_CORS_ORIGINS=https://a,https://b` 覆盖成两项。
//
// 弱类型转换只会把整串塞进一个元素（于是跨域全部失效且不报错），必须显式按逗号拆，
// 并顺手去掉首尾空格与空项——环境变量里带空格是最常见的手滑。
func stringToSlice(from, to reflect.Type, data any) (any, error) {
	if from.Kind() != reflect.String || to.Kind() != reflect.Slice {
		return data, nil
	}
	raw, ok := data.(string)
	if !ok {
		return data, nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// Validate 校验启动必需项，把配置问题暴露在启动阶段而不是首次请求。
func (c *Config) Validate() error {
	if err := c.Database.Validate(); err != nil {
		return err
	}
	if err := c.Cache.Validate(); err != nil {
		return err
	}

	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret 不能为空")
	}
	if c.JWT.Expire <= 0 {
		return fmt.Errorf("jwt.expire 必须大于 0（单位小时）")
	}
	if c.HTTP.ShutdownTimeout <= 0 {
		return fmt.Errorf("http.shutdown_timeout 必须大于 0（单位秒）")
	}
	// 负数是笔误、0 是「用默认值」。放过去会在别处炸：负数交给 time.NewTicker
	// 是 panic，而报错信息里根本看不出是哪个配置项引起的。
	if c.Database.MaxLifetime < 0 {
		return fmt.Errorf("database.max_lifetime 不能为负（0 表示取默认值）")
	}
	if c.Cache.Memory.SaveInterval < 0 {
		return fmt.Errorf("cache.memory.save_interval 不能为负（0 表示取默认值）")
	}
	if c.Cache.Redis.DialTimeout < 0 {
		return fmt.Errorf("cache.redis.dial_timeout 不能为负（0 表示取默认值）")
	}

	// 只认两个值：未知取值意味着下面所有生产校验都不会执行，
	// 而「env 写错了所以生产校验没生效」是最不该靠运气的事。
	switch c.App.Env {
	case "dev", "prod":
	default:
		return fmt.Errorf("app.env 只支持 dev / prod，实际 %q", c.App.Env)
	}

	if c.IsProd() {
		if len(c.JWT.Secret) < 32 {
			return fmt.Errorf("生产环境 jwt.secret 至少 32 位")
		}
		if c.Database.Seed {
			return fmt.Errorf("生产环境必须关闭 database.seed")
		}
		if c.Cache.FallbackToMemory {
			return fmt.Errorf("生产环境必须关闭 cache.fallback_to_memory：" +
				"静默退回进程内缓存会让多实例之间的会话与计数立刻不一致")
		}
		// 空名单在 CORS 中间件里的语义是「允许任意来源」——那是开发环境的顺手，
		// 不该成为生产的默认值。这里直接拒绝启动，而不是等某天发现任何站点都能读响应。
		if len(c.HTTP.CORSOrigins) == 0 {
			return fmt.Errorf("生产环境必须显式配置 http.cors_origins：" +
				"空列表会退化成允许任意来源")
		}
		// 生产不允许「连库时不加密」这种事悄悄发生：本地容器默认 disable 是合理的，
		// 但到了生产必须有人显式做这个决定，而不是继承开发环境的兜底。
		if c.Database.Driver == DriverPostgres && c.Database.DSN == "" {
			if _, ok := c.Database.Params["sslmode"]; !ok {
				return fmt.Errorf("生产环境必须显式配置 database.params.sslmode（如 require）")
			}
		}
	}
	return nil
}

// Validate 校验数据库配置是否足以建立连接。
func (d Database) Validate() error {
	switch d.Driver {
	case DriverSQLite:
		// sqlite 允许 dsn 与 name 都为空，此时走默认文件路径。
	case DriverPostgres, DriverMySQL:
		if d.DSN != "" {
			return nil
		}
		if d.Host == "" || d.Name == "" || d.User == "" {
			return fmt.Errorf("database.driver=%s 时需要 dsn，或同时配置 host / name / user", d.Driver)
		}
	default:
		return fmt.Errorf("不支持的 database.driver: %s（可选 %s / %s / %s）",
			d.Driver, DriverPostgres, DriverMySQL, DriverSQLite)
	}
	return nil
}

// Validate 校验缓存配置。
func (c Cache) Validate() error {
	switch c.Driver {
	case CacheMemory:
		if c.Memory.Persist != nil && *c.Memory.Persist && c.Memory.Path == "" {
			return fmt.Errorf("cache.driver=memory 且开启持久化时必须配置 cache.memory.path")
		}
	case CacheRedis:
		if c.Redis.Addr == "" {
			return fmt.Errorf("cache.driver=redis 时必须配置 cache.redis.addr")
		}
	default:
		return fmt.Errorf("不支持的 cache.driver: %s（可选 %s / %s）", c.Driver, CacheRedis, CacheMemory)
	}
	return nil
}

// ResolvedDSN 返回实际使用的连接串。
// 显式配置了 dsn 就用它；否则按驱动把拆分字段拼成该驱动认可的格式。
func (d Database) ResolvedDSN() string {
	if d.DSN != "" {
		return d.DSN
	}

	switch d.Driver {
	case DriverPostgres:
		kv := map[string]string{
			"host":   d.Host,
			"port":   strconv.Itoa(d.Port),
			"user":   d.User,
			"dbname": d.Name,
		}
		if d.Password != "" {
			kv["password"] = d.Password
		}
		// 只在非生产环境兜底成 disable：本地容器没有证书，不这样连不上。
		// 生产必须由配置显式声明（Validate 会强制），不在这里替人做「悄悄不加密」的决定。
		if _, ok := d.Params["sslmode"]; !ok && !d.prod {
			kv["sslmode"] = "disable"
		}
		for k, v := range d.Params {
			kv[k] = v
		}
		return postgresDSN(kv)

	case DriverMySQL:
		query := url.Values{}
		query.Set("charset", "utf8mb4")
		query.Set("parseTime", "True")
		query.Set("loc", "Local")
		// 让 UPDATE 返回「命中的行数」而不是「真正改动的行数」：
		// 仓储的乐观锁与 404 判断都建立在 RowsAffected 上，而 MySQL 默认语义
		// 是后者——值没变时返回 0，同一条 SQL 在 SQLite/Postgres 上是 200。
		query.Set("clientFoundRows", "true")
		for k, v := range d.Params {
			query.Set(k, v)
		}
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
			d.User, d.Password, d.Host, d.Port, d.Name, query.Encode())

	default:
		// sqlite：dsn 已在上方返回；走到这里说明只想给个文件名。
		name := d.Name
		if name == "" {
			name = "data/app.db"
		}
		// WAL 让读不阻塞写，busy_timeout 让并发写等锁而不是直接报错。
		return fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", name)
	}
}

// postgresDSN 拼 key=value 形式的连接串，含空格或引号的值需要加单引号。
func postgresDSN(kv map[string]string) string {
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := kv[k]
		if v == "" || strings.ContainsAny(v, " '\\") {
			v = "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v) + "'"
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

// IsProd 判断是否生产环境。
func (c *Config) IsProd() bool { return c.App.Env == "prod" }

// ShutdownDuration 返回优雅退出的等待时长。
func (c *Config) ShutdownDuration() time.Duration {
	return time.Duration(c.HTTP.ShutdownTimeout) * time.Second
}

// ExpireDuration 返回令牌有效期。
func (j JWT) ExpireDuration() time.Duration {
	return time.Duration(j.Expire) * time.Hour
}

// PersistEnabled 判断内存缓存是否落盘。
func (m MemoryConfig) PersistEnabled() bool {
	return m.Persist != nil && *m.Persist && m.Path != ""
}

func boolPtr(b bool) *bool { return &b }

// normalizeList 去掉空项与首尾空格。
//
// 跨域白名单是精确匹配：APP_HTTP_CORS_ORIGINS="https://a, https://b" 按逗号拆出来
// 的第二项带着前导空格，永远命不中，表现为「浏览器里就是 CORS 报错，配置看着没错」。
func normalizeList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
