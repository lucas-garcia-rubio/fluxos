package main

import (
	"fmt"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
)

// TargetSpec é a forma parseada de um spec de target CLI, antes de resolver
// contra o índice.
type TargetSpec struct {
	TypeName     string
	Method       string
	Signature    string
	HasSignature bool
}

// ParseTargetSpec separa um spec no formato `[FQCN.]TypeName.method[(signature)]`.
// É puro: não consulta o índice. Erros são determinísticos para que o CLI possa
// reportar o problema exato.
func ParseTargetSpec(spec string) (TargetSpec, error) {
	if spec == "" {
		return TargetSpec{}, fmt.Errorf("empty target spec")
	}

	parsed := TargetSpec{}
	remaining := spec

	if strings.HasSuffix(remaining, ")") {
		open := strings.LastIndexByte(remaining[:len(remaining)-1], '(')
		if open < 0 {
			return TargetSpec{}, fmt.Errorf("invalid target %q: ')' without matching '('", spec)
		}
		parsed.Signature = remaining[open+1 : len(remaining)-1]
		parsed.HasSignature = true
		remaining = remaining[:open]
		if err := validateSignature(parsed.Signature, spec); err != nil {
			return TargetSpec{}, err
		}
	} else if strings.ContainsAny(remaining, "()") {
		return TargetSpec{}, fmt.Errorf("invalid target %q: unexpected '(' or ')'", spec)
	}

	dot := strings.LastIndexByte(remaining, '.')
	if dot < 0 {
		return TargetSpec{}, fmt.Errorf("invalid target %q: expected TypeName.method or FQCN.method", spec)
	}
	parsed.TypeName = remaining[:dot]
	parsed.Method = remaining[dot+1:]

	if parsed.TypeName == "" {
		return TargetSpec{}, fmt.Errorf("invalid target %q: empty type name", spec)
	}
	if parsed.Method == "" {
		return TargetSpec{}, fmt.Errorf("invalid target %q: empty method name", spec)
	}
	if strings.ContainsAny(parsed.Method, ".()") {
		return TargetSpec{}, fmt.Errorf("invalid target %q: method name contains '.', '(' or ')'", spec)
	}
	return parsed, nil
}

// validateSignature confere formato da signature canonica do Passo 1: sem
// espaços, generics balanceados, sem parênteses aninhados, sem tokens vazios
// entre vírgulas.
func validateSignature(signature, originalSpec string) error {
	if signature == "" {
		return nil
	}
	depth := 0
	for _, r := range signature {
		switch r {
		case ' ', '\t':
			return fmt.Errorf("invalid signature in target %q: spaces are not allowed", originalSpec)
		case '<':
			depth++
		case '>':
			depth--
			if depth < 0 {
				return fmt.Errorf("invalid signature in target %q: '>' without matching '<'", originalSpec)
			}
		case '(', ')':
			return fmt.Errorf("invalid signature in target %q: nested parentheses are not allowed", originalSpec)
		}
	}
	if depth != 0 {
		return fmt.Errorf("invalid signature in target %q: unbalanced '<' and '>'", originalSpec)
	}
	for _, token := range strings.Split(signature, ",") {
		if token == "" {
			return fmt.Errorf("invalid signature in target %q: empty parameter type", originalSpec)
		}
	}
	return nil
}

// ResolveTarget aplica ParseTargetSpec e resolve contra o índice. Retorna
// (TypeDecl, MethodDecl) prontos para o walker. Erros são determinísticos:
// classe homônima lista FQCNs; overload sem signature lista signatures.
func ResolveTarget(table *index.Table, spec TargetSpec) (*java.TypeDecl, *java.MethodDecl, error) {
	typ, err := resolveTargetType(table, spec.TypeName)
	if err != nil {
		return nil, nil, err
	}
	declaringType, method, err := resolveTargetMethod(table, typ, spec)
	if err != nil {
		return nil, nil, err
	}
	return declaringType, method, nil
}

func resolveTargetType(table *index.Table, typeName string) (*java.TypeDecl, error) {
	if strings.Contains(typeName, ".") {
		typ, ok := table.TypeByFQCN(typeName)
		if !ok {
			return nil, fmt.Errorf("type %q not found", typeName)
		}
		return typ, nil
	}
	candidates := table.TypesBySimple(typeName)
	switch len(candidates) {
	case 0:
		return nil, fmt.Errorf("class %q not found", typeName)
	case 1:
		return candidates[0], nil
	default:
		fqcn := make([]string, len(candidates))
		for i, c := range candidates {
			fqcn[i] = c.FQCN
		}
		return nil, fmt.Errorf("ambiguous class name %q; candidates: %s", typeName, strings.Join(fqcn, ", "))
	}
}

func resolveTargetMethod(table *index.Table, typ *java.TypeDecl, spec TargetSpec) (*java.TypeDecl, *java.MethodDecl, error) {
	if spec.HasSignature {
		key := java.MethodKey{Name: spec.Method, Signature: formatSignature(spec.Signature)}
		candidates := table.EffectiveMethod(typ.FQCN, key)
		if len(candidates) == 0 {
			return nil, nil, fmt.Errorf("method %s%s not found in %s; available signatures: %s",
				spec.Method, formatSignature(spec.Signature), typ.FQCN,
				listSignatures(table, typ.FQCN, spec.Method))
		}
		if len(candidates) > 1 {
			return nil, nil, fmt.Errorf("ambiguous method %s%s in %s; candidates: %s",
				spec.Method, formatSignature(spec.Signature), typ.FQCN, listMethodCandidates(candidates))
		}
		return candidates[0].DeclaringType, candidates[0].Method, nil
	}
	candidates := table.EffectiveMethodCandidates(typ.FQCN, spec.Method)
	switch len(candidates) {
	case 0:
		return nil, nil, fmt.Errorf("method %q not found in %s", spec.Method, typ.FQCN)
	case 1:
		return candidates[0].DeclaringType, candidates[0].Method, nil
	default:
		if hasDuplicateSignatures(candidates) {
			return nil, nil, fmt.Errorf("ambiguous method %q in %s; candidates: %s",
				spec.Method, typ.FQCN, listMethodCandidates(candidates))
		}
		return nil, nil, fmt.Errorf("ambiguous method %q in %s; available signatures: %s",
			spec.Method, typ.FQCN, listSignatures(table, typ.FQCN, spec.Method))
	}
}

func hasDuplicateSignatures(candidates []index.MethodResolution) bool {
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, duplicate := seen[candidate.Method.Signature]; duplicate {
			return true
		}
		seen[candidate.Method.Signature] = struct{}{}
	}
	return false
}

func formatSignature(signature string) string {
	if signature == "" {
		return "()"
	}
	return "(" + signature + ")"
}

func listSignatures(table *index.Table, typeFQCN, methodName string) string {
	candidates := table.EffectiveMethodCandidates(typeFQCN, methodName)
	signatures := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		signatures = append(signatures, candidate.Method.Signature)
	}
	return strings.Join(signatures, ", ")
}

func listMethodCandidates(candidates []index.MethodResolution) string {
	values := make([]string, len(candidates))
	for i, candidate := range candidates {
		values[i] = candidate.DeclaringType.FQCN + "." + candidate.Method.Name + candidate.Method.Signature
	}
	return strings.Join(values, ", ")
}
