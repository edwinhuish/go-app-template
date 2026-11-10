package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// snapshotVersion 变更快照结构时递增，旧文件会被忽略而不是解析出错。
const snapshotVersion = 1

// snapshotFile 是落盘格式。用 JSON 而非 gob：
// 出问题时能直接 cat 看内容，也方便用别的语言写工具读取。
type snapshotFile struct {
	Version int                      `json:"version"`
	SavedAt time.Time                `json:"saved_at"`
	Entries map[string]snapshotEntry `json:"entries"`
}

type snapshotEntry struct {
	Value string `json:"value"`
	// ExpireAt 是 Unix 纳秒时间戳，0 表示永不过期。
	ExpireAt int64 `json:"expire_at,omitempty"`
}

// loadSnapshot 读取快照并丢弃已过期的键。
//
// 文件不存在是正常情况（首次启动）；内容损坏时备份后以空缓存继续，
// 不让一个缓存文件把整个服务挡在启动之外。
func loadSnapshot(path string, log *slog.Logger) (map[string]memEntry, error) {
	items := make(map[string]memEntry)

	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return items, nil
		}
		return nil, fmt.Errorf("读取缓存快照 %s: %w", path, err)
	}

	var f snapshotFile
	if err := json.Unmarshal(raw, &f); err != nil {
		backup := path + ".corrupt"
		if rerr := os.Rename(path, backup); rerr != nil {
			log.Warn("缓存快照损坏，将以空缓存启动", "path", path, "err", err, "backup_err", rerr)
		} else {
			log.Warn("缓存快照损坏，已备份并以空缓存启动", "path", path, "backup", backup, "err", err)
		}
		return items, nil
	}

	if f.Version != snapshotVersion {
		log.Warn("缓存快照版本不匹配，已忽略", "path", path,
			"version", f.Version, "expected", snapshotVersion)
		return items, nil
	}

	now := time.Now()
	expired := 0
	for k, e := range f.Entries {
		entry := memEntry{value: e.Value}
		if e.ExpireAt > 0 {
			entry.expireAt = time.Unix(0, e.ExpireAt)
		}
		if entry.expired(now) {
			expired++
			continue
		}
		items[k] = entry
	}
	if expired > 0 {
		log.Debug("缓存快照中的过期键已跳过", "path", path, "count", expired)
	}
	return items, nil
}

// writeSnapshot 原子写：先写同目录临时文件并 fsync，再 rename 覆盖。
// 中途崩溃只会留下一个 .tmp，不会产生半截的快照文件。
func writeSnapshot(path string, entries map[string]snapshotEntry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建快照目录 %s: %w", dir, err)
	}

	payload, err := json.Marshal(snapshotFile{
		Version: snapshotVersion,
		SavedAt: time.Now(),
		Entries: entries,
	})
	if err != nil {
		return fmt.Errorf("序列化缓存快照: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".cache-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时快照文件: %w", err)
	}
	tmpName := tmp.Name()

	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入缓存快照: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("同步缓存快照: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时快照文件: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("替换缓存快照 %s: %w", path, err)
	}
	committed = true
	return nil
}
