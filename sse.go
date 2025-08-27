package main

import (
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type SSEHub struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func NewSSEHub() *SSEHub {
	return &SSEHub{clients: make(map[chan struct{}]struct{})}
}

func (h *SSEHub) Serve(c *gin.Context) {
	// 首次 SSEvent 时 gin 会写入必要的 SSE 头，这里禁用缓存即可
	c.Header("Cache-Control", "no-cache")

	ch := make(chan struct{}, 32)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	ticker := time.NewTicker(25 * time.Second) // 心跳，避免中间代理超时
	defer ticker.Stop()

	ctxDone := c.Request.Context().Done()

	// 先发一次心跳，便于客户端确认连接
	c.SSEvent("ping", gin.H{})

	// 使用 gin 的 Stream + SSEvent
	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctxDone:
			h.mu.Lock()
			delete(h.clients, ch)
			h.mu.Unlock()
			return false
		case <-ch:
			// 仅发送“更新锚点”，不携带实际榜单数据
			c.SSEvent("update", gin.H{"type": "update_all"})
			return true
		case <-ticker.C:
			c.SSEvent("ping", gin.H{})
			return true
		}
	})
}

// BroadcastUpdate 向所有订阅者发出“有更新”的通知（非阻塞）
func (h *SSEHub) BroadcastUpdate() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- struct{}{}:
		default:
			// 客户端处理过慢，跳过本次，避免阻塞服务端
		}
	}
}
