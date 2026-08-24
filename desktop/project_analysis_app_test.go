package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectAnalysisRootValidation(t *testing.T) {
	root := t.TempDir()
	got, err := validateProjectAnalysisRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(root) {
		t.Fatalf("root = %q, want %q", got, root)
	}
	if _, err := validateProjectAnalysisRoot(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing root should fail")
	}
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateProjectAnalysisRoot(file); err == nil {
		t.Fatal("file root should fail")
	}
}

func TestProjectAnalysisCompletesWithGoParseSummary(t *testing.T) {
	app := NewApp()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := app.StartProjectAnalysis(root)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		view, err := app.ProjectAnalysisJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if view.State == "completed" {
			if view.Result == nil || view.Result.Parse == nil {
				t.Fatalf("completed result has no parse summary: %#v", view.Result)
			}
			if view.Result.Parse.ParsedFiles != 1 || view.Result.Parse.SyntaxNodes == 0 || view.Result.Resolution == nil || view.Result.Resolution.ResolvedSymbols == 0 || view.Result.CallGraph == nil || view.Result.DataFlow == nil {
				t.Fatalf("parse summary = %#v", view.Result.Parse)
			}
			return
		}
		if view.State == "failed" || view.State == "cancelled" {
			t.Fatalf("analysis ended in %q: %s", view.State, view.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("analysis did not complete before timeout")
}

func TestProjectAnalysisJobCanBeCancelled(t *testing.T) {
	app := NewApp()
	root := t.TempDir()
	job, err := app.StartProjectAnalysis(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.CancelProjectAnalysis(job.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.CancelProjectAnalysis("missing"); err == nil {
		t.Fatal("unknown job should fail")
	}
}
