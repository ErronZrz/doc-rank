package main

import "sort"

type Docs struct {
	m map[string]Doc
}

// NewDocs 创建文档集合
func NewDocs() *Docs {
	return &Docs{m: make(map[string]Doc, 1024)}
}

// Upsert 插入或更新文档
func (d *Docs) Upsert(doc Doc) {
	d.m[doc.ID] = doc
}

// Delete 删除文档
func (d *Docs) Delete(id string) {
	delete(d.m, id)
}

// Get 返回文档及存在标记
func (d *Docs) Get(id string) (Doc, bool) {
	v, ok := d.m[id]
	return v, ok
}

// List 返回按 ID 升序的文档列表
func (d *Docs) List() []Doc {
	out := make([]Doc, 0, len(d.m))
	for _, v := range d.m {
		out = append(out, v)
	}
	// 按 ID 升序
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}
