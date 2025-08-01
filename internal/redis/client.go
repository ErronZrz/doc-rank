package redis

import (
	"context"
	"github.com/ErronZrz/doc-rank/config"
	"github.com/redis/go-redis/v9"
	"log"
)

var (
	Client *redis.Client
	Ctx    = context.Background()
)

// InitRedis 初始化 Redis 客户端
func InitRedis(cfg config.Config) {
	Client = redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		DB:       cfg.RedisDB,
		Password: "",
	})

	// 测试连接
	if err := Client.Ping(Ctx).Err(); err != nil {
		log.Fatalf("Redis 连接失败: %v", err)
	}
	log.Println("Redis 连接成功")
}
