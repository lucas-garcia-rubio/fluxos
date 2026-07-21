package resolve

import (
	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

// DispatchPolicy decide o que fazer quando uma chamada polimórfica pode
// despachar para múltiplas implementations concretas. A implementação default
// (TerminalPolicy) preserva o comportamento M3: marca como terminal
// ambiguousImplementation com a lista de candidates. A AllPolicy (Passo 7)
// faz fan-out: converte cada candidate em ConcreteTarget, aplicando MaxImpls.
//
// A policy recebe todos os candidates já resolvidos pela implementação
// concreta do Resolver; ela não re-resolve, apenas decide.
type DispatchPolicy interface {
	Apply(caller ExecutionKey, receiver *java.TypeDecl, call java.CallSite, candidates []ImplementationCandidate) PolicyDecision
}

// PolicyDecision é o resultado de uma policy. Targets é o fan-out a admitir;
// Terminal é um terminal opcional a exibir no lugar do fan-out (mutuamente
// exclusivo com Targets na prática, mas a estrutura permite o caso misto
// quando uma policy quiser exibir alguns terminals isolados junto com
// ConcreteTargets). Omitted indica quantas impls foram deixadas de fora por
// limite (MaxImpls); Build converte isso em Truncation maxImpls.
type PolicyDecision struct {
	Targets  []ResolvedTarget
	Terminal *ResolvedTarget
	Omitted  int
}

// TerminalPolicy reproduz o comportamento M3: quando há múltiplas impls, devolve
// um terminal ambiguousImplementation carregando a lista completa de candidates
// como Candidates no DispatchSite (tratado pelo Build como NodeKind terminal).
type TerminalPolicy struct{}

func (TerminalPolicy) Apply(caller ExecutionKey, receiver *java.TypeDecl, call java.CallSite, candidates []ImplementationCandidate) PolicyDecision {
	if receiver == nil {
		return PolicyDecision{}
	}
	terminal := TerminalTarget(
		ResolutionAmbiguousImplementation, receiver.FQCN, call.MethodName, "",
		call,
		"multiple concrete implementations of "+receiver.FQCN, nil,
	)
	return PolicyDecision{Terminal: &terminal}
}

// AllPolicy faz fan-out sobre as candidates concretas. Candidates cujo Kind
// não é Concrete viram terminal individual (ex: impl que não declarou o método
// invocado). MaxImpls > 0 limita quantas impls admitidas; o restante vira
// Omitted (Truncation maxImpls no BuildResult). MaxImpls == 0 significa
// unlimited.
type AllPolicy struct {
	MaxImpls int
}

func (p AllPolicy) Apply(_ ExecutionKey, receiver *java.TypeDecl, call java.CallSite, candidates []ImplementationCandidate) PolicyDecision {
	admitted := 0
	targets := make([]ResolvedTarget, 0, len(candidates))
	var terminal *ResolvedTarget
	omitted := 0
	for _, candidate := range candidates {
		if candidate.Kind == ResolutionConcrete && candidate.Target.Method != "" {
			if p.MaxImpls > 0 && admitted >= p.MaxImpls {
				omitted++
				continue
			}
			key := ExecutionKey{Method: candidate.Target, RuntimeTypeFQCN: candidate.ImplementationFQCN}
			targets = append(targets, ConcreteTarget(key))
			admitted++
			continue
		}
		// Candidate não-Concrete: vira terminal isolado (uma vez apenas).
		// Reproduz o comportamento de candidate com target não resolvido.
		if terminal == nil {
			t := TerminalTarget(
				candidate.Kind, candidate.ImplementationFQCN, call.MethodName, "",
				call, candidate.Note, nil,
			)
			terminal = &t
		}
	}
	_ = receiver
	return PolicyDecision{Targets: targets, Terminal: terminal, Omitted: omitted}
}

// FixedChoiceKind classifica a escolha declarativa para um receiver.
type FixedChoiceKind int

const (
	// FixedChoiceNone mantém o terminal ambíguo sem fazer fan-out.
	FixedChoiceNone FixedChoiceKind = iota
	// FixedChoiceAll admite as impls Concrete candidates daquele receiver,
	// respeitando MaxImpls quando configurado.
	FixedChoiceAll
	// FixedChoiceExplicit admite apenas as impls cujo FQCN está em Impls.
	FixedChoiceExplicit
)

// FixedChoice é a decisão declarativa para um receiver polimórfico.
type FixedChoice struct {
	Kind  FixedChoiceKind
	Impls []string // FQCNs quando Kind == FixedChoiceExplicit
}

// FixedPolicy aplica escolhas declarativas do usuário. Receivers mapeados
// seguem a Choice correspondente; receivers não mapeados caem no Fallback
// (default TerminalPolicy). Pré-validação (FQCNs inválidos, receivers não
// ambíguos) é responsabilidade do CLI antes do Build começar.
type FixedPolicy struct {
	Choices  map[string]FixedChoice
	Fallback DispatchPolicy
	MaxImpls int
}

func (p FixedPolicy) Apply(caller ExecutionKey, receiver *java.TypeDecl, call java.CallSite, candidates []ImplementationCandidate) PolicyDecision {
	if receiver == nil {
		return p.fallback(caller, receiver, call, candidates)
	}
	choice, ok := p.Choices[receiver.FQCN]
	if !ok {
		return p.fallback(caller, receiver, call, candidates)
	}
	switch choice.Kind {
	case FixedChoiceNone:
		return TerminalPolicy{}.Apply(caller, receiver, call, candidates)
	case FixedChoiceAll:
		return AllPolicy{MaxImpls: p.MaxImpls}.Apply(caller, receiver, call, candidates)
	case FixedChoiceExplicit:
		want := make(map[string]bool, len(choice.Impls))
		for _, fqcn := range choice.Impls {
			want[fqcn] = true
		}
		targets := make([]ResolvedTarget, 0, len(choice.Impls))
		var terminal *ResolvedTarget
		for _, candidate := range candidates {
			if candidate.Kind == ResolutionConcrete && candidate.Target.Method != "" && want[candidate.ImplementationFQCN] {
				key := ExecutionKey{Method: candidate.Target, RuntimeTypeFQCN: candidate.ImplementationFQCN}
				targets = append(targets, ConcreteTarget(key))
				continue
			}
			if terminal == nil && (candidate.Kind != ResolutionConcrete || candidate.Target.Method == "") {
				t := TerminalTarget(
					candidate.Kind, candidate.ImplementationFQCN, call.MethodName, "",
					call, candidate.Note, nil,
				)
				terminal = &t
			}
		}
		return PolicyDecision{Targets: targets, Terminal: terminal}
	}
	return p.fallback(caller, receiver, call, candidates)
}

func (p FixedPolicy) fallback(caller ExecutionKey, receiver *java.TypeDecl, call java.CallSite, candidates []ImplementationCandidate) PolicyDecision {
	if p.Fallback == nil {
		return TerminalPolicy{}.Apply(caller, receiver, call, candidates)
	}
	return p.Fallback.Apply(caller, receiver, call, candidates)
}
