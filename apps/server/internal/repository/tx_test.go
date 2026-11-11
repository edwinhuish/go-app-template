package repository_test

// 本文件守住 TxManager 的语义：一处写失败，整段回滚。
//
// 为什么值得单独测：仓储方法都从 ctx 取连接（Base.conn → dbFor），所以「有没有真的
// 在同一个事务里」由这套约定决定，而不是由调用处看起来的嵌套结构决定。漏接一处，
// 表现是「失败路径上已经提交了一半」——平时跑得比谁都正常，只有出错时才有脏数据。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"example.com/app/internal/config"
	"example.com/app/internal/infra/database"
	"example.com/app/internal/model"
	"example.com/app/internal/repository"
)

type fixture struct {
	db    *gorm.DB
	notes repository.NoteRepository
	txs   repository.TxManager
}

// count 返回当前未删除的示例记录数（软删除的会被 gorm 自动排除）。
func (f fixture) count(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := f.db.Model(&model.Note{}).Count(&n).Error; err != nil {
		t.Fatalf("统计记录: %v", err)
	}
	return n
}

// newFixture 起一个独立的 SQLite 库，返回仓储、事务管理器与连接。
func newFixture(t *testing.T) fixture {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
		filepath.Join(t.TempDir(), "app.db"))

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(config.Database{Driver: config.DriverSQLite, DSN: dsn, LogLevel: "silent"}, log)
	if err != nil {
		t.Fatalf("打开测试库: %v", err)
	}
	t.Cleanup(func() { _ = database.Close(db) })

	if err := db.AutoMigrate(&model.Note{}); err != nil {
		t.Fatalf("建表: %v", err)
	}

	return fixture{db: db, notes: repository.NewNoteRepository(db), txs: repository.NewTxManager(db)}
}

// TestTxManagerRollback 验证 fn 返回错误时这段写入全部回滚。
func TestTxManagerRollback(t *testing.T) {
	f := newFixture(t)

	boom := errors.New("业务失败")
	err := f.txs.Transaction(context.Background(), func(ctx context.Context) error {
		if err := f.notes.Create(ctx, &model.Note{Title: "tx-1", Content: "in-tx"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Transaction 返回 %v，期望原样带回 %v", err, boom)
	}
	if n := f.count(t); n != 0 {
		t.Errorf("回滚后仍有 %d 条记录，期望 0", n)
	}
}

// TestTxManagerCommit 验证 fn 正常返回时写入落地。
func TestTxManagerCommit(t *testing.T) {
	f := newFixture(t)

	err := f.txs.Transaction(context.Background(), func(ctx context.Context) error {
		return f.notes.Create(ctx, &model.Note{Title: "tx-2", Content: "committed"})
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}
	if n := f.count(t); n != 1 {
		t.Errorf("提交后有 %d 条记录，期望 1", n)
	}
}

// TestTxManagerNested 验证内层不再开新事务：内层「成功」不能把外层将要回滚的写入提交掉。
func TestTxManagerNested(t *testing.T) {
	f := newFixture(t)

	boom := errors.New("外层失败")
	err := f.txs.Transaction(context.Background(), func(ctx context.Context) error {
		if err := f.txs.Transaction(ctx, func(ctx context.Context) error {
			return f.notes.Create(ctx, &model.Note{Title: "tx-nested", Content: "inner"})
		}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Transaction 返回 %v，期望 %v", err, boom)
	}
	if n := f.count(t); n != 0 {
		t.Errorf("外层回滚后仍有 %d 条记录，期望 0（内层不该自己提交）", n)
	}
}
