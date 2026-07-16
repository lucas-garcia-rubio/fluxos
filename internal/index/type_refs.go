package index

import (
	"sort"
	"strings"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

var knownJavaLangTypes = map[string]struct{}{
	"Boolean": {}, "Byte": {}, "Character": {}, "Class": {}, "Double": {},
	"Enum": {}, "Float": {}, "Integer": {}, "Long": {}, "Math": {},
	"Number": {}, "Object": {}, "Short": {}, "String": {}, "StringBuilder": {},
	"System": {}, "Throwable": {}, "Void": {},
}

// TypeRefResolution keeps sorted candidates so ambiguity is never resolved by order.
type TypeRefResolution struct {
	Ref        java.TypeRef
	Candidates []string
}

// ResolveTypeRef applies Java's type-import precedence within a compilation unit.
func (t *Table) ResolveTypeRef(ref java.TypeRef, unit *java.CompilationUnit) TypeRefResolution {
	return t.ResolveTypeRefInType(ref, unit, "")
}

// ResolveTypeRefInType also considers the lexical chain of a declaring type.
func (t *Table) ResolveTypeRefInType(ref java.TypeRef, unit *java.CompilationUnit, enclosingFQCN string) TypeRefResolution {
	if ref.Raw == "" || ref.Primitive {
		ref.Unresolved = false
		return TypeRefResolution{Ref: ref, Candidates: []string{}}
	}
	if ref.FQCN != "" && !ref.Unresolved {
		return TypeRefResolution{Ref: ref, Candidates: []string{ref.FQCN}}
	}
	if ref.BaseName() == "" {
		return TypeRefResolution{Ref: ref, Candidates: []string{}}
	}

	base := ref.BaseName()
	if _, ok := t.TypeByFQCN(base); ok {
		return resolvedTypeRef(ref, []string{base})
	}

	if unit != nil {
		for current := enclosingFQCN; current != ""; {
			typ, ok := t.TypeByFQCN(current)
			if !ok {
				break
			}
			if !strings.Contains(base, ".") && typ.Name == base {
				return resolvedTypeRef(ref, []string{typ.FQCN})
			}
			if candidate := current + "." + base; t.hasType(candidate) {
				return resolvedTypeRef(ref, []string{candidate})
			}
			if inherited := t.inheritedMemberTypeCandidates(current, base); len(inherited) > 0 {
				return resolvedTypeRef(ref, inherited)
			}
			current = typ.EnclosingFQCN
		}

		explicit := make([]string, 0)
		for _, importDecl := range unit.Imports {
			if importDecl.Static || importDecl.Wildcard {
				continue
			}
			if simpleName(importDecl.Target) == base {
				explicit = append(explicit, importDecl.Target)
				continue
			}
			first, rest := splitFirst(base)
			if rest != "" && simpleName(importDecl.Target) == first {
				explicit = append(explicit, importDecl.Target+"."+rest)
			}
		}
		if len(explicit) > 0 {
			return resolvedTypeRef(ref, explicit)
		}

		samePackage := base
		if unit.Package != "" {
			samePackage = unit.Package + "." + base
		}
		if t.hasType(samePackage) {
			return resolvedTypeRef(ref, []string{samePackage})
		}

		wildcards := make([]string, 0)
		for _, importDecl := range unit.Imports {
			if importDecl.Static || !importDecl.Wildcard {
				continue
			}
			candidate := importDecl.Target + "." + base
			first, _ := splitFirst(base)
			topLevel := importDecl.Target + "." + first
			if top, ok := t.TypeByFQCN(topLevel); ok && top.EnclosingFQCN == "" && t.hasType(candidate) {
				wildcards = append(wildcards, candidate)
			}
		}
		if len(wildcards) > 0 {
			return resolvedTypeRef(ref, wildcards)
		}
	}

	if _, ok := knownJavaLangTypes[base]; ok {
		return resolvedTypeRef(ref, []string{"java.lang." + base})
	}
	if strings.Contains(base, ".") {
		return resolvedTypeRef(ref, []string{base})
	}
	return resolvedTypeRef(ref, nil)
}

func (t *Table) inheritedMemberTypeCandidates(typeFQCN, name string) []string {
	for _, superclass := range t.SuperclassChain(typeFQCN) {
		if candidate := superclass.FQCN + "." + name; t.hasType(candidate) {
			return []string{candidate}
		}
	}
	candidates := make([]string, 0)
	for _, iface := range t.InterfaceClosure(typeFQCN) {
		if candidate := iface.FQCN + "." + name; t.hasType(candidate) {
			candidates = append(candidates, candidate)
		}
	}
	return uniqueSorted(candidates)
}

func (t *Table) hasType(fqcn string) bool {
	_, ok := t.TypeByFQCN(fqcn)
	return ok
}

func splitFirst(name string) (string, string) {
	if dot := strings.IndexByte(name, '.'); dot >= 0 {
		return name[:dot], name[dot+1:]
	}
	return name, ""
}

func (t *Table) canonicalizeUnit(unit *java.CompilationUnit) {
	for _, typ := range unit.Types {
		typ.SuperClass = t.ResolveTypeRefInType(typ.SuperClass, unit, typ.FQCN).Ref
		for i := range typ.Interfaces {
			typ.Interfaces[i] = t.ResolveTypeRefInType(typ.Interfaces[i], unit, typ.FQCN).Ref
		}
		for i := range typ.RecordComponents {
			typ.RecordComponents[i].Type = t.ResolveTypeRefInType(typ.RecordComponents[i].Type, unit, typ.FQCN).Ref
		}
		for i := range typ.Fields {
			typ.Fields[i].Type = t.ResolveTypeRefInType(typ.Fields[i].Type, unit, typ.FQCN).Ref
		}
		for i := range typ.Methods {
			method := &typ.Methods[i]
			method.ReturnType = t.ResolveTypeRefInType(method.ReturnType, unit, typ.FQCN).Ref
			for j := range method.Params {
				method.Params[j].Type = t.ResolveTypeRefInType(method.Params[j].Type, unit, typ.FQCN).Ref
			}
			for j := range method.LocalVars {
				method.LocalVars[j].Type = t.ResolveTypeRefInType(method.LocalVars[j].Type, unit, typ.FQCN).Ref
			}
			for j := range method.Calls {
				if method.Calls[j].TargetType == nil {
					continue
				}
				resolved := t.ResolveTypeRefInType(*method.Calls[j].TargetType, unit, typ.FQCN).Ref
				*method.Calls[j].TargetType = resolved
			}
			java.RebuildSignature(method)
		}
	}
}

func resolvedTypeRef(ref java.TypeRef, candidates []string) TypeRefResolution {
	candidates = uniqueSorted(candidates)
	ref.FQCN = ""
	ref.Unresolved = true
	if len(candidates) == 1 {
		ref.FQCN = candidates[0]
		ref.Unresolved = false
	}
	return TypeRefResolution{Ref: ref, Candidates: candidates}
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func simpleName(fqcn string) string {
	if dot := strings.LastIndexByte(fqcn, '.'); dot >= 0 {
		return fqcn[dot+1:]
	}
	return fqcn
}
