package projectanalysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildGoDependencyArtifactPreservesResolvedEdges(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/deps\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nimport \"fmt\"\nfunc main(){fmt.Println(1)}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Analyze(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	recon, err := BuildReconnaissanceArtifact(result)
	if err != nil {
		t.Fatal(err)
	}
	index, err := BuildRepositoryIndexArtifact(recon)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProject(context.Background(), root, index)
	if err != nil {
		t.Fatal(err)
	}
	symbols, err := ResolveGoSymbols(context.Background(), root, parsed)
	if err != nil {
		t.Fatal(err)
	}
	dependencies, err := BuildGoDependencyArtifact(symbols)
	if err != nil {
		t.Fatal(err)
	}
	if dependencies.Artifact.Stage != StageDependencies || len(dependencies.Artifact.References) == 0 {
		t.Fatalf("dependencies = %#v", dependencies.Artifact)
	}
	if err := dependencies.Validate(); err != nil {
		t.Fatal(err)
	}
}
