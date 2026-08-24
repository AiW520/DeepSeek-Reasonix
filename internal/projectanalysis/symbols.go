package projectanalysis

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
)

// ResolveGoSymbols resolves package-level declarations and statically visible
// Go references. It reparses source from the versioned file index, so a stale
// file can never silently become a symbol fact.
func ResolveGoSymbols(ctx context.Context, root string, parsed ArtifactEnvelope) (ArtifactEnvelope, error) {
	if err := parsed.Validate(); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("parse artifact: %w", err)
	}
	if parsed.Artifact.Stage != StageParse {
		return ArtifactEnvelope{}, fmt.Errorf("symbol resolution requires parse artifact, got %q", parsed.Artifact.Stage)
	}
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	if info, statErr := os.Stat(abs); statErr != nil || !info.IsDir() {
		return ArtifactEnvelope{}, errors.New("symbol resolution root is not an accessible directory")
	}

	artifact := AnalysisArtifact{
		Stage:       StageSymbols,
		Snapshot:    parsed.Artifact.Snapshot,
		Files:       append([]FileRecord(nil), parsed.Artifact.Files...),
		Diagnostics: append([]ParseDiagnostic(nil), parsed.Artifact.Diagnostics...),
		Symbols:     []CodeSymbol{},
		References:  []CodeReference{},
		Evidence:    []EvidenceRef{},
		Graph:       KnowledgeGraph{Nodes: []GraphNode{}, Edges: []GraphEdge{}},
		GeneratedAt: parsed.Artifact.GeneratedAt,
	}
	modulePath := readGoModulePath(abs, &artifact.Diagnostics)
	files, packages, err := loadGoFiles(ctx, abs, artifact.Files, modulePath, &artifact.Diagnostics)
	if err != nil {
		return ArtifactEnvelope{}, err
	}

	fileNodes := make(map[string]string, len(files))
	packageNodes := make(map[string]string, len(packages))
	for _, file := range files {
		fileNodes[file.rel] = stableID("file", file.rel)
		artifact.Graph.Nodes = append(artifact.Graph.Nodes, GraphNode{ID: fileNodes[file.rel], Kind: "file", Label: file.rel})
	}
	packageKeys := make([]string, 0, len(packages))
	for key := range packages {
		packageKeys = append(packageKeys, key)
	}
	sort.Strings(packageKeys)
	for _, key := range packageKeys {
		pkg := packages[key]
		packageNodes[key] = stableID("package", key)
		artifact.Graph.Nodes = append(artifact.Graph.Nodes, GraphNode{ID: packageNodes[key], Kind: "package", Label: packageLabel(pkg)})
		for _, file := range pkg.files {
			artifact.Graph.Edges = append(artifact.Graph.Edges, GraphEdge{ID: stableID("package-file", pkg.importPath+"\x00"+file.rel), Kind: "contains", From: packageNodes[key], To: fileNodes[file.rel], Confidence: 1})
		}
	}

	symbolIDs := make(map[string]string)
	functionIDs := make(map[*ast.FuncDecl]string)
	usedEvidence := make(map[string]bool)
	for _, file := range files {
		pkg := packages[file.packageKey]
		for _, declaration := range file.ast.Decls {
			switch node := declaration.(type) {
			case *ast.FuncDecl:
				name := node.Name.Name
				kind := "function"
				key := pkg.importPath + "::" + name
				if node.Recv != nil {
					kind = "method"
					receiver := receiverName(node.Recv)
					key = pkg.importPath + "::" + receiver + "." + name
				}
				id, _ := addResolvedSymbol(&artifact, file, node.Name, name, kind, key, usedEvidence)
				if id != "" {
					symbolIDs[key] = id
					functionIDs[node] = id
				}
			case *ast.GenDecl:
				for _, specification := range node.Specs {
					switch spec := specification.(type) {
					case *ast.TypeSpec:
						name := spec.Name.Name
						key := pkg.importPath + "::" + name
						id, _ := addResolvedSymbol(&artifact, file, spec.Name, name, "type", key, usedEvidence)
						if id != "" {
							symbolIDs[key] = id
						}
					case *ast.ValueSpec:
						kind := "var"
						if node.Tok.String() == "const" {
							kind = "const"
						}
						for _, nameNode := range spec.Names {
							name := nameNode.Name
							key := pkg.importPath + "::" + name
							id, _ := addResolvedSymbol(&artifact, file, nameNode, name, kind, key, usedEvidence)
							if id != "" {
								symbolIDs[key] = id
							}
						}
					}
				}
			}
		}
	}

	externalNodes := make(map[string]string)
	for _, file := range files {
		pkg := packages[file.packageKey]
		aliases := make(map[string]*goPackage)
		for _, spec := range file.ast.Imports {
			importPath := strings.Trim(spec.Path.Value, "\"")
			imported := packages[importPath]
			if imported != nil {
				aliases[importAlias(spec, imported)] = imported
			}
			targetID := ""
			if imported != nil {
				targetID = packageNodes[imported.importPath]
			} else {
				targetID = externalNodes[importPath]
				if targetID == "" {
					targetID = stableID("external", importPath)
					externalNodes[importPath] = targetID
					artifact.Graph.Nodes = append(artifact.Graph.Nodes, GraphNode{ID: targetID, Kind: "external_dependency", Label: importPath})
				}
			}
			if targetID == "" {
				continue
			}
			location := locationFor(file.fset, spec)
			evidenceID := addEvidence(&artifact, file, location, "import", 0.98, usedEvidence)
			artifact.Graph.Edges = append(artifact.Graph.Edges, GraphEdge{ID: stableID("imports", file.rel+"\x00"+importPath), Kind: "imports", From: fileNodes[file.rel], To: targetID, Confidence: 0.98, EvidenceID: []string{evidenceID}})
			artifact.References = append(artifact.References, CodeReference{ID: stableID("import-reference", file.rel+"\x00"+importPath), Kind: "import", FromNodeID: fileNodes[file.rel], ToNodeID: targetID, Location: location})
		}

		for _, declaration := range file.ast.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			fromID := functionIDs[function]
			if fromID == "" {
				continue
			}
			seen := make(map[string]bool)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name, target := resolveCallTarget(call.Fun, pkg, aliases, symbolIDs, packageNodes, externalNodes)
				if target == "" || name == "" || seen[target] {
					return true
				}
				seen[target] = true
				location := locationFor(file.fset, call.Fun)
				evidenceID := addEvidence(&artifact, file, location, "reference", 0.9, usedEvidence)
				kind := "references"
				if strings.Contains(name, "call:") {
					kind = "calls"
				}
				artifact.Graph.Edges = append(artifact.Graph.Edges, GraphEdge{ID: stableID("reference-edge", fromID+"\x00"+target+"\x00"+location.Path+fmt.Sprint(location.StartLine)), Kind: kind, From: fromID, To: target, Confidence: 0.9, EvidenceID: []string{evidenceID}})
				artifact.References = append(artifact.References, CodeReference{ID: stableID("code-reference", fromID+"\x00"+target+"\x00"+location.Path+fmt.Sprint(location.StartLine)), Kind: kind, FromNodeID: fromID, ToNodeID: target, Location: location})
				return true
			})
		}
	}

	envelope := ArtifactEnvelope{SchemaVersion: SchemaVersion, StageVersion: 1, Artifact: artifact}
	if err := envelope.Validate(); err != nil {
		return ArtifactEnvelope{}, err
	}
	return envelope, nil
}

