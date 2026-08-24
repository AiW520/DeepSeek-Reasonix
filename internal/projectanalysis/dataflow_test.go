package projectanalysis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildGoDataFlowTracksLocalDefinitionsUsesArgumentsAndReturns(t *testing.T) {
	root := t.TempDir()
	writeDataFlowFixture(t, root, `package main
func sink(value int) {}
func compute(input int) int {
	value := input
	if input > 0 { value = input + 1 }
	sink(value)
	return value
}
`)
	flow := buildDataFlowFixture(t, root)
	summary, err := SummarizeDataFlow(flow)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Functions != 2 || summary.Definitions < 3 || summary.Uses < 5 || summary.FlowEdges < 5 {
		t.Fatalf("unexpected data flow summary: %#v", summary)
	}
	kinds := map[string]bool{}
	for _, node := range flow.Artifact.Graph.Nodes {
		kinds[node.Kind] = true
	}
	for _, kind := range []string{"parameter", "data_definition", "data_use", "call_argument", "return_value"} {
		if !kinds[kind] {
			t.Fatalf("data flow missing node kind %q", kind)
		}
	}
	for _, edge := range flow.Artifact.Graph.Edges {
		if edge.Kind == "flows_to" && len(edge.EvidenceID) == 0 {
			t.Fatalf("data flow edge lacks evidence: %#v", edge)
		}
	}
}

func TestBuildGoDataFlowDoesNotInventSourceForCallResult(t *testing.T) {
	root := t.TempDir()
	writeDataFlowFixture(t, root, `package main
func source() int { return 1 }
func main() { value := source(); _ = value }
`)
	flow := buildDataFlowFixture(t, root)
	for _, edge := range flow.Artifact.Graph.Edges {
		if edge.Kind != "flows_to" {
			continue
		}
		from, to := graphNode(flow.Artifact.Graph.Nodes, edge.From), graphNode(flow.Artifact.Graph.Nodes, edge.To)
		if from.Label == "source" || to.Label == "source" {
			t.Fatalf("call result was promoted to an invented value flow: %#v", edge)
		}
	}
}

func writeDataFlowFixture(t *testing.T, root, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/dataflow\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func buildDataFlowFixture(t *testing.T, root string) ArtifactEnvelope {
	t.Helper()
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
	symbols, err := ResolveGoSymbols(context.Background(), root, parsed)
	if err != nil {
		t.Fatal(err)
	}
	dependencies, err := BuildGoDependencyArtifact(symbols)
	if err != nil {
		t.Fatal(err)
	}
	callGraph, err := BuildGoCallGraph(symbols, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := BuildGoDataFlow(context.Background(), root, symbols, callGraph)
	if err != nil {
		t.Fatal(err)
	}
	return flow
}

func graphNode(nodes []GraphNode, id string) GraphNode {
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
	}
	return GraphNode{}
}
