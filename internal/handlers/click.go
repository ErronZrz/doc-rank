package handlers

import (
	"github.com/ErronZrz/doc-rank/internal/redis"
	"github.com/ErronZrz/doc-rank/internal/sse"
	"github.com/gin-gonic/gin"
	"net/http"
)

type ClickRequest struct {
	DocID string `json:"doc_id" binding:"required"`
}

func ClickHandler(c *gin.Context) {
	// log.Printf("Received click request: %s", c.Request.URL)
	var req ClickRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "doc_id is required"})
		return
	}

	if err := redis.RecordClick(req.DocID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record click"})
		return
	}

	// 触发 SSE 推送最新排行榜
	go broadcastUpdateSignal()

	c.JSON(http.StatusOK, gin.H{"message": "click recorded"})
}

// 构造广播数据
func broadcastUpdateSignal() {
	sse.Broadcast([]byte(`{"type":"update_all"}`))
}
