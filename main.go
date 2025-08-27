package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	cfg := LoadConfig()

	p, err := NewPersist(cfg.DataDir, cfg.WALSyncEveryWrite)
	if err != nil {
		log.Fatalf("persist init error: %v", err)
	}
	defer p.Close()
	log.Printf("persist ready: %s", p.DebugPaths())

	// 恢复
	state, err := p.Restore()
	if err != nil {
		log.Fatalf("restore error: %v", err)
	}
	log.Printf("restored: docs=%d counts=%d seq=%d recentClicks=%d",
		len(state.Docs), len(state.Counts), state.Seq, len(state.RecentClicks))

	// SSE hub + Store
	sse := NewSSEHub()
	store := NewStore(p, sse, cfg)
	store.Load(state)

	// 启动 recent 窗口推进器
	stopSnap := make(chan struct{})
	store.StartRecentAdvancer(stopSnap)

	// HTTP
	router := SetupRouter(store, sse, cfg)
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	// 周期快照
	go func() {
		ticker := time.NewTicker(cfg.SnapshotInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				counts := store.CountsSnapshot()
				docs := store.ListDocs()
				if err := p.SaveSnapshot(docs, counts); err != nil {
					log.Printf("snapshot error: %v", err)
				} else {
					log.Printf("snapshot saved: docs=%d counts=%d", len(docs), len(counts))
				}
			case <-stopSnap:
				return
			}
		}
	}()

	// 启动 HTTP
	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	fmt.Println("\nshutting down...")

	close(stopSnap)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 最后一拍快照
	counts := store.CountsSnapshot()
	docs := store.ListDocs()
	if err := p.SaveSnapshot(docs, counts); err != nil {
		log.Printf("final snapshot error: %v", err)
	}

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	if err := p.Close(); err != nil {
		log.Printf("persist close error: %v", err)
	}
	log.Println("bye.")
}
