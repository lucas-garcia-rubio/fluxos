package project

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeJavaFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("class Fixture {}"), 0o600); err != nil {
		t.Fatalf("write Java fixture: %v", err)
	}
}

func TestDiscoverMainSourceRootsAcrossModules(t *testing.T) {
	root := t.TempDir()
	aMain := filepath.Join(root, "module-a", "src", "main", "java")
	bMain := filepath.Join(root, "module-b", "src", "main", "java")
	aFile := filepath.Join(aMain, "com", "example", "A.java")
	bFile := filepath.Join(bMain, "com", "example", "B.java")
	writeJavaFile(t, bFile)
	writeJavaFile(t, aFile)
	writeJavaFile(t, filepath.Join(root, "module-a", "src", "test", "java", "ATest.java"))
	writeJavaFile(t, filepath.Join(root, "module-a", "target", "Generated.java"))
	writeJavaFile(t, filepath.Join(root, "build", "BuildOutput.java"))
	writeJavaFile(t, filepath.Join(root, ".git", "Ignored.java"))
	writeJavaFile(t, filepath.Join(root, "node_modules", "Ignored.java"))

	project, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	wantRoots := []SourceRoot{
		{Path: aMain, Scope: ScopeMain},
		{Path: bMain, Scope: ScopeMain},
	}
	if !reflect.DeepEqual(project.SourceRoots, wantRoots) {
		t.Fatalf("source roots = %+v, want %+v", project.SourceRoots, wantRoots)
	}
	wantFiles := []JavaFile{
		{Path: aFile, SourceRoot: aMain, Scope: ScopeMain},
		{Path: bFile, SourceRoot: bMain, Scope: ScopeMain},
	}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverAcceptsMainSourceRootDirectly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "src", "main", "java")
	file := filepath.Join(root, "Example.java")
	buildPackageFile := filepath.Join(root, "com", "example", "build", "Builder.java")
	writeJavaFile(t, file)
	writeJavaFile(t, buildPackageFile)

	project, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	wantRoots := []SourceRoot{{Path: root, Scope: ScopeMain}}
	if !reflect.DeepEqual(project.SourceRoots, wantRoots) {
		t.Fatalf("source roots = %+v, want %+v", project.SourceRoots, wantRoots)
	}
	wantFiles := []JavaFile{
		{Path: file, SourceRoot: root, Scope: ScopeMain},
		{Path: buildPackageFile, SourceRoot: root, Scope: ScopeMain},
	}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverFallsBackToExplicitRoot(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "z", "Second.java")
	second := filepath.Join(root, "a", "First.java")
	writeJavaFile(t, first)
	writeJavaFile(t, second)
	writeJavaFile(t, filepath.Join(root, "src", "test", "java", "IgnoredTest.java"))
	writeJavaFile(t, filepath.Join(root, "target", "IgnoredOutput.java"))
	if err := os.WriteFile(filepath.Join(root, "README.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatalf("write non-Java fixture: %v", err)
	}

	project, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	wantRoots := []SourceRoot{{Path: root, Scope: ScopeExplicit}}
	if !reflect.DeepEqual(project.SourceRoots, wantRoots) {
		t.Fatalf("source roots = %+v, want %+v", project.SourceRoots, wantRoots)
	}
	wantFiles := []JavaFile{
		{Path: second, SourceRoot: root, Scope: ScopeExplicit},
		{Path: first, SourceRoot: root, Scope: ScopeExplicit},
	}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want ordered %+v", project.Files, wantFiles)
	}
}

func TestDiscoverExplicitRootPreservesPackageNamedBuild(t *testing.T) {
	root := t.TempDir()
	buildFile := filepath.Join(root, "com", "example", "build", "Builder.java")
	srcFile := filepath.Join(root, "com", "example", "src", "Source.java")
	writeJavaFile(t, buildFile)
	writeJavaFile(t, srcFile)

	project, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	wantFiles := []JavaFile{
		{Path: buildFile, SourceRoot: root, Scope: ScopeExplicit},
		{Path: srcFile, SourceRoot: root, Scope: ScopeExplicit},
	}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverExcludesGeneratedSourceRoots(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "legacy", "Example.java")
	writeJavaFile(t, file)
	writeJavaFile(t, filepath.Join(root, "generated", "module", "src", "main", "java", "Generated.java"))
	writeJavaFile(t, filepath.Join(root, "generated-sources", "module", "src", "main", "java", "Generated.java"))

	project, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	wantFiles := []JavaFile{{Path: file, SourceRoot: root, Scope: ScopeExplicit}}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverReturnsNonNilEmptyFiles(t *testing.T) {
	root := t.TempDir()
	project, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if project.Root != root || project.SourceRoots == nil || project.Files == nil {
		t.Fatalf("empty project = %+v", project)
	}
}
