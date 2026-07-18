package project

// ScopeMode selects which source scopes a command should include. It is
// separate from Scope, which classifies source roots after discovery.
type ScopeMode string

const (
	ScopeModeMain ScopeMode = "main"
	ScopeModeAll  ScopeMode = "all"
)

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
