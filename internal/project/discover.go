// Package project descobre source roots e arquivos Java de um projeto.
package project

import (
	"io/fs"
	"path/filepath"
	"sort"
)

var alwaysSkippedDirectories = map[string]struct{}{
	".git":         {},
	".gradle":      {},
	"node_modules": {},
}

var outputDirectories = map[string]struct{}{
	"build":             {},
	"generated":         {},
	"generated-sources": {},
	"out":               {},
	"target":            {},
}

// DiscoverOptions configura o comportamento de DiscoverWithOptions.
type DiscoverOptions struct {
	Scope ScopeMode
}

// Discover mantem o comportamento M3: apenas main roots, com fallback explicit
// quando nenhum source root e encontrado. Callers M2/M3 continuam funcionando
// sem alteracao.
func Discover(root string) (*Project, error) {
	return DiscoverWithOptions(root, DiscoverOptions{Scope: ScopeModeMain})
}

// DiscoverWithOptions executa discovery respeitando o ScopeMode informado.
//
//   - ScopeModeMain: apenas roots em src/main/java; fallback explicit preserva o
//     comportamento M3, pulando subdiretorios src/test/java.
//   - ScopeModeAll: roots em src/main/java e src/test/java; fallback explicit
//     inclui subdiretorios src/test/java. Quando o root passado e exatamente
//     <base>/src/test/java, o fallback classifica como ScopeTest.
//
// Generated/build outputs continuam excluidos em ambos os modos.
func DiscoverWithOptions(root string, opts DiscoverOptions) (*Project, error) {
	root = filepath.Clean(root)
	sourceRoots, err := discoverSourceRoots(root, opts.Scope)
	if err != nil {
		return nil, err
	}
	if len(sourceRoots) == 0 {
		sourceRoots = append(sourceRoots, fallbackSourceRoot(root, opts.Scope))
	}
	sortSourceRoots(sourceRoots)

	files := make([]JavaFile, 0)
	seenFiles := make(map[string]struct{})
	for _, sourceRoot := range sourceRoots {
		discovered, err := discoverJavaFiles(sourceRoot, opts.Scope)
		if err != nil {
			return nil, err
		}
		for _, file := range discovered {
			if _, exists := seenFiles[file.Path]; exists {
				continue
			}
			seenFiles[file.Path] = struct{}{}
			files = append(files, file)
		}
	}
	sortFiles(files)

	return &Project{
		Root:        root,
		SourceRoots: sourceRoots,
		Files:       files,
	}, nil
}

func discoverSourceRoots(root string, mode ScopeMode) ([]SourceRoot, error) {
	results := make([]SourceRoot, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && shouldSkipProjectDirectory(d.Name()) {
			return filepath.SkipDir
		}
		if d.IsDir() && isMainSourceRoot(path) {
			results = append(results, SourceRoot{Path: filepath.Clean(path), Scope: ScopeMain})
			return filepath.SkipDir
		}
		if mode == ScopeModeAll && d.IsDir() && isTestSourceRoot(path) {
			results = append(results, SourceRoot{Path: filepath.Clean(path), Scope: ScopeTest})
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func fallbackSourceRoot(root string, mode ScopeMode) SourceRoot {
	if mode == ScopeModeAll && isTestSourceRoot(root) {
		return SourceRoot{Path: root, Scope: ScopeTest}
	}
	return SourceRoot{Path: root, Scope: ScopeExplicit}
}

func discoverJavaFiles(sourceRoot SourceRoot, mode ScopeMode) ([]JavaFile, error) {
	results := make([]JavaFile, 0)
	err := filepath.WalkDir(sourceRoot.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != sourceRoot.Path && sourceRoot.Scope == ScopeExplicit {
			if shouldSkipExplicitDirectory(sourceRoot.Path, path, d.Name()) {
				return filepath.SkipDir
			}
			if mode == ScopeModeMain && isTestSourceRoot(path) {
				return filepath.SkipDir
			}
		}
		if !d.IsDir() && filepath.Ext(path) == ".java" {
			results = append(results, JavaFile{
				Path:       filepath.Clean(path),
				SourceRoot: sourceRoot.Path,
				Scope:      sourceRoot.Scope,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func isMainSourceRoot(path string) bool {
	return filepath.Base(path) == "java" &&
		filepath.Base(filepath.Dir(path)) == "main" &&
		filepath.Base(filepath.Dir(filepath.Dir(path))) == "src"
}

func isTestSourceRoot(path string) bool {
	return filepath.Base(path) == "java" &&
		filepath.Base(filepath.Dir(path)) == "test" &&
		filepath.Base(filepath.Dir(filepath.Dir(path))) == "src"
}

func shouldSkipProjectDirectory(name string) bool {
	if _, skip := alwaysSkippedDirectories[name]; skip {
		return true
	}
	_, skip := outputDirectories[name]
	return skip
}

func shouldSkipExplicitDirectory(root, path, name string) bool {
	if _, skip := alwaysSkippedDirectories[name]; skip {
		return true
	}
	if _, output := outputDirectories[name]; !output {
		return false
	}
	// Nested names may be valid Java packages in a custom source root.
	return filepath.Dir(path) == root
}

func sortSourceRoots(roots []SourceRoot) {
	sort.SliceStable(roots, func(i, j int) bool {
		if roots[i].Scope != roots[j].Scope {
			return scopeRank(roots[i].Scope) < scopeRank(roots[j].Scope)
		}
		return roots[i].Path < roots[j].Path
	})
}

func sortFiles(files []JavaFile) {
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Scope != files[j].Scope {
			return scopeRank(files[i].Scope) < scopeRank(files[j].Scope)
		}
		return files[i].Path < files[j].Path
	})
}

func scopeRank(s Scope) int {
	switch s {
	case ScopeMain:
		return 0
	case ScopeTest:
		return 1
	case ScopeExplicit:
		return 2
	case ScopeGenerated:
		return 3
	}
	return 4
}
