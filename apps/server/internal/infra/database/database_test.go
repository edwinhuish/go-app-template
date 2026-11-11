package database

// 本文件测两处「看不出错」的换算：sqlite 的 DSN → 目录、日志级别字符串 → 两套枚举。
//
// 前者错了会在 os.MkdirAll 时把目录名当成文件名（或反着来），后者错了会让
// 「把 log.level 调成 debug」对 SQL 日志完全不生效——两种都是静默的。

import (
	"log/slog"
	"testing"

	gormlogger "gorm.io/gorm/logger"
)

func TestSQLiteFilePath(t *testing.T) {
	cases := map[string]string{
		// 带 _pragma 的完整 DSN：目录只取 ? 之前的部分。
		"file:data/app.db?_pragma=busy_timeout(5000)": "data/app.db",
		"file:/abs/path/app.db":                       "/abs/path/app.db",
		// 内存库没有目录，不能被当成相对路径去建目录。
		"file::memory:?cache=shared": ":memory:",
		"file:app.db":                "app.db",
	}

	for dsn, want := range cases {
		if got := sqliteFilePath(dsn); got != want {
			t.Errorf("sqliteFilePath(%q) = %q，期望 %q", dsn, got, want)
		}
	}
}

func TestLogLevel(t *testing.T) {
	cases := map[string]gormlogger.LogLevel{
		"silent": gormlogger.Silent,
		"ERROR":  gormlogger.Error,
		"info":   gormlogger.Info,
		// 空值与未知值都退到 Warn：只打慢查询与错误，是安全的默认档。
		"":      gormlogger.Warn,
		"weird": gormlogger.Warn,
	}

	for in, want := range cases {
		if got := logLevel(in); got != want {
			t.Errorf("logLevel(%q) = %v，期望 %v", in, got, want)
		}
	}
}

func TestSlogLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug":  slog.LevelDebug,
		"error":  slog.LevelError,
		"silent": slog.LevelError, // 关掉 SQL 日志时也不该降到 Info
		"":       slog.LevelWarn,
		"warn":   slog.LevelWarn,
	}

	for in, want := range cases {
		if got := slogLevel(in); got != want {
			t.Errorf("slogLevel(%q) = %v，期望 %v", in, got, want)
		}
	}
}
