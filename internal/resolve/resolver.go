// Package resolve define o contrato entre extração (M1) e resolução de chamadas
// (M2 Passos 6-8). A implementação concreta do Resolver (syntactic, baseada em
// tree-sitter) vive em syntactic.go a partir do Passo 6.
package resolve

import (
	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// MethodHandle identifica unicamente um método no projeto.
// TypeFQCN é o FQCN da classe que contém o método (ex.: "com.foo.UserServiceImpl").
// Method é o nome simples do método (ex.: "create") e Signature identifica
// overloads (ex.: "(String,int)").
type MethodHandle struct {
	TypeFQCN  string
	Method    string
	Signature string
}

// Resolution é o resultado de resolver um CallSite.
// Targets pode ter:
//   - 0 elementos: unresolved. Note explica por quê.
//   - 1 elemento: concreto.
//   - N elementos: polimórfico (interface com múltiplas impls — M3).
//
// Note é string livre com dica humana. Exemplos: "external lib",
// "reflection", "interface sem impl", "complex receiver",
// "método não encontrado".
type Resolution struct {
	Targets []MethodHandle
	Note    string
}

// MethodContext é o que o resolver precisa saber sobre o método que faz a
// chamada (o "caller"). Inclui o tipo que o contém, os parâmetros, as
// variáveis locais (com ranges para scoped lookup), e o arquivo source.
type MethodContext struct {
	EnclosingType *java.TypeDecl      // classe/interface onde o caller está declarado
	Params        []java.Param        // parâmetros do método caller
	LocalVars     []java.LocalVarDecl // locais com ScopeStart/ScopeEnd/DeclStart
	File          string              // path do arquivo (pra warnings file:line)
}

// Resolver é a interface que transforma CallSite em Resolution.
// Implementação sintática (baseada em tree-sitter + heurísticas sobre a AST)
// vem em Passo 6+ (syntactic.go).
//
// Para resolver, o caller passa o CallSite extraído pelo pacote java e o
// MethodContext do método caller. O Resolver devolve Resolution dizendo
// qual método (ou métodos, em caso de polimorfismo) aquele call site aponta.
type Resolver interface {
	Resolve(call java.CallSite, ctx MethodContext) Resolution
}
