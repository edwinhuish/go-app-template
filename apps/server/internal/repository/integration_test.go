// 本文件在真实驱动上验证仓储语义。默认跳过：CI 与本地单测都不需要数据库。
//
// 为什么必须有它：SQLite 跑不出驱动差异，而下面这几条恰好都是驱动相关的——
//   - MySQL 默认的 UPDATE 返回「真正改动的行数」，值没变时是 0，仓储会把
//     「保存成功」误判成 404。连接串必须带 clientFoundRows（见 config.ResolvedDSN）；
//   - 软删除的唯一索引 (title, deleted_at) 在三种库上的表现要一致；
//   - 乐观锁的 updated_at 往返精度：MySQL 的 datetime(3) 截到毫秒，Postgres 到微秒。
//
// 跑法（起一个一次性容器，测完删掉）：
//
//	docker run --rm -d --name probe-mysql -p 13306:3306 \
//	  -e MYSQL_ROOT_PASSWORD=root123 -e MYSQL_DATABASE=app \
//	  -e MYSQL_USER=app -e MYSQL_PASSWORD=app mysql:8.4
//	APP_TEST_DRIVER=mysql APP_TEST_PORT=13306 \
//	  go test ./internal/repository -run TestRepositoryAgainstRealDriver -v
package repository_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	"example.com/app/internal/config"
	"example.com/app/internal/infra/database"
	"example.com/app/internal/model"
	"example.com/app/internal/pkg/errs"
	"example.com/app/internal/repository"
)

func TestRepositoryAgainstRealDriver(t *testing.T) {
	driver := os.Getenv("APP_TEST_DRIVER")
	if driver == "" {
		t.Skip("未设置 APP_TEST_DRIVER，跳过真实驱动用例")
	}

	db := openTestDB(t, driver)
	if err := database.AutoMigrate(db, &model.Note{}); err != nil {
		t.Fatalf("同步表结构: %v", err)
	}
	// 每个用例从空表开始：容器可能被复用来跑多次。
	if err := db.Exec("DELETE FROM biz_note").Error; err != nil {
		t.Fatalf("清空表: %v", err)
	}

	repo := repository.NewNoteRepository(db)
	ctx := context.Background()

	title := "probe-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	note := &model.Note{Title: title, Content: "v1", OwnerID: 1}
	if err := repo.Create(ctx, note); err != nil {
		t.Fatalf("创建: %v", err)
	}

	// --- ExistsByTitle：判存在必须取到真实结果（换实现后不能恒 false）---
	if ok, err := repo.ExistsByTitle(ctx, title, 0); err != nil || !ok {
		t.Fatalf("ExistsByTitle(存在) = %v, %v，期望 true", ok, err)
	}
	if ok, err := repo.ExistsByTitle(ctx, title+"-missing", 0); err != nil || ok {
		t.Fatalf("ExistsByTitle(不存在) = %v, %v，期望 false", ok, err)
	}

	// --- 唯一约束：重复标题要被翻成 409，而不是裸的驱动错误 ---
	err := repo.Create(ctx, &model.Note{Title: title, Content: "dup"})
	assertCode(t, err, errs.CodeConflict, "重复标题")

	// --- 无条件更新：即使值没变也必须算成功 ---
	if err := repo.UpdateFields(ctx, note.ID, map[string]any{"title": title, "content": "v1"}); err != nil {
		t.Fatalf("无条件更新: %v（MySQL 未启用 clientFoundRows 时会报 404）", err)
	}

	// 直接量一次「值没变」的行数：这是 clientFoundRows 的唯一判据。
	// 三家驱动在启用该选项后都返回「匹配行数」，所以这里必须是 1。
	res := db.Exec("UPDATE biz_note SET title = title WHERE id = ?", note.ID)
	if res.Error != nil {
		t.Fatalf("探测 RowsAffected: %v", res.Error)
	}
	if res.RowsAffected != 1 {
		t.Errorf("RowsAffected = %d，期望 1：MySQL 连接串缺 clientFoundRows 时仓储会把「值没变」误判成 404",
			res.RowsAffected)
	}

	// --- 乐观锁：带当前版本号能改，带过期版本号是 409 ---
	current, err := repo.FindByID(ctx, note.ID)
	if err != nil {
		t.Fatalf("读取: %v", err)
	}
	if err := repo.UpdateFields(ctx, note.ID, map[string]any{
		"title": title, "content": "v2",
		repository.FieldUpdatedAt: current.UpdatedAt,
	}); err != nil {
		t.Fatalf("带版本号更新: %v（updated_at 往返精度可能对不上）", err)
	}

	err = repo.UpdateFields(ctx, note.ID, map[string]any{
		"title": title, "content": "v3",
		repository.FieldUpdatedAt: current.UpdatedAt.Add(-time.Hour),
	})
	assertCode(t, err, errs.CodeConflict, "过期版本号")

	// --- 更新不存在的记录是 404，不是 409 ---
	err = repo.UpdateFields(ctx, 1<<30, map[string]any{"title": "no-such-row"})
	assertCode(t, err, errs.CodeNotFound, "更新不存在的记录")

	// --- 软删除：删两次第二次是 404，删完同名可以重建 ---
	if err := repo.DeleteByID(ctx, note.ID); err != nil {
		t.Fatalf("软删除: %v", err)
	}
	assertCode(t, repo.DeleteByID(ctx, note.ID), errs.CodeNotFound, "重复删除")

	if err := repo.Create(ctx, &model.Note{Title: title, Content: "reborn"}); err != nil {
		t.Errorf("软删除后同名应可重建: %v（唯一索引没把 deleted_at 排进去？）", err)
	}

	// --- 分页：软删的行不该出现在结果里 ---
	list, total, err := repo.Paginate(ctx, repository.Pagination{Page: 1, PageSize: 10}, repository.NoteFilter{})
	if err != nil {
		t.Fatalf("分页: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Errorf("分页 total/len = %d/%d，期望 1/1（软删的行不该计入）", total, len(list))
	}
}

// openTestDB 用与生产相同的装配路径连接数据库：走 config.Database.ResolvedDSN，
// 这样 MySQL 的 clientFoundRows 之类「连接串细节」也在被测范围内。
func openTestDB(t *testing.T, driver string) *gorm.DB {
	t.Helper()

	cfg := config.Database{
		Driver:      driver,
		Host:        envOr("APP_TEST_HOST", "127.0.0.1"),
		User:        envOr("APP_TEST_USER", "app"),
		Password:    envOr("APP_TEST_PASSWORD", "app"),
		Name:        envOr("APP_TEST_DB", "app"),
		LogLevel:    "silent",
		MaxOpen:     4,
		MaxIdle:     2,
		MaxLifetime: 60,
	}
	cfg.Port = defaultTestPort(driver)
	if v := os.Getenv("APP_TEST_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("APP_TEST_PORT 不是数字: %v", err)
		}
		cfg.Port = p
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := database.Open(cfg, log)
	if err != nil {
		t.Fatalf("连接 %s: %v", driver, err)
	}
	t.Cleanup(func() { _ = database.Close(db) })
	return db
}

func defaultTestPort(driver string) int {
	if driver == config.DriverMySQL {
		return 3306
	}
	return 5432
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// assertCode 断言错误是期望的业务码。
func assertCode(t *testing.T, err error, want errs.Code, what string) {
	t.Helper()

	e, ok := errs.From(err)
	if !ok {
		t.Fatalf("%s: err = %v，期望业务码 %d", what, err, want)
	}
	if e.Code != want {
		t.Fatalf("%s: 业务码 = %d（%s），期望 %d", what, e.Code, e.Msg, want)
	}
}
