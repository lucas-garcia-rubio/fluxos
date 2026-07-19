package project

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverAllIncludesTestSources(t *testing.T) {
	root := t.TempDir()
	mainRoot := filepath.Join(root, "src", "main", "java")
	testRoot := filepath.Join(root, "src", "test", "java")
	mainFile := filepath.Join(mainRoot, "app", "Service.java")
	testFile := filepath.Join(testRoot, "app", "ServiceTest.java")
	writeJavaFile(t, mainFile)
	writeJavaFile(t, testFile)

	project, err := DiscoverWithOptions(root, DiscoverOptions{Scope: ScopeModeAll})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
	}
	wantRoots := []SourceRoot{
		{Path: mainRoot, Scope: ScopeMain},
		{Path: testRoot, Scope: ScopeTest},
	}
	if !reflect.DeepEqual(project.SourceRoots, wantRoots) {
		t.Fatalf("source roots = %+v, want %+v", project.SourceRoots, wantRoots)
	}
	wantFiles := []JavaFile{
		{Path: mainFile, SourceRoot: mainRoot, Scope: ScopeMain},
		{Path: testFile, SourceRoot: testRoot, Scope: ScopeTest},
	}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverMainExcludesTestSources(t *testing.T) {
	root := t.TempDir()
	mainRoot := filepath.Join(root, "src", "main", "java")
	testRoot := filepath.Join(root, "src", "test", "java")
	mainFile := filepath.Join(mainRoot, "app", "Service.java")
	writeJavaFile(t, mainFile)
	writeJavaFile(t, filepath.Join(testRoot, "app", "ServiceTest.java"))

	project, err := DiscoverWithOptions(root, DiscoverOptions{Scope: ScopeModeMain})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
	}
	wantRoots := []SourceRoot{{Path: mainRoot, Scope: ScopeMain}}
	if !reflect.DeepEqual(project.SourceRoots, wantRoots) {
		t.Fatalf("source roots = %+v, want %+v", project.SourceRoots, wantRoots)
	}
	wantFiles := []JavaFile{{Path: mainFile, SourceRoot: mainRoot, Scope: ScopeMain}}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverAllOrdersMainBeforeTest(t *testing.T) {
	root := t.TempDir()
	moduleA := filepath.Join(root, "module-a")
	moduleB := filepath.Join(root, "module-b")
	mainA := filepath.Join(moduleA, "src", "main", "java")
	testA := filepath.Join(moduleA, "src", "test", "java")
	mainB := filepath.Join(moduleB, "src", "main", "java")
	testB := filepath.Join(moduleB, "src", "test", "java")
	mainAFile := filepath.Join(mainA, "app", "A.java")
	testAFile := filepath.Join(testA, "app", "ATest.java")
	mainBFile := filepath.Join(mainB, "app", "B.java")
	testBFile := filepath.Join(testB, "app", "BTest.java")
	for _, p := range []string{mainAFile, testAFile, mainBFile, testBFile} {
		writeJavaFile(t, p)
	}

	project, err := DiscoverWithOptions(root, DiscoverOptions{Scope: ScopeModeAll})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
	}
	wantRoots := []SourceRoot{
		{Path: mainA, Scope: ScopeMain},
		{Path: mainB, Scope: ScopeMain},
		{Path: testA, Scope: ScopeTest},
		{Path: testB, Scope: ScopeTest},
	}
	if !reflect.DeepEqual(project.SourceRoots, wantRoots) {
		t.Fatalf("source roots = %+v, want %+v", project.SourceRoots, wantRoots)
	}
	wantFiles := []JavaFile{
		{Path: mainAFile, SourceRoot: mainA, Scope: ScopeMain},
		{Path: mainBFile, SourceRoot: mainB, Scope: ScopeMain},
		{Path: testAFile, SourceRoot: testA, Scope: ScopeTest},
		{Path: testBFile, SourceRoot: testB, Scope: ScopeTest},
	}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverAllGeneratedDirectoriesAreSkipped(t *testing.T) {
	root := t.TempDir()
	mainRoot := filepath.Join(root, "src", "main", "java")
	testRoot := filepath.Join(root, "src", "test", "java")
	mainFile := filepath.Join(mainRoot, "app", "Service.java")
	testFile := filepath.Join(testRoot, "app", "ServiceTest.java")
	writeJavaFile(t, mainFile)
	writeJavaFile(t, testFile)
	writeJavaFile(t, filepath.Join(root, "generated", "module", "src", "main", "java", "app", "Generated.java"))
	writeJavaFile(t, filepath.Join(root, "build", "classes", "app", "Built.java"))

	project, err := DiscoverWithOptions(root, DiscoverOptions{Scope: ScopeModeAll})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
	}
	wantFiles := []JavaFile{
		{Path: mainFile, SourceRoot: mainRoot, Scope: ScopeMain},
		{Path: testFile, SourceRoot: testRoot, Scope: ScopeTest},
	}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverMainFallbackExplicitSkipsTestRoots(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "legacy", "Service.java")
	writeJavaFile(t, legacy)
	writeJavaFile(t, filepath.Join(root, "src", "test", "java", "app", "ServiceTest.java"))

	project, err := DiscoverWithOptions(root, DiscoverOptions{Scope: ScopeModeMain})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
	}
	wantRoots := []SourceRoot{{Path: root, Scope: ScopeExplicit}}
	if !reflect.DeepEqual(project.SourceRoots, wantRoots) {
		t.Fatalf("source roots = %+v, want %+v", project.SourceRoots, wantRoots)
	}
	wantFiles := []JavaFile{{Path: legacy, SourceRoot: root, Scope: ScopeExplicit}}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverAllFallbackExplicitIncludesLegacyFiles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "z", "First.java")
	second := filepath.Join(root, "a", "Second.java")
	writeJavaFile(t, first)
	writeJavaFile(t, second)

	project, err := DiscoverWithOptions(root, DiscoverOptions{Scope: ScopeModeAll})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
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
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverAllWithExplicitTestRootClassifiesAsTest(t *testing.T) {
	root := t.TempDir()
	testRoot := filepath.Join(root, "src", "test", "java")
	testFile := filepath.Join(testRoot, "app", "OnlyTest.java")
	writeJavaFile(t, testFile)

	project, err := DiscoverWithOptions(testRoot, DiscoverOptions{Scope: ScopeModeAll})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
	}
	wantRoots := []SourceRoot{{Path: testRoot, Scope: ScopeTest}}
	if !reflect.DeepEqual(project.SourceRoots, wantRoots) {
		t.Fatalf("source roots = %+v, want %+v", project.SourceRoots, wantRoots)
	}
	wantFiles := []JavaFile{{Path: testFile, SourceRoot: testRoot, Scope: ScopeTest}}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverAllWithExplicitTestRootMainModeStaysExplicit(t *testing.T) {
	root := t.TempDir()
	testRoot := filepath.Join(root, "src", "test", "java")
	testFile := filepath.Join(testRoot, "app", "OnlyTest.java")
	writeJavaFile(t, testFile)

	project, err := DiscoverWithOptions(testRoot, DiscoverOptions{Scope: ScopeModeMain})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
	}
	wantRoots := []SourceRoot{{Path: testRoot, Scope: ScopeExplicit}}
	if !reflect.DeepEqual(project.SourceRoots, wantRoots) {
		t.Fatalf("source roots = %+v, want %+v", project.SourceRoots, wantRoots)
	}
	wantFiles := []JavaFile{{Path: testFile, SourceRoot: testRoot, Scope: ScopeExplicit}}
	if !reflect.DeepEqual(project.Files, wantFiles) {
		t.Fatalf("files = %+v, want %+v", project.Files, wantFiles)
	}
}

func TestDiscoverWrapperDefaultsToMain(t *testing.T) {
	root := t.TempDir()
	mainRoot := filepath.Join(root, "src", "main", "java")
	testRoot := filepath.Join(root, "src", "test", "java")
	mainFile := filepath.Join(mainRoot, "app", "Service.java")
	writeJavaFile(t, mainFile)
	writeJavaFile(t, filepath.Join(testRoot, "app", "ServiceTest.java"))

	wrapperProject, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	explicitProject, err := DiscoverWithOptions(root, DiscoverOptions{Scope: ScopeModeMain})
	if err != nil {
		t.Fatalf("DiscoverWithOptions: %v", err)
	}
	if !reflect.DeepEqual(wrapperProject, explicitProject) {
		t.Fatalf("Discover != DiscoverWithOptions(main): wrapper=%+v explicit=%+v", wrapperProject, explicitProject)
	}
}
