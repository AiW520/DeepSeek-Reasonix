package projectanalysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	gotoken "go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const maxSyntaxNodes = 200000

// ParseProject builds a real syntax artifact from a previously validated
// reconnaissance artifact. Go uses the standard library AST in the default
// build. Other languages are reported as unsupported until their optional
// parser worker is enabled; no regex declaration is promoted to AST fact.
func ParseProject(ctx context.Context, root string, repositoryIndex ArtifactEnvelope) (ArtifactEnvelope, error) {
	if err := repositoryIndex.Validate(); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("repository index artifact: %w", err)
	}
	if repositoryIndex.Artifact.Stage != StageRepositoryIndex {
		return ArtifactEnvelope{}, fmt.Errorf("parse requires repository index artifact, got %q", repositoryIndex.Artifact.Stage)
	}
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return ArtifactEnvelope{}, errors.New("parse root is not an accessible directory")
	}
	artifact := AnalysisArtifact{
		Stage:       StageParse,
		Snapshot:    repositoryIndex.Artifact.Snapshot,
		Files:       append([]FileRecord(nil), repositoryIndex.Artifact.Files...),
		Syntax:      []SyntaxNode{},
		Diagnostics: []ParseDiagnostic{},
		Graph:       KnowledgeGraph{Nodes: []GraphNode{}, Edges: []GraphEdge{}},
		GeneratedAt: time.Now(),
	}
	for _, file := range repositoryIndex.Artifact.Files {
		if err := ctx.Err(); err != nil {
			return ArtifactEnvelope{}, err
		}
		if file.Sensitive {
			artifact.Diagnostics = append(artifact.Diagnostics, ParseDiagnostic{Path: file.Path, Severity: "blocked", Message: "sensitive file was not read"})
			continue
		}
		if file.Language == "" {
			continue
		}
		if file.Language != "Go" {
			artifact.Diagnostics = append(artifact.Diagnostics, ParseDiagnostic{Path: file.Path, Severity: "unsupported", Message: "formal parser is not enabled for this language"})
			continue
		}
		remainingNodes := maxSyntaxNodes - len(artifact.Syntax)
		if remainingNodes <= 0 {
			artifact.Diagnostics = append(artifact.Diagnostics, ParseDiagnostic{Path: file.Path, Severity: "limit", Message: "project syntax node limit reached; remaining files were not parsed"})
			break
		}
		full, err := safeAnalysisPath(abs, file.Path)
		if err != nil {
			return ArtifactEnvelope{}, err
		}
		source, err := os.ReadFile(full)
		if err != nil {
			artifact.Diagnostics = append(artifact.Diagnostics, ParseDiagnostic{Path: file.Path, Severity: "error", Message: "file could not be read"})
			continue
		}
		if file.Hash != "" && contentHash(source) != file.Hash {
			artifact.Diagnostics = append(artifact.Diagnostics, ParseDiagnostic{Path: file.Path, Severity: "stale", Message: "file changed after reconnaissance; parse skipped"})
			continue
		}
		nodes, diagnostics := parseGoSyntax(ctx, file.Path, source, remainingNodes)
		artifact.Syntax = append(artifact.Syntax, nodes...)
		artifact.Diagnostics = append(artifact.Diagnostics, diagnostics...)
	}
	envelope := ArtifactEnvelope{SchemaVersion: SchemaVersion, StageVersion: 1, Artifact: artifact}
	if err := envelope.Validate(); err != nil {
		return ArtifactEnvelope{}, err
	}
	return envelope, nil
}

