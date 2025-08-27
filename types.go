package main

type Doc struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type ClickReq struct {
	DocID string `json:"doc_id" binding:"required"`
}

type RankItem struct {
	DocID  string `json:"doc_id"`
	Clicks int    `json:"clicks"`
}

type RankResp struct {
	Rank []RankItem `json:"rank"`
}

type DocsResp struct {
	Documents []Doc `json:"documents"`
}

type UpsertDocReq struct {
	ID    string `json:"id" binding:"required"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// SSE 消息结构（简单统一）
type SSEMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// WAL 条目（用于持久化）
type walEntry struct {
	Seq   uint64 `json:"seq"`
	Op    string `json:"op"` // ADD / UPDATE / DEL / CLICK
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}
