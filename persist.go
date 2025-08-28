package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Persist struct {
	walPath      string
	snapPath     string
	mu           sync.Mutex
	walFile      *os.File
	walBufWriter *bufio.Writer
	seq          uint64
	syncEvery    bool
}

type snapshotModel struct {
	Docs   []Doc          `json:"docs"`
	Counts map[string]int `json:"counts"`
	Seq    uint64         `json:"seq"`
}

// NewPersist 创建持久化管理器
func NewPersist(dir string, syncEveryWrite bool) (*Persist, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	p := &Persist{
		walPath:   filepath.Join(dir, "wal.jsonl"),
		snapPath:  filepath.Join(dir, "snapshot.json"),
		syncEvery: syncEveryWrite,
	}
	if err := p.openWAL(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Persist) openWAL() error {
	f, err := os.OpenFile(p.walPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	p.walFile = f
	p.walBufWriter = bufio.NewWriterSize(f, 1<<20) // 1 MB buffer
	return nil
}

// AppendWAL 追加一条 WAL 记录 (线程安全)
func (p *Persist) AppendWAL(e walEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// 维护 seq
	if e.Seq == 0 {
		p.seq++
		e.Seq = p.seq
	} else if e.Seq > p.seq {
		p.seq = e.Seq
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := p.walBufWriter.Write(b); err != nil {
		return err
	}
	if err := p.walBufWriter.WriteByte('\n'); err != nil {
		return err
	}
	if p.syncEvery {
		if err := p.walBufWriter.Flush(); err != nil {
			return err
		}
		if err := p.walFile.Sync(); err != nil {
			return err
		}
	}
	return nil
}

// Flush 刷新缓冲区并落盘
func (p *Persist) Flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.walBufWriter.Flush(); err != nil {
		return err
	}
	return p.walFile.Sync()
}

// Close 刷新并关闭 WAL
func (p *Persist) Close() error {
	if err := p.Flush(); err != nil {
		return err
	}
	return p.walFile.Close()
}

// SaveSnapshot 写入快照并轮转 WAL 保留最近 600 秒点击
func (p *Persist) SaveSnapshot(docs []Doc, counts map[string]int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 写快照
	model := snapshotModel{
		Docs:   docs,
		Counts: counts,
		Seq:    p.seq,
	}
	tmp := p.snapPath + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&model); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, p.snapPath); err != nil {
		return err
	}

	// 轮转 WAL，只保留最近 600 秒 CLICK
	cutoff := time.Now().Add(-600 * time.Second).Unix()

	// 关闭旧 writer 和 file
	if err := p.walBufWriter.Flush(); err != nil {
		return err
	}
	if err := p.walFile.Close(); err != nil {
		return err
	}

	oldPath := p.walPath + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(p.walPath, oldPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// 创建新 WAL
	newFile, err := os.OpenFile(p.walPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	newWriter := bufio.NewWriterSize(newFile, 1<<20)

	// 从 old 复制近 600 秒内的 CLICK
	if fOld, err := os.Open(oldPath); err == nil {
		defer func(fOld *os.File) {
			err := fOld.Close()
			if err != nil {
				log.Fatalf("persist close error: %v", err)
			}
		}(fOld)
		reader := bufio.NewReader(fOld)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				var e walEntry
				if json.Unmarshal(line, &e) == nil {
					if e.Op == "CLICK" && e.Ts >= cutoff {
						if _, err := newWriter.Write(line); err != nil {
							return err
						}
					}
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return err
			}
		}
	}

	if err := newWriter.Flush(); err != nil {
		return err
	}
	if err := newFile.Sync(); err != nil {
		return err
	}
	p.walFile = newFile
	p.walBufWriter = newWriter

	_ = os.Remove(oldPath)
	return nil
}

// RestoreState 表示恢复用的快照与 WAL 汇总
type RestoreState struct {
	Docs         []Doc
	Counts       map[string]int
	Seq          uint64
	RecentClicks []walEntry
}

// Restore 读取快照并回放 WAL 最近 600 秒点击
func (p *Persist) Restore() (*RestoreState, error) {
	state := &RestoreState{
		Counts:       make(map[string]int, 1024),
		RecentClicks: make([]walEntry, 0, 4096),
	}
	// 读快照
	if f, err := os.Open(p.snapPath); err == nil {
		defer func(f *os.File) {
			err := f.Close()
			if err != nil {
				log.Fatalf("persist close error: %v", err)
			}
		}(f)
		var snap snapshotModel
		if err := json.NewDecoder(f).Decode(&snap); err == nil {
			state.Docs = snap.Docs
			state.Counts = snap.Counts
			state.Seq = snap.Seq
		}
	}
	// 读 WAL (仅保留近 600 秒 CLICK)
	wf, err := os.Open(p.walPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			p.seq = state.Seq
			return state, nil
		}
		return nil, err
	}
	defer func(wf *os.File) {
		err := wf.Close()
		if err != nil {
			log.Fatalf("persist close error: %v", err)
		}
	}(wf)

	cutoff := time.Now().Add(-600 * time.Second).Unix()
	nowSec := time.Now().Unix()

	reader := bufio.NewReader(wf)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var e walEntry
			if err := json.Unmarshal(line, &e); err == nil {
				if e.Op == "CLICK" {
					// 仅保留近 600 秒内且非未来
					if e.Ts >= cutoff && e.Ts <= nowSec {
						state.RecentClicks = append(state.RecentClicks, e)
					}
				}
				if e.Seq > state.Seq {
					state.Seq = e.Seq
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}
	p.seq = state.Seq
	return state, nil
}

// NextSeq 返回下一个序号
func (p *Persist) NextSeq() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seq++
	return p.seq
}

// DebugPaths 返回调试路径信息
func (p *Persist) DebugPaths() string {
	return fmt.Sprintf("snapshot=%s wal=%s", p.snapPath, p.walPath)
}
