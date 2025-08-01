package sse

import (
	"github.com/gin-gonic/gin"
	"io"
)

func ServerSentEventHandler(c *gin.Context) {
	client := Subscribe()
	defer Unsubscribe(client)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	// 推送循环
	c.Stream(func(w io.Writer) bool {
		if msg, ok := <-client; ok {
			// 事件名可自定义
			c.SSEvent("update", string(msg))
			return true
		}
		return false
	})
}
