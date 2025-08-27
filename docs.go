package main

type Docs struct {
	m map[string]Doc
}

func NewDocs() *Docs {
	return &Docs{m: make(map[string]Doc, 1024)}
}

func (d *Docs) Upsert(doc Doc) {
	d.m[doc.ID] = doc
}

func (d *Docs) Delete(id string) {
	delete(d.m, id)
}

func (d *Docs) Get(id string) (Doc, bool) {
	v, ok := d.m[id]
	return v, ok
}

func (d *Docs) List() []Doc {
	out := make([]Doc, 0, len(d.m))
	for _, v := range d.m {
		out = append(out, v)
	}
	return out
}
