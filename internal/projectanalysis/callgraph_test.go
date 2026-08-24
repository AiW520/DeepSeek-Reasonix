package projectanalysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildGoCallGraphContainsOnlyResolvedLocalCalls(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/calls\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nimport \"fmt\"\nfunc helper() {}\nfunc main(){ helper(); fmt.Println(1) }\n"), 0o600); err != nil {
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
	graph, err := BuildGoCallGraph(symbols, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Artifact.Graph.Edges) != 1 || graph.Artifact.Graph.Edges[0].Kind != "calls" {
		t.Fatalf("call graph edges = %#v", graph.Artifact.Graph.Edges)
	}
	for _, node := range graph.Artifact.Graph.Nodes {
		if node.Kind != "symbol" {
			t.Fatalf("call graph contains non-symbol node: %#v", node)
		}
	}
	if summary, err := SummarizeCallGraph(graph); err != nil || summary.CallEdges != 1 {
		t.Fatalf("summary = %#v err=%v", summary, err)
	}
}
