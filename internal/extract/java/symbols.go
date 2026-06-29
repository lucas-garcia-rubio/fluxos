// Package java contém os tipos que podem aparecer em um código java
package java

import "fmt"

type TypeKind int

const (
	TypeKindClass TypeKind = iota
	TypeKindInterface
	TypeKindEnum
	TypeKindRecord
)

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

type (
	FieldDecl struct {
		Name     string
		Modifier []string
		Type     string
	}
	Param struct {
		Name string
		Type string
	}
	MethodDecl struct {
		Name       string
		Modifier   []string
		ReturnType string
		Params     []Param
	}
)

type TypeDecl struct {
	Kind       TypeKind
	Name       string
	FQCN       string
	SuperClass string
	Interfaces []string
	File       string
	Line       int // NOTE: não sei se precisa mesmo
	Fields     []FieldDecl
	Methods    []MethodDecl
}
