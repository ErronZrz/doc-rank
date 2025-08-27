package handlers

import (
	"strconv"

	"github.com/ErronZrz/doc-rank/internal/core"
	"github.com/gin-gonic/gin"
)

type RankHandlerDeps struct {
	State *core.State
}

func (h *RankHandlerDeps) TotalRankingHandler(c *gin.Context) {
	limit := parseLimit(c.Query("limit"))
	items := h.State.GetTotalTopK(limit)
	JSONOK(c, gin.H{"rank": items})
}

func (h *RankHandlerDeps) RecentRankingHandler(c *gin.Context) {
	limit := parseLimit(c.Query("limit"))
	items := h.State.GetRecentTopK(limit)
	JSONOK(c, gin.H{"rank": items})
}

func parseLimit(s string) int {
	if s == "" {
		return 0 // 0 表示全量
	}
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return 0
}
