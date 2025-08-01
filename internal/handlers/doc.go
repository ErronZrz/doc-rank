package handlers

import (
	"fmt"
	"github.com/ErronZrz/doc-rank/internal/redis"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Document struct {
	ID    string `json:"id" binding:"required"`
	Title string `json:"title" binding:"required"`
	URL   string `json:"url"`
}

// ListDocuments 获取所有文档
func ListDocuments(c *gin.Context) {
	keys, err := redis.Client.Keys(redis.Ctx, "doc_meta:*").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Redis KEYS error"})
		return
	}

	var docs []Document
	for _, key := range keys {
		values, err := redis.Client.HGetAll(redis.Ctx, key).Result()
		if err != nil {
			continue
		}
		id := key[len("doc_meta:"):]
		docs = append(docs, Document{
			ID:    id,
			Title: values["title"],
			URL:   values["url"],
		})
	}

	c.JSON(http.StatusOK, gin.H{"documents": docs})
}

// SaveDocument 添加或更新文档
func SaveDocument(c *gin.Context) {
	var doc Document
	if err := c.ShouldBindJSON(&doc); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	key := fmt.Sprintf("doc_meta:%s", doc.ID)
	if err := redis.Client.HSet(redis.Ctx, key, "title", doc.Title, "url", doc.URL).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save"})
		return
	}

	// 重新推送排行榜（不需要等 click 触发）
	go broadcastUpdateSignal()

	c.JSON(http.StatusOK, gin.H{"message": "Saved"})
}

// DeleteDocument 删除文档（包括点击数据）
func DeleteDocument(c *gin.Context) {
	docID := c.Param("id")
	if docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing ID"})
		return
	}

	keysToDelete := []string{
		fmt.Sprintf("doc_meta:%s", docID),
		fmt.Sprintf("doc_click_timeline:%s", docID),
	}

	// 删除总榜中的 doc_id 元素
	if err := redis.Client.ZRem(redis.Ctx, "doc_click_total", docID).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ZREM failed"})
		return
	}

	// 删除 meta + timeline
	if err := redis.Client.Del(redis.Ctx, keysToDelete...).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "DEL failed"})
		return
	}

	// 重新推送排行榜（不需要等 click 触发）
	go broadcastUpdateSignal()

	c.JSON(http.StatusOK, gin.H{"message": "Deleted"})
}
