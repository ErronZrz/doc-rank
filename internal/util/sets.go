package util

type Void struct{}

type StringSet map[string]Void

func NewStringSet() StringSet         { return make(StringSet) }
func (s StringSet) Add(k string)      { s[k] = Void{} }
func (s StringSet) Remove(k string)   { delete(s, k) }
func (s StringSet) Has(k string) bool { _, ok := s[k]; return ok }
func (s StringSet) Len() int          { return len(s) }
