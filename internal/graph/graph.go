// Package graph define o multigrafo direcionado usado pra representar o
// grafo de chamadas. Mantém Nodes (mapa de ExecutionKey → *Node) e Edges
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
	Key        resolve.ExecutionKey
	State      int
	Kind       NodeKind
	Note       string
	Candidates []string
}

// Edge é uma aresta dirigida no grafo de chamadas.
// Call é a CallSite que gerou esta aresta — útil pra reportar ao usuário
// qual chamada (com file:line) produziu cada conexão.
type Edge struct {
	From         resolve.ExecutionKey
	To           resolve.ExecutionKey
	Call         java.CallSite
	DispatchSite *resolve.DispatchSite
	Cycle        bool
}

// Graph é o multigrafo direcionado de chamadas.
// Nodes é mapa chaveado por ExecutionKey (struct de strings é comparable).
// Edges permite múltiplas arestas entre o mesmo par (multi-chamadas de A pra B).
type Graph struct {
	Nodes map[resolve.ExecutionKey]*Node
	Edges []Edge
}

// NewGraph inicializa Graph com Nodes como mapa vazio (mapa zero é nil;
// atribuir direto panic). Edges slice zero é nil mas pode ser usada com append.
func NewGraph() *Graph {
	return &Graph{
		Nodes: map[resolve.ExecutionKey]*Node{},
	}
}

// GetOrCreate devolve o Node para key. Cria com StateWhite se não existe.
// Sempre devolve Node não-nil.
func (g *Graph) GetOrCreate(key resolve.ExecutionKey) *Node {
	if n, ok := g.Nodes[key]; ok {
		return n
	}
	n := &Node{Key: key, State: StateWhite}
	g.Nodes[key] = n
	return n
}

// MarkGray marca key como visiting (StateGray). Cria Node se faltava.
// Útil no início do Walk de um método.
func (g *Graph) MarkGray(key resolve.ExecutionKey) {
	g.GetOrCreate(key).State = StateGray
}

// MarkBlack marca key como done (StateBlack). Cria Node se faltava.
// Útil no fim do Walk, quando todos os descendentes foram processados.
func (g *Graph) MarkBlack(key resolve.ExecutionKey) {
	g.GetOrCreate(key).State = StateBlack
}

// IsGray devolve true se key existe e está em StateGray.
// Usado pra detectar ciclo (back-edge).
func (g *Graph) IsGray(key resolve.ExecutionKey) bool {
	n, ok := g.Nodes[key]
	return ok && n.State == StateGray
}

// IsBlack devolve true se key existe e está em StateBlack.
// Usado pra pular métodos já processados (não recurse).
func (g *Graph) IsBlack(key resolve.ExecutionKey) bool {
	n, ok := g.Nodes[key]
	return ok && n.State == StateBlack
}

// AddEdge adiciona aresta dirigida de from → to com a CallSite que a gerou
// e informa se ela fecha um ciclo na travessia DFS.
// Garante que ambos os Nodes existem no grafo (cria com StateWhite se faltarem).
// Permite múltiplas arestas entre o mesmo par (A → B com 2 chamadas vira 2 Edges).
func (g *Graph) AddEdge(from, to resolve.ExecutionKey, call java.CallSite, dispatchSite *resolve.DispatchSite, cycle bool) {
	g.GetOrCreate(from)
	g.GetOrCreate(to)
	g.Edges = append(g.Edges, Edge{
		From:         from,
		To:           to,
		Call:         cloneCallSite(call),
		DispatchSite: resolve.CloneDispatchSite(dispatchSite),
		Cycle:        cycle,
	})
}

// MarkTerminal classifica key como terminal de kind. Cria o Node se ainda
// não existia, copia candidates defensivamente e é idempotente: chamar duas
// vezes com os mesmos argumentos não acumula estado. Kind deve ser um dos
// NodeTerminal* (NodeMethod/NodeExternal seriam no-op aqui, mas a API confia
// que callers usem MarkExternal para o caso externo).
func (g *Graph) MarkTerminal(key resolve.ExecutionKey, kind NodeKind, note string, candidates []string) {
	n := g.GetOrCreate(key)
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

// MarkExternal classifica key como NodeExternal (não terminal, não
// descendente). Não sobrescreve Nodes já marcados como terminal.
func (g *Graph) MarkExternal(key resolve.ExecutionKey) {
	n := g.GetOrCreate(key)
	if n.Kind == NodeMethod {
		n.Kind = NodeExternal
	}
}

func cloneCallSite(call java.CallSite) java.CallSite {
	clone := call
	clone.Args = append([]string(nil), call.Args...)
	if call.TargetType != nil {
		targetType := *call.TargetType
		clone.TargetType = &targetType
	}
	return clone
}
