package main

// 频次桶：支持 add/del/inc/topK，严格 O(1)/O(K)
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
	zeroB   *bucket // 可选：用于新建时放到 0 桶
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
	// maxB 至少为 0 桶
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
		// 向前（更小 count）回退，直到第一个非空；找不到则指向 0 桶
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
	e, ok := b.entries[id]
	if !ok {
		// 自动创建到 0 桶，再 +1
		b.Add(id)
		e = b.entries[id]
	}
	oldB := e.b
	newCount := e.count + 1
	nb := b.bmap[newCount]
	if nb == nil {
		// 在 oldB 之后插入新桶（count +1）
		nb = &bucket{count: newCount}
		b.bmap[newCount] = nb
		// 将 nb 挂到 oldB 后
		nb.prev = oldB
		nb.next = oldB.next
		if oldB.next != nil {
			oldB.next.prev = nb
		}
		oldB.next = nb
	}
	// 从旧桶移除并插入到新桶头
	b.removeEntryFromBucket(oldB, e)
	e.count = newCount
	b.insertEntryToBucketHead(nb, e)
	e.b = nb
	// 更新 maxB
	if b.maxB == nil || nb.count > b.maxB.count {
		b.maxB = nb
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
	// 若桶空且不是 0 桶，可从链条中摘除（无需删除 bmap，保留以简化链）
	if bb.size == 0 && bb != b.zeroB {
		if bb.prev != nil {
			bb.prev.next = bb.next
		}
		if bb.next != nil {
			bb.next.prev = bb.prev
		}
		// 不从 bmap 删除，保留可复用；若希望紧凑，可删除：
		// delete(b.bmap, bb.count)
		// 并断开指针
		bb.prev, bb.next = nil, nil
	}
}
