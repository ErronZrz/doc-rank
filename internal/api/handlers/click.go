package handlers

import (
	"net/http"

	"github.com/ErronZrz/doc-rank/internal/core"
	"github.com/ErronZrz/doc-rank/internal/persistence/wal"

	"github.com/gin-gonic/gin"
)

type ClickDeps interface {
	NowSec() int64
}

type ClickHandlerDeps struct {
	State *core.State
	WAL   *wal.Log
}

type clickReq struct {
	DocID string `json:"doc_id"`
}

func (h *ClickHandlerDeps) ClickHandler(c *gin.Context) {
	var req clickReq
	if err := c.BindJSON(&req); err != nil || req.DocID == "" {
		JSONBadRequest(c, "invalid doc_id")
		return
	}

	now := h.State.NowSec()

	// 1) WAL 先行
	if err := h.WAL.AppendClick(req.DocID, now); err != nil {
		JSONServerErr(c, "wal append failed")
		return
	}
	// 2) 内存更新
	h.State.Click(core.DocID(req.DocID), now)

	// 可选：标记 SSE 需要推送（放到 hub）
	c.Status(http.StatusNoContent)
}
