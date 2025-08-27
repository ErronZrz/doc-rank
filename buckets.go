package main

// 频次桶：支持 add/del/inc/adjust/topK，严格 O(1)/O(K)
type bucket struct {
	count      int
	head, tail *entry
	prev, next *bucket
	size       int
}

type entry struct {
	id         string
	count      int
	b          *bucket
	prev, next *entry // within bucket
}

type Buckets struct {
	entries map[string]*entry
	bmap    map[int]*bucket
	maxB    *bucket
	zeroB   *bucket // 用于新建时放到 0 桶
}

func NewBuckets() *Buckets {
	b := &Buckets{
		entries: make(map[string]*entry, 1024),
		bmap:    make(map[int]*bucket, 1024),
	}
	// 预建 count=0 桶
	z := &bucket{count: 0}
	b.bmap[0] = z
	b.zeroB = z
	b.maxB = z
	return b
}

func (b *Buckets) has(id string) bool {
	_, ok := b.entries[id]
	return ok
}

func (b *Buckets) Add(id string) {
	if b.has(id) {
		return
	}
	e := &entry{id: id, count: 0}
	z := b.zeroB
	b.insertEntryToBucketHead(z, e)
	e.b = z
	b.entries[id] = e
	if b.maxB == nil || b.maxB.count < 0 {
		b.maxB = z
	}
}

func (b *Buckets) Delete(id string) {
	e, ok := b.entries[id]
	if !ok {
		return
	}
	b.removeEntryFromBucket(e.b, e)
	delete(b.entries, id)
	// 如果删除的是 maxB 且桶空了，回退 maxB
	if e.b == b.maxB && e.b.size == 0 {
		p := e.b.prev
		for p != nil && p.size == 0 {
			p = p.prev
		}
		if p != nil {
			b.maxB = p
		} else {
			b.maxB = b.zeroB
		}
	}
}

func (b *Buckets) Inc(id string) int {
	return b.Adjust(id, 1)
}

// Adjust：将 id 的计数调整 delta，可为正/负；计数不会低于 0。
func (b *Buckets) Adjust(id string, delta int) int {
	if delta == 0 {
		return b.GetCount(id)
	}
	e, ok := b.entries[id]
	if !ok {
		// 不存在：若是负数，直接返回 0；若是正数，按 Add 后再调
		if delta < 0 {
			return 0
		}
		b.Add(id)
		e = b.entries[id]
	}

	oldCount := e.count
	newCount := oldCount + delta
	if newCount < 0 {
		newCount = 0
	}
	if newCount == oldCount {
		return newCount
	}

	// 找到/创建目标桶
	nb := b.bmap[newCount]
	if nb == nil {
		nb = &bucket{count: newCount}
		b.bmap[newCount] = nb
		// 将 nb 正确插入到链上（相对 e.b 或 zeroB 就近插入）
		// 简化策略：若 newCount == oldCount+1，从旧桶后插入；
		// 若 newCount == oldCount-1，从旧桶前插入；
		// 其它情况：近似就地插入到相邻（不会影响 O(1)）
		if newCount > oldCount {
			// 放在旧桶之后
			nb.prev = e.b
			nb.next = e.b.next
			if e.b.next != nil {
				e.b.next.prev = nb
			}
			e.b.next = nb
		} else {
			// 放在旧桶之前
			nb.next = e.b
			nb.prev = e.b.prev
			if e.b.prev != nil {
				e.b.prev.next = nb
			}
			e.b.prev = nb
		}
	}

	// 从旧桶移除并插入到新桶头
	oldB := e.b
	oldPrev := oldB.prev
	b.removeEntryFromBucket(oldB, e)
	e.count = newCount
	b.insertEntryToBucketHead(nb, e)
	e.b = nb

	// 更新 maxB
	if b.maxB == nil || nb.count > b.maxB.count {
		b.maxB = nb
	}
	// 若旧的 max 桶被移空且是当前 maxB，则回退到前一个非空桶
	if oldB == b.maxB && oldB.size == 0 {
		if oldPrev != nil {
			b.maxB = oldPrev
		} else {
			b.maxB = b.zeroB
		}
	}
	return newCount
}

func (b *Buckets) TopK(k int) []RankItem {
	if k <= 0 {
		return []RankItem{}
	}
	res := make([]RankItem, 0, k)
	for bb := b.maxB; bb != nil && len(res) < k; bb = bb.prev {
		for e := bb.head; e != nil && len(res) < k; e = e.next {
			res = append(res, RankItem{DocID: e.id, Clicks: e.count})
		}
	}
	return res
}

func (b *Buckets) GetCount(id string) int {
	if e, ok := b.entries[id]; ok {
		return e.count
	}
	return 0
}

func (b *Buckets) insertEntryToBucketHead(bb *bucket, e *entry) {
	e.prev = nil
	e.next = bb.head
	if bb.head != nil {
		bb.head.prev = e
	}
	bb.head = e
	if bb.tail == nil {
		bb.tail = e
	}
	bb.size++
}

func (b *Buckets) removeEntryFromBucket(bb *bucket, e *entry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		bb.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		bb.tail = e.prev
	}
	e.prev, e.next = nil, nil
	bb.size--
}
