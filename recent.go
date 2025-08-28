package main

import "time"

type Recent struct {
	ring        [600]map[string]int
	lastUnixSec int64
	bkt         *Buckets
}

// NewRecent 创建最近窗口结构
func NewRecent() *Recent {
	r := &Recent{
		lastUnixSec: time.Now().Unix(),
		bkt:         NewBuckets(),
	}
	for i := 0; i < 600; i++ {
		r.ring[i] = make(map[string]int)
	}
	return r
}

// SetBase 重置窗口基准秒，仅用于恢复
func (r *Recent) SetBase(tsSec int64) {
	r.lastUnixSec = tsSec
}

func (r *Recent) advanceTo(targetSec int64) bool {
	if targetSec <= r.lastUnixSec {
		return false
	}
	changed := false
	steps := targetSec - r.lastUnixSec
	if steps > 600 {
		// 超过窗口，相当于清空
		for i := 0; i < 600; i++ {
			for id, cnt := range r.ring[i] {
				if cnt > 0 {
					r.bkt.Adjust(id, -cnt)
					changed = true
				}
			}
			clear(r.ring[i])
		}
		r.lastUnixSec = targetSec
		return changed
	}
	for s := int64(0); s < steps; s++ {
		// 过期的是新秒的前一秒
		oldIdx := int((r.lastUnixSec + 1 + s - 600) % 600)
		if oldIdx < 0 {
			oldIdx += 600
		}
		for id, cnt := range r.ring[oldIdx] {
			if cnt > 0 {
				r.bkt.Adjust(id, -cnt)
				changed = true
			}
		}
		clear(r.ring[oldIdx])
	}
	r.lastUnixSec = targetSec
	return changed
}

// AddClick 按事件时间写入窗口，窗口外丢弃
func (r *Recent) AddClick(docID string, tsSec int64) {
	// 在右侧则先推进
	if tsSec > r.lastUnixSec {
		_ = r.advanceTo(tsSec)
	}
	// 左侧之外丢弃
	if tsSec < r.lastUnixSec-599 {
		return
	}
	// 窗口内落位
	idx := int(tsSec % 600)
	if idx < 0 {
		idx += 600
	}
	r.ring[idx][docID] = r.ring[idx][docID] + 1
	r.bkt.Adjust(docID, +1)
}

// TopK 返回最近窗口的前 K 项
func (r *Recent) TopK(k int) []RankItem {
	return r.bkt.TopK(k)
}
