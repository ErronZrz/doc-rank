package main

import (
	"sync"
	"time"
)

type Store struct {
	mu     sync.RWMutex
	bkt    *Buckets
	docs   *Docs
	p      *Persist
	sse    *SSEHub
	config Config

	recent   *Recent
	lastPush time.Time
}

// NewStore 创建 Store
func NewStore(p *Persist, sse *SSEHub, cfg Config) *Store {
	return &Store{
		bkt:    NewBuckets(),
		docs:   NewDocs(),
		p:      p,
		sse:    sse,
		config: cfg,
		recent: NewRecent(),
	}
}

// Load 从恢复状态装载文档、总榜与最近榜
func (s *Store) Load(state *RestoreState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 文档恢复
	for _, d := range state.Docs {
		s.docs.Upsert(d)
	}
	// 补齐快照中缺失的文档计数为 0
	for _, d := range state.Docs {
		if _, ok := state.Counts[d.ID]; !ok {
			state.Counts[d.ID] = 0
		}
	}
	// 用快照计数重建总榜
	s.bkt.ResetFromCounts(state.Counts)

	// 最近榜恢复
	if len(state.RecentClicks) > 0 {
		minTs := state.RecentClicks[0].Ts
		for _, e := range state.RecentClicks {
			if e.Ts > 0 && e.Ts < minTs {
				minTs = e.Ts
			}
		}
		s.recent.SetBase(minTs - 1)
		for _, e := range state.RecentClicks {
			if e.Op == "CLICK" && e.ID != "" && e.Ts > 0 {
				s.recent.AddClick(e.ID, e.Ts)
			}
		}
	}
}

// Click 记录一次点击并更新排行榜
func (s *Store) Click(docID string) (int, bool, error) {
	ts := time.Now().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.docs.Get(docID); !ok {
		return 0, false, nil
	}
	// 记录 WAL，带时间戳
	if err := s.p.AppendWAL(walEntry{Op: "CLICK", ID: docID, Ts: ts}); err != nil {
		return 0, false, err
	}

	// 总榜 +1
	newCount := s.bkt.Adjust(docID, +1)
	// 最近榜 +1
	s.recent.AddClick(docID, ts)

	// 节流后广播点击更新
	s.maybeBroadcastTopKLocked()
	return newCount, true, nil
}

// TopK 返回总榜前 K 项
func (s *Store) TopK(k int) []RankItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bkt.TopK(k)
}

// TopKRecent 返回最近榜前 K 项
func (s *Store) TopKRecent(k int) []RankItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recent.TopK(k)
}

// AddOrUpdateDoc 新增或更新文档
func (s *Store) AddOrUpdateDoc(doc Doc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	op := "ADD"
	if _, ok := s.docs.Get(doc.ID); ok {
		op = "UPDATE"
	}
	if err := s.p.AppendWAL(walEntry{Op: op, ID: doc.ID, Title: doc.Title, URL: doc.URL}); err != nil {
		return err
	}
	if op == "ADD" {
		s.bkt.Add(doc.ID)
	}
	s.docs.Upsert(doc)
	// 广播文档更新
	s.sse.BroadcastUpdateDoc()
	return nil
}

// DeleteDoc 删除文档
func (s *Store) DeleteDoc(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.docs.Get(id); !ok {
		return nil
	}
	if err := s.p.AppendWAL(walEntry{Op: "DEL", ID: id}); err != nil {
		return err
	}
	s.docs.Delete(id)
	s.bkt.Delete(id)
	// 立即从最近榜移除 id
	s.recent.bkt.Delete(id)

	// 广播文档更新
	s.sse.BroadcastUpdateDoc()
	return nil
}

// ListDocs 返回全部文档
func (s *Store) ListDocs() []Doc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.docs.List()
}

// CountsSnapshot 返回总榜计数快照
func (s *Store) CountsSnapshot() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int, len(s.bkt.entries))
	for id, e := range s.bkt.entries {
		out[id] = e.count
	}
	return out
}

// maybeBroadcastTopKLocked 节流后广播点击更新
func (s *Store) maybeBroadcastTopKLocked() {
	now := time.Now()
	// 100 ms 内最多一次
	if now.Sub(s.lastPush) < 100*time.Millisecond {
		return
	}
	s.lastPush = now
	// 广播点击更新
	s.sse.BroadcastUpdateClick()
}

// StartRecentAdvancer 启动定时推进最近窗口
func (s *Store) StartRecentAdvancer(stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sec := time.Now().Unix()
				s.mu.Lock()
				changed := s.recent.advanceTo(sec)
				// 仅在排行榜变动时广播
				if changed {
					s.maybeBroadcastTopKLocked()
				}
				s.mu.Unlock()
			case <-stop:
				return
			}
		}
	}()
}
