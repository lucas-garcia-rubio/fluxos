package graph

import (
	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/resolve"
)

// Walk faz DFS recursivo a partir de (enclosingType, method), construindo o
// grafo transitivo de chamadas em g. Usa gray/black coloring pra detectar
// ciclos e evitar reprocessamento.
//
// Algoritmo:
//  1. Se handle atual é gray → ciclo; retorna (aresta já foi adicionada pelo caller).
//  2. Se handle atual é black → já processado; retorna.
//  3. Marca gray.
//  4. Para cada CallSite em method.Calls, pergunta Resolver → Resolution.
//     Para cada target em Resolution.Targets, adiciona aresta e recursa
//     (só se o target existe no projeto; external targets ficam como aresta sem recursão).
//  5. Marca black.
//
// method.Calls já vem populado do Passo 2 (extractCalls). Walk não re-extrai.
func Walk(
	g *Graph,
	enclosingType *java.TypeDecl,
	method java.MethodDecl,
	types []*java.TypeDecl,
	resolver resolve.Resolver,
) {
	handle := resolve.MethodHandle{
		TypeFQCN: enclosingType.FQCN,
		Method:   method.Name,
	}

	// Cycle detection: gray = visitando agora na pilha atual.
	if g.IsGray(handle) {
		return
	}
	// Já processado em visita anterior.
	if g.IsBlack(handle) {
		return
	}

	g.MarkGray(handle)

	for _, call := range method.Calls {
		ctx := resolve.MethodContext{
			EnclosingType: enclosingType,
			LocalVars:     method.LocalVars,
			File:          enclosingType.File,
		}
		resolution := resolver.Resolve(call, ctx)
		for _, target := range resolution.Targets {
			g.AddEdge(handle, target, call)
			// Recursão só se target existe no projeto. External target
			// (biblioteca, reflexão) fica como aresta terminal.
			targetMethod, targetType, ok := findMethodByHandle(types, target)
			if ok {
				Walk(g, targetType, targetMethod, types, resolver)
			}
		}
	}

	g.MarkBlack(handle)
}

// findTypeByFQCN busca linear em types por FQCN. Devolve nil se não acha.
// Para projetos grandes, vale refatorar pra map[string]*java.TypeDecl.
func findTypeByFQCN(types []*java.TypeDecl, fqcn string) *java.TypeDecl {
	for _, t := range types {
		if t.FQCN == fqcn {
			return t
		}
	}
	return nil
}

// findMethodInType busca linear em class.Methods por nome. Devolve (MethodDecl, true)
// se acha; (zero, false) caso contrário.
func findMethodInType(class *java.TypeDecl, name string) (java.MethodDecl, bool) {
	for _, m := range class.Methods {
		if m.Name == name {
			return m, true
		}
	}
	return java.MethodDecl{}, false
}

// findMethodByHandle combina findTypeByFQCN + findMethodInType. Devolve
// (method, type, true) se ambos existirem; (zero, type-or-nil, false) caso contrário.
func findMethodByHandle(types []*java.TypeDecl, h resolve.MethodHandle) (java.MethodDecl, *java.TypeDecl, bool) {
	t := findTypeByFQCN(types, h.TypeFQCN)
	if t == nil {
		return java.MethodDecl{}, nil, false
	}
	m, ok := findMethodInType(t, h.Method)
	if !ok {
		return java.MethodDecl{}, t, false
	}
	return m, t, true
}
