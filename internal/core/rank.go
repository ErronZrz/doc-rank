package core

// 计数桶 + max 指针
type void struct{}
type strset map[DocID]void

type Rank struct {
	buckets map[int64]strset // count -> docs
	maxCnt  int64
}

func NewRank() *Rank {
	return &Rank{
		buckets: make(map[int64]strset),
		maxCnt:  0,
	}
}

// Add 在 old->new 之间移桶，并维护 maxCnt
func (r *Rank) Add(doc DocID, oldCnt, newCnt int64) {
	if oldCnt == newCnt {
		return
	}
	// 从旧桶移除
	if oldCnt > 0 {
		if s, ok := r.buckets[oldCnt]; ok {
			delete(s, doc)
			if len(s) == 0 {
				delete(r.buckets, oldCnt)
				// 若刚好是最大桶被清空，向下回退到下一个非空桶
				if r.maxCnt == oldCnt {
					for r.maxCnt > 0 {
						r.maxCnt--
						if _, ok := r.buckets[r.maxCnt]; ok {
							break
						}
					}
				}
			}
		}
	}
	// newCnt==0 表示移出排行榜（不进入 0 桶）
	if newCnt <= 0 {
		return
	}

	// 放入新桶
	s2, ok := r.buckets[newCnt]
	if !ok {
		s2 = make(strset)
		r.buckets[newCnt] = s2
	}
	s2[doc] = void{}

	if newCnt > r.maxCnt {
		r.maxCnt = newCnt
	}
}

// RemoveDocAt 从给定计数桶删除文档（删除文档时调用）
func (r *Rank) RemoveDocAt(doc DocID, cnt int64) {
	if cnt <= 0 {
		return
	}
	if s, ok := r.buckets[cnt]; ok {
		delete(s, doc)
		if len(s) == 0 {
			delete(r.buckets, cnt)
			if r.maxCnt == cnt {
				for r.maxCnt > 0 {
					r.maxCnt--
					if _, ok := r.buckets[r.maxCnt]; ok {
						break
					}
				}
			}
		}
	}
}

// TopK 从 maxCnt 向下生成 K 个条目；k<=0 表示全量
func (r *Rank) TopK(k int) []RankItem {
	// 由于 buckets 的 key 是稀疏的，这里采用 “从 maxCnt 逐级递减并检查是否存在桶”的做法。
	// 在实际点击以 +1 增长的场景下，桶较为密集，退化成本可接受。
	var out []RankItem
	wantAll := k <= 0
	if !wantAll {
		out = make([]RankItem, 0, k)
	} else {
		// 粗略估计容量，避免频繁扩容
		out = make([]RankItem, 0, 1024)
	}

	for c := r.maxCnt; c > 0; c-- {
		if s, ok := r.buckets[c]; ok {
			for id := range s {
				out = append(out, RankItem{DocID: id, Clicks: c})
				if !wantAll && len(out) >= k {
					return out
				}
			}
		}
	}
	return out
}

// RebuildFrom 用计数字典重建桶结构（恢复/快照后调用）
func (r *Rank) RebuildFrom(counts map[DocID]int64) {
	r.buckets = make(map[int64]strset)
	r.maxCnt = 0
	for id, c := range counts {
		if c <= 0 {
			continue
		}
		s := r.buckets[c]
		if s == nil {
			s = make(strset)
			r.buckets[c] = s
		}
		s[id] = void{}
		if c > r.maxCnt {
			r.maxCnt = c
		}
	}
}
