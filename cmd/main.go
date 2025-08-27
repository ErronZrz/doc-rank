package main

import (
	"context"

	"github.com/ErronZrz/doc-rank/internal/api/handlers"
	"github.com/ErronZrz/doc-rank/internal/api/sse"
	"github.com/ErronZrz/doc-rank/internal/app"
)

func main() {
	// 伪示例：根据你的 cfg 构建 app
	aw, _ := app.NewApp(app.WiringConfig{
		RecentWindowSeconds: cfg.RecentWindowSeconds,
		SSEPushInterval:     cfg.SSEPushInterval,
		SSETopK:             cfg.SSETopK,
		WALPath:             cfg.WALPath,
		WALGroupEvery:       cfg.WALGroupCommitEvery,
		WALBatch:            cfg.WALGroupBatch,
		SnapshotPath:        cfg.SnapshotPath,
		SnapshotInterval:    cfg.SnapshotInterval,
	})
	_ = aw.Start(context.Background())
	defer aw.Shutdown(context.Background())

	handlers := &handlers.RankHandlerDeps{State: aw.State}
	clicks := &handlers.ClickHandlerDeps{State: aw.State, WAL: aw.WAL}
	docs := &handlers.DocsHandlerDeps{State: aw.State, WAL: aw.WAL}

	r.POST("/click", clicks.ClickHandler)
	r.GET("/rank/total", handlers.TotalRankingHandler) // 注意：此处应使用 handlers 变量名不同避免覆盖
	r.GET("/rank/recent", handlers.RecentRankingHandler)
	r.GET("/events", sse.ServerSentEventHandler(aw.SSEHub))

	r.GET("/docs", docs.ListDocuments)
	r.POST("/docs", docs.SaveDocument)
	r.DELETE("/docs/:id", docs.DeleteDocument)
}
