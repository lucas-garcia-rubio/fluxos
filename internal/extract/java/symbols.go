// Package java contém os tipos que podem aparecer em um código java
package java

import (
	"encoding/json"
	"fmt"
)

type TypeKind int

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
	FieldDecl struct {
		Name     string   `json:"name"`
		Modifier []string `json:"modifier"`
		Type     string   `json:"type"`
	}
	Param struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	MethodDecl struct {
		Name       string   `json:"name"`
		Modifier   []string `json:"modifier"`
		ReturnType string   `json:"returnType"`
		Params     []Param  `json:"params"`
	}
)

type TypeDecl struct {
	Kind       TypeKind     `json:"kind"`
	Name       string       `json:"name"`
	FQCN       string       `json:"fqcn"`
	SuperClass string       `json:"superClass"`
	Interfaces []string     `json:"interfaces"`
	File       string       `json:"file"`
	Line       int          `json:"line"`
	Fields     []FieldDecl  `json:"fields"`
	Methods    []MethodDecl `json:"methods"`
}
