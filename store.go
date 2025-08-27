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

	// 简单节流：合并多次变化再推 TopK
	lastPush time.Time
}

func NewStore(p *Persist, sse *SSEHub, cfg Config) *Store {
	return &Store{
		bkt:    NewBuckets(),
		docs:   NewDocs(),
		p:      p,
		sse:    sse,
		config: cfg,
	}
}

// 恢复：将 snapshot+WAL 的状态灌入内存结构
func (s *Store) Load(state *RestoreState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range state.Docs {
		s.docs.Upsert(d)
		if _, ok := state.Counts[d.ID]; !ok {
			state.Counts[d.ID] = 0
		}
	}
	// 先把所有文档放入 0 桶
	for _, d := range state.Docs {
		s.bkt.Add(d.ID)
	}
	// 再把点击数“回放”到对应桶（只为构建结构，复杂度 O(N)）
	for id, c := range state.Counts {
		for i := 0; i < c; i++ {
			s.bkt.Inc(id)
		}
	}
}

func (s *Store) Click(docID string) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 只有存在文档才允许点击
	if _, ok := s.docs.Get(docID); !ok {
		return 0, false, nil
	}
	// 先写 WAL
	if err := s.p.AppendWAL(walEntry{Op: "CLICK", ID: docID}); err != nil {
		return 0, false, err
	}
	newCount := s.bkt.Inc(docID)
	s.maybeBroadcastTopKLocked()
	return newCount, true, nil
}

func (s *Store) TopK(k int) []RankItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bkt.TopK(k)
}

func (s *Store) AddOrUpdateDoc(doc Doc) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// WAL
	op := "ADD"
	if _, ok := s.docs.Get(doc.ID); ok {
		op = "UPDATE"
	}
	if err := s.p.AppendWAL(walEntry{Op: op, ID: doc.ID, Title: doc.Title, URL: doc.URL}); err != nil {
		return err
	}
	// 内存
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
	s.bkt.Delete(id)
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
	// 节流：100ms 内最多发一次
	now := time.Now()
	if now.Sub(s.lastPush) < 100*time.Millisecond {
		return
	}
	s.lastPush = now
	top := s.bkt.TopK(s.config.TopKDefault)
	s.sse.Broadcast(SSEMessage{Type: "total_topk", Data: RankResp{Rank: top}})
}
