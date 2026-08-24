package projectanalysis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveGoSymbolsBuildsCrossFilePackageAndCallReferences(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test/app\n\ngo 1.25\n")
	write("main.go", "package main\n\nimport (\n  \"fmt\"\n  \"example.test/app/util\"\n)\n\nfunc main() {\n  fmt.Println(util.Add(1, 2))\n}\n")
	write("util/math.go", "package util\n\nfunc Add(a, b int) int { return a + b }\n")

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
	resolved, err := ResolveGoSymbols(context.Background(), root, parsed)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Artifact.Stage != StageSymbols || !hasResolvedSymbol(resolved.Artifact.Symbols, "Add") || !hasResolvedSymbol(resolved.Artifact.Symbols, "main") {
		t.Fatalf("resolved symbols = %#v", resolved.Artifact.Symbols)
	}
	if !hasGraphKind(resolved.Artifact.Graph, "package") || !hasGraphKind(resolved.Artifact.Graph, "symbol") {
		t.Fatalf("graph kinds = %#v", resolved.Artifact.Graph.Nodes)
	}
	if !hasReferenceKind(resolved.Artifact.References, "calls") {
		t.Fatalf("call reference missing: %#v", resolved.Artifact.References)
	}
	if !hasGraphNodeLabel(resolved.Artifact.Graph, "fmt") {
		t.Fatalf("external dependency node missing: %#v", resolved.Artifact.Graph.Nodes)
	}
	if err := resolved.Validate(); err != nil {
		t.Fatal(err)
	}
	if summary, err := SummarizeResolutionArtifact(resolved); err != nil || summary.ResolvedSymbols == 0 || summary.LocalReferences == 0 {
		t.Fatalf("resolution summary = %#v, err=%v", summary, err)
	}
}

func mustMarshal(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func TestResolveGoSymbolsRejectsStaleSourceWithoutLeakingContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package main\nfunc Original() {}\n"), 0o600); err != nil {
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
	if err := os.WriteFile(path, []byte("package main\nfunc ChangedSecretName() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveGoSymbols(context.Background(), root, parsed)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Artifact.Symbols) != 0 || !hasDiagnosticSeverity(resolved.Artifact.Diagnostics, "stale") {
		t.Fatalf("stale resolution = symbols=%#v diagnostics=%#v", resolved.Artifact.Symbols, resolved.Artifact.Diagnostics)
	}
	if strings.Contains(string(mustMarshal(resolved)), "ChangedSecretName") {
		t.Fatal("stale source content leaked into artifact")
	}
}

func hasResolvedSymbol(symbols []CodeSymbol, name string) bool {
	for _, symbol := range symbols {
		if symbol.Name == name {
			return true
		}
	}
	return false
}

func hasGraphKind(graph KnowledgeGraph, kind string) bool {
	for _, node := range graph.Nodes {
		if node.Kind == kind {
			return true
		}
	}
	return false
}

func hasGraphNodeLabel(graph KnowledgeGraph, label string) bool {
	for _, node := range graph.Nodes {
		if strings.Contains(node.Label, label) {
			return true
		}
	}
	return false
}

func hasReferenceKind(references []CodeReference, kind string) bool {
	for _, reference := range references {
		if reference.Kind == kind {
			return true
		}
	}
	return false
}

func hasDiagnosticSeverity(diagnostics []ParseDiagnostic, severity string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == severity {
			return true
		}
	}
	return false
}
