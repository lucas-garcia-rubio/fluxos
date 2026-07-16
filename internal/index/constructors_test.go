package index

import (
	"reflect"
	"testing"

	"github.com/lucas-garcia-rubio/fluxos/internal/extract/java"
)

func TestBuildSynthesizesDefaultClassConstructors(t *testing.T) {
	tests := []struct {
		name      string
		modifiers []string
		want      []string
	}{
		{name: "ordinary package-private class", want: []string{}},
		{name: "public class", modifiers: []string{"public", "final"}, want: []string{"public"}},
		{name: "abstract protected class", modifiers: []string{"protected", "abstract"}, want: []string{"protected"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := &java.TypeDecl{Kind: java.TypeKindClass, Name: "Value", FQCN: "app.Value", Modifier: tt.modifiers}
			table := buildConstructorTable(t, typ)

			got, ok := table.Constructor(typ.FQCN, "()")
			if !ok {
				t.Fatal("synthetic default constructor not found")
			}
			if got.DeclaringType != typ || got.Method.Name != "<init>" || got.Method.Kind != java.MethodConstructor || !got.Method.Synthetic {
				t.Fatalf("synthetic constructor = %+v", got)
			}
			if !reflect.DeepEqual(got.Method.Modifier, tt.want) {
				t.Fatalf("constructor modifiers = %v, want %v", got.Method.Modifier, tt.want)
			}
			if got.Method.Params == nil || got.Method.Calls == nil {
				t.Fatalf("synthetic slices must be non-nil: %+v", got.Method)
			}
		})
	}
}

