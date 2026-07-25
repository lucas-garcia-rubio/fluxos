package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVersionBuildInjection(t *testing.T) {
	repositoryRoot := testRepositoryRoot(t)
	tests := []struct {
		name    string
		ldflags string
		want    string
	}{
		{name: "default", want: "fluxos dev\n"},
		{name: "injected", ldflags: "-X main.version=v12.2.0-test", want: "fluxos v12.2.0-test\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executableName := "fluxos"
			if runtime.GOOS == "windows" {
				executableName += ".exe"
			}
			executable := filepath.Join(t.TempDir(), executableName)

			args := []string{"build"}
			if tt.ldflags != "" {
				args = append(args, "-ldflags", tt.ldflags)
			}
			args = append(args, "-o", executable, "./cmd/fluxos")
			build := exec.Command("go", args...)
			build.Dir = repositoryRoot
			if output, err := build.CombinedOutput(); err != nil {
				t.Fatalf("go %v: %v\n%s", args, err, output)
			}

			var stdout, stderr bytes.Buffer
			versionCommand := exec.Command(executable, "--version")
			versionCommand.Dir = repositoryRoot
			versionCommand.Stdout = &stdout
			versionCommand.Stderr = &stderr
			if err := versionCommand.Run(); err != nil {
				t.Fatalf("%s --version: %v; stdout=%q stderr=%q", executable, err, stdout.String(), stderr.String())
			}
			if stdout.String() != tt.want || stderr.Len() != 0 {
				t.Fatalf("version output = stdout %q, stderr %q; want stdout %q and empty stderr", stdout.String(), stderr.String(), tt.want)
			}
		})
	}
}

func testRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	absoluteFile, err := filepath.Abs(file)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", file, err)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(absoluteFile), "..", ".."))
}
