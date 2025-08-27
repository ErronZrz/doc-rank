package sse

import (
	"net/http"
	"time"

	"github.com/ErronZrz/doc-rank/internal/core"

	"github.com/gin-gonic/gin"
)

type Hub struct {
	State        *core.State
	PushInterval time.Duration
	TopK         int
	// TODO: 维护客户端集合、变更聚合集合
}

func NewHub(state *core.State, pushInterval time.Duration, topK int) *Hub {
	return &Hub{State: state, PushInterval: pushInterval, TopK: topK}
}

func (h *Hub) Run(stop <-chan struct{}) {
	// TODO: 每 PushInterval 拉取 TopK（total & recent），向所有客户端 broadcast
}

func ServerSentEventHandler(h *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.WriteHeader(http.StatusOK)

		// TODO: 注册客户端；循环写入：event: total\n data: {...}\n\n
		// 注意：避免持锁序列化；先复制数据再无锁编码写出
	}
}
