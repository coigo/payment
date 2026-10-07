package mcp

type Tes interface {
	Teste(i int)int
} 


type TesteS struct {}

func (t *TesteS) Teste (i int) int {
	return i
}

var _ Tes = (*TesteS)(nil)