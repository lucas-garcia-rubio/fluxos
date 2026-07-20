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

// ExecutionKey identifies a declared method body under the runtime receiver
// assumed for that execution. Static methods use their declaring type as the
// runtime type; inherited instance bodies retain the concrete receiver type.
type ExecutionKey struct {
	Method          MethodHandle
	RuntimeTypeFQCN string
}

// ResolutionKind classifica cada saída do resolver. Os sete valores cobrem os
// caminhos previstos pela política M3 de dispatch polimórfico (Passo 13).
type ResolutionKind int

const (
	// ResolutionConcrete é um método concreto alcançável por chamada direta.
	// Implica Descend=true.
	ResolutionConcrete ResolutionKind = iota
	// ResolutionExternal é reservado a resolvers que conhecem um handle externo
	// completo. O resolver sintatico sem classpath usa Unresolved em vez de
	// inventar owner/signature de biblioteca. Não é terminal e não descend.
	ResolutionExternal
	// ResolutionUnresolved cobre receivers ou métodos que não foram encontrados
	// no contexto atual. Vira terminal [unresolved] no grafo.
	ResolutionUnresolved
	// ResolutionNoImplementation cobre receivers interface/abstract sem
	// implementations conhecidas e sem default method aplicável. Terminal
	// [no implementation].
	ResolutionNoImplementation
	// ResolutionAmbiguousType cobre receivers cujo simple-name lookup produz
	// múltiplos FQCNs candidatos. Terminal [ambiguous type].
	ResolutionAmbiguousType
	// ResolutionAmbiguousOverload cobre overload que não reduz a um candidato
	// por arity/signature. Terminal [ambiguous overload].
	ResolutionAmbiguousOverload
	// ResolutionAmbiguousImplementation cobre receivers interface/abstract com
	// duas ou mais implementations; M3 não faz fan-out. Terminal [ambiguous: N
	// implementations] com a lista ordenada em Candidates.
	ResolutionAmbiguousImplementation
)

// ResolvedTarget é uma saída individual do resolver. Kind determina se a
// traversal desce no body; Note é informativo e nunca é parseado pelo renderer.
type ResolvedTarget struct {
	Key  ExecutionKey
	Kind ResolutionKind
	Note string
}

// ConcreteTarget wraps a complete execution identity as a concrete target.
func ConcreteTarget(key ExecutionKey) ResolvedTarget {
	return ResolvedTarget{Key: key, Kind: ResolutionConcrete}
}

// Resolution é o resultado de resolver um CallSite.
// Targets pode ter:
//   - 0 elementos: nenhum terminal criado; Note descreve o motivo (casos de
//     "unsupported feature" que não viram terminal).
//   - 1 elemento: concreto, terminal ou external.
//   - N elementos: futuro (M4 com --all-impls); hoje o resolver produz N>1 só
//     em caminhos excepcionais e o Walk respeita cada target individualmente.
//
// Note continua existindo para os casos em que nenhum target é produzido.
//
// Truncations carrega omissions produzidas pela DispatchPolicy (ex: impls
// excedentes ao MaxImpls). O Build copia essas entradas para o BuildResult.
type Resolution struct {
	Targets      []ResolvedTarget
	DispatchSite *DispatchSite
	Note         string
	Truncations  []PolicyTruncation
}

// PolicyTruncation é a omissão reportada por uma DispatchPolicy, expressa em
// tipos do package resolve para evitar import cycle com graph. O Build
// converte cada entrada em graph.Truncation com Kind=maxImpls.
type PolicyTruncation struct {
	Caller  ExecutionKey
	Call    java.CallSite
	Omitted int
	Note    string
}

// MethodContext é o que o resolver precisa saber sobre o método que faz a
// chamada (o "caller"). Inclui o tipo que o contém, os parâmetros, as
// variáveis locais (com ranges para scoped lookup), e o arquivo source.
type MethodContext struct {
	EnclosingType *java.TypeDecl      // classe/interface onde o caller está declarado
	Execution     ExecutionKey        // body atual + receiver runtime; zero falls back lexically
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
