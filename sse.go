package main

import (
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type sseClient struct {
	ch chan SSEMessage
}

type SSEHub struct {
	mu      sync.Mutex
	clients map[*sseClient]struct{}
}

func NewSSEHub() *SSEHub {
	return &SSEHub{clients: make(map[*sseClient]struct{})}
}

func (h *SSEHub) Serve(c *gin.Context) {
	// 标准 SSE 头由 gin 在第一次 SSEvent 时写入，这里只禁止缓存
	c.Header("Cache-Control", "no-cache")

	client := &sseClient{ch: make(chan SSEMessage, 32)}
	h.mu.Lock()
	h.clients[client] = struct{}{}
	h.mu.Unlock()

	// 心跳，防止代理超时
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	ctxDone := c.Request.Context().Done()

	// 立即发送一次 ping，便于前端建立连接后的快速确认
	c.SSEvent("ping", gin.H{})

	// 使用 gin 的 Stream + SSEvent 推送
	c.Stream(func(w io.Writer) bool {
		select {
		case <-ctxDone:
			h.mu.Lock()
			delete(h.clients, client)
			h.mu.Unlock()
			return false
		case msg := <-client.ch:
			// 事件名用 msg.Type，更直观；如 "total_topk"
			if msg.Type == "" {
				msg.Type = "message"
			}
			c.SSEvent(msg.Type, msg.Data)
			return true
		case <-ticker.C:
			c.SSEvent("ping", gin.H{})
			return true
		}
	})
}

func (h *SSEHub) Broadcast(msg SSEMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for cl := range h.clients {
		select {
		case cl.ch <- msg:
		default:
			// 客户端处理过慢，丢弃本条以避免阻塞
		}
	}
}
