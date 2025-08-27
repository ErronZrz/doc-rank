package handlers

import (
	"github.com/ErronZrz/doc-rank/internal/core"
	"github.com/ErronZrz/doc-rank/internal/persistence/wal"

	"github.com/gin-gonic/gin"
)

type DocsHandlerDeps struct {
	State *core.State
	WAL   *wal.Log
}

type saveDocReq struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func (h *DocsHandlerDeps) ListDocuments(c *gin.Context) {
	JSONOK(c, gin.H{"documents": h.State.GetDocs()})
}

func (h *DocsHandlerDeps) SaveDocument(c *gin.Context) {
	var req saveDocReq
	if err := c.BindJSON(&req); err != nil || req.ID == "" {
		JSONBadRequest(c, "invalid payload")
		return
	}
	if err := h.WAL.AppendDocUpsert(req.ID, req.Title, req.URL, h.State.NowSec()); err != nil {
		JSONServerErr(c, "wal append failed")
		return
	}
	h.State.UpsertDoc(core.Document{ID: req.ID, Title: req.Title, URL: req.URL})
	JSONOK(c, gin.H{"ok": true})
}

func (h *DocsHandlerDeps) DeleteDocument(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		JSONBadRequest(c, "invalid id")
		return
	}
	if err := h.WAL.AppendDocDelete(id, h.State.NowSec()); err != nil {
		JSONServerErr(c, "wal append failed")
		return
	}
	h.State.DeleteDoc(core.DocID(id))
	JSONOK(c, gin.H{"ok": true})
}
