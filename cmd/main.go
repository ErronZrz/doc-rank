package main

import (
	"fmt"
	"github.com/ErronZrz/doc-rank/config"
	"github.com/ErronZrz/doc-rank/internal/handlers"
	"github.com/ErronZrz/doc-rank/internal/redis"
	"github.com/ErronZrz/doc-rank/internal/sse"
	"github.com/gin-contrib/cors"
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()
	redis.InitRedis(cfg)

	r := gin.Default()

	// 启用 CORS
	r.Use(cors.Default())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	r.POST("/click", handlers.ClickHandler)

	r.GET("/rank/total", handlers.TotalRankingHandler)
	r.GET("/rank/recent", handlers.RecentRankingHandler)
	r.GET("/events", sse.SSEHandler)

	log.Printf("Server is running at http://localhost:%s", cfg.Port)
	if err := r.Run(fmt.Sprintf(":%s", cfg.Port)); err != nil {
		log.Fatalf("Server start failed: %v", err)
	}
}
