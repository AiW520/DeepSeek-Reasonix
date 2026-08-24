package projectanalysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

type FileChangeKind string

const (
	FileAdded    FileChangeKind = "added"
	FileModified FileChangeKind = "modified"
	FileDeleted  FileChangeKind = "deleted"
	FileRenamed  FileChangeKind = "renamed"
)

type FileChange struct {
	Path    string         `json:"path"`
	OldPath string         `json:"oldPath,omitempty"`
	Kind    FileChangeKind `json:"kind"`
}

type ChangeSet struct {
	BaseRevision   string       `json:"baseRevision"`
	TargetRevision string       `json:"targetRevision"`
	Files          []FileChange `json:"files"`
}

type InvalidationPlan struct {
	Stages []StageID `json:"stages"`
	Reason string    `json:"reason,omitempty"`
}

func PlanIncrementalAnalysis(contract PipelineContract, changes ChangeSet) (InvalidationPlan, error) {
	if err := contract.Validate(); err != nil {
		return InvalidationPlan{}, err
	}
	if len(changes.Files) == 0 {
		return InvalidationPlan{Stages: []StageID{}}, nil
	}
	start := StageRepositoryIndex
	reason := "repository metadata changed"
	for _, change := range changes.Files {
		if err := validateWorkspacePath(change.Path); err != nil {
			return InvalidationPlan{}, fmt.Errorf("changed file %q: %w", change.Path, err)
		}
		if change.Kind != FileAdded && change.Kind != FileModified && change.Kind != FileDeleted && change.Kind != FileRenamed {
			return InvalidationPlan{}, fmt.Errorf("changed file %q has invalid kind %q", change.Path, change.Kind)
		}
		if change.OldPath != "" {
			if err := validateWorkspacePath(change.OldPath); err != nil {
				return InvalidationPlan{}, fmt.Errorf("old changed file %q: %w", change.OldPath, err)
			}
		}
		if isAnalysisManifest(change.Path) || (change.OldPath != "" && isAnalysisManifest(change.OldPath)) {
			start, reason = StageReconnaissance, "project manifest changed"
			break
		}
		if isSourcePath(change.Path) || (change.OldPath != "" && isSourcePath(change.OldPath)) {
			start, reason = StageParse, "source files changed"
		}
	}
	invalid := downstreamStages(contract, start)
	return InvalidationPlan{Stages: invalid, Reason: reason}, nil
}

func downstreamStages(contract PipelineContract, start StageID) []StageID {
	invalid := map[StageID]bool{start: true}
	changed := true
	for changed {
		changed = false
		for _, stage := range contract.Stages {
			if invalid[stage.ID] {
				continue
			}
			for _, dependency := range stage.DependsOn {
				if invalid[dependency] {
					invalid[stage.ID] = true
					changed = true
					break
				}
			}
		}
	}
	out := make([]StageID, 0, len(invalid))
	for _, stage := range contract.Stages {
		if invalid[stage.ID] {
			out = append(out, stage.ID)
		}
	}
	return out
}

func ArtifactCacheKey(schemaVersion int, stage StageDescriptor, revision string, inputHashes []string) (string, error) {
	if schemaVersion <= 0 || stage.ID == "" || stage.Version <= 0 || strings.TrimSpace(revision) == "" {
		return "", fmt.Errorf("schema version, stage, stage version, and revision are required")
	}
	hashes := append([]string(nil), inputHashes...)
	sort.Strings(hashes)
	payload, err := json.Marshal(struct {
		SchemaVersion int      `json:"schemaVersion"`
		Stage         StageID  `json:"stage"`
		StageVersion  int      `json:"stageVersion"`
		Revision      string   `json:"revision"`
		InputHashes   []string `json:"inputHashes"`
	}{schemaVersion, stage.ID, stage.Version, revision, hashes})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func isAnalysisManifest(value string) bool {
	base := strings.ToLower(path.Base(value))
	return isManifest(base) || base == "go.sum" || base == "pnpm-workspace.yaml" || base == "workspace.json"
}

func isSourcePath(value string) bool {
	switch strings.ToLower(path.Ext(value)) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".py", ".rs", ".java", ".c", ".cc", ".cpp", ".h", ".hpp":
		return true
	}
	return false
}
