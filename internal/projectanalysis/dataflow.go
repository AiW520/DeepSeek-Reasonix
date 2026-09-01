package projectanalysis

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxDataFlowNodes = 200000
	maxDataFlowEdges = 400000
)

type DataFlowSummary struct {
	Stage              string   `json:"stage"`
	Functions          int      `json:"functions"`
	Definitions        int      `json:"definitions"`
	Uses               int      `json:"uses"`
	FlowEdges          int      `json:"flowEdges"`
	Diagnostics        int      `json:"diagnostics"`
	SupportedLanguages []string `json:"supportedLanguages,omitempty"`
}

// BuildGoDataFlow computes conservative, function-local def-use edges. It does
// not infer pointer aliases, heap identity, goroutine ordering, reflection, or
// inter-procedural value propagation.
func BuildGoDataFlow(ctx context.Context, root string, symbols, callGraph ArtifactEnvelope) (ArtifactEnvelope, error) {
	if err := symbols.Validate(); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("symbol artifact: %w", err)
	}
	if err := callGraph.Validate(); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("call graph artifact: %w", err)
	}
	if symbols.Artifact.Stage != StageSymbols || callGraph.Artifact.Stage != StageCallGraph {
		return ArtifactEnvelope{}, errors.New("data flow requires symbols and call graph artifacts")
	}
	if symbols.Artifact.Snapshot != callGraph.Artifact.Snapshot {
		return ArtifactEnvelope{}, errors.New("data flow inputs belong to different project snapshots")
	}
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return ArtifactEnvelope{}, err
	}

	artifact := AnalysisArtifact{
		Stage: StageDataFlow, Snapshot: symbols.Artifact.Snapshot,
		Files:       append([]FileRecord(nil), symbols.Artifact.Files...),
		Diagnostics: append([]ParseDiagnostic(nil), symbols.Artifact.Diagnostics...),
		Symbols:     append([]CodeSymbol(nil), symbols.Artifact.Symbols...),
		Graph:       KnowledgeGraph{Nodes: []GraphNode{}, Edges: []GraphEdge{}}, GeneratedAt: time.Now(),
	}
	files, _, err := loadGoFiles(ctx, abs, artifact.Files, "", &artifact.Diagnostics)
	if err != nil {
		return ArtifactEnvelope{}, err
	}

	evidenceByID := make(map[string]EvidenceRef, len(symbols.Artifact.Evidence))
	for _, evidence := range symbols.Artifact.Evidence {
		evidenceByID[evidence.ID] = evidence
	}
	graphNodeByID := make(map[string]GraphNode, len(symbols.Artifact.Graph.Nodes))
	for _, node := range symbols.Artifact.Graph.Nodes {
		graphNodeByID[node.ID] = node
	}
	functionByKey := make(map[string]string)
	usedEvidence := make(map[string]bool)
	for _, symbol := range symbols.Artifact.Symbols {
		if symbol.Kind != "function" && symbol.Kind != "method" {
			continue
		}
		key := functionLocationKey(symbol.Location.Path, symbol.Location.StartLine, symbol.Name)
		functionByKey[key] = symbol.ID
		if node, ok := graphNodeByID[symbol.ID]; ok {
			artifact.Graph.Nodes = append(artifact.Graph.Nodes, node)
			for _, evidenceID := range node.EvidenceID {
				if evidence, exists := evidenceByID[evidenceID]; exists && !usedEvidence[evidenceID] {
					usedEvidence[evidenceID] = true
					artifact.Evidence = append(artifact.Evidence, evidence)
				}
			}
		}
	}

	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return ArtifactEnvelope{}, err
		}
		for _, declaration := range file.ast.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			line := file.fset.Position(function.Name.Pos()).Line
			functionID := functionByKey[functionLocationKey(file.rel, line, function.Name.Name)]
			if functionID == "" {
				continue
			}
			builder := &goDataFlowBuilder{artifact: &artifact, file: file, functionID: functionID, usedEvidence: usedEvidence}
			env := make(flowEnvironment)
			builder.defineFields(function.Type.Params, "parameter", env)
			builder.defineFields(function.Type.Results, "result", env)
			builder.analyzeStatements(ctx, function.Body.List, env)
			if err := ctx.Err(); err != nil {
				return ArtifactEnvelope{}, err
			}
			if builder.limitReached {
				artifact.Diagnostics = append(artifact.Diagnostics, ParseDiagnostic{Path: file.rel, Severity: "limit", Message: "project data-flow node limit reached; remaining facts were skipped"})
				break
			}
		}
		if len(artifact.Graph.Nodes) >= maxDataFlowNodes {
			break
		}
	}

	sort.Slice(artifact.Graph.Nodes, func(i, j int) bool { return artifact.Graph.Nodes[i].ID < artifact.Graph.Nodes[j].ID })
	sort.Slice(artifact.Graph.Edges, func(i, j int) bool { return artifact.Graph.Edges[i].ID < artifact.Graph.Edges[j].ID })
	sort.Slice(artifact.References, func(i, j int) bool { return artifact.References[i].ID < artifact.References[j].ID })
	sort.Slice(artifact.Evidence, func(i, j int) bool { return artifact.Evidence[i].ID < artifact.Evidence[j].ID })
	envelope := ArtifactEnvelope{SchemaVersion: SchemaVersion, StageVersion: 1, Artifact: artifact}
	if err := envelope.Validate(); err != nil {
		return ArtifactEnvelope{}, err
	}
	return envelope, nil
}

