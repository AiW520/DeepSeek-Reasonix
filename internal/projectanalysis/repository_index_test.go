package projectanalysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryIndexPreservesVersionedFilesWithoutSourceContent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
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
	if index.Artifact.Stage != StageRepositoryIndex || len(index.Artifact.Files) != len(recon.Artifact.Files) {
		t.Fatalf("repository index = stage %q files %d", index.Artifact.Stage, len(index.Artifact.Files))
	}
	if len(index.Artifact.Syntax) != 0 || len(index.Artifact.Symbols) != 0 || len(index.Artifact.Graph.Edges) != 0 {
		t.Fatalf("repository index invented downstream facts: %#v", index.Artifact)
	}
}
