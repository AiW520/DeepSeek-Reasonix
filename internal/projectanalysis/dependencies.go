package projectanalysis

import (
	"fmt"
	"time"
)

// BuildGoDependencyArtifact keeps formal import and reference edges separate
// from the symbol artifact so later call/data-flow stages can consume a small,
// stable dependency contract.
func BuildGoDependencyArtifact(symbols ArtifactEnvelope) (ArtifactEnvelope, error) {
	if err := symbols.Validate(); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("symbol artifact: %w", err)
	}
	if symbols.Artifact.Stage != StageSymbols {
		return ArtifactEnvelope{}, fmt.Errorf("dependency resolution requires symbol artifact, got %q", symbols.Artifact.Stage)
	}
	artifact := AnalysisArtifact{
		Stage:       StageDependencies,
		Snapshot:    symbols.Artifact.Snapshot,
		Files:       append([]FileRecord(nil), symbols.Artifact.Files...),
		Symbols:     append([]CodeSymbol(nil), symbols.Artifact.Symbols...),
		References:  append([]CodeReference(nil), symbols.Artifact.References...),
		Evidence:    append([]EvidenceRef(nil), symbols.Artifact.Evidence...),
		Diagnostics: append([]ParseDiagnostic(nil), symbols.Artifact.Diagnostics...),
		Graph:       cloneKnowledgeGraph(symbols.Artifact.Graph),
		GeneratedAt: time.Now(),
	}
	envelope := ArtifactEnvelope{SchemaVersion: SchemaVersion, StageVersion: 1, Artifact: artifact}
	if err := envelope.Validate(); err != nil {
		return ArtifactEnvelope{}, err
	}
	return envelope, nil
}

func cloneKnowledgeGraph(graph KnowledgeGraph) KnowledgeGraph {
	return KnowledgeGraph{
		Nodes: append([]GraphNode(nil), graph.Nodes...),
		Edges: append([]GraphEdge(nil), graph.Edges...),
	}
}
