package wal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type pending struct {
	rec Record
	ack chan error
}

// Log 既是 Writer 也是 Reader；内部有 group-commit 协程
type Log struct {
	path string

	f  *os.File
	w  *bufio.Writer
	mu sync.Mutex // 保护 f/w 在极少数 Sync/Close 路径的串行化（loop 内部单线程写）

	ch      chan pending    // 追加记录的通道（带 ack）
	flushCh chan chan error // 外部主动 flush 请求
	quit    chan struct{}

	interval time.Duration
	batch    int
}

func ensureDir(p string) error {
	dir := filepath.Dir(p)
	return os.MkdirAll(dir, 0o755)
}

func OpenLog(path string, groupCommitEvery time.Duration, batch int) (*Log, error) {
	if err := ensureDir(path); err != nil {
		return nil, err
	}
	// 以可读写+创建方式打开；写入采用追加语义（bufio.Writer 会保持偏移）
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	// 将写游标移动到文件末尾
	if _, err := f.Seek(0, os.SEEK_END); err != nil {
		_ = f.Close()
		return nil, err
	}

	l := &Log{
		path:     path,
		f:        f,
		w:        bufio.NewWriterSize(f, 1<<20), // 1MB 缓冲
		ch:       make(chan pending, 8192),
		flushCh:  make(chan chan error, 64),
		quit:     make(chan struct{}),
		interval: groupCommitEvery,
		batch:    batch,
	}

	go l.loop()
	return l, nil
}

func (l *Log) AppendClick(docID string, ts int64) error {
	return l.enqueue(Record{Type: Click, TS: ts, ID: docID})
}
func (l *Log) AppendDocUpsert(id, title, url string, ts int64) error {
	return l.enqueue(Record{Type: DocUpsert, TS: ts, ID: id, Title: title, URL: url})
}
func (l *Log) AppendDocDelete(id string, ts int64) error {
	return l.enqueue(Record{Type: DocDelete, TS: ts, ID: id})
}

func (l *Log) enqueue(rec Record) error {
	ack := make(chan error, 1)
	select {
	case l.ch <- pending{rec: rec, ack: ack}:
		// 阻塞等待“本批次 fsync 完成”
		return <-ack
	case <-l.quit:
		return errors.New("wal closed")
	}
}

// SyncNow 立即触发一次 flush+fsync 并等待完成
func (l *Log) SyncNow() error {
	ack := make(chan error, 1)
	select {
	case l.flushCh <- ack:
		return <-ack
	case <-l.quit:
		return errors.New("wal closed")
	}
}

func (l *Log) Close() error {
	// 停止后台循环并做最后一次 flush
	select {
	case <-l.quit:
		// 已经关闭
	default:
		close(l.quit)
	}
	// 保险：主动同步一次
	_ = l.SyncNow()

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.w != nil {
		_ = l.w.Flush()
	}
	if l.f != nil {
		_ = l.f.Sync()
		err := l.f.Close()
		l.f = nil
		l.w = nil
		return err
	}
	return nil
}

func (l *Log) Path() string { return l.path }

// Replay 从文件头或偏移重放；返回最后 offset（下一条记录的起始偏移）
func (l *Log) Replay(fromOffset int64, on func(rec Record, offset int64) error) (int64, error) {
	f, err := os.Open(l.path)
	if err != nil {
		// 如果文件不存在，视为空 WAL
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	if fromOffset > 0 {
		if _, err := f.Seek(fromOffset, os.SEEK_SET); err != nil {
			return 0, err
		}
	}

	reader := bufio.NewReaderSize(f, 1<<20)
	var off = fromOffset

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var rec Record
			// 容忍尾部半行：如果解码失败，认为到达坏点/文件尾，停止重放
			if e := json.Unmarshal(line, &rec); e != nil {
				// 停止在坏点，返回当前 offset
				return off, nil
			}
			if on != nil {
				if e := on(rec, off); e != nil {
					return off, e
				}
			}
			off += int64(len(line))
		}
		if err != nil {
			// EOF：正常结束
			return off, nil
		}
	}
}

// --- 后台 flush loop（group commit） ---
func (l *Log) loop() {
	tk := time.NewTicker(l.interval)
	defer tk.Stop()

	var batch []pending
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		// 单线程写，避免加锁。序列化并写入缓冲。
		for _, p := range batch {
			b, err := encodeLine(p.rec)
			if err != nil {
				for _, q := range batch {
					q.ack <- err
				}
				batch = batch[:0]
				return err
			}
			if _, err := l.w.Write(b); err != nil {
				for _, q := range batch {
					q.ack <- err
				}
				batch = batch[:0]
				return err
			}
		}
		// 刷新缓冲到文件页缓存
		if err := l.w.Flush(); err != nil {
			for _, q := range batch {
				q.ack <- err
			}
			batch = batch[:0]
			return err
		}
		// fsync（或 fdatasync）确保落盘
		if err := fdatasync(l.f); err != nil {
			for _, q := range batch {
				q.ack <- err
			}
			batch = batch[:0]
			return err
		}
		// 成功：唤醒所有等待者
		for _, q := range batch {
			q.ack <- nil
		}
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case p := <-l.ch:
			batch = append(batch, p)
			if len(batch) >= l.batch {
				_ = flush()
			}
		case ack := <-l.flushCh:
			err := flush()
			ack <- err
		case <-tk.C:
			_ = flush()
		case <-l.quit:
			_ = flush()
			return
		}
	}
}

// 序列化单条记录
func encodeLine(rec Record) ([]byte, error) {
	b, err := json.Marshal(rec)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// 在 Linux/Unix 上优先使用 fdatasync，Windows 退化为 Sync
func fdatasync(f *os.File) error {
	return f.Sync()
}

// （可选）debug：打印错误
func _debug(err error) {
	if err != nil {
		fmt.Println("wal:", err)
	}
}
