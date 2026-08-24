package projectanalysis

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type RepositoryRevision struct {
	Provider string `json:"provider"`
	Value    string `json:"value"`
	Dirty    bool   `json:"dirty,omitempty"`
}

type ProjectSnapshot struct {
	ProjectID string             `json:"projectId"`
	Name      string             `json:"name"`
	Revision  RepositoryRevision `json:"revision"`
}

type SourceLocation struct {
	Path      string `json:"path"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

type FileRecord struct {
	Path      string `json:"path"`
	Language  string `json:"language,omitempty"`
	Hash      string `json:"hash,omitempty"`
	Bytes     int64  `json:"bytes,omitempty"`
	Lines     int    `json:"lines,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

type CodeSymbol struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Kind      string         `json:"kind"`
	Location  SourceLocation `json:"location"`
	Signature string         `json:"signature,omitempty"`
}

type CodeReference struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	FromNodeID string         `json:"fromNodeId"`
	ToNodeID   string         `json:"toNodeId"`
	Location   SourceLocation `json:"location"`
}

type SyntaxNode struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Named      bool           `json:"named"`
	Location   SourceLocation `json:"location"`
	TextHash   string         `json:"textHash,omitempty"`
	ChildCount int            `json:"childCount,omitempty"`
}

type ParseDiagnostic struct {
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line,omitempty"`
}

type EvidenceRef struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Location   SourceLocation `json:"location"`
	SourceHash string         `json:"sourceHash,omitempty"`
	Confidence float64        `json:"confidence"`
	Sensitive  bool           `json:"sensitive,omitempty"`
}

type GraphNode struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Label      string   `json:"label"`
	EvidenceID []string `json:"evidenceIds,omitempty"`
}

type GraphEdge struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	From       string   `json:"from"`
	To         string   `json:"to"`
	Confidence float64  `json:"confidence"`
	EvidenceID []string `json:"evidenceIds,omitempty"`
}

type Claim struct {
	ID         string   `json:"id"`
	Text       string   `json:"text"`
	Confidence float64  `json:"confidence"`
	EvidenceID []string `json:"evidenceIds"`
}

type KnowledgeGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type AnalysisArtifact struct {
	Stage       StageID           `json:"stage"`
	Snapshot    ProjectSnapshot   `json:"snapshot"`
	Files       []FileRecord      `json:"files,omitempty"`
	Syntax      []SyntaxNode      `json:"syntax,omitempty"`
	Diagnostics []ParseDiagnostic `json:"diagnostics,omitempty"`
	Symbols     []CodeSymbol      `json:"symbols,omitempty"`
	References  []CodeReference   `json:"references,omitempty"`
	Evidence    []EvidenceRef     `json:"evidence,omitempty"`
	Claims      []Claim           `json:"claims,omitempty"`
	Graph       KnowledgeGraph    `json:"graph"`
	GeneratedAt time.Time         `json:"generatedAt"`
}

type ArtifactEnvelope struct {
	SchemaVersion int              `json:"schemaVersion"`
	StageVersion  int              `json:"stageVersion"`
	Artifact      AnalysisArtifact `json:"artifact"`
}

