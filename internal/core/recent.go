package core

// 秒级环形窗口（默认 600 秒）
type RecentWindow struct {
	winBuckets []map[DocID]int32 // len = windowSize
	index      int               // 当前“窗口头”的下标（与上次推进后对齐）
	windowSize int
}

func NewRecentWindow(windowSize int) *RecentWindow {
	w := &RecentWindow{
		winBuckets: make([]map[DocID]int32, windowSize),
		index:      0,
		windowSize: windowSize,
	}
	for i := range w.winBuckets {
		w.winBuckets[i] = make(map[DocID]int32)
	}
	return w
}

// Bump：在 nowSec 对应的桶累加 doc 的增量
func (w *RecentWindow) Bump(nowSec int64, doc DocID) {
	idx := w.IndexFor(nowSec)
	m := w.winBuckets[idx]
	m[doc] = m[doc] + 1
}

// AdvanceTo：把窗口从当前 index 推进到 targetIdx（逐秒过期）
// 注意：参数 currentIndex 实际上传入的是“目标下标”（与骨架命名保持一致避免改签名）
// onExpire(doc, delta) 用于回调扣 recentCnt 并更新 recentRank
func (w *RecentWindow) AdvanceTo(targetIdx int, _ int64, onExpire func(doc DocID, delta int32)) {
	// 逐步推进 index，直到与目标下标一致
	for w.index != targetIdx {
		// 下一个将被覆盖的桶位置
		w.index = (w.index + 1) % w.windowSize
		expire := w.winBuckets[w.index]
		if len(expire) > 0 {
			for doc, delta := range expire {
				if delta != 0 {
					onExpire(doc, delta)
				}
			}
			// 清空被覆盖桶，避免残留
			w.winBuckets[w.index] = make(map[DocID]int32)
		}
	}
}

// IndexFor 返回 nowSec 对应的桶下标
func (w *RecentWindow) IndexFor(nowSec int64) int {
	return int(nowSec % int64(w.windowSize))
}
