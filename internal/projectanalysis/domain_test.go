package projectanalysis

import (
	"strings"
	"testing"
	"time"
)

func TestArtifactEnvelopeRejectsClaimWithoutEvidence(t *testing.T) {
	envelope := validEnvelope()
	envelope.Artifact.Claims = []Claim{{ID: "claim-1", Text: "A claim", Confidence: 0.8}}
	if err := envelope.Validate(); err == nil || !strings.Contains(err.Error(), "must cite evidence") {
		t.Fatalf("error = %v", err)
	}
}

func TestArtifactEnvelopeRejectsDanglingGraphEdge(t *testing.T) {
	envelope := validEnvelope()
	envelope.Artifact.Graph.Edges = []GraphEdge{{ID: "edge-1", Kind: "imports", From: "node-1", To: "missing", Confidence: 1, EvidenceID: []string{"ev-1"}}}
	if err := envelope.Validate(); err == nil || !strings.Contains(err.Error(), "dangling") {
		t.Fatalf("error = %v", err)
	}
}

func TestArtifactEnvelopeRejectsSensitiveEvidenceAndUnsafePaths(t *testing.T) {
	t.Run("sensitive evidence", func(t *testing.T) {
		envelope := validEnvelope()
		envelope.Artifact.Evidence[0].Sensitive = true
		if err := envelope.Validate(); err == nil || !strings.Contains(err.Error(), "sensitive") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("escaping path", func(t *testing.T) {
		envelope := validEnvelope()
		envelope.Artifact.Files[0].Path = "../secret.txt"
		if err := envelope.Validate(); err == nil || !strings.Contains(err.Error(), "workspace") {
			t.Fatalf("error = %v", err)
		}
	})
}

func validEnvelope() ArtifactEnvelope {
	return ArtifactEnvelope{SchemaVersion: SchemaVersion, StageVersion: 1, Artifact: AnalysisArtifact{
		Stage:       StageReconnaissance,
		Snapshot:    ProjectSnapshot{ProjectID: "project-1", Name: "test", Revision: RepositoryRevision{Provider: "git", Value: "abc123"}},
		Files:       []FileRecord{{Path: "main.go", Language: "Go", Hash: "hash", Lines: 1}},
		Evidence:    []EvidenceRef{{ID: "ev-1", Kind: "declaration", Location: SourceLocation{Path: "main.go", StartLine: 1, EndLine: 1}, SourceHash: "hash", Confidence: 1}},
		Graph:       KnowledgeGraph{Nodes: []GraphNode{{ID: "node-1", Kind: "file", Label: "main.go", EvidenceID: []string{"ev-1"}}}, Edges: []GraphEdge{}},
		GeneratedAt: time.Now(),
	}}
}
