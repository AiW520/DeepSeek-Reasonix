package projectanalysis

import (
	"fmt"
	"time"
)

// BuildRepositoryIndexArtifact promotes the scanner's immutable file snapshot
// into the pipeline. It carries no source content and performs no filesystem IO.
func BuildRepositoryIndexArtifact(reconnaissance ArtifactEnvelope) (ArtifactEnvelope, error) {
	if err := reconnaissance.Validate(); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("reconnaissance artifact: %w", err)
	}
	if reconnaissance.Artifact.Stage != StageReconnaissance {
		return ArtifactEnvelope{}, fmt.Errorf("repository index requires reconnaissance artifact, got %q", reconnaissance.Artifact.Stage)
	}
	artifact := AnalysisArtifact{
		Stage:       StageRepositoryIndex,
		Snapshot:    reconnaissance.Artifact.Snapshot,
		Files:       append([]FileRecord(nil), reconnaissance.Artifact.Files...),
		Graph:       KnowledgeGraph{Nodes: []GraphNode{}, Edges: []GraphEdge{}},
		GeneratedAt: time.Now(),
	}
	envelope := ArtifactEnvelope{SchemaVersion: SchemaVersion, StageVersion: 1, Artifact: artifact}
	if err := envelope.Validate(); err != nil {
		return ArtifactEnvelope{}, err
	}
	return envelope, nil
}
