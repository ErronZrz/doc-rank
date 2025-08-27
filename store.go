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

    // --- 总榜恢复（保持不变） ---
    for _, d := range state.Docs {
        s.docs.Upsert(d)
        if _, ok := state.Counts[d.ID]; !ok {
            state.Counts[d.ID] = 0
        }
    }
    for _, d := range state.Docs {
        s.bkt.Add(d.ID)
    }
    for id, c := range state.Counts {
        if c > 0 {
            s.bkt.Adjust(id, c)
        }
    }

    // --- 最近榜恢复：先确定最早 ts，设置基准，然后回放 ---
    if len(state.RecentClicks) > 0 {
        minTs := state.RecentClicks[0].Ts
        for _, e := range state.RecentClicks {
            if e.Ts > 0 && e.Ts < minTs {
                minTs = e.Ts
            }
        }
        // 基准设为“最早一条点击的前一秒”
        s.recent.SetBase(minTs - 1)
        for _, e := range state.RecentClicks {
            if e.Op == "CLICK" && e.ID != "" && e.Ts > 0 {
                s.recent.AddClick(e.ID, e.Ts)
            }
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
    s.bkt.Delete(id)
    // 让最近榜即时移除（不会去改 ring；后续过期时发现不存在也无副作用）
    s.recent.bkt.Delete(id)

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

// 替换原有 maybeBroadcastTopKLocked 实现
func (s *Store) maybeBroadcastTopKLocked() {
	now := time.Now()
	// 节流：100ms 内最多推送一次“update”
	if now.Sub(s.lastPush) < 100*time.Millisecond {
		return
	}
	s.lastPush = now
	// 只发送“数据有更新”的锚点
	s.sse.BroadcastUpdate()
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
				changed := s.recent.advanceTo(sec)
				// 仅在窗口推进导致 recent 榜变动时再广播
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
