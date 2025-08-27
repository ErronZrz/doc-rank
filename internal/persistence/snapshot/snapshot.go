package snapshot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/ErronZrz/doc-rank/internal/core"
)

type Snapshot struct {
	WALOffset int64                `json:"wal_offset"`
	Docs      []core.Document      `json:"docs"`
	TotalCnt  map[core.DocID]int64 `json:"total_cnt"`
	// recentCnt 不保存；重启时通过最近窗口期 WAL 计算
}

type Store interface {
	Load(path string) (*Snapshot, error)
	Save(path string, s *Snapshot) error
}

type FileStore struct{}

func NewFileStore() *FileStore { return &FileStore{} }

func (fs *FileStore) Load(path string) (*Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	defer f.Close()

	var snap Snapshot
	dec := json.NewDecoder(f)
	if err := dec.Decode(&snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (fs *FileStore) Save(path string, s *Snapshot) error {
	if err := ensureDir(path); err != nil {
		return err
	}
	tmp := path + ".tmp"

	// 1) 写临时文件
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "") // 紧凑；如需美化可设为 "  "
	if err := enc.Encode(s); err != nil {
		_ = f.Close()
		return err
	}
	// 2) 同步临时文件
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// 3) 原子替换
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// 4) 同步父目录，确保 rename 持久（部分平台不支持，对错误容忍）
	_ = fsyncDir(filepath.Dir(path))
	return nil
}

func ensureDir(p string) error {
	dir := filepath.Dir(p)
	return os.MkdirAll(dir, 0o755)
}

// 同步目录（macOS 上 fsync 目录可能返回 ENOTSUP，容忍）
func fsyncDir(dir string) error {
	df, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer df.Close()
	// Windows 不支持；macOS 可能 ENOTSUP；Linux OK
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := df.Sync(); err != nil {
		// macOS 上对目录 Sync 可能返回 “operation not supported”
		if errors.Is(err, syscall.ENOTSUP) {
			return nil
		}
		return err
	}
	return nil
}