func SummarizeDataFlow(artifact ArtifactEnvelope) (DataFlowSummary, error) {
	if err := artifact.Validate(); err != nil {
		return DataFlowSummary{}, err
	}
	if artifact.Artifact.Stage != StageDataFlow {
		return DataFlowSummary{}, errors.New("data flow summary requires data flow artifact")
	}
	summary := DataFlowSummary{Stage: string(StageDataFlow), Diagnostics: len(artifact.Artifact.Diagnostics), SupportedLanguages: []string{"Go"}}
	for _, node := range artifact.Artifact.Graph.Nodes {
		switch node.Kind {
		case "symbol":
			summary.Functions++
		case "data_definition", "parameter", "result":
			summary.Definitions++
		case "data_use", "call_argument", "return_value":
			summary.Uses++
		}
	}
	for _, edge := range artifact.Artifact.Graph.Edges {
		if edge.Kind == "flows_to" {
			summary.FlowEdges++
		}
	}
	return summary, nil
}

// flowEnvironment tracks definitions by the parser's object identity. The
// identity is intentionally opaque here so this analysis does not depend on
// the deprecated concrete ast.Object type.
type flowEnvironment map[any][]string

type goDataFlowBuilder struct {
	artifact     *AnalysisArtifact
	file         *goSourceFile
	functionID   string
	usedEvidence map[string]bool
	limitReached bool
}

func (builder *goDataFlowBuilder) defineFields(fields *ast.FieldList, kind string, env flowEnvironment) {
	if fields == nil {
		return
	}
	for _, field := range fields.List {
		for _, name := range field.Names {
			builder.define(name, kind, env)
		}
	}
}

