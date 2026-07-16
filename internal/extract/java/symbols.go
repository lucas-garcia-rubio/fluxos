// Package java contém os tipos que podem aparecer em um código java
package java

import (
	"encoding/json"
	"fmt"
)

type TypeKind int

type MethodKind int

const (
	MethodOrdinary MethodKind = iota
	MethodConstructor
	MethodCompactConstructor
	MethodSyntheticLambda
)

func (k MethodKind) String() string {
	switch k {
	case MethodOrdinary:
		return "ordinary"
	case MethodConstructor:
		return "constructor"
	case MethodCompactConstructor:
		return "compactConstructor"
	case MethodSyntheticLambda:
		return "syntheticLambda"
	default:
		return fmt.Sprintf("unknown(%d)", k)
	}
}

func (k MethodKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

type CallKind int

const (
	CallInvocation CallKind = iota
	CallObjectCreation
	CallThisConstructor
	CallSuperConstructor
	CallMethodReference
	CallConstructorReference
)

func (k CallKind) String() string {
	switch k {
	case CallInvocation:
		return "invocation"
	case CallObjectCreation:
		return "objectCreation"
	case CallThisConstructor:
		return "thisConstructor"
	case CallSuperConstructor:
		return "superConstructor"
	case CallMethodReference:
		return "methodReference"
	case CallConstructorReference:
		return "constructorReference"
	default:
		return fmt.Sprintf("unknown(%d)", k)
	}
}

func (k CallKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

const (
	TypeKindClass TypeKind = iota
	TypeKindInterface
	TypeKindEnum
	TypeKindRecord
)

// String devolve o nome do tipo em PascalCase — usado por fmt.Printf("%v") no debug.
func (k TypeKind) String() string {
	switch k {
	case TypeKindClass:
		return "Class"
	case TypeKindInterface:
		return "Interface"
	case TypeKindEnum:
		return "Enum"
	case TypeKindRecord:
		return "Record"
	default:
		return fmt.Sprintf("Unknown(%d)", k)
	}
}

// MarshalJSON devolve o nome do tipo em lowercase — convenção pra campos de JSON.
// Separado do String() pra debug (PascalCase) e data interchange (lowercase) terem
// formatos independentes.
func (k TypeKind) MarshalJSON() ([]byte, error) {
	switch k {
	case TypeKindClass:
		return json.Marshal("class")
	case TypeKindInterface:
		return json.Marshal("interface")
	case TypeKindEnum:
		return json.Marshal("enum")
	case TypeKindRecord:
		return json.Marshal("record")
	default:
		return json.Marshal(fmt.Sprintf("unknown(%d)", k))
	}
}

type (
	ImportDecl struct {
		Target   string `json:"target"`
		Static   bool   `json:"static"`
		Wildcard bool   `json:"wildcard"`
	}
	CompilationUnit struct {
		File       string       `json:"file"`
		SourceRoot string       `json:"sourceRoot"`
		Package    string       `json:"package"`
		Imports    []ImportDecl `json:"imports"`
		Types      []*TypeDecl  `json:"types"`
	}
	TypeRef struct {
		Raw        string `json:"raw"`
		FQCN       string `json:"fqcn,omitempty"`
		ArrayDepth int    `json:"arrayDepth,omitempty"`
		Primitive  bool   `json:"primitive,omitempty"`
		Unresolved bool   `json:"unresolved,omitempty"`
	}
	FieldDecl struct {
		Name        string   `json:"name"`
		Modifier    []string `json:"modifier"`
		Type        TypeRef  `json:"type"`
		Initializer string   `json:"initializer,omitempty"`
	}
	Param struct {
		Name     string  `json:"name"`
		Type     TypeRef `json:"type"`
		Variadic bool    `json:"variadic,omitempty"`
	}
	CallSite struct {
		Kind       CallKind `json:"kind"`
		MethodName string   `json:"methodName"`
		Receiver   string   `json:"receiver"`
		Args       []string `json:"args"`
		ArgCount   int      `json:"argCount"`
		File       string   `json:"file"`
		Line       int      `json:"line"`
		StartByte  uint     `json:"startByte"`
		EndByte    uint     `json:"endByte"`
	}
	MethodKey struct {
		Name      string
		Signature string
	}
	LocalVarDecl struct {
		Name       string  `json:"name"`
		Type       TypeRef `json:"type"`
		ScopeStart uint    `json:"scopeStart"`
		ScopeEnd   uint    `json:"scopeEnd"`
		DeclStart  uint    `json:"declStart"`
	}
	MethodDecl struct {
		Kind       MethodKind     `json:"kind"`
		Name       string         `json:"name"`
		Signature  string         `json:"signature"`
		Modifier   []string       `json:"modifier"`
		ReturnType TypeRef        `json:"returnType"`
		Params     []Param        `json:"params"`
		Calls      []CallSite     `json:"calls"`
		LocalVars  []LocalVarDecl `json:"localVars,omitempty"`
	}
)

func (m MethodDecl) Key() MethodKey {
	return MethodKey{Name: m.Name, Signature: m.Signature}
}

// HasModifier reports whether modifiers contains the exact Java modifier.
func HasModifier(modifiers []string, modifier string) bool {
	for _, candidate := range modifiers {
		if candidate == modifier {
			return true
		}
	}
	return false
}

type TypeDecl struct {
	Kind       TypeKind     `json:"kind"`
	Name       string       `json:"name"`
	FQCN       string       `json:"fqcn"`
	Modifier   []string     `json:"modifier"`
	SuperClass TypeRef      `json:"superClass"`
	Interfaces []TypeRef    `json:"interfaces"`
	File       string       `json:"file"`
	Line       int          `json:"line"`
	Fields     []FieldDecl  `json:"fields"`
	Methods    []MethodDecl `json:"methods"`
}
