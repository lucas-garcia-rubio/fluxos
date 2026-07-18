// Package graph define o multigrafo direcionado usado pra representar o
// grafo de chamadas. Mantém Nodes (mapa de MethodHandle → *Node) e Edges
// (lista de arestas, cada uma com From, To, e a CallSite que a gerou).
//
// State em Node é usado pelo DFS com cycle detection (Passo 5):
//   - StateWhite (0): não visitado.
//   - StateGray  (1): visiting — está na pilha de DFS atual.
//   - StateBlack  (2): done — totalmente processado.
//
// Ciclo é detectado quando DFS encontra nó gray (back-edge).
package graph

import (
	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

const (
	StateWhite = iota // 0 — não visitado
	StateGray         // 1 — visiting (na pilha de DFS)
	StateBlack        // 2 — done
)

// NodeKind classifica Nodes para o renderer. NodeMethod é o default; NodeExternal
// cobre handles externos (não descendem, não são terminais); os cinco
// NodeTerminal* carregam Note/Candidates e são renderizados com sufixo de label.
type NodeKind int

const (
	NodeMethod NodeKind = iota
	NodeExternal
	NodeTerminalUnresolved
	NodeTerminalNoImplementation
	NodeTerminalAmbiguousType
	NodeTerminalAmbiguousOverload
	NodeTerminalAmbiguousImplementation
)

// Node representa um método no grafo. State controla o DFS. Kind/Note/Candidates
// são populados por MarkTerminal e MarkExternal; Nodes concretos permanecem
// com Kind=NodeMethod (zero value).
type Node struct {
	Handle     resolve.MethodHandle
	State      int
	Kind       NodeKind
	Note       string
	Candidates []string
}

// Edge é uma aresta dirigida no grafo de chamadas.
// Call é a CallSite que gerou esta aresta — útil pra reportar ao usuário
// qual chamada (com file:line) produziu cada conexão.
type Edge struct {
	From  resolve.MethodHandle
	To    resolve.MethodHandle
	Call  java.CallSite
	Cycle bool
}

// Graph é o multigrafo direcionado de chamadas.
// Nodes é mapa chaveado por MethodHandle (struct de strings é comparable).
// Edges permite múltiplas arestas entre o mesmo par (multi-chamadas de A pra B).
type Graph struct {
	Nodes map[resolve.MethodHandle]*Node
	Edges []Edge
}

// NewGraph inicializa Graph com Nodes como mapa vazio (mapa zero é nil;
// atribuir direto panic). Edges slice zero é nil mas pode ser usada com append.
func NewGraph() *Graph {
	return &Graph{
		Nodes: map[resolve.MethodHandle]*Node{},
	}
}

// GetOrCreate devolve o Node para handle. Cria com StateWhite se não existe.
// Sempre devolve Node não-nil.
func (g *Graph) GetOrCreate(handle resolve.MethodHandle) *Node {
	if n, ok := g.Nodes[handle]; ok {
		return n
	}
	n := &Node{Handle: handle, State: StateWhite}
	g.Nodes[handle] = n
	return n
}

// MarkGray marca handle como visiting (StateGray). Cria Node se faltava.
// Útil no início do Walk de um método.
func (g *Graph) MarkGray(handle resolve.MethodHandle) {
	g.GetOrCreate(handle).State = StateGray
}

// MarkBlack marca handle como done (StateBlack). Cria Node se faltava.
// Útil no fim do Walk, quando todos os descendentes foram processados.
func (g *Graph) MarkBlack(handle resolve.MethodHandle) {
	g.GetOrCreate(handle).State = StateBlack
}

// IsGray devolve true se handle existe e está em StateGray.
// Usado pra detectar ciclo (back-edge).
func (g *Graph) IsGray(handle resolve.MethodHandle) bool {
	n, ok := g.Nodes[handle]
	return ok && n.State == StateGray
}

// IsBlack devolve true se handle existe e está em StateBlack.
// Usado pra pular métodos já processados (não recurse).
func (g *Graph) IsBlack(handle resolve.MethodHandle) bool {
	n, ok := g.Nodes[handle]
	return ok && n.State == StateBlack
}

// AddEdge adiciona aresta dirigida de from → to com a CallSite que a gerou
// e informa se ela fecha um ciclo na travessia DFS.
// Garante que ambos os Nodes existem no grafo (cria com StateWhite se faltarem).
// Permite múltiplas arestas entre o mesmo par (A → B com 2 chamadas vira 2 Edges).
func (g *Graph) AddEdge(from, to resolve.MethodHandle, call java.CallSite, cycle bool) {
	g.GetOrCreate(from)
	g.GetOrCreate(to)
	g.Edges = append(g.Edges, Edge{From: from, To: to, Call: call, Cycle: cycle})
}

// MarkTerminal classifica handle como terminal de kind. Cria o Node se ainda
// não existia, copia candidates defensivamente e é idempotente: chamar duas
// vezes com os mesmos argumentos não acumula estado. Kind deve ser um dos
// NodeTerminal* (NodeMethod/NodeExternal seriam no-op aqui, mas a API confia
// que callers usem MarkExternal para o caso externo).
func (g *Graph) MarkTerminal(handle resolve.MethodHandle, kind NodeKind, note string, candidates []string) {
	n := g.GetOrCreate(handle)
	n.Kind = kind
	n.Note = note
	if len(candidates) > 0 {
		defensive := make([]string, len(candidates))
		copy(defensive, candidates)
		n.Candidates = defensive
	} else {
		n.Candidates = nil
	}
}

// MarkExternal classifica handle como NodeExternal (não terminal, não
// descendente). Não sobrescreve Nodes já marcados como terminal.
func (g *Graph) MarkExternal(handle resolve.MethodHandle) {
	n := g.GetOrCreate(handle)
	if n.Kind == NodeMethod {
		n.Kind = NodeExternal
	}
}
