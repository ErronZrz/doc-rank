package main

import "time"

type Recent struct {
    ring        [600]map[string]int
    lastUnixSec int64
    bkt         *Buckets
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

// 仅在恢复阶段使用：把 lastUnixSec 重置为给定值（通常是最早点击的前一秒）
func (r *Recent) SetBase(tsSec int64) {
    r.lastUnixSec = tsSec
    // ring 已在 NewRecent 初始化；无需清空
}

func (r *Recent) advanceTo(targetSec int64) bool {
    if targetSec <= r.lastUnixSec {
        return false
    }
    changed := false
    steps := targetSec - r.lastUnixSec
    if steps > 600 {
        // 超过整个窗口，等价于清空
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
        // 过期的是“新秒的前一秒”的槽
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

// AddClick：严格在窗口内落位；过窗外（< last-599）直接丢弃，不再把旧点击“抬到当前秒”
func (r *Recent) AddClick(docID string, tsSec int64) {
    // 在窗口右侧则先推进
    if tsSec > r.lastUnixSec {
        _ = r.advanceTo(tsSec)
    }
    // 若在窗口左侧之外，丢弃
    if tsSec < r.lastUnixSec-599 {
        return
    }
    // 在窗口内：直接按 tsSec 对应的槽写入
    idx := int(tsSec % 600)
    if idx < 0 {
        idx += 600
    }
    r.ring[idx][docID] = r.ring[idx][docID] + 1
    r.bkt.Adjust(docID, +1)
}

// TopK 获取最近窗口的 TopK
func (r *Recent) TopK(k int) []RankItem {
	return r.bkt.TopK(k)
}
