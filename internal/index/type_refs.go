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
	if strings.Contains(base, ".") {
		return resolvedTypeRef(ref, []string{base})
	}

	if unit != nil {
		explicit := make([]string, 0)
		for _, importDecl := range unit.Imports {
			if importDecl.Static || importDecl.Wildcard || simpleName(importDecl.Target) != base {
				continue
			}
			explicit = append(explicit, importDecl.Target)
		}
		if len(explicit) > 0 {
			return resolvedTypeRef(ref, explicit)
		}

		samePackage := base
		if unit.Package != "" {
			samePackage = unit.Package + "." + base
		}
		if _, ok := t.TypeByFQCN(samePackage); ok {
			return resolvedTypeRef(ref, []string{samePackage})
		}

		wildcards := make([]string, 0)
		for _, importDecl := range unit.Imports {
			if importDecl.Static || !importDecl.Wildcard {
				continue
			}
			candidate := importDecl.Target + "." + base
			if _, ok := t.TypeByFQCN(candidate); ok {
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
	return resolvedTypeRef(ref, nil)
}

func (t *Table) canonicalizeUnit(unit *java.CompilationUnit) {
	for _, typ := range unit.Types {
		typ.SuperClass = t.ResolveTypeRef(typ.SuperClass, unit).Ref
		for i := range typ.Interfaces {
			typ.Interfaces[i] = t.ResolveTypeRef(typ.Interfaces[i], unit).Ref
		}
		for i := range typ.Fields {
			typ.Fields[i].Type = t.ResolveTypeRef(typ.Fields[i].Type, unit).Ref
		}
		for i := range typ.Methods {
			method := &typ.Methods[i]
			method.ReturnType = t.ResolveTypeRef(method.ReturnType, unit).Ref
			for j := range method.Params {
				method.Params[j].Type = t.ResolveTypeRef(method.Params[j].Type, unit).Ref
			}
			for name, ref := range method.LocalVars {
				method.LocalVars[name] = t.ResolveTypeRef(ref, unit).Ref
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
