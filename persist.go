package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
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
	Docs   []Doc            `json:"docs"`
	Counts map[string]int   `json:"counts"`
	Seq    uint64           `json:"seq"`
}

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
	p.walBufWriter = bufio.NewWriterSize(f, 1<<20) // 1MB buffer
	return nil
}

// AppendWAL 线程安全
func (p *Persist) AppendWAL(e walEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// 自增 seq
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

func (p *Persist) Flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.walBufWriter.Flush(); err != nil {
		return err
	}
	return p.walFile.Sync()
}

func (p *Persist) Close() error {
	if err := p.Flush(); err != nil {
		return err
	}
	return p.walFile.Close()
}

// SaveSnapshot 原子写快照，并截断 WAL（简化：保留 WAL，不截断也可）
func (p *Persist) SaveSnapshot(docs []Doc, counts map[string]int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 准备模型
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

	// （可选）轮转 WAL：这里简单粗暴地重建 wal 文件
	if err := p.walFile.Close(); err != nil {
		return err
	}
	if err := os.Remove(p.walPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return p.openWAL()
}

type RestoreState struct {
	Docs   []Doc
	Counts map[string]int
	Seq    uint64
}

// Restore 读取 snapshot + 回放 WAL
func (p *Persist) Restore() (*RestoreState, error) {
	state := &RestoreState{
		Counts: make(map[string]int, 1024),
	}
	// 1) 读快照
	if f, err := os.Open(p.snapPath); err == nil {
		defer f.Close()
		var snap snapshotModel
		if err := json.NewDecoder(f).Decode(&snap); err == nil {
			state.Docs = snap.Docs
			state.Counts = snap.Counts
			state.Seq = snap.Seq
		}
	}
	// 2) 回放 WAL
	wf, err := os.Open(p.walPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			p.seq = state.Seq
			return state, nil
		}
		return nil, err
	}
	defer wf.Close()

	reader := bufio.NewReader(wf)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var e walEntry
			if err := json.Unmarshal(line, &e); err == nil {
				if e.Seq <= state.Seq {
					continue // 已包含在快照内
				}
				switch e.Op {
				case "ADD", "UPDATE":
					// 更新到 docs 列表（去重）
					found := false
					for i := range state.Docs {
						if state.Docs[i].ID == e.ID {
							state.Docs[i].Title = e.Title
							state.Docs[i].URL = e.URL
							found = true
							break
						}
					}
					if !found {
						state.Docs = append(state.Docs, Doc{ID: e.ID, Title: e.Title, URL: e.URL})
					}
					// 确保有 count 项
					if _, ok := state.Counts[e.ID]; !ok {
						state.Counts[e.ID] = 0
					}
				case "DEL":
					// 从 docs 移除
					dst := state.Docs[:0]
					for _, d := range state.Docs {
						if d.ID != e.ID {
							dst = append(dst, d)
						}
					}
					state.Docs = dst
					delete(state.Counts, e.ID)
				case "CLICK":
					state.Counts[e.ID] = state.Counts[e.ID] + 1
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

func (p *Persist) NextSeq() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seq++
	return p.seq
}

func (p *Persist) DebugPaths() string {
	return fmt.Sprintf("snapshot=%s wal=%s", p.snapPath, p.walPath)
}