func (builder *goDataFlowBuilder) analyzeStatements(ctx context.Context, statements []ast.Stmt, env flowEnvironment) flowEnvironment {
	for _, statement := range statements {
		if ctx.Err() != nil || builder.limitReached {
			return env
		}
		switch node := statement.(type) {
		case *ast.AssignStmt:
			rhsDefinitions := make([][]string, len(node.Rhs))
			for index, rhs := range node.Rhs {
				rhsDefinitions[index] = sourceDefinitions(rhs, env)
				builder.useExpression(rhs, "data_use", env)
			}
			for index, lhs := range node.Lhs {
				if name, ok := lhs.(*ast.Ident); ok {
					definitionID := builder.define(name, "data_definition", env)
					if len(node.Rhs) == 1 {
						builder.connectDefinitions(rhsDefinitions[0], definitionID, name)
					} else if index < len(rhsDefinitions) {
						builder.connectDefinitions(rhsDefinitions[index], definitionID, name)
					}
				} else {
					builder.useExpression(lhs, "data_use", env)
				}
			}
		case *ast.DeclStmt:
			if declaration, ok := node.Decl.(*ast.GenDecl); ok {
				for _, item := range declaration.Specs {
					if value, ok := item.(*ast.ValueSpec); ok {
						rhsDefinitions := make([][]string, len(value.Values))
						for index, expression := range value.Values {
							rhsDefinitions[index] = sourceDefinitions(expression, env)
							builder.useExpression(expression, "data_use", env)
						}
						for index, name := range value.Names {
							definitionID := builder.define(name, "data_definition", env)
							if len(value.Values) == 1 {
								builder.connectDefinitions(rhsDefinitions[0], definitionID, name)
							} else if index < len(rhsDefinitions) {
								builder.connectDefinitions(rhsDefinitions[index], definitionID, name)
							}
						}
					}
				}
			}
		case *ast.ExprStmt:
			builder.useExpression(node.X, "data_use", env)
		case *ast.ReturnStmt:
			for _, expression := range node.Results {
				builder.useExpression(expression, "return_value", env)
			}
		case *ast.IncDecStmt:
			builder.useExpression(node.X, "data_use", env)
			if name, ok := node.X.(*ast.Ident); ok {
				builder.define(name, "data_definition", env)
			}
		case *ast.IfStmt:
			base := cloneFlowEnvironment(env)
			if node.Init != nil {
				base = builder.analyzeStatements(ctx, []ast.Stmt{node.Init}, base)
			}
			builder.useExpression(node.Cond, "data_use", base)
			thenEnv := builder.analyzeStatements(ctx, node.Body.List, cloneFlowEnvironment(base))
			elseEnv := cloneFlowEnvironment(base)
			if node.Else != nil {
				elseEnv = builder.analyzeStatements(ctx, []ast.Stmt{node.Else}, elseEnv)
			}
			env = mergeFlowEnvironments(thenEnv, elseEnv)
		case *ast.BlockStmt:
			env = builder.analyzeStatements(ctx, node.List, env)
		case *ast.ForStmt:
			loopBase := cloneFlowEnvironment(env)
			if node.Init != nil {
				loopBase = builder.analyzeStatements(ctx, []ast.Stmt{node.Init}, loopBase)
			}
			builder.useExpression(node.Cond, "data_use", loopBase)
			bodyEnv := builder.analyzeStatements(ctx, node.Body.List, cloneFlowEnvironment(loopBase))
			if node.Post != nil {
				bodyEnv = builder.analyzeStatements(ctx, []ast.Stmt{node.Post}, bodyEnv)
			}
			env = mergeFlowEnvironments(loopBase, bodyEnv)
		case *ast.RangeStmt:
			builder.useExpression(node.X, "data_use", env)
			bodyEnv := cloneFlowEnvironment(env)
			if name, ok := node.Key.(*ast.Ident); ok {
				builder.define(name, "data_definition", bodyEnv)
			}
			if name, ok := node.Value.(*ast.Ident); ok {
				builder.define(name, "data_definition", bodyEnv)
			}
			bodyEnv = builder.analyzeStatements(ctx, node.Body.List, bodyEnv)
			env = mergeFlowEnvironments(env, bodyEnv)
		default:
			ast.Inspect(statement, func(child ast.Node) bool {
				if expression, ok := child.(ast.Expr); ok {
					builder.useExpression(expression, "data_use", env)
					return false
				}
				return true
			})
		}
	}
	return env
}

func (builder *goDataFlowBuilder) useExpression(expression ast.Expr, kind string, env flowEnvironment) {
	if expression == nil || builder.limitReached {
		return
	}
	if call, ok := expression.(*ast.CallExpr); ok {
		builder.useExpression(call.Fun, "data_use", env)
		for _, argument := range call.Args {
			builder.useExpression(argument, "call_argument", env)
		}
		return
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		name, ok := node.(*ast.Ident)
		if !ok || name.Obj == nil {
			return true
		}
		definitions := env[name.Obj]
		if len(definitions) == 0 {
			return true
		}
		builder.addUse(name, kind, definitions)
		return true
	})
}

func (builder *goDataFlowBuilder) define(name *ast.Ident, kind string, env flowEnvironment) string {
	if name == nil || name.Name == "_" || name.Obj == nil || builder.limitReached {
		return ""
	}
	nodeID := builder.addValueNode(name, kind)
	if nodeID != "" {
		env[name.Obj] = []string{nodeID}
	}
	return nodeID
}

