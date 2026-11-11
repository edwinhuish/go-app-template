package cache

// 本文件守住落盘快照的四条边界：文件不存在、内容损坏、版本不匹配、键已过期。
//
// 为什么值得写：cache.memory.persist 默认是 true，这段代码在开发环境的默认路径上
// 就会跑。它的失败模式又都是「静默降级」——损坏时备份、版本不匹配时忽略，
// 全都是打一行日志继续启动。没有用例，就没人知道它到底还能不能读回来。

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestLoadSnapshotMissing 首次启动没有快照文件，应当是空缓存而不是错误。
func TestLoadSnapshotMissing(t *testing.T) {
	items, err := loadSnapshot(filepath.Join(t.TempDir(), "nope.json"), quietLog())
	if err != nil {
		t.Fatalf("文件不存在时返回错误: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("读到 %d 条，期望 0", len(items))
	}
}

// TestLoadSnapshotCorrupt 内容损坏时备份并从空缓存启动——不能因为一个缓存文件起不来。
func TestLoadSnapshotCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	if err := os.WriteFile(path, []byte("这不是 JSON"), 0o600); err != nil {
		t.Fatalf("写测试文件: %v", err)
	}

	items, err := loadSnapshot(path, quietLog())
	if err != nil {
		t.Fatalf("损坏时应降级而不是报错: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("损坏文件读到 %d 条，期望 0", len(items))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("损坏的文件应当被挪走，否则下次启动还会读到它")
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Errorf("损坏的文件没有被备份到 .corrupt：%v", err)
	}
}

// TestLoadSnapshotVersionMismatch 版本不匹配时忽略内容，但保留文件（可能是新版本写的）。
func TestLoadSnapshotVersionMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	raw := `{"version":99,"entries":{"k":{"value":"v"}}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("写测试文件: %v", err)
	}

	items, err := loadSnapshot(path, quietLog())
	if err != nil {
		t.Fatalf("版本不匹配时应降级而不是报错: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("版本不匹配却读入了 %d 条", len(items))
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("版本不匹配时不该删除或改名文件：%v", err)
	}
}

// TestLoadSnapshotSkipsExpired 过期键丢弃、未过期键保留。
func TestLoadSnapshotSkipsExpired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	f := snapshotFile{
		Version: snapshotVersion,
		SavedAt: time.Now(),
		Entries: map[string]snapshotEntry{
			"live":    {Value: "v1"},
			"future":  {Value: "v2", ExpireAt: time.Now().Add(time.Hour).UnixNano()},
			"expired": {Value: "v3", ExpireAt: time.Now().Add(-time.Minute).UnixNano()},
		},
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("序列化测试数据: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("写测试文件: %v", err)
	}

	items, err := loadSnapshot(path, quietLog())
	if err != nil {
		t.Fatalf("加载快照: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("读到 %d 条，期望 2（过期键应被跳过）：%v", len(items), items)
	}
	if items["live"].value != "v1" || items["future"].value != "v2" {
		t.Errorf("保留的键内容不对：%v", items)
	}
	if _, ok := items["expired"]; ok {
		t.Error("过期键没有被丢弃")
	}
}

// TestWriteSnapshotAtomic 写完后没有残留的临时文件，且内容可被读回。
func TestWriteSnapshotAtomic(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "data")
	path := filepath.Join(dir, "cache.json")

	entries := map[string]snapshotEntry{
		"session:1": {Value: "alice", ExpireAt: time.Now().Add(time.Hour).UnixNano()},
		"flag":      {Value: "on"},
	}
	if err := writeSnapshot(path, entries); err != nil {
		t.Fatalf("写快照: %v", err)
	}

	// 目录会被自动创建（首次落盘时 data/ 往往还不存在）。
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("快照文件不存在：%v", err)
	}

	// .tmp 必须清干净：留着不但占空间，还会让人怀疑上次写是不是半途崩了。
	leftovers, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatalf("列目录: %v", err)
	}
	if len(leftovers) != 0 {
		t.Errorf("残留临时文件：%v", leftovers)
	}

	items, err := loadSnapshot(path, quietLog())
	if err != nil {
		t.Fatalf("读回快照: %v", err)
	}
	if len(items) != 2 || items["session:1"].value != "alice" {
		t.Errorf("读回的内容与写入不一致：%v", items)
	}
}

// TestWriteSnapshotFailsOnBadDir 目录建不出来时要报错，而不是静默丢数据。
func TestWriteSnapshotFailsOnBadDir(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("准备阻塞文件: %v", err)
	}

	err := writeSnapshot(filepath.Join(blocker, "sub", "cache.json"), map[string]snapshotEntry{"k": {Value: "v"}})
	if err == nil {
		t.Fatal("目录不可创建时应返回错误")
	}
	if !strings.Contains(err.Error(), "创建快照目录") {
		t.Errorf("错误信息 = %q，期望指明是目录问题", err)
	}
}
