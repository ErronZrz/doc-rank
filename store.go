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

// 恢复：将 snapshot 灌入总榜；再用 WAL（仅近 600 秒 CLICK）恢复 recent
func (s *Store) Load(state *RestoreState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range state.Docs {
		s.docs.Upsert(d)
		if _, ok := state.Counts[d.ID]; !ok {
			state.Counts[d.ID] = 0
		}
	}
	// 总榜：所有文档入 0 桶，然后把 count 次 Inc（O(N)）
	for _, d := range state.Docs {
		s.bkt.Add(d.ID)
	}
	for id, c := range state.Counts {
		if c <= 0 {
			continue
		}
		// 用 Adjust(+c) 替代循环 Inc，O(1)
		s.bkt.Adjust(id, c)
	}

	// recent：回放 WAL 中的近 600 秒 CLICK
	for _, e := range state.RecentClicks {
		if e.Op == "CLICK" && e.ID != "" && e.Ts > 0 {
			s.recent.AddClick(e.ID, e.Ts)
		}
	}
}

// Click：写 WAL（带 ts）→ 更新总榜 → 更新 recent
func (s *Store) Click(docID string) (int, bool, error) {
	ts := time.Now().Unix()

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.docs.Get(docID); !ok {
		return 0, false, nil
	}
	// WAL：CLICK + ts
	if err := s.p.AppendWAL(walEntry{Op: "CLICK", ID: docID, Ts: ts}); err != nil {
		return 0, false, err
	}

	// 总榜 +1
	newCount := s.bkt.Adjust(docID, +1)
	// recent +1（按 ts）
	s.recent.AddClick(docID, ts)

	s.maybeBroadcastTopKLocked()
	return newCount, true, nil
}

func (s *Store) TopK(k int) []RankItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bkt.TopK(k)
}

func (s *Store) TopKRecent(k int) []RankItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recent.TopK(k)
}

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
	s.maybeBroadcastTopKLocked()
	return nil
}

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
	// 从总榜移除
	s.bkt.Delete(id)
	// recent：无需主动回收 ring 中历史秒里的 ID，过期时跳过即可。
	s.maybeBroadcastTopKLocked()
	return nil
}

func (s *Store) ListDocs() []Doc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.docs.List()
}

func (s *Store) CountsSnapshot() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]int, len(s.bkt.entries))
	for id, e := range s.bkt.entries {
		out[id] = e.count
	}
	return out
}

func (s *Store) maybeBroadcastTopKLocked() {
	now := time.Now()
	if now.Sub(s.lastPush) < 100*time.Millisecond {
		return
	}
	s.lastPush = now
	total := s.bkt.TopK(s.config.TopKDefault)
	recent := s.recent.TopK(s.config.TopKDefault)
	s.sse.Broadcast(SSEMessage{Type: "total_topk", Data: RankResp{Rank: total}})
	s.sse.Broadcast(SSEMessage{Type: "recent_topk", Data: RankResp{Rank: recent}})
}

// StartRecentAdvancer 启动一个每秒推进 recent 窗口的 goroutine
func (s *Store) StartRecentAdvancer(stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sec := time.Now().Unix()
				s.mu.Lock()
				s.recent.advanceTo(sec)
				// 推进后 recent 榜可能变化，做一次节流广播
				s.maybeBroadcastTopKLocked()
				s.mu.Unlock()
			case <-stop:
				return
			}
		}
	}()
}
