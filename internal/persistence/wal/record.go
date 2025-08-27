package wal

type Kind string

const (
	Click     Kind = "click"
	DocUpsert Kind = "doc_upsert"
	DocDelete Kind = "doc_delete"
)

type Record struct {
	Type  Kind   `json:"type"`
	TS    int64  `json:"ts"`
	ID    string `json:"doc,omitempty"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
	// TODO: 可选 CRC/校验
}