func (builder *goDataFlowBuilder) connectDefinitions(sources []string, target string, node ast.Node) {
	if target == "" || len(sources) == 0 {
		return
	}
	location := locationFor(builder.file.fset, node)
	evidenceID := builder.artifact.Graph.Nodes[len(builder.artifact.Graph.Nodes)-1].EvidenceID
	for _, source := range sources {
		if !builder.canAddEdge() {
			return
		}
		builder.artifact.Graph.Edges = append(builder.artifact.Graph.Edges, GraphEdge{ID: stableID("data-flow-assignment", source+"\x00"+target), Kind: "flows_to", From: source, To: target, Confidence: 0.9, EvidenceID: evidenceID})
		builder.artifact.References = append(builder.artifact.References, CodeReference{ID: stableID("data-flow-assignment-reference", source+"\x00"+target), Kind: "flows_to", FromNodeID: source, ToNodeID: target, Location: location})
	}
}

func sourceDefinitions(expression ast.Expr, env flowEnvironment) []string {
	if expression == nil {
		return nil
	}
	if _, call := expression.(*ast.CallExpr); call {
		return nil
	}
	seen := make(map[string]bool)
	definitions := []string{}
	ast.Inspect(expression, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		name, ok := node.(*ast.Ident)
		if !ok || name.Obj == nil {
			return true
		}
		for _, definition := range env[name.Obj] {
			if !seen[definition] {
				seen[definition] = true
				definitions = append(definitions, definition)
			}
		}
		return true
	})
	return definitions
}

func (builder *goDataFlowBuilder) addUse(name *ast.Ident, kind string, definitions []string) {
	useID := builder.addValueNode(name, kind)
	if useID == "" {
		return
	}
	location := locationFor(builder.file.fset, name)
	for _, definitionID := range definitions {
		if !builder.canAddEdge() {
			return
		}
		edgeID := stableID("data-flow-edge", definitionID+"\x00"+useID)
		builder.artifact.Graph.Edges = append(builder.artifact.Graph.Edges, GraphEdge{ID: edgeID, Kind: "flows_to", From: definitionID, To: useID, Confidence: 0.9, EvidenceID: builder.artifact.Graph.Nodes[len(builder.artifact.Graph.Nodes)-1].EvidenceID})
		builder.artifact.References = append(builder.artifact.References, CodeReference{ID: stableID("data-flow-reference", edgeID), Kind: "flows_to", FromNodeID: definitionID, ToNodeID: useID, Location: location})
	}
}

func (builder *goDataFlowBuilder) addValueNode(name *ast.Ident, kind string) string {
	if len(builder.artifact.Graph.Nodes) >= maxDataFlowNodes {
		builder.limitReached = true
		return ""
	}
	location := locationFor(builder.file.fset, name)
	position := builder.file.fset.PositionFor(name.Pos(), false)
	id := stableID("data-value", builder.functionID+"\x00"+kind+fmt.Sprintf("\x00%s:%d", builder.file.rel, position.Offset))
	evidenceID := addEvidence(builder.artifact, builder.file, location, "data-flow", 0.9, builder.usedEvidence)
	builder.artifact.Graph.Nodes = append(builder.artifact.Graph.Nodes, GraphNode{ID: id, Kind: kind, Label: name.Name, EvidenceID: []string{evidenceID}})
	if !builder.canAddEdge() {
		return id
	}
	builder.artifact.Graph.Edges = append(builder.artifact.Graph.Edges, GraphEdge{ID: stableID("function-value", builder.functionID+"\x00"+id), Kind: "contains", From: builder.functionID, To: id, Confidence: 1, EvidenceID: []string{evidenceID}})
	return id
}

func (builder *goDataFlowBuilder) canAddEdge() bool {
	if len(builder.artifact.Graph.Edges) >= maxDataFlowEdges {
		builder.limitReached = true
		return false
	}
	return true
}

func functionLocationKey(path string, line int, name string) string {
	return path + fmt.Sprintf(":%d:", line) + name
}

func cloneFlowEnvironment(source flowEnvironment) flowEnvironment {
	clone := make(flowEnvironment, len(source))
	for object, definitions := range source {
		clone[object] = append([]string(nil), definitions...)
	}
	return clone
}

func mergeFlowEnvironments(left, right flowEnvironment) flowEnvironment {
	merged := cloneFlowEnvironment(left)
	for object, definitions := range right {
		seen := make(map[string]bool)
		for _, id := range merged[object] {
			seen[id] = true
		}
		for _, id := range definitions {
			if !seen[id] {
				merged[object] = append(merged[object], id)
				seen[id] = true
			}
		}
	}
	return merged
}
