// Package projectanalysis contains the safe, local-first project reconnaissance
// engine used by the desktop learning workspace. It deliberately stores
// metadata and evidence locations, never source contents or secrets.
package projectanalysis

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/projectiondb"
)

const (
	maxFiles     = 30000
	maxFileBytes = 2 << 20
	maxSymbols   = 100000
	maxEvidence  = 2000
	projectionV1 = 1
)

type Progress struct {
	Phase string `json:"phase"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

type File struct {
	Path      string `json:"path"`
	Language  string `json:"language,omitempty"`
	Bytes     int64  `json:"bytes"`
	Lines     int    `json:"lines"`
	Hash      string `json:"hash"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Signature string `json:"signature,omitempty"`
}

type Dependency struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Line   int    `json:"line"`
}

type Evidence struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Title      string  `json:"title"`
	SourceFile string  `json:"sourceFile"`
	Line       int     `json:"line"`
	Confidence float64 `json:"confidence"`
	Detail     string  `json:"detail,omitempty"`
}

type ParseSummary struct {
	Stage            string   `json:"stage"`
	ParsedFiles      int      `json:"parsedFiles"`
	SyntaxNodes      int      `json:"syntaxNodes"`
	UnsupportedFiles []string `json:"unsupportedFiles,omitempty"`
	BlockedFiles     []string `json:"blockedFiles,omitempty"`
	StaleFiles       []string `json:"staleFiles,omitempty"`
	DiagnosticCount  int      `json:"diagnosticCount"`
	ParserLanguages  []string `json:"parserLanguages,omitempty"`
}

type ResolutionSummary struct {
	Stage                string   `json:"stage"`
	ResolvedSymbols      int      `json:"resolvedSymbols"`
	LocalPackages        int      `json:"localPackages"`
	LocalReferences      int      `json:"localReferences"`
	ImportedPackages     int      `json:"importedPackages"`
	ExternalDependencies int      `json:"externalDependencies"`
	Diagnostics          int      `json:"diagnostics"`
	SupportedLanguages   []string `json:"supportedLanguages,omitempty"`
}

type Result struct {
	ProjectID      string             `json:"projectId"`
	Root           string             `json:"root"`
	Name           string             `json:"name"`
	AnalyzedAt     time.Time          `json:"analyzedAt"`
	DurationMs     int64              `json:"durationMs"`
	Files          []File             `json:"files"`
	Symbols        []Symbol           `json:"symbols"`
	Dependencies   []Dependency       `json:"dependencies"`
	Evidence       []Evidence         `json:"evidence"`
	Languages      []string           `json:"languages"`
	Frameworks     []string           `json:"frameworks"`
	PackageManager string             `json:"packageManager,omitempty"`
	SensitiveFiles []string           `json:"sensitiveFiles"`
	SkippedFiles   int                `json:"skippedFiles"`
	Errors         []string           `json:"errors"`
	Parse          *ParseSummary      `json:"parse,omitempty"`
	Resolution     *ResolutionSummary `json:"resolution,omitempty"`
	CallGraph      *CallGraphSummary  `json:"callGraph,omitempty"`
	DataFlow       *DataFlowSummary   `json:"dataFlow,omitempty"`
}

type ProgressFunc func(Progress)

