package project

type Scope int

const (
	ScopeMain Scope = iota
	ScopeTest
	ScopeGenerated
	ScopeExplicit
)

type SourceRoot struct {
	Path  string
	Scope Scope
}

type JavaFile struct {
	Path       string
	SourceRoot string
	Scope      Scope
}

type Project struct {
	Root        string
	SourceRoots []SourceRoot
	Files       []JavaFile
}