func SummarizeResolutionArtifact(artifact ArtifactEnvelope) (ResolutionSummary, error) {
	if err := artifact.Validate(); err != nil {
		return ResolutionSummary{}, err
	}
	if artifact.Artifact.Stage != StageSymbols {
		return ResolutionSummary{}, fmt.Errorf("resolution summary requires symbol artifact, got %q", artifact.Artifact.Stage)
	}
	summary := ResolutionSummary{Stage: string(StageSymbols), ResolvedSymbols: len(artifact.Artifact.Symbols), Diagnostics: len(artifact.Artifact.Diagnostics), SupportedLanguages: []string{"Go"}}
	packageRefs := map[string]bool{}
	for _, node := range artifact.Artifact.Graph.Nodes {
		switch node.Kind {
		case "package":
			summary.LocalPackages++
		case "external_dependency":
			summary.ExternalDependencies++
		}
	}
	for _, reference := range artifact.Artifact.References {
		switch reference.Kind {
		case "calls", "references":
			summary.LocalReferences++
		case "import":
			if !packageRefs[reference.ToNodeID] {
				packageRefs[reference.ToNodeID] = true
				summary.ImportedPackages++
			}
		}
	}
	return summary, nil
}

type goSourceFile struct {
	rel, packageKey string
	ast             *ast.File
	fset            *gotoken.FileSet
}

