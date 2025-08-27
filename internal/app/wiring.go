package app

import (
	"time"

	"github.com/ErronZrz/doc-rank/internal/api/sse"
	"github.com/ErronZrz/doc-rank/internal/core"
	"github.com/ErronZrz/doc-rank/internal/persistence/snapshot"
	"github.com/ErronZrz/doc-rank/internal/persistence/wal"
	"github.com/ErronZrz/doc-rank/internal/util"
)

type WiringConfig struct {
	RecentWindowSeconds int
	SSEPushInterval     time.Duration
	SSETopK             int
	WALPath             string
	WALGroupEvery       time.Duration
	WALBatch            int
	SnapshotPath        string
	SnapshotInterval    time.Duration
}

func NewApp(cfg WiringConfig) (*App, error) {
	state := core.NewState(util.RealClock{}, cfg.RecentWindowSeconds)

	log, err := wal.OpenLog(cfg.WALPath, cfg.WALGroupEvery, cfg.WALBatch)
	if err != nil {
		return nil, err
	}

	fs := snapshot.NewFileStore()
	hub := sse.NewHub(state, cfg.SSEPushInterval, cfg.SSETopK)

	a := &App{
		State:     state,
		WAL:       log,
		SnapStore: fs,
		SSEHub:    hub,
		cfg:       cfgWrap{cfg},
	}
	return a, nil
}

// 适配 ConfigLike
type cfgWrap struct{ WiringConfig }

func (c cfgWrap) GetSnapshotInterval() time.Duration { return c.SnapshotInterval }
func (c cfgWrap) GetRecentWindowSeconds() int        { return c.RecentWindowSeconds }
