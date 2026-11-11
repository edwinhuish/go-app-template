package config

// 本文件测连接串的拼装：它只在启动时跑一次，写错了却要到连库时才炸，
// 而且报错往往只给一句「连接失败」，看不出是哪个字段被拼坏了。

import (
	"strings"
	"testing"
)

func TestResolvedDSNPostgres(t *testing.T) {
	// 密码里有空格与引号：必须加单引号并转义，否则 libpq 会把后半段当成另一个参数。
	d := Database{
		Driver:   DriverPostgres,
		Host:     "db.internal",
		Port:     5432,
		User:     "app",
		Password: `p@ss word's`,
		Name:     "app",
	}

	dsn := d.ResolvedDSN()
	for _, want := range []string{
		"host=db.internal",
		"port=5432",
		"user=app",
		"dbname=app",
		`password='p@ss word\'s'`,
		// 非生产环境兜底成 disable：本地容器没有证书。
		"sslmode=disable",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN = %q，缺少 %q", dsn, want)
		}
	}
}

func TestResolvedDSNPostgresProdKeepsSSLMode(t *testing.T) {
	d := Database{
		Driver: DriverPostgres,
		Host:   "db",
		Port:   5432,
		User:   "app",
		Name:   "app",
		Params: map[string]string{"sslmode": "require"},
		prod:   true,
	}

	dsn := d.ResolvedDSN()
	if !strings.Contains(dsn, "sslmode=require") {
		t.Errorf("DSN = %q，期望使用配置里的 sslmode", dsn)
	}
	if strings.Contains(dsn, "sslmode=disable") {
		t.Errorf("DSN = %q，生产环境不该兜底成 disable", dsn)
	}
}

func TestResolvedDSNMySQL(t *testing.T) {
	d := Database{
		Driver: DriverMySQL,
		Host:   "db",
		Port:   3306,
		User:   "app",
		Name:   "app",
	}

	dsn := d.ResolvedDSN()
	// clientFoundRows 不是可选优化：仓储把 RowsAffected 当「命中的行数」用，
	// 缺了它，值没变化的 UPDATE 会被判成 404 / 409。
	if !strings.Contains(dsn, "clientFoundRows=true") {
		t.Errorf("DSN = %q，缺少 clientFoundRows=true", dsn)
	}
	if !strings.Contains(dsn, "charset=utf8mb4") {
		t.Errorf("DSN = %q，缺少 charset=utf8mb4", dsn)
	}
}

func TestResolvedDSNSQLite(t *testing.T) {
	d := Database{Driver: DriverSQLite}
	dsn := d.ResolvedDSN()
	if !strings.HasPrefix(dsn, "file:data/app.db?") {
		t.Errorf("DSN = %q，期望默认落到 data/app.db", dsn)
	}
	// WAL + busy_timeout 是这个工程对并发写的两条假设，拼装时不能被漏掉。
	if !strings.Contains(dsn, "journal_mode(WAL)") || !strings.Contains(dsn, "busy_timeout(5000)") {
		t.Errorf("DSN = %q，缺少 WAL 或 busy_timeout", dsn)
	}
}

func TestResolvedDSNExplicitWins(t *testing.T) {
	d := Database{Driver: DriverPostgres, DSN: "postgres://example", Name: "ignored"}
	if got := d.ResolvedDSN(); got != "postgres://example" {
		t.Errorf("DSN = %q，期望原样使用显式配置", got)
	}
}
