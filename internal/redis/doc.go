package redis

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"time"
)

const (
	KeyClickTotal    = "doc_click_total"
	KeyClickTimeline = "doc_click_timeline:%s"
)

// RecordClick 记录点击行为：总榜 + 时间线
func RecordClick(docID string) error {
	now := time.Now().Unix()
	clickID := fmt.Sprintf("%d-%s-%s", now, docID, uuid.NewString())
	timelineKey := fmt.Sprintf(KeyClickTimeline, docID)

	// 1. 增加总点击量
	if err := Client.ZIncrBy(Ctx, KeyClickTotal, 1, docID).Err(); err != nil {
		return fmt.Errorf("ZIncrBy failed: %w", err)
	}

	// 2. 添加点击事件到时间线（用于近10分钟榜）
	if err := Client.ZAdd(Ctx, timelineKey, redis.Z{
		Score:  float64(now),
		Member: clickID,
	}).Err(); err != nil {
		return fmt.Errorf("ZAdd timeline failed: %w", err)
	}

	return nil
}
