package projectanalysis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProjectBuildsRealGoSyntaxAndReportsUnsupportedLanguages(t *testing.T) {
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
	write("go.mod", "module example.test/parse\n")
	write("main.go", "package main\nimport \"fmt\"\ntype Server struct{}\nfunc (Server) Run(){fmt.Println(1)}\n")
	write("ui/app.ts", "export function App() { return null }\n")

	reconResult, err := Analyze(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	recon, err := BuildReconnaissanceArtifact(reconResult)
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
	if parsed.Artifact.Stage != StageParse || len(parsed.Artifact.Syntax) == 0 {
		t.Fatalf("parse artifact = stage %q syntax=%d", parsed.Artifact.Stage, len(parsed.Artifact.Syntax))
	}
	if !hasSyntaxKind(parsed.Artifact.Syntax, "go.FuncDecl") || !hasSyntaxKind(parsed.Artifact.Syntax, "go.TypeSpec") {
		t.Fatalf("Go AST facts missing: %#v", parsed.Artifact.Syntax[:minInt(8, len(parsed.Artifact.Syntax))])
	}
	if !hasDiagnosticFor(parsed.Artifact.Diagnostics, "ui/app.ts", "unsupported") {
		t.Fatalf("unsupported language diagnostic missing: %#v", parsed.Artifact.Diagnostics)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatal(err)
	}
	summary, err := SummarizeParseArtifact(parsed)
	if err != nil || summary.SyntaxNodes == 0 || summary.ParsedFiles == 0 {
		t.Fatalf("parse summary = %#v, err=%v", summary, err)
	}
}

func TestParseProjectNeverReadsSensitiveFilesAndDetectsStaleInputs(t *testing.T) {
	root := t.TempDir()
	secretPath := filepath.Join(root, ".env")
	if err := os.WriteFile(secretPath, []byte("TOKEN=do-not-read"), 0o600); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "main.go")
	if err := os.WriteFile(mainPath, []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reconResult, err := Analyze(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	recon, err := BuildReconnaissanceArtifact(reconResult)
	if err != nil {
		t.Fatal(err)
	}
	index, err := BuildRepositoryIndexArtifact(recon)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("package main\nfunc changed() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProject(context.Background(), root, index)
	if err != nil {
		t.Fatal(err)
	}
	if hasDiagnosticFor(parsed.Artifact.Diagnostics, ".env", "blocked") == false {
		t.Fatalf("sensitive file was not explicitly blocked: %#v", parsed.Artifact.Diagnostics)
	}
	if !hasDiagnosticFor(parsed.Artifact.Diagnostics, "main.go", "stale") {
		t.Fatalf("stale source was parsed instead of rejected: %#v", parsed.Artifact.Diagnostics)
	}
	encodedBytes, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(encodedBytes)
	if strings.Contains(encoded, "do-not-read") {
		t.Fatal("sensitive file content leaked into parse artifact")
	}
}

func TestParseProjectReportsSyntaxErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "broken.go")
	if err := os.WriteFile(path, []byte("package main\nfunc broken( {\n"), 0o600); err != nil {
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
	if !hasDiagnosticFor(parsed.Artifact.Diagnostics, "broken.go", "error") {
		t.Fatalf("syntax error diagnostic missing: %#v", parsed.Artifact.Diagnostics)
	}
}

func hasSyntaxKind(nodes []SyntaxNode, kind string) bool {
	for _, node := range nodes {
		if node.Kind == kind {
			return true
		}
	}
	return false
}

func hasDiagnosticFor(diagnostics []ParseDiagnostic, path, severity string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Path == path && diagnostic.Severity == severity {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
