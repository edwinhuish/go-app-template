// Package config 负责加载与校验应用配置。
//
// 优先级：配置文件 < 环境变量（APP_ 前缀）。
// 生产环境的密码、连接串一律走环境变量，不进仓库。
//
// 存储相关的配置都遵循同一个思路：**用配置项切换实现，而不是改代码**。
// 数据库靠 database.driver（postgres | mysql | sqlite），
// 缓存靠 cache.driver（redis | memory），两者互不牵连，可以任意组合。
package config

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

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
	// Env 取 dev | prod，影响 gin 模式与若干安全校验。
	Env string `yaml:"env"`

	// SeedAdminUsername / SeedAdminPassword 仅在 database.seed=true 时生效，
	// 用于创建首个管理员账号。上线前务必改掉。
	SeedAdminUsername string `yaml:"seed_admin_username"`
	SeedAdminPassword string `yaml:"seed_admin_password"`
}

// HTTP 是 HTTP 服务配置。
type HTTP struct {
	Addr        string   `yaml:"addr"`
	Mode        string   `yaml:"mode"` // debug | release
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

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s: %w", path, err)
	}

	cfg.applyDefaults()
	cfg.applyEnv()

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

// applyEnv 只覆盖最常因环境而变的几项，其余仍以配置文件为准。
func (c *Config) applyEnv() {
	setStr(&c.App.Env, "APP_ENV")
	setStr(&c.App.SeedAdminUsername, "APP_SEED_ADMIN_USERNAME")
	setStr(&c.App.SeedAdminPassword, "APP_SEED_ADMIN_PASSWORD")

	setStr(&c.HTTP.Addr, "APP_HTTP_ADDR")
	setStr(&c.HTTP.Mode, "APP_HTTP_MODE")

	setStr(&c.Database.Driver, "APP_DB_DRIVER")
	setStr(&c.Database.DSN, "APP_DB_DSN")
	setStr(&c.Database.Host, "APP_DB_HOST")
	setStr(&c.Database.User, "APP_DB_USER")
	setStr(&c.Database.Password, "APP_DB_PASSWORD")
	setStr(&c.Database.Name, "APP_DB_NAME")
	setStr(&c.Database.LogLevel, "APP_DB_LOG_LEVEL")
	setInt(&c.Database.Port, "APP_DB_PORT")

	setStr(&c.Cache.Driver, "APP_CACHE_DRIVER")
	setStr(&c.Cache.Memory.Path, "APP_CACHE_MEMORY_PATH")
	setStr(&c.Cache.Redis.Addr, "APP_REDIS_ADDR")
	setStr(&c.Cache.Redis.Password, "APP_REDIS_PASSWORD")
	setInt(&c.Cache.Redis.DB, "APP_REDIS_DB")
	setInt(&c.Cache.Memory.SaveInterval, "APP_CACHE_SAVE_INTERVAL")
	setBool(&c.Cache.FallbackToMemory, "APP_CACHE_FALLBACK_TO_MEMORY")
	setBoolPtr(&c.Cache.Memory.Persist, "APP_CACHE_MEMORY_PERSIST")

	setStr(&c.JWT.Secret, "APP_JWT_SECRET")
	setStr(&c.Log.Level, "APP_LOG_LEVEL")
	setStr(&c.Log.Format, "APP_LOG_FORMAT")

	if v := os.Getenv("APP_HTTP_CORS_ORIGINS"); v != "" {
		c.HTTP.CORSOrigins = strings.Split(v, ",")
	}
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
	if c.IsProd() {
		if len(c.JWT.Secret) < 32 {
			return fmt.Errorf("生产环境 jwt.secret 至少 32 位")
		}
		if c.Database.Seed {
			return fmt.Errorf("生产环境必须关闭 database.seed")
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
		// 生产默认要求 TLS；本地容器没证书，这里只给「没配就用 disable」的兜底。
		if _, ok := d.Params["sslmode"]; !ok {
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

func setStr(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func setInt(dst *int, key string) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

func setBool(dst *bool, key string) {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*dst = b
		}
	}
}

// setBoolPtr 处理三态开关（未配置 / true / false），用于 persist 这类默认开启的选项。
func setBoolPtr(dst **bool, key string) {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*dst = &b
		}
	}
}