func SummarizeParseArtifact(artifact ArtifactEnvelope) (ParseSummary, error) {
	if err := artifact.Validate(); err != nil {
		return ParseSummary{}, err
	}
	if artifact.Artifact.Stage != StageParse {
		return ParseSummary{}, fmt.Errorf("parse summary requires parse artifact, got %q", artifact.Artifact.Stage)
	}
	summary := ParseSummary{Stage: string(StageParse), SyntaxNodes: len(artifact.Artifact.Syntax), DiagnosticCount: len(artifact.Artifact.Diagnostics), ParserLanguages: []string{"Go"}}
	parsedPaths := map[string]bool{}
	for _, node := range artifact.Artifact.Syntax {
		parsedPaths[node.Location.Path] = true
	}
	summary.ParsedFiles = len(parsedPaths)
	for _, diagnostic := range artifact.Artifact.Diagnostics {
		switch diagnostic.Severity {
		case "unsupported":
			summary.UnsupportedFiles = appendUnique(summary.UnsupportedFiles, diagnostic.Path)
		case "blocked":
			summary.BlockedFiles = appendUnique(summary.BlockedFiles, diagnostic.Path)
		case "stale":
			summary.StaleFiles = appendUnique(summary.StaleFiles, diagnostic.Path)
		}
	}
	return summary, nil
}

func appendUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func parseGoSyntax(ctx context.Context, rel string, source []byte, nodeLimit int) ([]SyntaxNode, []ParseDiagnostic) {
	fset := gotoken.NewFileSet()
	file, parseErr := parser.ParseFile(fset, rel, source, parser.AllErrors|parser.ParseComments)
	diagnostics := []ParseDiagnostic{}
	if parseErr != nil {
		var list scanner.ErrorList
		if errors.As(parseErr, &list) {
			for _, item := range list {
				diagnostics = append(diagnostics, ParseDiagnostic{Path: rel, Severity: "error", Message: item.Error(), Line: item.Pos.Line})
			}
		} else {
			diagnostics = append(diagnostics, ParseDiagnostic{Path: rel, Severity: "error", Message: parseErr.Error()})
		}
	}
	if file == nil {
		return nil, diagnostics
	}
	nodes := make([]SyntaxNode, 0, 128)
	var walkErr error
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil || walkErr != nil {
			return walkErr == nil
		}
		if err := ctx.Err(); err != nil {
			walkErr = err
			return false
		}
		if len(nodes) >= nodeLimit {
			walkErr = errors.New("syntax node limit exceeded")
			return false
		}
		start := fset.PositionFor(node.Pos(), false)
		end := fset.PositionFor(node.End(), false)
		if start.Line < 1 || end.Line < start.Line {
			return true
		}
		textHash := ""
		startOffset := fset.File(node.Pos()).Offset(node.Pos())
		endOffset := fset.File(node.End()).Offset(node.End())
		if startOffset >= 0 && endOffset >= startOffset && endOffset <= len(source) && endOffset-startOffset <= 4096 {
			textHash = contentHash(source[startOffset:endOffset])
		}
		kind := fmt.Sprintf("go.%s", nodeKind(node))
		nodes = append(nodes, SyntaxNode{
			ID:   stableID("syntax", fmt.Sprintf("%s:%d:%d:%s:%d", rel, start.Line, end.Line, kind, len(nodes))),
			Kind: kind, Named: true,
			Location: SourceLocation{Path: rel, StartLine: start.Line, EndLine: end.Line},
			// ChildCount is reserved for parser-specific structural metadata. The
			// standard Go AST does not expose direct children cheaply, so leaving it
			// unset avoids an O(n^2) traversal on large syntax trees.
			TextHash: textHash,
		})
		return true
	})
	if walkErr != nil {
		diagnostics = append(diagnostics, ParseDiagnostic{Path: rel, Severity: "error", Message: walkErr.Error()})
	}
	return nodes, diagnostics
}

func nodeKind(node ast.Node) string {
	value := fmt.Sprintf("%T", node)
	if index := strings.LastIndex(value, "."); index >= 0 {
		value = value[index+1:]
	}
	return strings.TrimPrefix(value, "*")
}

func safeAnalysisPath(root, rel string) (string, error) {
	if err := validateWorkspacePath(rel); err != nil {
		return "", fmt.Errorf("parse path %q: %w", rel, err)
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	cleanFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(cleanRoot, cleanFull)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("parse path %q escapes project root", rel)
	}
	return cleanFull, nil
}

func contentHash(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}
