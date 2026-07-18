package resolve

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
	"github.com/lucas-garcia-rubio/fluxos/internal/index"
)

var (
	identifierArgument = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
	integerArgument    = regexp.MustCompile(`^[+-]?(?:0[xX][0-9A-Fa-f_]+|0[bB][01_]+|0[0-7_]*|[0-9][0-9_]*)(?:[lL])?$`)
	floatingArgument   = regexp.MustCompile(`^[+-]?(?:(?:[0-9][0-9_]*)?\.[0-9][0-9_]*|[0-9][0-9_]*[eE][+-]?[0-9][0-9_]*)(?:[eE][+-]?[0-9][0-9_]*)?[fFdD]?$`)
)

type inferredArgument struct {
	ref    java.TypeRef
	known  bool
	isNull bool
}

func (r *SyntacticResolver) selectMethodCandidates(candidates []index.MethodResolution, call java.CallSite, receiver *java.TypeDecl, ctx MethodContext) candidateSelection {
	applicable := make([]index.MethodResolution, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.DeclaringType != nil && candidate.Method != nil && arityCompatible(*candidate.Method, call.ArgCount) {
			applicable = append(applicable, candidate)
		}
	}
	if len(applicable) == 0 {
		return candidateSelection{}
	}

	if len(call.Args) > 0 {
		arguments := make([]inferredArgument, len(call.Args))
		for i, source := range call.Args {
			arguments[i] = r.inferArgumentType(source, call, ctx)
		}
		filtered := applicable[:0]
		for _, candidate := range applicable {
			if r.argumentsCompatible(*candidate.Method, arguments) {
				filtered = append(filtered, candidate)
			}
		}
		applicable = filtered
	}
	if len(applicable) == 0 {
		return candidateSelection{}
	}

	subject := "<unknown>"
	if receiver != nil {
		subject = receiver.FQCN
	} else if len(candidates) > 0 && candidates[0].DeclaringType != nil {
		subject = candidates[0].DeclaringType.FQCN
	}
	if len(applicable) == 1 {
		candidate := applicable[0]
		handle := MethodHandle{
			TypeFQCN:  candidate.DeclaringType.FQCN,
			Method:    candidate.Method.Name,
			Signature: candidate.Method.Signature,
		}
		return candidateSelection{
			Found:      true,
			Candidate:  &candidate,
			Resolution: Resolution{Targets: []ResolvedTarget{ConcreteTarget(ExecutionKey{Method: handle, RuntimeTypeFQCN: candidate.DeclaringType.FQCN})}},
		}
	}

	descriptions := make([]string, len(applicable))
	sameOwner := true
	owner := applicable[0].DeclaringType.FQCN
	for i, candidate := range applicable {
		if candidate.DeclaringType.FQCN != owner {
			sameOwner = false
		}
		descriptions[i] = candidate.DeclaringType.FQCN + "." + candidate.Method.Name + candidate.Method.Signature
	}
	if sameOwner {
		for i, candidate := range applicable {
			descriptions[i] = candidate.Method.Signature
		}
	}
	sort.Strings(descriptions)
	if receiver == nil {
		subject = owner
	}
	note := fmt.Sprintf("ambiguous overload %q on %s: %s", call.MethodName, subject, strings.Join(descriptions, ", "))
	return candidateSelection{
		Found: true,
		Resolution: Resolution{Targets: []ResolvedTarget{TerminalTarget(
			ResolutionAmbiguousOverload, subject, call.MethodName, "", call, note, nil,
		)}},
	}
}

func unresolvedSelection(owner string, call java.CallSite, note string) Resolution {
	return Resolution{Targets: []ResolvedTarget{TerminalTarget(
		ResolutionUnresolved, owner, call.MethodName, "", call, note, nil,
	)}}
}

func (r *SyntacticResolver) inferArgumentType(source string, call java.CallSite, ctx MethodContext) inferredArgument {
	source = strings.TrimSpace(source)
	if source == "" {
		return inferredArgument{}
	}
	if source == "null" {
		return inferredArgument{known: true, isNull: true}
	}
	if source == "true" || source == "false" {
		return knownArgument(java.NewTypeRef("boolean", false))
	}
	if isWholeQuotedLiteral(source, '"') {
		return r.knownArgumentRef(java.NewTypeRef("java.lang.String", false), ctx)
	}
	if isWholeQuotedLiteral(source, '\'') {
		return knownArgument(java.NewTypeRef("char", false))
	}
	if integerArgument.MatchString(source) {
		typeName := "int"
		if strings.HasSuffix(source, "l") || strings.HasSuffix(source, "L") {
			typeName = "long"
		}
		return knownArgument(java.NewTypeRef(typeName, false))
	}
	if floatingArgument.MatchString(source) {
		typeName := "double"
		if strings.HasSuffix(source, "f") || strings.HasSuffix(source, "F") {
			typeName = "float"
		}
		return knownArgument(java.NewTypeRef(typeName, false))
	}
	if typeSource, ok := wholeObjectCreationType(source); ok {
		return r.knownArgumentRef(java.NewTypeRef(typeSource, false), ctx)
	}
	if source == "this" && ctx.EnclosingType != nil {
		ref := java.NewTypeRef(ctx.EnclosingType.FQCN, false)
		ref.FQCN = ctx.EnclosingType.FQCN
		ref.Unresolved = false
		return knownArgument(ref)
	}
	if strings.HasPrefix(source, "(") {
		if close := strings.IndexByte(source, ')'); close > 1 {
			candidate := strings.TrimSpace(source[1:close])
			operand := strings.TrimSpace(source[close+1:])
			if isWholeCastOperand(operand) && (strings.ContainsAny(candidate, ".[]<>") || identifierArgument.MatchString(candidate)) {
				if inferred := r.knownArgumentRef(java.NewTypeRef(candidate, false), ctx); inferred.known {
					return inferred
				}
			}
		}
	}
	if identifierArgument.MatchString(source) {
		if local, ok := findLocalVarAt(ctx.LocalVars, source, call.StartByte); ok {
			return r.knownArgumentRef(local.Type, ctx)
		}
		if param := findParam(ctx.Params, source); param != nil {
			return r.knownArgumentRef(param.Type, ctx)
		}
		if ctx.EnclosingType != nil {
			if field, ok := r.effectiveField(ctx.EnclosingType, source); ok {
				fieldCtx := ctx
				fieldCtx.EnclosingType = field.DeclaringType
				fieldCtx.File = field.DeclaringType.File
				return r.knownArgumentRef(field.Field.Type, fieldCtx)
			}
		}
	}
	return inferredArgument{}
}

