package projectanalysis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeFindsStackSymbolsDependenciesAndProtectsSecrets(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test/app\n")
	write("main.go", "package main\nimport \"fmt\"\ntype Server struct{}\nfunc (Server) Run(){fmt.Println(1)}\n")
	write("web/app.tsx", "import React from 'react'\nexport function Dashboard() { return null }\n")
	write(".env", "TOKEN=must-not-leak")
	write("node_modules/noise.js", "function Noise() {}")

	result, err := Analyze(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SensitiveFiles) != 1 || result.SensitiveFiles[0] != ".env" {
		t.Fatalf("sensitive files = %#v", result.SensitiveFiles)
	}
	if len(result.Frameworks) == 0 || result.PackageManager != "Go modules" {
		t.Fatalf("stack = %#v manager=%q", result.Frameworks, result.PackageManager)
	}
	if !hasSymbol(result.Symbols, "Server") || !hasSymbol(result.Symbols, "Run") || !hasSymbol(result.Symbols, "Dashboard") {
		t.Fatalf("symbols = %#v", result.Symbols)
	}
	if len(result.Dependencies) < 2 {
		t.Fatalf("dependencies = %#v", result.Dependencies)
	}
	if !hasDependency(result.Dependencies, "react") {
		t.Fatalf("default import target missing: %#v", result.Dependencies)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-leak") {
		t.Fatal("sensitive file content leaked into result")
	}
	for _, file := range result.Files {
		if file.Path == "node_modules/noise.js" {
			t.Fatal("noise directory was scanned")
		}
	}
}

func TestAnalyzeHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Analyze(ctx, t.TempDir(), nil); err == nil {
		t.Fatal("expected cancellation")
	}
}

func hasSymbol(symbols []Symbol, name string) bool {
	for _, symbol := range symbols {
		if symbol.Name == name {
			return true
		}
	}
	return false
}

func hasDependency(dependencies []Dependency, target string) bool {
	for _, dependency := range dependencies {
		if dependency.To == target {
			return true
		}
	}
	return false
}
