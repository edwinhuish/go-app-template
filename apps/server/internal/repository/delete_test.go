package repository

// 本文件覆盖 DeleteByID 内部的两条路径，都不经过 HTTP：
//   - 重复删除必须是 404，而不是把一个已删的行再删一次；
//   - 删除标记撞复合唯一索引时要自己换一个毫秒重试，而不是把等待（Sleep）
//     塞进请求路径，也不是把这种「同一毫秒的巧合」暴露成 409。
//
// 为什么用内部测试（package repository 而不是 repository_test）：第二条路径要求
// 「两次删除算出的标记完全相同」，真实时钟只能碰运气，只能把 softDeleteMarker
// 换成一个固定值。

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"example.com/app/internal/config"
	"example.com/app/internal/infra/database"
	"example.com/app/internal/model"
	"example.com/app/internal/pkg/errs"
)

func newDeleteFixture(t *testing.T) (*gorm.DB, NoteRepository) {
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
	return db, NewNoteRepository(db)
}

func TestDeleteByIDRejectsSecondDelete(t *testing.T) {
	_, repo := newDeleteFixture(t)
	ctx := context.Background()

	note := &model.Note{Title: "only-once", Content: "x"}
	if err := repo.Create(ctx, note); err != nil {
		t.Fatalf("创建: %v", err)
	}
	if err := repo.DeleteByID(ctx, note.ID); err != nil {
		t.Fatalf("删除: %v", err)
	}

	err := repo.DeleteByID(ctx, note.ID)
	e, ok := errs.From(err)
	if !ok || e.Code != errs.CodeNotFound {
		t.Fatalf("重复删除 err = %v，期望 404（已删的行不该再被命中一次）", err)
	}
}

func TestDeleteByIDAdvancesMarkerOnConflict(t *testing.T) {
	db, repo := newDeleteFixture(t)
	ctx := context.Background()

	// 固定标记，模拟「两条同名记录在同一毫秒被删」。
	marker := int64(1_700_000_000_000)
	old := softDeleteMarker
	softDeleteMarker = func() int64 { return marker }
	t.Cleanup(func() { softDeleteMarker = old })

	first := &model.Note{Title: "dup", Content: "a"}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("创建: %v", err)
	}
	if err := repo.DeleteByID(ctx, first.ID); err != nil {
		t.Fatalf("删除第一条: %v", err)
	}

	// 同名可以重建：唯一索引是 (title, deleted_at)，已删那条的标记不是 0。
	second := &model.Note{Title: "dup", Content: "b"}
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("重建同名: %v", err)
	}
	if err := repo.DeleteByID(ctx, second.ID); err != nil {
		t.Fatalf("撞标记后应自动换一个毫秒重试，实际: %v", err)
	}

	var markers []int64
	if err := db.Raw("SELECT deleted_at FROM biz_note ORDER BY id").Scan(&markers).Error; err != nil {
		t.Fatalf("读回标记: %v", err)
	}
	if len(markers) != 2 || markers[0] != marker || markers[1] != marker+1 {
		t.Errorf("deleted_at = %v，期望 [%d %d]", markers, marker, marker+1)
	}
}
