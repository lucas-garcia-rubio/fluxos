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

func Discover(root string) (*Project, error) {
	root = filepath.Clean(root)
	sourceRoots, err := discoverMainSourceRoots(root)
	if err != nil {
		return nil, err
	}
	if len(sourceRoots) == 0 {
		sourceRoots = append(sourceRoots, SourceRoot{Path: root, Scope: ScopeExplicit})
	}
	sort.Slice(sourceRoots, func(i, j int) bool {
		return sourceRoots[i].Path < sourceRoots[j].Path
	})

	files := make([]JavaFile, 0)
	seenFiles := make(map[string]struct{})
	for _, sourceRoot := range sourceRoots {
		discovered, err := discoverJavaFiles(sourceRoot)
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
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return &Project{
		Root:        root,
		SourceRoots: sourceRoots,
		Files:       files,
	}, nil
}

func discoverMainSourceRoots(root string) ([]SourceRoot, error) {
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
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func discoverJavaFiles(sourceRoot SourceRoot) ([]JavaFile, error) {
	results := make([]JavaFile, 0)
	err := filepath.WalkDir(sourceRoot.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != sourceRoot.Path && sourceRoot.Scope == ScopeExplicit {
			if shouldSkipExplicitDirectory(sourceRoot.Path, path, d.Name()) || isTestSourceRoot(path) {
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