type goPackage struct {
	importPath, name, dir string
	files                 []*goSourceFile
}

func loadGoFiles(ctx context.Context, root string, records []FileRecord, modulePath string, diagnostics *[]ParseDiagnostic) ([]*goSourceFile, map[string]*goPackage, error) {
	files := make([]*goSourceFile, 0)
	packages := make(map[string]*goPackage)
	for _, record := range records {
		if record.Sensitive || record.Language != "Go" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		full, err := safeAnalysisPath(root, record.Path)
		if err != nil {
			return nil, nil, err
		}
		source, err := os.ReadFile(full)
		if err != nil {
			*diagnostics = append(*diagnostics, ParseDiagnostic{Path: record.Path, Severity: "error", Message: "file could not be read during symbol resolution"})
			continue
		}
		if record.Hash != "" && contentHash(source) != record.Hash {
			*diagnostics = append(*diagnostics, ParseDiagnostic{Path: record.Path, Severity: "stale", Message: "file changed before symbol resolution"})
			continue
		}
		fset := gotoken.NewFileSet()
		file, parseErr := parser.ParseFile(fset, record.Path, source, parser.AllErrors)
		if parseErr != nil {
			*diagnostics = append(*diagnostics, ParseDiagnostic{Path: record.Path, Severity: "error", Message: "Go syntax could not be resolved"})
			continue
		}
		dir := path.Dir(record.Path)
		if dir == "." {
			dir = ""
		}
		importPath := modulePath
		if dir != "" && modulePath != "" {
			importPath += "/" + filepath.ToSlash(dir)
		}
		key := importPath
		if key == "" {
			key = "local:" + dir
		}
		pkg := packages[key]
		if pkg == nil {
			pkg = &goPackage{importPath: key, name: file.Name.Name, dir: dir}
			packages[key] = pkg
		}
		if pkg.name != file.Name.Name {
			*diagnostics = append(*diagnostics, ParseDiagnostic{Path: record.Path, Severity: "error", Message: "files in one Go package declare different package names"})
			continue
		}
		parsed := &goSourceFile{rel: record.Path, packageKey: key, ast: file, fset: fset}
		pkg.files = append(pkg.files, parsed)
		files = append(files, parsed)
	}
	return files, packages, nil
}

func readGoModulePath(root string, diagnostics *[]ParseDiagnostic) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		*diagnostics = append(*diagnostics, ParseDiagnostic{Path: "go.mod", Severity: "unsupported", Message: "Go module path unavailable; local imports will remain unresolved"})
		return ""
	}
	file, err := modfile.Parse("go.mod", data, nil)
	if err != nil || file.Module == nil || file.Module.Mod.Path == "" {
		*diagnostics = append(*diagnostics, ParseDiagnostic{Path: "go.mod", Severity: "error", Message: "Go module path could not be read"})
		return ""
	}
	return file.Module.Mod.Path
}

