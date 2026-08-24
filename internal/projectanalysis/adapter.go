package projectanalysis

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// BuildReconnaissanceArtifact adapts the existing local scanner output into
// the versioned contract. It intentionally does not infer calls, data flow,
// architecture, or knowledge-graph concepts.
func BuildReconnaissanceArtifact(result Result) (ArtifactEnvelope, error) {
	if result.ProjectID == "" {
		return ArtifactEnvelope{}, fmt.Errorf("reconnaissance result project ID is required")
	}
	artifact := AnalysisArtifact{
		Stage: StageReconnaissance,
		Snapshot: ProjectSnapshot{
			ProjectID: result.ProjectID,
			Name:      result.Name,
			Revision:  RepositoryRevision{Provider: "content-snapshot", Value: reconnaissanceRevision(result.Files)},
		},
		GeneratedAt: result.AnalyzedAt,
		Files:       make([]FileRecord, 0, len(result.Files)),
		Symbols:     make([]CodeSymbol, 0, len(result.Symbols)),
		Evidence:    make([]EvidenceRef, 0, len(result.Evidence)+len(result.Symbols)+len(result.Dependencies)),
		Graph:       KnowledgeGraph{Nodes: []GraphNode{}, Edges: []GraphEdge{}},
	}
	if artifact.GeneratedAt.IsZero() {
		return ArtifactEnvelope{}, fmt.Errorf("reconnaissance result analysis time is required")
	}

	fileHashes := make(map[string]string, len(result.Files))
	fileNodes := make(map[string]string, len(result.Files))
	projectNodeID := stableID("project", result.ProjectID)
	artifact.Graph.Nodes = append(artifact.Graph.Nodes, GraphNode{ID: projectNodeID, Kind: "project", Label: result.Name})
	for index, file := range result.Files {
		record := FileRecord{Path: file.Path, Language: file.Language, Hash: file.Hash, Bytes: file.Bytes, Lines: file.Lines, Sensitive: file.Sensitive}
		if file.Sensitive {
			record.Language, record.Hash, record.Bytes, record.Lines = "", "", 0, 0
		}
		artifact.Files = append(artifact.Files, record)
		fileHashes[file.Path] = record.Hash
		fileNodeID := stableID("file", file.Path)
		fileNodes[file.Path] = fileNodeID
		artifact.Graph.Nodes = append(artifact.Graph.Nodes, GraphNode{ID: fileNodeID, Kind: "file", Label: file.Path})
		artifact.Graph.Edges = append(artifact.Graph.Edges, GraphEdge{
			ID: stableID("contains-file", fmt.Sprintf("%d:%s", index, file.Path)), Kind: "contains", From: projectNodeID, To: fileNodeID, Confidence: 1,
		})
	}

	usedEvidence := map[string]bool{}
	for _, item := range result.Evidence {
		if usedEvidence[item.ID] {
			return ArtifactEnvelope{}, fmt.Errorf("duplicate reconnaissance evidence %q", item.ID)
		}
		usedEvidence[item.ID] = true
		artifact.Evidence = append(artifact.Evidence, EvidenceRef{
			ID: item.ID, Kind: item.Kind, Location: SourceLocation{Path: item.SourceFile, StartLine: item.Line, EndLine: item.Line},
			SourceHash: fileHashes[item.SourceFile], Confidence: item.Confidence,
		})
	}

	for index, symbol := range result.Symbols {
		fileNodeID, exists := fileNodes[symbol.File]
		if !exists {
			return ArtifactEnvelope{}, fmt.Errorf("symbol %q references unknown file %q", symbol.Name, symbol.File)
		}
		symbolID := stableID("symbol", fmt.Sprintf("%s:%d:%s:%s:%d", symbol.File, symbol.Line, symbol.Kind, symbol.Name, index))
		evidenceID := uniqueEvidenceID("recon-symbol", fmt.Sprintf("%s:%d:%d", symbol.File, symbol.Line, index), usedEvidence)
		location := SourceLocation{Path: symbol.File, StartLine: symbol.Line, EndLine: symbol.Line}
		artifact.Evidence = append(artifact.Evidence, EvidenceRef{ID: evidenceID, Kind: "declaration", Location: location, SourceHash: fileHashes[symbol.File], Confidence: reconnaissanceSymbolConfidence(symbol.File)})
		artifact.Symbols = append(artifact.Symbols, CodeSymbol{ID: symbolID, Name: symbol.Name, Kind: symbol.Kind, Location: location, Signature: symbol.Signature})
		artifact.Graph.Nodes = append(artifact.Graph.Nodes, GraphNode{ID: symbolID, Kind: "symbol", Label: symbol.Name, EvidenceID: []string{evidenceID}})
		artifact.Graph.Edges = append(artifact.Graph.Edges, GraphEdge{ID: stableID("contains-symbol", symbolID), Kind: "contains", From: fileNodeID, To: symbolID, Confidence: 1, EvidenceID: []string{evidenceID}})
	}

	externalNodes := map[string]string{}
	for index, dependency := range result.Dependencies {
		fileNodeID, exists := fileNodes[dependency.From]
		if !exists {
			return ArtifactEnvelope{}, fmt.Errorf("dependency target %q references unknown source file %q", dependency.To, dependency.From)
		}
		targetNodeID := externalNodes[dependency.To]
		if targetNodeID == "" {
			targetNodeID = stableID("dependency", dependency.To)
			externalNodes[dependency.To] = targetNodeID
			artifact.Graph.Nodes = append(artifact.Graph.Nodes, GraphNode{ID: targetNodeID, Kind: "external_dependency", Label: dependency.To})
		}
		location := SourceLocation{Path: dependency.Source, StartLine: dependency.Line, EndLine: dependency.Line}
		evidenceID := uniqueEvidenceID("recon-import", fmt.Sprintf("%s:%d:%s:%d", dependency.Source, dependency.Line, dependency.To, index), usedEvidence)
		artifact.Evidence = append(artifact.Evidence, EvidenceRef{ID: evidenceID, Kind: "import", Location: location, SourceHash: fileHashes[dependency.Source], Confidence: 0.9})
		referenceID := stableID("reference", fmt.Sprintf("%s:%s:%d", fileNodeID, targetNodeID, index))
		artifact.References = append(artifact.References, CodeReference{ID: referenceID, Kind: dependency.Kind, FromNodeID: fileNodeID, ToNodeID: targetNodeID, Location: location})
		artifact.Graph.Edges = append(artifact.Graph.Edges, GraphEdge{ID: stableID("import-edge", referenceID), Kind: "imports", From: fileNodeID, To: targetNodeID, Confidence: 0.9, EvidenceID: []string{evidenceID}})
	}

	envelope := ArtifactEnvelope{SchemaVersion: SchemaVersion, StageVersion: 1, Artifact: artifact}
	if err := envelope.Validate(); err != nil {
		return ArtifactEnvelope{}, err
	}
	return envelope, nil
}

func reconnaissanceRevision(files []File) string {
	items := make([]string, 0, len(files))
	for _, file := range files {
		items = append(items, strings.Join([]string{file.Path, file.Hash, fmt.Sprint(file.Sensitive)}, "\x00"))
	}
	sort.Strings(items)
	return stableID("snapshot", strings.Join(items, "\x01"))
}

func reconnaissanceSymbolConfidence(file string) float64 {
	if strings.EqualFold(pathExtension(file), ".go") {
		return 0.99
	}
	return 0.75
}

func pathExtension(value string) string {
	index := strings.LastIndex(value, ".")
	if index < 0 {
		return ""
	}
	return value[index:]
}

func uniqueEvidenceID(prefix, value string, used map[string]bool) string {
	id := stableID(prefix, value)
	for used[id] {
		id += "x"
	}
	used[id] = true
	return id
}

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(prefix + "\x00" + value))
	return prefix + "-" + hex.EncodeToString(sum[:8])
}
