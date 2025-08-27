package core

type DocID = string

type Document struct {
	ID    DocID  `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type RankItem struct {
	DocID  DocID `json:"doc_id"`
	Clicks int64 `json:"clicks"`
}
