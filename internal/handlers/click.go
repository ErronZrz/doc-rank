package handlers

import (
	"encoding/json"
	"github.com/ErronZrz/doc-rank/internal/redis"
	"github.com/ErronZrz/doc-rank/internal/sse"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
	go broadcastLatestRanking()

	c.JSON(http.StatusOK, gin.H{"message": "click recorded"})
}

// 构造广播数据
func broadcastLatestRanking() {
	const topN = 10
	now := time.Now().Unix()
	cutoff := now - 600

	// ------- 1. 获取总榜 -------
	totalResults, _ := redis.Client.ZRevRangeWithScores(redis.Ctx, "doc_click_total", 0, topN-1).Result()
	totalRank := make([]RankItem, 0, len(totalResults))
	for _, r := range totalResults {
		totalRank = append(totalRank, RankItem{
			DocID:  r.Member.(string),
			Clicks: int64(r.Score),
		})
	}

	// ------- 2. 获取近 10 分钟榜 -------
	// 从 Redis 中提取 doc_click_timeline:* 键
	keys, err := redis.Client.Keys(redis.Ctx, "doc_click_timeline:*").Result()
	if err != nil {
		log.Printf("failed to scan timeline keys: %v", err)
		return
	}

	recentRankMap := make(map[string]int64)
	for _, key := range keys {
		// 清理 10 分钟前点击
		if err := redis.Client.ZRemRangeByScore(redis.Ctx, key, "-inf", strconv.FormatInt(cutoff, 10)).Err(); err != nil {
			continue
		}
		count, err := redis.Client.ZCard(redis.Ctx, key).Result()
		if err != nil {
			continue
		}
		docID := strings.TrimPrefix(key, "doc_click_timeline:")
		recentRankMap[docID] = count
	}

	// 排序 recent 排行榜
	recentRank := make([]RankItem, 0, len(recentRankMap))
	for docID, clicks := range recentRankMap {
		recentRank = append(recentRank, RankItem{
			DocID:  docID,
			Clicks: clicks,
		})
	}
	slices.SortFunc(recentRank, func(p, q RankItem) int {
		return int(q.Clicks - p.Clicks)
	})

	// ------- 3. 合并消息体并推送 -------
	payload, _ := json.Marshal(gin.H{
		"total_rank":  totalRank,
		"recent_rank": recentRank,
	})
	sse.Broadcast(payload)
}
