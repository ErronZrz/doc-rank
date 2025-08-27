package main

import "time"

// Recent 维护最近 10 分钟窗口（600 秒）
// - ring[600]：每秒一个 map[docID]int
// - bkt：一个 Buckets（计数等于最近 600 秒内点击数）
type Recent struct {
	ring         [600]map[string]int
	lastUnixSec  int64
	bkt          *Buckets
}

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

// advanceTo 推进到目标秒（可能推进多个秒）；老槽逐个过期
func (r *Recent) advanceTo(targetSec int64) {
	if targetSec <= r.lastUnixSec {
		return
	}
	steps := targetSec - r.lastUnixSec
	if steps > 600 {
		// 超过一整个窗口，相当于清空所有 recent
		for i := 0; i < 600; i++ {
			for id, cnt := range r.ring[i] {
				if cnt > 0 {
					r.bkt.Adjust(id, -cnt)
				}
			}
			clear(r.ring[i])
		}
		r.lastUnixSec = targetSec
		return
	}
	// 逐秒推进
	for s := int64(0); s < steps; s++ {
		oldIdx := int((r.lastUnixSec + 1 + s) % 600) // 即将成为“最新秒”的槽索引的前一个就是要过期的
		// 过期 oldIdx（注意：lastUnixSec+s 是“过期完成后的秒”）
		for id, cnt := range r.ring[oldIdx] {
			if cnt > 0 {
				r.bkt.Adjust(id, -cnt)
			}
		}
		clear(r.ring[oldIdx])
	}
	r.lastUnixSec = targetSec
}

// AddClick 在指定时间戳（秒）上记录一次点击，并调整 recent 排行
func (r *Recent) AddClick(docID string, tsSec int64) {
	// 推进到 tsSec（保证窗口时间向前单调）
	if tsSec < r.lastUnixSec {
		// 倒退的时间戳：按当前 lastUnixSec 计入，避免写入过旧槽
		tsSec = r.lastUnixSec
	}
	r.advanceTo(tsSec)

	idx := int(tsSec % 600)
	r.ring[idx][docID] = r.ring[idx][docID] + 1
	r.bkt.Adjust(docID, +1)
}

// TopK 获取最近窗口的 TopK
func (r *Recent) TopK(k int) []RankItem {
	return r.bkt.TopK(k)
}