func packageLabel(pkg *goPackage) string {
	if pkg.importPath != "" {
		return pkg.importPath
	}
	return pkg.name
}

func importAlias(spec *ast.ImportSpec, pkg *goPackage) string {
	if spec.Name != nil && spec.Name.Name != "" {
		return spec.Name.Name
	}
	return pkg.name
}

func receiverName(field *ast.FieldList) string {
	if field == nil || len(field.List) == 0 {
		return ""
	}
	var expression ast.Expr = field.List[0].Type
	if pointer, ok := expression.(*ast.StarExpr); ok {
		expression = pointer.X
	}
	if identifier, ok := expression.(*ast.Ident); ok {
		return identifier.Name
	}
	if selector, ok := expression.(*ast.SelectorExpr); ok {
		return selector.Sel.Name
	}
	return ""
}

func addResolvedSymbol(artifact *AnalysisArtifact, file *goSourceFile, nameNode *ast.Ident, name, kind, key string, usedEvidence map[string]bool) (string, string) {
	if nameNode == nil || name == "" {
		return "", ""
	}
	location := locationFor(file.fset, nameNode)
	evidenceID := addEvidence(artifact, file, location, "declaration", 0.99, usedEvidence)
	symbolID := stableID("resolved-symbol", key+"\x00"+file.rel+fmt.Sprint(location.StartLine))
	artifact.Symbols = append(artifact.Symbols, CodeSymbol{ID: symbolID, Name: name, Kind: kind, Location: location})
	artifact.Graph.Nodes = append(artifact.Graph.Nodes, GraphNode{ID: symbolID, Kind: "symbol", Label: name, EvidenceID: []string{evidenceID}})
	artifact.Graph.Edges = append(artifact.Graph.Edges, GraphEdge{ID: stableID("package-symbol", key+"\x00"+symbolID), Kind: "contains", From: stableID("package", file.packageKey), To: symbolID, Confidence: 1, EvidenceID: []string{evidenceID}})
	return symbolID, evidenceID
}

func addEvidence(artifact *AnalysisArtifact, file *goSourceFile, location SourceLocation, kind string, confidence float64, used map[string]bool) string {
	id := uniqueEvidenceID("symbol-"+kind, location.Path+fmt.Sprintf(":%d:%d", location.StartLine, location.EndLine), used)
	artifact.Evidence = append(artifact.Evidence, EvidenceRef{ID: id, Kind: kind, Location: location, Confidence: confidence})
	return id
}

func locationFor(fset *gotoken.FileSet, node ast.Node) SourceLocation {
	start := fset.Position(node.Pos())
	end := fset.Position(node.End())
	return SourceLocation{Path: start.Filename, StartLine: start.Line, EndLine: end.Line}
}

func resolveCallTarget(expression ast.Expr, current *goPackage, aliases map[string]*goPackage, symbols map[string]string, packageNodes, externalNodes map[string]string) (string, string) {
	switch node := expression.(type) {
	case *ast.Ident:
		if node.Obj != nil && node.Obj.Kind != ast.Fun {
			return "", ""
		}
		if id := symbols[current.importPath+"::"+node.Name]; id != "" {
			return "call:" + node.Name, id
		}
	case *ast.SelectorExpr:
		identifier, ok := node.X.(*ast.Ident)
		if !ok {
			return "", ""
		}
		pkg := aliases[identifier.Name]
		if pkg == nil {
			return "", ""
		}
		if id := symbols[pkg.importPath+"::"+node.Sel.Name]; id != "" {
			return "call:" + node.Sel.Name, id
		}
		if id := packageNodes[pkg.importPath]; id != "" {
			return "package:" + pkg.importPath, id
		}
		if id := externalNodes[pkg.importPath]; id != "" {
			return "package:" + pkg.importPath, id
		}
	}
	return "", ""
}
