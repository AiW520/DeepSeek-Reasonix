package projectanalysis

import (
	"reflect"
	"slices"
	"testing"
)

func TestPlanIncrementalAnalysisIsDeterministic(t *testing.T) {
	contract := DefaultPipelineContract()
	first, err := PlanIncrementalAnalysis(contract, ChangeSet{Files: []FileChange{{Path: "src/app.ts", Kind: FileModified}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := PlanIncrementalAnalysis(contract, ChangeSet{Files: []FileChange{{Path: "src/app.ts", Kind: FileModified}}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first.Stages) == 0 || first.Stages[0] != StageParse {
		t.Fatalf("plans differ or start at wrong stage: %#v %#v", first, second)
	}
	if containsStage(first.Stages, StageReconnaissance) {
		t.Fatalf("source-only change invalidated reconnaissance: %#v", first.Stages)
	}
}

func TestPlanIncrementalAnalysisManifestAndNoChanges(t *testing.T) {
	contract := DefaultPipelineContract()
	empty, err := PlanIncrementalAnalysis(contract, ChangeSet{})
	if err != nil || len(empty.Stages) != 0 {
		t.Fatalf("empty plan = %#v, err=%v", empty, err)
	}
	manifest, err := PlanIncrementalAnalysis(contract, ChangeSet{Files: []FileChange{{Path: "package.json", Kind: FileModified}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Stages) != len(contract.Stages) || manifest.Stages[0] != StageReconnaissance {
		t.Fatalf("manifest plan = %#v", manifest)
	}
}

func TestArtifactCacheKeySortsInputHashesAndIncludesVersions(t *testing.T) {
	stage := StageDescriptor{ID: StageParse, Version: 1, Availability: StagePlanned}
	first, err := ArtifactCacheKey(SchemaVersion, stage, "revision", []string{"b", "a"})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := ArtifactCacheKey(SchemaVersion, stage, "revision", []string{"a", "b"})
	if first != second {
		t.Fatalf("cache key depends on input order: %q != %q", first, second)
	}
	stage.Version++
	third, _ := ArtifactCacheKey(SchemaVersion, stage, "revision", []string{"a", "b"})
	if first == third {
		t.Fatal("cache key ignored stage version")
	}
}

func containsStage(stages []StageID, want StageID) bool {
	return slices.Contains(stages, want)
}
