package projectanalysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPipelineWorkersExecuteRepositoryIndexAndGoParse(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
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
	executor, err := NewPipelineExecutor(DefaultPipelineContract(), RepositoryIndexWorker{}, ParseWorker{Root: root}, SymbolWorker{Root: root}, DependencyWorker{}, CallGraphWorker{}, DataFlowWorker{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	index, err := executor.RunStage(context.Background(), StageRepositoryIndex, StageRequest{
		Mode: AnalysisModeFull, Snapshot: recon.Artifact.Snapshot, Inputs: []ArtifactEnvelope{recon}, IdempotencyKey: "test:index",
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := executor.RunStage(context.Background(), StageParse, StageRequest{
		Mode: AnalysisModeFull, Snapshot: recon.Artifact.Snapshot, Inputs: []ArtifactEnvelope{index}, IdempotencyKey: "test:parse",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasSyntaxKind(parsed.Artifact.Syntax, "go.FuncDecl") {
		t.Fatalf("pipeline parse output missing function declaration: %#v", parsed.Artifact.Syntax)
	}
	symbols, err := executor.RunStage(context.Background(), StageSymbols, StageRequest{
		Mode: AnalysisModeFull, Snapshot: recon.Artifact.Snapshot, Inputs: []ArtifactEnvelope{parsed}, IdempotencyKey: "test:symbols",
	})
	if err != nil {
		t.Fatal(err)
	}
	dependencies, err := executor.RunStage(context.Background(), StageDependencies, StageRequest{
		Mode: AnalysisModeFull, Snapshot: recon.Artifact.Snapshot, Inputs: []ArtifactEnvelope{parsed, symbols}, IdempotencyKey: "test:dependencies",
	})
	if err != nil {
		t.Fatal(err)
	}
	callGraph, err := executor.RunStage(context.Background(), StageCallGraph, StageRequest{
		Mode: AnalysisModeFull, Snapshot: recon.Artifact.Snapshot, Inputs: []ArtifactEnvelope{symbols, dependencies}, IdempotencyKey: "test:callgraph",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.RunStage(context.Background(), StageDataFlow, StageRequest{
		Mode: AnalysisModeFull, Snapshot: recon.Artifact.Snapshot, Inputs: []ArtifactEnvelope{symbols, callGraph}, IdempotencyKey: "test:dataflow",
	}); err != nil {
		t.Fatal(err)
	}
}
