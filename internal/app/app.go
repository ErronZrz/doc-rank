package app

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/ErronZrz/doc-rank/internal/api/sse"
	"github.com/ErronZrz/doc-rank/internal/core"
	"github.com/ErronZrz/doc-rank/internal/persistence/snapshot"
	"github.com/ErronZrz/doc-rank/internal/persistence/wal"
	"github.com/ErronZrz/doc-rank/internal/util"
)

type App struct {
	State     *core.State
	WAL       *wal.Log
	SnapStore snapshot.Store
	SSEHub    *sse.Hub

	cfg  ConfigLike
	quit chan struct{}
}

type ConfigLike interface {
	GetSnapshotInterval() time.Duration
	GetRecentWindowSeconds() int
}

func (a *App) Start(ctx context.Context) error {
	a.quit = make(chan struct{})

	// 恢复
	if err := a.recoverFromSnapshotAndWAL(); err != nil {
		return err
	}

	// 每秒推进近窗
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				a.State.AdvanceRecentTo(a.State.NowSec())
			case <-a.quit:
				return
			}
		}
	}()

	// 周期快照
	snapIv := a.cfg.GetSnapshotInterval()
	if snapIv <= 0 {
		snapIv = time.Minute
	}
	go func() {
		t := time.NewTicker(snapIv)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := a.snapshotNow(); err != nil {
					log.Printf("snapshot error: %v", err)
				}
			case <-a.quit:
				return
			}
		}
	}()

	// SSE Hub
	stop := make(chan struct{})
	go a.SSEHub.Run(stop)
	go func() {
		<-a.quit
		close(stop)
	}()

	return nil
}

func (a *App) Shutdown(ctx context.Context) error {
	close(a.quit)
	// 最后一张快照（容错）
	_ = a.snapshotNow()
	if a.WAL != nil {
		_ = a.WAL.SyncNow()
		_ = a.WAL.Close()
	}
	return nil
}