func isWholeQuotedLiteral(source string, quote byte) bool {
	if len(source) < 2 || source[0] != quote {
		return false
	}
	escaped := false
	for i := 1; i < len(source); i++ {
		switch {
		case escaped:
			escaped = false
		case source[i] == '\\':
			escaped = true
		case source[i] == quote:
			return i == len(source)-1
		}
	}
	return false
}

func wholeObjectCreationType(source string) (string, bool) {
	if !strings.HasPrefix(source, "new ") {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(source, "new "))
	open := strings.IndexByte(rest, '(')
	if open <= 0 {
		return "", false
	}
	depth := 0
	quote := byte(0)
	escaped := false
	for i := open; i < len(rest); i++ {
		ch := rest[i]
		if quote != 0 {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == quote:
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				if strings.TrimSpace(rest[i+1:]) != "" {
					return "", false
				}
				return strings.TrimSpace(rest[:open]), true
			}
		}
	}
	return "", false
}

func isWholeCastOperand(source string) bool {
	if source == "" {
		return false
	}
	if identifierArgument.MatchString(source) || source == "this" || source == "null" || source == "true" || source == "false" {
		return true
	}
	if isWholeQuotedLiteral(source, '"') || isWholeQuotedLiteral(source, '\'') || integerArgument.MatchString(source) || floatingArgument.MatchString(source) {
		return true
	}
	_, ok := wholeObjectCreationType(source)
	return ok
}

func knownArgument(ref java.TypeRef) inferredArgument {
	return inferredArgument{ref: ref, known: true}
}

func (r *SyntacticResolver) knownArgumentRef(ref java.TypeRef, ctx MethodContext) inferredArgument {
	if ref.Primitive {
		return knownArgument(ref)
	}
	if r.Index == nil {
		return inferredArgument{}
	}
	resolution := r.Index.ResolveTypeRefInType(ref, r.unitForContext(ctx), enclosingFQCN(ctx))
	if len(resolution.Candidates) != 1 || resolution.Ref.FQCN == "" {
		return inferredArgument{}
	}
	return knownArgument(resolution.Ref)
}

func enclosingFQCN(ctx MethodContext) string {
	if ctx.EnclosingType == nil {
		return ""
	}
	return ctx.EnclosingType.FQCN
}

func (r *SyntacticResolver) argumentsCompatible(method java.MethodDecl, arguments []inferredArgument) bool {
	for i, argument := range arguments {
		param, ok := parameterForArgument(method, i)
		if !ok {
			return false
		}
		if r.argumentCompatible(argument, param) {
			continue
		}
		if i == len(method.Params)-1 && method.Params[i].Variadic && r.argumentCompatible(argument, method.Params[i].Type) {
			continue
		}
		return false
	}
	return true
}

func parameterForArgument(method java.MethodDecl, argumentIndex int) (java.TypeRef, bool) {
	if argumentIndex < len(method.Params) {
		param := method.Params[argumentIndex]
		if !param.Variadic || argumentIndex < len(method.Params)-1 {
			return param.Type, true
		}
		element := param.Type
		if element.ArrayDepth > 0 {
			element.ArrayDepth--
		}
		return element, true
	}
	if len(method.Params) == 0 || !method.Params[len(method.Params)-1].Variadic {
		return java.TypeRef{}, false
	}
	element := method.Params[len(method.Params)-1].Type
	if element.ArrayDepth > 0 {
		element.ArrayDepth--
	}
	return element, true
}

func (r *SyntacticResolver) argumentCompatible(argument inferredArgument, param java.TypeRef) bool {
	if !argument.known {
		return true
	}
	if argument.isNull {
		return !param.Primitive
	}
	if argument.ref.Primitive || param.Primitive {
		return argument.ref.Primitive && param.Primitive && argument.ref.SignatureToken() == param.SignatureToken()
	}
	if argument.ref.ArrayDepth != param.ArrayDepth {
		return false
	}
	argumentType := argument.ref.FQCN
	if argumentType == "" {
		argumentType = argument.ref.BaseName()
	}
	paramType := param.FQCN
	if paramType == "" {
		paramType = param.BaseName()
	}
	if argumentType == "" || paramType == "" {
		return true
	}
	if argumentType == paramType || paramType == "java.lang.Object" {
		return true
	}
	if r.Index == nil {
		return false
	}
	for _, superclass := range r.Index.SuperclassChain(argumentType) {
		if superclass.FQCN == paramType {
			return true
		}
	}
	for _, iface := range r.Index.InterfaceClosure(argumentType) {
		if iface.FQCN == paramType {
			return true
		}
	}
	return false
}
