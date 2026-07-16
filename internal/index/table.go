package index

import (
	"fmt"
	"sort"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

type Table struct {
	UnitsByFile       map[string]*java.CompilationUnit
	TypesByFQCN       map[string]*java.TypeDecl
	TypesBySimpleName map[string][]*java.TypeDecl
	TypesByPackage    map[string][]*java.TypeDecl
	MethodsByType     map[string]map[java.MethodKey]*java.MethodDecl

	unitsByType map[string]*java.CompilationUnit
}

func Build(units []*java.CompilationUnit) (*Table, error) {
	table := &Table{
		UnitsByFile:       make(map[string]*java.CompilationUnit),
		TypesByFQCN:       make(map[string]*java.TypeDecl),
		TypesBySimpleName: make(map[string][]*java.TypeDecl),
		TypesByPackage:    make(map[string][]*java.TypeDecl),
		MethodsByType:     make(map[string]map[java.MethodKey]*java.MethodDecl),
		unitsByType:       make(map[string]*java.CompilationUnit),
	}

	orderedUnits := append([]*java.CompilationUnit(nil), units...)
	sort.Slice(orderedUnits, func(i, j int) bool {
		if orderedUnits[i] == nil {
			return orderedUnits[j] != nil
		}
		if orderedUnits[j] == nil {
			return false
		}
		if orderedUnits[i].File != orderedUnits[j].File {
			return orderedUnits[i].File < orderedUnits[j].File
		}
		return orderedUnits[i].SourceRoot < orderedUnits[j].SourceRoot
	})

	for _, unit := range orderedUnits {
		if unit == nil {
			return nil, fmt.Errorf("index: nil compilation unit")
		}
		if previous, exists := table.UnitsByFile[unit.File]; exists {
			roots := []string{previous.SourceRoot, unit.SourceRoot}
			sort.Strings(roots)
			return nil, fmt.Errorf("index: duplicate compilation unit file %q for source roots %q and %q", unit.File, roots[0], roots[1])
		}
		table.UnitsByFile[unit.File] = unit

		for _, typ := range unit.Types {
			if typ == nil {
				return nil, fmt.Errorf("index: nil type in %q", unit.File)
			}
			if previous, exists := table.TypesByFQCN[typ.FQCN]; exists {
				files := []string{typeFile(previous, table.unitsByType[previous.FQCN]), typeFile(typ, unit)}
				sort.Strings(files)
				return nil, fmt.Errorf("index: duplicate FQCN %q in %q and %q", typ.FQCN, files[0], files[1])
			}

			table.TypesByFQCN[typ.FQCN] = typ
			table.TypesBySimpleName[typ.Name] = append(table.TypesBySimpleName[typ.Name], typ)
			table.TypesByPackage[unit.Package] = append(table.TypesByPackage[unit.Package], typ)
			table.unitsByType[typ.FQCN] = unit

		}
	}

	for _, candidates := range table.TypesBySimpleName {
		sortTypes(candidates)
	}
	for _, candidates := range table.TypesByPackage {
		sortTypes(candidates)
	}
	for _, unit := range orderedUnits {
		table.canonicalizeUnit(unit)
	}
	for _, unit := range orderedUnits {
		for _, typ := range unit.Types {
			synthesizeImplicitConstructor(typ)
			methods := make(map[java.MethodKey]*java.MethodDecl, len(typ.Methods))
			for i := range typ.Methods {
				method := &typ.Methods[i]
				key := method.Key()
				if _, exists := methods[key]; exists {
					return nil, fmt.Errorf("index: duplicate method %s.%s%s", typ.FQCN, key.Name, key.Signature)
				}
				methods[key] = method
			}
			table.MethodsByType[typ.FQCN] = methods
		}
	}

	return table, nil
}

func (t *Table) UnitForType(fqcn string) *java.CompilationUnit {
	if t == nil {
		return nil
	}
	return t.unitsByType[fqcn]
}

func (t *Table) TypeByFQCN(fqcn string) (*java.TypeDecl, bool) {
	if t == nil {
		return nil, false
	}
	typ, ok := t.TypesByFQCN[fqcn]
	return typ, ok
}

func (t *Table) TypesBySimple(name string) []*java.TypeDecl {
	if t == nil {
		return []*java.TypeDecl{}
	}
	return cloneTypes(t.TypesBySimpleName[name])
}

func (t *Table) TypesInPackage(pkg string) []*java.TypeDecl {
	if t == nil {
		return []*java.TypeDecl{}
	}
	return cloneTypes(t.TypesByPackage[pkg])
}

func (t *Table) Method(typeFQCN string, key java.MethodKey) (*java.MethodDecl, bool) {
	if t == nil {
		return nil, false
	}
	method, ok := t.MethodsByType[typeFQCN][key]
	return method, ok
}

func (t *Table) MethodCandidates(typeFQCN, name string) []*java.MethodDecl {
	if t == nil {
		return []*java.MethodDecl{}
	}
	methods := t.MethodsByType[typeFQCN]
	candidates := make([]*java.MethodDecl, 0)
	for key, method := range methods {
		if key.Name == name {
			candidates = append(candidates, method)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Signature < candidates[j].Signature
	})
	return candidates
}

func typeFile(typ *java.TypeDecl, unit *java.CompilationUnit) string {
	if typ.File != "" {
		return typ.File
	}
	if unit != nil {
		return unit.File
	}
	return ""
}

func sortTypes(types []*java.TypeDecl) {
	sort.Slice(types, func(i, j int) bool {
		return types[i].FQCN < types[j].FQCN
	})
}

func cloneTypes(types []*java.TypeDecl) []*java.TypeDecl {
	cloned := make([]*java.TypeDecl, len(types))
	copy(cloned, types)
	return cloned
}