var sensitiveName = regexp.MustCompile(`(?i)^(\.env(?:\..*)?|\.npmrc|\.pypirc|credentials|kubeconfig|secrets?(?:\..*)?|id_rsa|id_ed25519|.*\.(pem|key|p12|pfx|crt|jks|keystore))$`)
var declPatterns = map[string]*regexp.Regexp{
	"TypeScript": regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?(?:function|class|interface|type|enum)\s+([A-Za-z_$][\w$]*)|^\s*(?:export\s+)?const\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^\n]*\)|[A-Za-z_$][\w$]*)\s*=>`),
	"JavaScript": regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?(?:function|class)\s+([A-Za-z_$][\w$]*)|^\s*(?:export\s+)?const\s+([A-Za-z_$][\w$]*)\s*=\s*(?:async\s*)?(?:\([^\n]*\)|[A-Za-z_$][\w$]*)\s*=>`),
	"Python":     regexp.MustCompile(`(?m)^\s*(?:async\s+)?(?:def|class)\s+([A-Za-z_]\w*)`),
	"Rust":       regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:async\s+)?(?:fn|struct|enum|trait|impl)\s+([A-Za-z_]\w*)`),
}

var moduleImportPattern = regexp.MustCompile(`(?m)^\s*(?:import\s+(?:[^\n]*?\s+from\s+)?|from\s+|require\s*\()\s*["']([^"']+)["']`)
var rustUsePattern = regexp.MustCompile(`(?m)^\s*use\s+([^;\s]+)`)

func Analyze(ctx context.Context, root string, onProgress ProgressFunc) (Result, error) {
	start := time.Now()
	abs, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Result{}, fmt.Errorf("project root: %w", err)
	}
	if !info.IsDir() {
		return Result{}, errors.New("project root is not a directory")
	}
	if onProgress == nil {
		onProgress = func(Progress) {}
	}
	result := Result{ProjectID: projectID(abs), Root: abs, Name: filepath.Base(abs), AnalyzedAt: time.Now(), Files: []File{}, Symbols: []Symbol{}, Dependencies: []Dependency{}, Evidence: []Evidence{}, SensitiveFiles: []string{}, Errors: []string{}}
	onProgress(Progress{Phase: "安全预检", Done: 0, Total: 1})
	if err := ctx.Err(); err != nil {
		return result, err
	}

	var paths []string
	err = filepath.WalkDir(abs, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			result.Errors = append(result.Errors, safeRel(abs, path)+": "+walkErr.Error())
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != abs && skipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(paths) >= maxFiles {
			result.SkippedFiles++
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return result, err
	}
	sort.Strings(paths)
	onProgress(Progress{Phase: "扫描文件", Done: 0, Total: len(paths)})
	languages := map[string]bool{}
	frameworks := map[string]bool{}
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		rel := safeRel(abs, path)
		base := filepath.Base(path)
		if sensitiveName.MatchString(base) {
			result.SensitiveFiles = append(result.SensitiveFiles, rel)
			result.Files = append(result.Files, File{Path: rel, Sensitive: true})
			result.SkippedFiles++
			onProgress(Progress{Phase: "扫描文件", Done: i + 1, Total: len(paths)})
			continue
		}
		st, statErr := os.Stat(path)
		if statErr != nil || st.Size() > maxFileBytes {
			result.SkippedFiles++
			continue
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			result.Errors = append(result.Errors, rel+": "+readErr.Error())
			continue
		}
		lang := languageFor(path)
		if lang != "" {
			languages[lang] = true
		}
		file := File{Path: rel, Language: lang, Bytes: st.Size(), Lines: lineCount(body), Hash: hash(body)}
		result.Files = append(result.Files, file)
		if isManifest(base) || base == "README.md" || base == "README" {
			detectManifest(resultPtr(&result), rel, base, body, frameworks)
		}
		if len(result.Symbols) < maxSymbols && lang != "" {
			result.Symbols = append(result.Symbols, extractSymbols(abs, rel, lang, body)...)
		}
		result.Dependencies = append(result.Dependencies, extractDependencies(rel, lang, body)...)
		onProgress(Progress{Phase: "建立符号与依赖", Done: i + 1, Total: len(paths)})
	}
	for lang := range languages {
		result.Languages = append(result.Languages, lang)
	}
	for fw := range frameworks {
		result.Frameworks = append(result.Frameworks, fw)
	}
	sort.Strings(result.Languages)
	sort.Strings(result.Frameworks)
	sort.Strings(result.SensitiveFiles)
	if len(result.Evidence) > maxEvidence {
		result.Evidence = result.Evidence[:maxEvidence]
	}
	result.DurationMs = time.Since(start).Milliseconds()
	onProgress(Progress{Phase: "完成", Done: len(paths), Total: len(paths)})
	return result, nil
}

// resultPtr makes the manifest helper's intent explicit without exposing
// mutable analysis state outside this package.
func resultPtr(r *Result) *Result { return r }

func projectID(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:8])
}
func hash(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func lineCount(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	return 1 + strings.Count(string(body), "\n")
}
func safeRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}
func skipDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".reasonix", "node_modules", "vendor", "dist", "build", "target", ".idea", ".venv", "__pycache__":
		return true
	}
	return false
}
func languageFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "Go"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "JavaScript"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".cpp", ".cc", ".h", ".hpp":
		return "C++"
	}
	return ""
}
func isManifest(base string) bool {
	switch strings.ToLower(base) {
	case "go.mod", "package.json", "pnpm-lock.yaml", "yarn.lock", "package-lock.json", "pyproject.toml", "requirements.txt", "cargo.toml", "dockerfile", "compose.yaml", "docker-compose.yml":
		return true
	}
	return false
}

func detectManifest(result *Result, rel, base string, body []byte, frameworks map[string]bool) {
	text := string(body)
	addEvidence := func(title, detail string, confidence float64) {
		if len(result.Evidence) < maxEvidence {
			id := fmt.Sprintf("ev-%03d", len(result.Evidence)+1)
			result.Evidence = append(result.Evidence, Evidence{ID: id, Kind: "manifest", Title: title, SourceFile: rel, Line: 1, Confidence: confidence, Detail: detail})
		}
	}
	switch strings.ToLower(base) {
	case "go.mod":
		result.PackageManager = "Go modules"
		addEvidence("Go module manifest", "go.mod confirms a Go module", 0.99)
		frameworks["Go"] = true
	case "package.json":
		result.PackageManager = "npm-compatible"
		addEvidence("JavaScript package manifest", "package.json is present", 0.99)
		for _, name := range []string{"react", "vue", "next", "vite", "electron", "wails"} {
			if strings.Contains(text, "\""+name+"\"") {
				frameworks[name] = true
			}
		}
	case "pnpm-lock.yaml":
		result.PackageManager = "pnpm"
		addEvidence("pnpm lockfile", "pnpm-lock.yaml is present", 0.99)
	case "yarn.lock":
		result.PackageManager = "Yarn"
		addEvidence("Yarn lockfile", "yarn.lock is present", 0.99)
	case "package-lock.json":
		result.PackageManager = "npm"
		addEvidence("npm lockfile", "package-lock.json is present", 0.99)
	case "pyproject.toml":
		result.PackageManager = "Python packaging"
		addEvidence("Python project manifest", "pyproject.toml is present", 0.99)
		frameworks["Python"] = true
	case "requirements.txt":
		result.PackageManager = "pip"
		addEvidence("Python requirements", "requirements.txt is present", 0.95)
	case "cargo.toml":
		result.PackageManager = "Cargo"
		addEvidence("Rust manifest", "Cargo.toml is present", 0.99)
		frameworks["Rust"] = true
	case "dockerfile", "compose.yaml", "docker-compose.yml":
		frameworks["Docker"] = true
		addEvidence("Container configuration", base+" is present", 0.96)
	}
}

func extractSymbols(root, rel, lang string, body []byte) []Symbol {
	if lang == "Go" {
		return extractGoSymbols(filepath.Join(root, filepath.FromSlash(rel)), rel)
	}
	re := declPatterns[lang]
	if re == nil {
		return nil
	}
	text := string(body)
	lines := strings.Split(text, "\n")
	matches := re.FindAllStringSubmatchIndex(text, -1)
	out := make([]Symbol, 0, len(matches))
	for _, match := range matches {
		name := firstCapture(text, match)
		if name == "" {
			continue
		}
		line := 1 + strings.Count(text[:match[0]], "\n")
		declaration := text[match[0]:match[1]]
		kind := "function"
		switch {
		case strings.Contains(declaration, "interface"):
			kind = "interface"
		case strings.Contains(declaration, "class"):
			kind = "class"
		case strings.Contains(declaration, "enum"):
			kind = "enum"
		case strings.Contains(declaration, "type") || strings.Contains(declaration, "struct") || strings.Contains(declaration, "trait"):
			kind = "type"
		}
		signature := ""
		if line-1 < len(lines) {
			signature = strings.TrimSpace(lines[line-1])
		}
		out = append(out, Symbol{Name: name, Kind: kind, File: rel, Line: line, Signature: signature})
	}
	return out
}

func firstCapture(text string, match []int) string {
	for i := 2; i+1 < len(match); i += 2 {
		if match[i] >= 0 && match[i+1] >= match[i] {
			return text[match[i]:match[i+1]]
		}
	}
	return ""
}

func extractGoSymbols(path, rel string) []Symbol {
	fset := gotoken.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil
	}
	out := []Symbol{}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncDecl:
			kind := "function"
			if n.Recv != nil {
				kind = "method"
			}
			out = append(out, Symbol{Name: n.Name.Name, Kind: kind, File: rel, Line: fset.Position(n.Pos()).Line, Signature: "func " + n.Name.Name})
		case *ast.TypeSpec:
			out = append(out, Symbol{Name: n.Name.Name, Kind: "type", File: rel, Line: fset.Position(n.Pos()).Line, Signature: n.Name.Name})
		}
		return true
	})
	return out
}

func extractDependencies(rel, lang string, body []byte) []Dependency {
	if lang == "Go" {
		fset := gotoken.NewFileSet()
		file, err := parser.ParseFile(fset, rel, body, parser.ImportsOnly)
		if err == nil {
			out := make([]Dependency, 0, len(file.Imports))
			for _, spec := range file.Imports {
				out = append(out, Dependency{From: rel, To: strings.Trim(spec.Path.Value, "\""), Kind: "import", Source: rel, Line: fset.Position(spec.Pos()).Line})
			}
			return out
		}
	}
	text := string(body)
	out := dependenciesFromPattern(rel, text, moduleImportPattern)
	if lang == "Rust" {
		out = append(out, dependenciesFromPattern(rel, text, rustUsePattern)...)
	}
	return out
}

func dependenciesFromPattern(rel, text string, pattern *regexp.Regexp) []Dependency {
	matches := pattern.FindAllStringSubmatchIndex(text, -1)
	out := make([]Dependency, 0, len(matches))
	for _, match := range matches {
		if len(match) < 4 || match[2] < 0 {
			continue
		}
		target := text[match[2]:match[3]]
		out = append(out, Dependency{From: rel, To: target, Kind: "import", Source: rel, Line: 1 + strings.Count(text[:match[0]], "\n")})
	}
	return out
}

func projectionMigrations() []projectiondb.Migration {
	return []projectiondb.Migration{{Version: projectionV1, Apply: func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `CREATE TABLE analysis_projects (project_id TEXT PRIMARY KEY, root TEXT NOT NULL, result_json BLOB NOT NULL, analyzed_at INTEGER NOT NULL)`)
		return err
	}}}
}

func Store(ctx context.Context, result Result) (string, error) {
	root := config.CacheDir()
	if root == "" {
		return "", errors.New("cache directory unavailable")
	}
	path := filepath.Join(root, "project-analysis", result.ProjectID+".sqlite")
	handle, err := projectiondb.Open(ctx, projectiondb.OpenOptions{Path: path, Migrations: projectionMigrations(), MaxOpenConns: 1, SecureDelete: true, AutoVacuum: true})
	if err != nil {
		return "", err
	}
	defer handle.DB.Close()
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	_, err = handle.DB.ExecContext(ctx, `INSERT INTO analysis_projects(project_id, root, result_json, analyzed_at) VALUES(?,?,?,?) ON CONFLICT(project_id) DO UPDATE SET root=excluded.root,result_json=excluded.result_json,analyzed_at=excluded.analyzed_at`, result.ProjectID, result.Root, data, result.AnalyzedAt.UnixMilli())
	return path, err
}

func Load(ctx context.Context, projectID string) (Result, error) {
	path := filepath.Join(config.CacheDir(), "project-analysis", projectID+".sqlite")
	handle, err := projectiondb.Open(ctx, projectiondb.OpenOptions{Path: path, Migrations: projectionMigrations(), MaxOpenConns: 1})
	if err != nil {
		return Result{}, err
	}
	defer handle.DB.Close()
	var data []byte
	if err := handle.DB.QueryRowContext(ctx, `SELECT result_json FROM analysis_projects WHERE project_id=?`, projectID).Scan(&data); err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return Result{}, err
	}
	return result, nil
}