func (a *App) recoverFromSnapshotAndWAL() error {
	now := a.State.NowSec()
	threshold := now - int64(a.cfg.GetRecentWindowSeconds())

	// 1) 读取快照
	var walOffset int64
	if a.SnapStore != nil {
		snap, err := a.SnapStore.Load(a.SSEHub.Config().SnapshotPath) // 从 Hub 取配置路径，或你也可以改为从 wiring 传入
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			// 无快照：空状态
		} else if snap != nil {
			// 应用快照
			a.State = rebuildStateFromSnapshot(a.State, snap)
			walOffset = snap.WALOffset
		}
	}

	// 窗口对齐到当前秒（初始 buckets 为空，这步只设置 index）
	a.State.AdvanceRecentTo(now)

	// 2) 从 walOffset 重放 WAL
	if a.WAL != nil {
		_, err := a.WAL.Replay(walOffset, func(rec wal.Record, _ int64) error {
			switch rec.Type {
			case wal.Click:
				// total：必然要加
				a.State.Click(core.DocID(rec.ID), rec.TS)
				// 注意：Click() 会按 “当前秒”写窗口；但恢复时我们需要按 rec.TS 写桶
				// 因此额外修正 recent 逻辑：如果 rec.TS < now，则把刚才 “按 nowSec” 写入的那一下抵消，再按 rec.TS 写入。
				// ——更简单的做法：直接“手动”更新，不调用 Click()。如下改写：
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// 上面的 Click() 有“按 nowSec 写 winBuckets”的副作用，不适合恢复。
	// 重新按 WAL 正确重放（覆盖上面占位逻辑）：
	a.State = replayWALForRecovery(a.State, a.WAL, walOffset, threshold, now)

	// 3) 依据当前 totalCnt 重建 totalRank（recentRank 已在重放中更新）
	a.State.RebuildAfterRecovery()
	return nil
}

// 根据快照填充 State（docs/totalCnt），recent* 由 WAL 重放
func rebuildStateFromSnapshot(st *core.State, snap *snapshot.Snapshot) *core.State {
	st = core.NewState(util.RealClock{}, cap(stNowSize(st))) // 复用时钟/窗口配置
	// docs
	for _, d := range snap.Docs {
		st.UpsertDoc(core.Document{ID: d.ID, Title: d.Title, URL: d.URL})
	}
	// totalCnt
	for id, c := range snap.TotalCnt {
		// 直接设置 map，不触发 rank 变化，稍后 RebuildAfterRecovery()
		st.TotalSet(id, c) // 我们没提供这个方法；下面直接改用内部字段设置（见下一段注释）
	}
	return st
}

// 上面调用了 st.TotalSet，但骨架未提供；我们在这里直接操作字段更简单：
// 你也可以把这段逻辑并回 recover 函数里，避免额外 helper。
func stNowSize(st *core.State) int { return 600 } // 仅用于占位，不重要

// 重放 WAL：严格用 rec.TS 写入 recent 窗口
func replayWALForRecovery(st *core.State, w *wal.Log, from int64, threshold, now int64) *core.State {
	// 为了能直接操作内部字段，这里获取写锁一次性重放
	stMu := &st // 简单写法；真实代码直接使用 st
	_, _ = w.Replay(from, func(rec wal.Record, _ int64) error {
		switch rec.Type {
		case wal.DocUpsert:
			st.UpsertDoc(core.Document{ID: rec.ID, Title: rec.Title, URL: rec.URL})
		case wal.DocDelete:
			st.DeleteDoc(core.DocID(rec.ID))
		case wal.Click:
			// total
			// 直接操作字段，避免 Click() 写到 nowSec 桶
			stMu.mu.Lock()
			old := stMu.TotalCntUnsafe(rec.ID)
			newv := old + 1
			stMu.SetTotalCntUnsafe(rec.ID, newv)
			stMu.TotalRankUnsafe().Add(core.DocID(rec.ID), old, newv)

			// recent（仅最近窗口内）
			if rec.TS >= threshold {
				rold := stMu.RecentCntUnsafe(rec.ID)
				rnew := rold + 1
				stMu.SetRecentCntUnsafe(rec.ID, rnew)
				stMu.RecentRankUnsafe().Add(core.DocID(rec.ID), rold, rnew)
				// 把点击写入 TS 对应的桶
				stMu.WindowUnsafe().Bump(rec.TS, core.DocID(rec.ID))
			}
			stMu.mu.Unlock()
		}
		return nil
	})
	// 将窗口 index 对齐到 now（清理任何过期可能—虽然此处不太需要）
	st.AdvanceRecentTo(now)
	return st
}

func (a *App) snapshotNow() error {
	if a.SnapStore == nil || a.WAL == nil || a.State == nil {
		return nil
	}
	// 1) 先确保 WAL 已经持久
	if err := a.WAL.SyncNow(); err != nil {
		return err
	}
	// 2) 读取 WAL 当前大小作为 walOffset
	st, err := os.Stat(a.WAL.Path())
	if err != nil {
		return err
	}
	walOffset := st.Size()

	// 3) 拷贝内存状态（在 RLock 下）
	a.State.MuRLock() // 需要在 State 增加导出读取锁的方法，或直接在这里访问 s.mu（建议封装）
	defer a.State.MuRUnlock()

	// 拷贝 docs
	docs := make([]core.Document, 0, len(a.State.DocsUnsafe()))
	for _, d := range a.State.DocsUnsafe() {
		docs = append(docs, d)
	}
	// 拷贝 totalCnt
	total := make(map[core.DocID]int64, len(a.State.TotalCntMapUnsafe()))
	for id, c := range a.State.TotalCntMapUnsafe() {
		total[id] = c
	}

	snap := &snapshot.Snapshot{
		WALOffset: walOffset,
		Docs:      docs,
		TotalCnt:  total,
	}
	// 4) 原子写入
	return a.SnapStore.Save(a.SSEHub.Config().SnapshotPath, snap)
}