func TestBuildSynthesizesCanonicalRecordConstructor(t *testing.T) {
	record := &java.TypeDecl{
		Kind:     java.TypeKindRecord,
		Name:     "Entry",
		FQCN:     "app.Entry",
		Modifier: []string{"public"},
		RecordComponents: []java.Param{
			{Name: "value", Type: java.NewTypeRef("Value", false)},
			{Name: "count", Type: java.NewTypeRef("int", false)},
		},
		Methods: []java.MethodDecl{{
			Kind:   java.MethodConstructor,
			Name:   "<init>",
			Params: []java.Param{{Name: "value", Type: java.NewTypeRef("Value", false)}},
		}},
	}
	value := &java.TypeDecl{Kind: java.TypeKindClass, Name: "Value", FQCN: "model.Value"}
	table, err := Build([]*java.CompilationUnit{
		{File: "Entry.java", Package: "app", Imports: []java.ImportDecl{{Target: value.FQCN}}, Types: []*java.TypeDecl{record}},
		{File: "Value.java", Package: "model", Types: []*java.TypeDecl{value}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got, ok := table.Constructor(record.FQCN, "(model.Value,int)")
	if !ok || !got.Method.Synthetic {
		t.Fatalf("canonical constructor = %+v, %v", got, ok)
	}
	if got.Method.Params == nil || got.Method.Calls == nil || len(got.Method.Params) != 2 {
		t.Fatalf("synthetic canonical constructor slices = %+v", got.Method)
	}
	if record.RecordComponents[0].Type.FQCN != "model.Value" || got.Method.Params[0].Type.FQCN != "model.Value" {
		t.Fatalf("record component types were not canonicalized: %+v", record)
	}
	got.Method.Params[0].Name = "changed"
	if record.RecordComponents[0].Name != "value" {
		t.Fatal("synthetic params share their backing array with record components")
	}
}

func TestBuildDoesNotDuplicateDeclaredConstructors(t *testing.T) {
	tests := []struct {
		name    string
		typ     *java.TypeDecl
		wantSig string
	}{
		{
			name: "class declaration",
			typ: &java.TypeDecl{Kind: java.TypeKindClass, Name: "Value", FQCN: "app.Value", Methods: []java.MethodDecl{{
				Kind: java.MethodConstructor, Name: "<init>", Params: []java.Param{{Type: java.NewTypeRef("int", false)}},
			}}},
			wantSig: "(int)",
		},
		{
			name: "record canonical declaration",
			typ: &java.TypeDecl{Kind: java.TypeKindRecord, Name: "Value", FQCN: "app.Value",
				RecordComponents: []java.Param{{Name: "value", Type: java.NewTypeRef("int", false)}},
				Methods:          []java.MethodDecl{{Kind: java.MethodConstructor, Name: "<init>", Params: []java.Param{{Name: "value", Type: java.NewTypeRef("int", false)}}}},
			},
			wantSig: "(int)",
		},
		{
			name: "record compact declaration",
			typ: &java.TypeDecl{Kind: java.TypeKindRecord, Name: "Value", FQCN: "app.Value",
				RecordComponents: []java.Param{{Name: "value", Type: java.NewTypeRef("int", false)}},
				Methods:          []java.MethodDecl{{Kind: java.MethodCompactConstructor, Name: "<init>", Params: []java.Param{{Name: "value", Type: java.NewTypeRef("int", false)}}}},
			},
			wantSig: "(int)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := buildConstructorTable(t, tt.typ)
			constructors := table.ConstructorCandidates(tt.typ.FQCN)
			if len(constructors) != 1 || constructors[0].Method.Signature != tt.wantSig || constructors[0].Method.Synthetic {
				t.Fatalf("constructors = %+v", constructors)
			}
		})
	}
}

func TestBuildDoesNotSynthesizeInterfaceOrEnumConstructors(t *testing.T) {
	for _, kind := range []java.TypeKind{java.TypeKindInterface, java.TypeKindEnum} {
		typ := &java.TypeDecl{Kind: kind, Name: "Value", FQCN: "app." + kind.String()}
		table := buildConstructorTable(t, typ)
		if got := table.ConstructorCandidates(typ.FQCN); got == nil || len(got) != 0 {
			t.Fatalf("kind %v constructors = %+v", kind, got)
		}
	}
}

func TestConstructorCandidatesAreDirectSortedAndDefensive(t *testing.T) {
	parent := &java.TypeDecl{Kind: java.TypeKindClass, Name: "Parent", FQCN: "app.Parent", Methods: []java.MethodDecl{
		{Kind: java.MethodConstructor, Name: "<init>", Params: []java.Param{{Type: java.NewTypeRef("long", false)}}},
	}}
	child := &java.TypeDecl{
		Kind:       java.TypeKindClass,
		Name:       "Child",
		FQCN:       "app.Child",
		SuperClass: java.NewTypeRef(parent.FQCN, false),
		Methods: []java.MethodDecl{
			{Kind: java.MethodConstructor, Name: "<init>", Params: []java.Param{{Type: java.NewTypeRef("String", false)}}},
			{Kind: java.MethodConstructor, Name: "<init>", Params: []java.Param{}},
			{Kind: java.MethodOrdinary, Name: "<init>", Params: []java.Param{{Type: java.NewTypeRef("boolean", false)}}},
		},
	}
	table, err := Build([]*java.CompilationUnit{{File: "Types.java", Package: "app", Types: []*java.TypeDecl{parent, child}}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got := table.ConstructorCandidates(child.FQCN)
	if len(got) != 2 || got[0].Method.Signature != "()" || got[1].Method.Signature != "(java.lang.String)" {
		t.Fatalf("constructor candidates = %+v", got)
	}
	for _, candidate := range got {
		if candidate.DeclaringType != child {
			t.Fatalf("inherited constructor returned: %+v", candidate)
		}
	}
	if _, ok := table.Constructor(child.FQCN, "(long)"); ok {
		t.Fatal("Constructor inherited a parent declaration")
	}
	if _, ok := table.Constructor(child.FQCN, "(boolean)"); ok {
		t.Fatal("ordinary method named <init> treated as constructor")
	}

	got[0] = MethodResolution{}
	again := table.ConstructorCandidates(child.FQCN)
	if again[0].Method == nil || again[0].Method.Signature != "()" {
		t.Fatalf("ConstructorCandidates exposed its result slice: %+v", again)
	}
}

func TestConstructorLookupsOnNilAndMissingTables(t *testing.T) {
	var nilTable *Table
	if got := nilTable.ConstructorCandidates("missing.Type"); got == nil || len(got) != 0 {
		t.Fatalf("nil table candidates = %+v", got)
	}
	if got, ok := nilTable.Constructor("missing.Type", "()"); ok || got != (MethodResolution{}) {
		t.Fatalf("nil table constructor = %+v, %v", got, ok)
	}

	table, err := Build(nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := table.ConstructorCandidates("missing.Type"); got == nil || len(got) != 0 {
		t.Fatalf("missing type candidates = %+v", got)
	}
}

func TestBuildCanonicalizesConstructorCallTargetType(t *testing.T) {
	target := java.NewTypeRef("Value", false)
	caller := &java.TypeDecl{Kind: java.TypeKindClass, Name: "Caller", FQCN: "app.Caller", Methods: []java.MethodDecl{{
		Kind:  java.MethodOrdinary,
		Name:  "run",
		Calls: []java.CallSite{{Kind: java.CallObjectCreation, MethodName: "<init>", TargetType: &target}},
	}}}
	value := &java.TypeDecl{Kind: java.TypeKindClass, Name: "Value", FQCN: "model.Value"}
	table, err := Build([]*java.CompilationUnit{
		{File: "Caller.java", Package: "app", Imports: []java.ImportDecl{{Target: value.FQCN}}, Types: []*java.TypeDecl{caller}},
		{File: "Value.java", Package: "model", Types: []*java.TypeDecl{value}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	_ = table
	if got := caller.Methods[0].Calls[0].TargetType; got != &target || got.FQCN != value.FQCN || got.Unresolved {
		t.Fatalf("constructor target type = %+v", got)
	}
}

func buildConstructorTable(t *testing.T, typ *java.TypeDecl) *Table {
	t.Helper()
	table, err := Build([]*java.CompilationUnit{{File: typ.Name + ".java", Package: "app", Types: []*java.TypeDecl{typ}}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return table
}
