package core

import (
	"sort"
	"sync"
	"time"

	"github.com/ErronZrz/doc-rank/internal/util"
)

type State struct {
	mu sync.RWMutex

	// docs
	docs map[DocID]Document

	// total
	totalCnt  map[DocID]int64
	totalRank *Rank

	// recent
	recentCnt  map[DocID]int64
	recentRank *Rank
	window     *RecentWindow

	clock util.Clock
	// 可选：统计/指标
}

func NewState(clock util.Clock, recentWindowSeconds int) *State {
	return &State{
		docs:       make(map[DocID]Document),
		totalCnt:   make(map[DocID]int64),
		totalRank:  NewRank(),
		recentCnt:  make(map[DocID]int64),
		recentRank: NewRank(),
		window:     NewRecentWindow(recentWindowSeconds),
		clock:      clock,
	}
}

// --- 读接口 ---

func (s *State) GetDocs() []Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Document, 0, len(s.docs))
	for _, d := range s.docs {
		out = append(out, d)
	}
	// 稳定输出（可选）
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *State) GetTotalTopK(k int) []RankItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalRank.TopK(k)
}

func (s *State) GetRecentTopK(k int) []RankItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recentRank.TopK(k)
}

// --- 写接口 ---

func (s *State) Click(doc DocID, nowSec int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// total
	old := s.totalCnt[doc]
	newv := old + 1
	s.totalCnt[doc] = newv
	s.totalRank.Add(doc, old, newv)

	// recent
	rold := s.recentCnt[doc]
	rnew := rold + 1
	s.recentCnt[doc] = rnew
	s.recentRank.Add(doc, rold, rnew)

	// window
	s.window.Bump(nowSec, doc)
}

func (s *State) UpsertDoc(d Document) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[d.ID] = d
}

func (s *State) DeleteDoc(id DocID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.docs, id)

	// 移出 total / recent 排行结构
	if c, ok := s.totalCnt[id]; ok {
		s.totalRank.RemoveDocAt(id, c)
		delete(s.totalCnt, id)
	}
	if c, ok := s.recentCnt[id]; ok {
		s.recentRank.RemoveDocAt(id, c)
		delete(s.recentCnt, id)
	}
}

// 每秒推进近窗：把 (now-WindowSize..now] 之外的增量过期掉
func (s *State) AdvanceRecentTo(nowSec int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	targetIdx := s.window.IndexFor(nowSec)
	s.window.AdvanceTo(targetIdx, nowSec, func(doc DocID, delta int32) {
		// recentCnt 扣减 & recentRank 下移
		old := s.recentCnt[doc]
		newv := old - int64(delta)
		if newv < 0 {
			newv = 0 // 防御
		}
		s.recentCnt[doc] = newv
		s.recentRank.Add(doc, old, newv)
		if newv == 0 {
			// 可选：清理零计数
		}
	})
}

// RebuildAfterRecovery 在恢复后重建桶结构（total 必须；recent 可随重放构建）
func (s *State) RebuildAfterRecovery() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalRank = NewRank()
	s.totalRank.RebuildFrom(s.totalCnt)
	// recentRank 通常无需重建（随重放已经在位），若需要：
	// s.recentRank = NewRank(); s.recentRank.RebuildFrom(s.recentCnt)
}

// NowSec 辅助：面向 handlers
func (s *State) NowSec() int64 {
	if s.clock != nil {
		return s.clock.Now().Unix()
	}
	return time.Now().Unix()
}

// 恢复期/快照期的只读锁导出（可选）
func (s *State) MuRLock()   { s.mu.RLock() }
func (s *State) MuRUnlock() { s.mu.RUnlock() }

// 只读快照用
func (s *State) DocsUnsafe() map[DocID]Document     { return s.docs }
func (s *State) TotalCntMapUnsafe() map[DocID]int64 { return s.totalCnt }

// 恢复用低级写入（避免 Click() 的 nowSec 副作用）
func (s *State) TotalCntUnsafe(id string) int64        { return s.totalCnt[DocID(id)] }
func (s *State) SetTotalCntUnsafe(id string, v int64)  { s.totalCnt[DocID(id)] = v }
func (s *State) TotalRankUnsafe() *Rank                { return s.totalRank }
func (s *State) RecentCntUnsafe(id string) int64       { return s.recentCnt[DocID(id)] }
func (s *State) SetRecentCntUnsafe(id string, v int64) { s.recentCnt[DocID(id)] = v }
func (s *State) RecentRankUnsafe() *Rank               { return s.recentRank }
func (s *State) WindowUnsafe() *RecentWindow           { return s.window }