func (envelope ArtifactEnvelope) Validate() error {
	if envelope.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported artifact schema version %d", envelope.SchemaVersion)
	}
	if envelope.StageVersion <= 0 {
		return errors.New("artifact stage version must be positive")
	}
	artifact := envelope.Artifact
	if artifact.Stage == "" || artifact.Snapshot.ProjectID == "" {
		return errors.New("artifact stage and project ID are required")
	}
	if artifact.Snapshot.Revision.Provider == "" || artifact.Snapshot.Revision.Value == "" {
		return errors.New("artifact repository revision provider and value are required")
	}
	if artifact.GeneratedAt.IsZero() {
		return errors.New("artifact generation time is required")
	}
	files := map[string]bool{}
	for _, file := range artifact.Files {
		if err := validateWorkspacePath(file.Path); err != nil {
			return fmt.Errorf("file %q: %w", file.Path, err)
		}
		if files[file.Path] {
			return fmt.Errorf("file path %q is duplicated", file.Path)
		}
		files[file.Path] = true
		if file.Bytes < 0 || file.Lines < 0 {
			return fmt.Errorf("file %q has negative size metadata", file.Path)
		}
		if file.Sensitive && (file.Hash != "" || file.Language != "" || file.Bytes != 0 || file.Lines != 0) {
			return fmt.Errorf("sensitive file %q contains derived content metadata", file.Path)
		}
	}
	evidence := make(map[string]bool, len(artifact.Evidence))
	for _, item := range artifact.Evidence {
		if item.ID == "" || evidence[item.ID] {
			return fmt.Errorf("evidence ID %q is empty or duplicated", item.ID)
		}
		if item.Sensitive {
			return fmt.Errorf("sensitive evidence %q is forbidden", item.ID)
		}
		if err := validateLocation(item.Location); err != nil {
			return fmt.Errorf("evidence %q: %w", item.ID, err)
		}
		if err := validateConfidence(item.Confidence); err != nil {
			return fmt.Errorf("evidence %q: %w", item.ID, err)
		}
		evidence[item.ID] = true
	}
	symbols := map[string]bool{}
	for _, symbol := range artifact.Symbols {
		if symbol.ID == "" || symbol.Name == "" {
			return errors.New("symbol ID and name are required")
		}
		if symbols[symbol.ID] {
			return fmt.Errorf("symbol ID %q is duplicated", symbol.ID)
		}
		symbols[symbol.ID] = true
		if err := validateLocation(symbol.Location); err != nil {
			return fmt.Errorf("symbol %q: %w", symbol.ID, err)
		}
	}
	syntaxNodes := map[string]bool{}
	for _, syntax := range artifact.Syntax {
		if syntax.ID == "" || syntax.Kind == "" {
			return errors.New("syntax node ID and kind are required")
		}
		if syntaxNodes[syntax.ID] {
			return fmt.Errorf("syntax node ID %q is duplicated", syntax.ID)
		}
		syntaxNodes[syntax.ID] = true
		if err := validateLocation(syntax.Location); err != nil {
			return fmt.Errorf("syntax node %q: %w", syntax.ID, err)
		}
		if syntax.ChildCount < 0 {
			return fmt.Errorf("syntax node %q has negative child count", syntax.ID)
		}
	}
	for _, diagnostic := range artifact.Diagnostics {
		if err := validateWorkspacePath(diagnostic.Path); err != nil {
			return fmt.Errorf("parse diagnostic: %w", err)
		}
		if diagnostic.Severity == "" || strings.TrimSpace(diagnostic.Message) == "" {
			return errors.New("parse diagnostic severity and message are required")
		}
		if diagnostic.Line < 0 {
			return errors.New("parse diagnostic line cannot be negative")
		}
	}
	if err := validateGraph(artifact.Graph, evidence); err != nil {
		return err
	}
	graphNodes := make(map[string]bool, len(artifact.Graph.Nodes))
	for _, node := range artifact.Graph.Nodes {
		graphNodes[node.ID] = true
	}
	references := map[string]bool{}
	for _, reference := range artifact.References {
		if reference.ID == "" || reference.Kind == "" || reference.FromNodeID == "" || reference.ToNodeID == "" {
			return errors.New("reference ID, kind, and endpoints are required")
		}
		if references[reference.ID] {
			return fmt.Errorf("reference ID %q is duplicated", reference.ID)
		}
		references[reference.ID] = true
		if !graphNodes[reference.FromNodeID] || !graphNodes[reference.ToNodeID] {
			return fmt.Errorf("reference %q has a dangling endpoint", reference.ID)
		}
		if err := validateLocation(reference.Location); err != nil {
			return fmt.Errorf("reference %q: %w", reference.ID, err)
		}
	}
	for _, claim := range artifact.Claims {
		if claim.ID == "" || strings.TrimSpace(claim.Text) == "" {
			return errors.New("claim ID and text are required")
		}
		if err := validateConfidence(claim.Confidence); err != nil {
			return fmt.Errorf("claim %q: %w", claim.ID, err)
		}
		if len(claim.EvidenceID) == 0 {
			return fmt.Errorf("claim %q must cite evidence", claim.ID)
		}
		if err := validateEvidenceIDs(claim.EvidenceID, evidence); err != nil {
			return fmt.Errorf("claim %q: %w", claim.ID, err)
		}
	}
	return nil
}

func validateGraph(graph KnowledgeGraph, evidence map[string]bool) error {
	nodes := make(map[string]bool, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if node.ID == "" || node.Kind == "" || nodes[node.ID] {
			return fmt.Errorf("graph node ID %q is empty or duplicated", node.ID)
		}
		if err := validateEvidenceIDs(node.EvidenceID, evidence); err != nil {
			return fmt.Errorf("graph node %q: %w", node.ID, err)
		}
		nodes[node.ID] = true
	}
	edges := map[string]bool{}
	for _, edge := range graph.Edges {
		if edge.ID == "" || edge.Kind == "" || edges[edge.ID] {
			return fmt.Errorf("graph edge ID %q is empty or duplicated", edge.ID)
		}
		if !nodes[edge.From] || !nodes[edge.To] {
			return fmt.Errorf("graph edge %q has a dangling endpoint", edge.ID)
		}
		if err := validateConfidence(edge.Confidence); err != nil {
			return fmt.Errorf("graph edge %q: %w", edge.ID, err)
		}
		if err := validateEvidenceIDs(edge.EvidenceID, evidence); err != nil {
			return fmt.Errorf("graph edge %q: %w", edge.ID, err)
		}
		edges[edge.ID] = true
	}
	return nil
}

func validateEvidenceIDs(ids []string, available map[string]bool) error {
	for _, id := range ids {
		if !available[id] {
			return fmt.Errorf("references unknown evidence %q", id)
		}
	}
	return nil
}

func validateConfidence(value float64) error {
	if value < 0 || value > 1 {
		return fmt.Errorf("confidence %.3f is outside [0,1]", value)
	}
	return nil
}

func validateLocation(location SourceLocation) error {
	if err := validateWorkspacePath(location.Path); err != nil {
		return err
	}
	if location.StartLine < 1 || location.EndLine < location.StartLine {
		return errors.New("source line range is invalid")
	}
	return nil
}

func validateWorkspacePath(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || path.IsAbs(value) || filepath.IsAbs(value) {
		return errors.New("path must be a workspace-relative slash path")
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return errors.New("path escapes or is not normalized relative to the workspace")
	}
	return nil
}
