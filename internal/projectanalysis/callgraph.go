package projectanalysis

import (
	"fmt"
	"sort"
	"time"
)

type CallGraphSummary struct {
	Stage              string   `json:"stage"`
	Functions          int      `json:"functions"`
	CallEdges          int      `json:"callEdges"`
	SupportedLanguages []string `json:"supportedLanguages,omitempty"`
}

// BuildGoCallGraph projects only evidence-backed calls already resolved by the
// symbol stage. It does not infer interface dispatch, reflection, or runtime
// callback targets.
func BuildGoCallGraph(symbols, dependencies ArtifactEnvelope) (ArtifactEnvelope, error) {
	if err := symbols.Validate(); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("symbol artifact: %w", err)
	}
	if err := dependencies.Validate(); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("dependency artifact: %w", err)
	}
	if symbols.Artifact.Stage != StageSymbols || dependencies.Artifact.Stage != StageDependencies {
		return ArtifactEnvelope{}, fmt.Errorf("call graph requires symbols and dependencies artifacts")
	}
	if symbols.Artifact.Snapshot != dependencies.Artifact.Snapshot {
		return ArtifactEnvelope{}, fmt.Errorf("call graph inputs belong to different project snapshots")
	}

	evidenceByID := make(map[string]EvidenceRef, len(dependencies.Artifact.Evidence))
	for _, item := range dependencies.Artifact.Evidence {
		evidenceByID[item.ID] = item
	}
	nodeByID := make(map[string]GraphNode, len(dependencies.Artifact.Graph.Nodes))
	for _, node := range dependencies.Artifact.Graph.Nodes {
		nodeByID[node.ID] = node
	}
	selectedNodes := map[string]GraphNode{}
	selectedEvidence := map[string]EvidenceRef{}
	edges := make([]GraphEdge, 0)
	references := make([]CodeReference, 0)
	for _, edge := range dependencies.Artifact.Graph.Edges {
		if edge.Kind != "calls" {
			continue
		}
		from, fromOK := nodeByID[edge.From]
		to, toOK := nodeByID[edge.To]
		if !fromOK || !toOK || from.Kind != "symbol" || to.Kind != "symbol" {
			continue
		}
		selectedNodes[from.ID], selectedNodes[to.ID] = from, to
		edges = append(edges, edge)
		for _, id := range edge.EvidenceID {
			if evidence, ok := evidenceByID[id]; ok {
				selectedEvidence[id] = evidence
			}
		}
	}
	for _, reference := range dependencies.Artifact.References {
		if reference.Kind == "calls" && selectedNodes[reference.FromNodeID].ID != "" && selectedNodes[reference.ToNodeID].ID != "" {
			references = append(references, reference)
		}
	}
	nodeIDs := make([]string, 0, len(selectedNodes))
	for id := range selectedNodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	nodes := make([]GraphNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		node := selectedNodes[id]
		filtered := make([]string, 0, len(node.EvidenceID))
		for _, evidenceID := range node.EvidenceID {
			if evidence, ok := evidenceByID[evidenceID]; ok {
				selectedEvidence[evidenceID] = evidence
				filtered = append(filtered, evidenceID)
			}
		}
		node.EvidenceID = append([]string(nil), filtered...)
		nodes = append(nodes, node)
	}
	evidenceIDs := make([]string, 0, len(selectedEvidence))
	for id := range selectedEvidence {
		evidenceIDs = append(evidenceIDs, id)
	}
	sort.Strings(evidenceIDs)
	evidence := make([]EvidenceRef, 0, len(evidenceIDs))
	for _, id := range evidenceIDs {
		evidence = append(evidence, selectedEvidence[id])
	}
	artifact := AnalysisArtifact{
		Stage: StageCallGraph, Snapshot: symbols.Artifact.Snapshot,
		Files: append([]FileRecord(nil), symbols.Artifact.Files...), References: references,
		Evidence: evidence, Graph: KnowledgeGraph{Nodes: nodes, Edges: edges}, GeneratedAt: time.Now(),
	}
	envelope := ArtifactEnvelope{SchemaVersion: SchemaVersion, StageVersion: 1, Artifact: artifact}
	if err := envelope.Validate(); err != nil {
		return ArtifactEnvelope{}, err
	}
	return envelope, nil
}

func SummarizeCallGraph(artifact ArtifactEnvelope) (CallGraphSummary, error) {
	if err := artifact.Validate(); err != nil {
		return CallGraphSummary{}, err
	}
	if artifact.Artifact.Stage != StageCallGraph {
		return CallGraphSummary{}, fmt.Errorf("call graph summary requires call graph artifact")
	}
	return CallGraphSummary{Stage: string(StageCallGraph), Functions: len(artifact.Artifact.Graph.Nodes), CallEdges: len(artifact.Artifact.Graph.Edges), SupportedLanguages: []string{"Go"}}, nil
}
