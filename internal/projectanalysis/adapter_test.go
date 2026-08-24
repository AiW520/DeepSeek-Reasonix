package projectanalysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeResultBuildsValidReconnaissanceArtifactOnly(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.test/atlas\n",
		"main.go": "package main\nimport \"fmt\"\nfunc main(){fmt.Println(1)}\n",
		".env":    "TOKEN=must-not-enter-artifact",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Analyze(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := BuildReconnaissanceArtifact(result)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Artifact.Stage != StageReconnaissance {
		t.Fatalf("stage = %q", envelope.Artifact.Stage)
	}
	for _, edge := range envelope.Artifact.Graph.Edges {
		switch edge.Kind {
		case "contains", "imports":
		default:
			t.Fatalf("reconnaissance adapter invented graph edge kind %q", edge.Kind)
		}
	}
	for _, file := range envelope.Artifact.Files {
		if file.Path == ".env" && (!file.Sensitive || file.Hash != "" || file.Bytes != 0 || file.Lines != 0) {
			t.Fatalf("sensitive file metadata leaked: %#v", file)
		}
	}
}
