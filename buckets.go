package main

import "slices"

// bucket 是频次桶的节点，支持 O(1) 插入和删除
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
	prev, next *entry // 桶内链
}

// Buckets 维护 id 到计数的桶式结构，支持 O(1) 调整 和 O(K) 取前 K
type Buckets struct {
	entries map[string]*entry
	bmap    map[int]*bucket
	maxB    *bucket
	zeroB   *bucket // 初始放入 0 桶
}

// NewBuckets 创建 Buckets
func NewBuckets() *Buckets {
	b := &Buckets{
		entries: make(map[string]*entry, 1024),
		bmap:    make(map[int]*bucket, 1024),
	}
	// 预建 count = 0 桶
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

// Add 将 id 放入 0 桶
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

// Delete 移除 id
func (b *Buckets) Delete(id string) {
	e, ok := b.entries[id]
	if !ok {
		return
	}
	b.removeEntryFromBucket(e.b, e)
	delete(b.entries, id)
	// 必要时回退 maxB
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

// Inc 计数加 1
func (b *Buckets) Inc(id string) int {
	return b.Adjust(id, 1)
}

// Adjust 调整 id 的计数到 newCount = old + delta 不低于 0
func (b *Buckets) Adjust(id string, delta int) int {
	if delta == 0 {
		return b.GetCount(id)
	}
	e, ok := b.entries[id]
	if !ok {
		// 不存在时仅处理正增量
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

	// 定位或创建目标桶
	nb := b.bmap[newCount]
	if nb == nil {
		nb = &bucket{count: newCount}
		b.bmap[newCount] = nb
		// 就近插入到旧桶前后
		if newCount > oldCount {
			// 旧桶之后
			nb.prev = e.b
			nb.next = e.b.next
			if e.b.next != nil {
				e.b.next.prev = nb
			}
			e.b.next = nb
		} else {
			// 旧桶之前
			nb.next = e.b
			nb.prev = e.b.prev
			if e.b.prev != nil {
				e.b.prev.next = nb
			}
			e.b.prev = nb
		}
	}

	// 从旧桶移除并插入新桶头
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
	// 必要时回退 maxB
	if oldB == b.maxB && oldB.size == 0 {
		if oldPrev != nil {
			b.maxB = oldPrev
		} else {
			b.maxB = b.zeroB
		}
	}
	return newCount
}

// TopK 返回前 K 项
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

// GetCount 返回 id 的计数
func (b *Buckets) GetCount(id string) int {
	if e, ok := b.entries[id]; ok {
		return e.count
	}
	return 0
}

// ResetFromCounts 用计数快照重建桶链
func (b *Buckets) ResetFromCounts(counts map[string]int) {
	// 采样并排序 (按计数升序，id 次序保证稳定)
	type pair struct {
		id string
		c  int
	}
	arr := make([]pair, 0, len(counts))
	for id, c := range counts {
		if c < 0 {
			c = 0
		}
		arr = append(arr, pair{id: id, c: c})
	}
	slices.SortFunc(arr, func(p, q pair) int {
		return p.c - q.c
	})

	// 重置结构并按升序链接桶，使 next 指向更大计数，prev 指向更小计数
	b.entries = make(map[string]*entry, len(counts)+16)
	b.bmap = make(map[int]*bucket, len(counts)+1)
	// 先创建 0 桶，确保存在
	z := &bucket{count: 0}
	b.bmap[0] = z
	b.zeroB = z
	last := z

	// 遍历排序后的数组，逐个插入
	for _, p := range arr {
		// 确保桶存在且按顺序链接
		nb, ok := b.bmap[p.c]
		if !ok {
			nb = &bucket{count: p.c}
			b.bmap[p.c] = nb
			// 链接到 last 之后
			nb.prev = last
			if last != nil {
				last.next = nb
			}
			last = nb
		}
		// 放入条目
		e := &entry{id: p.id, count: p.c, b: nb}
		b.insertEntryToBucketHead(nb, e)
		b.entries[p.id] = e
	}
	// 设置 maxB 为最后一个非空桶 (若仅 0 桶，则为 0 桶)
	b.maxB = last
	if b.maxB == nil {
		b.maxB = b.zeroB
	}
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
