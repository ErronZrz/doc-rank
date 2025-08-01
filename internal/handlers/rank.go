package handlers

import (
	"github.com/ErronZrz/doc-rank/internal/redis"
	"github.com/gin-gonic/gin"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

type RankItem struct {
	DocID  string `json:"doc_id"`
	Clicks int64  `json:"clicks"`
}

func TotalRankingHandler(c *gin.Context) {
	const defaultLimit = 100

	results, err := redis.Client.ZRevRangeWithScores(redis.Ctx, "doc_click_total", 0, defaultLimit-1).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redis error"})
		return
	}

	rankings := make([]RankItem, 0, len(results))
	for _, item := range results {
		score := int64(item.Score)
		rankings = append(rankings, RankItem{
			DocID:  item.Member.(string),
			Clicks: score,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"rank": rankings,
	})
}

// RecentRankingHandler 获取近 10 分钟排行榜
func RecentRankingHandler(c *gin.Context) {
	// 当前时间戳
	now := time.Now().Unix()
	cutoff := now - 600

	// 获取所有文档 ID（从 Redis 中所有 doc_click_timeline:* 中提取）
	keys, err := redis.Client.Keys(redis.Ctx, "doc_click_timeline:*").Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan timeline keys"})
		return
	}

	rankMap := make(map[string]int64)

	for _, key := range keys {
		// 清理过期点击事件
		if err := redis.Client.ZRemRangeByScore(redis.Ctx, key, "-inf", strconv.FormatInt(cutoff, 10)).Err(); err != nil {
			continue // 跳过异常项
		}

		count, err := redis.Client.ZCard(redis.Ctx, key).Result()
		if err != nil {
			continue
		}

		docID := extractDocIDFromTimelineKey(key)
		rankMap[docID] = count
	}

	// 转换为可排序列表
	rankings := make([]RankItem, 0, len(rankMap))
	for docID, clicks := range rankMap {
		rankings = append(rankings, RankItem{
			DocID:  docID,
			Clicks: clicks,
		})
	}

	// 按点击量排序（降序）
	slices.SortFunc(rankings, func(p, q RankItem) int {
		return int(q.Clicks - p.Clicks)
	})

	c.JSON(http.StatusOK, gin.H{
		"rank": rankings,
	})
}

// 提取 docID：从 "doc_click_timeline:<doc_id>" 中得到 <doc_id>
func extractDocIDFromTimelineKey(key string) string {
	return strings.TrimPrefix(key, "doc_click_timeline:")
}
