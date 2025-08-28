package main

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// SetupRouter 构建 HTTP 路由
func SetupRouter(store *Store, sse *SSEHub, cfg Config) *gin.Engine {
	r := gin.Default()

	// SSE
	r.GET("/events", sse.Serve)

	// 点击
	r.POST("/click", func(c *gin.Context) {
		var req ClickReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "bad request"})
			return
		}
		n, ok, err := store.Click(req.DocID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
			return
		}
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "document not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"doc_id": req.DocID, "clicks": n})
	})

	// 统一排行榜：同时返回总榜与最近榜
	r.GET("/rank", func(c *gin.Context) {
		limitStr := c.Query("limit")
		limit := cfg.TopKDefault
		if limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
				limit = v
			}
		}
		total := store.TopK(limit)
		recent := store.TopKRecent(limit)
		c.JSON(http.StatusOK, gin.H{
			"total":  RankResp{Rank: total},
			"recent": RankResp{Rank: recent},
		})
	})

	// 兼容旧的总排行榜
	r.GET("/rank/total", func(c *gin.Context) {
		limitStr := c.Query("limit")
		limit := cfg.TopKDefault
		if limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
				limit = v
			}
		}
		top := store.TopK(limit)
		c.JSON(http.StatusOK, RankResp{Rank: top})
	})

	// 兼容旧的近 10 分钟排行榜
	r.GET("/rank/recent", func(c *gin.Context) {
		limitStr := c.Query("limit")
		limit := cfg.TopKDefault
		if limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
				limit = v
			}
		}
		top := store.TopKRecent(limit)
		c.JSON(http.StatusOK, RankResp{Rank: top})
	})

	// 文档列表
	r.GET("/docs", func(c *gin.Context) {
		c.JSON(http.StatusOK, DocsResp{Documents: store.ListDocs()})
	})

	// 新增或修改文档
	r.POST("/docs", func(c *gin.Context) {
		var req UpsertDocReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "bad request"})
			return
		}
		doc := Doc{ID: req.ID, Title: req.Title, URL: req.URL}
		if err := store.AddOrUpdateDoc(doc); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, doc)
	})

	// 删除文档
	r.DELETE("/docs/:id", func(c *gin.Context) {
		id := c.Param("id")
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "bad request"})
			return
		}
		if err := store.DeleteDoc(id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": id})
	})

	return r
}
