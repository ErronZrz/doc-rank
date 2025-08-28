package main

import (
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type SSEHub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

// NewSSEHub 创建 SSEHub
func NewSSEHub() *SSEHub {
	return &SSEHub{clients: make(map[chan string]struct{})}
}

// Serve 提供 SSE 连接
func (h *SSEHub) Serve(c *gin.Context) {
	// 禁用缓存
	c.Header("Cache-Control", "no-cache")

	ch := make(chan string, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	// 心跳，避免代理超时
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	ctxDone := c.Request.Context().Done()

	// 先发一次心跳
	c.SSEvent("ping", gin.H{})

	// 使用 gin 的 Stream + SSEvent
	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctxDone:
			h.mu.Lock()
			delete(h.clients, ch)
			h.mu.Unlock()
			return false
		case typ := <-ch:
			// 仅发送更新锚点
			if typ == "" {
				typ = "update_click"
			}
			c.SSEvent("update", gin.H{"type": typ})
			return true
		case <-ticker.C:
			c.SSEvent("ping", gin.H{})
			return true
		}
	})
}

// BroadcastUpdateClick 广播点击更新
func (h *SSEHub) BroadcastUpdateClick() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- "update_click":
		default:
			// 跳过慢客户端
		}
	}
}

// BroadcastUpdateDoc 广播文档更新
func (h *SSEHub) BroadcastUpdateDoc() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- "update_doc":
		default:
			// 跳过慢客户端
		}
	}
}
