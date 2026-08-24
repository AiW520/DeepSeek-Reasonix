package projectanalysis

import (
	"strings"
	"testing"
)

func TestDefaultPipelineContractIsValidAndTruthful(t *testing.T) {
	contract := DefaultPipelineContract()
	if err := contract.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, stage := range contract.Stages {
		want := StagePlanned
		if stage.ID == StageReconnaissance || stage.ID == StageRepositoryIndex || stage.ID == StageParse || stage.ID == StageSymbols || stage.ID == StageDependencies || stage.ID == StageCallGraph || stage.ID == StageDataFlow {
			want = StageAvailable
		}
		if stage.Availability != want {
			t.Fatalf("stage %q availability = %q, want %q", stage.ID, stage.Availability, want)
		}
		if stage.ID == StageParse && (len(stage.Coverage) != 1 || stage.Coverage[0] != "Go") {
			t.Fatalf("parse coverage = %#v, want Go only", stage.Coverage)
		}
		if (stage.ID == StageSymbols || stage.ID == StageDependencies) && (len(stage.Coverage) != 1 || stage.Coverage[0] != "Go") {
			t.Fatalf("stage %q coverage = %#v, want Go only", stage.ID, stage.Coverage)
		}
		if (stage.ID == StageCallGraph || stage.ID == StageDataFlow) && (len(stage.Coverage) != 1 || stage.Coverage[0] != "Go") {
			t.Fatalf("stage %q coverage = %#v, want Go only", stage.ID, stage.Coverage)
		}
	}
}

func TestPipelineContractRejectsInvalidDAGs(t *testing.T) {
	tests := []struct {
		name   string
		stages []StageDescriptor
		want   string
	}{
		{"duplicate", []StageDescriptor{{ID: "a", Version: 1, Availability: StageAvailable}, {ID: "a", Version: 1, Availability: StageAvailable}}, "duplicate"},
		{"missing dependency", []StageDescriptor{{ID: "a", Version: 1, Availability: StagePlanned, DependsOn: []StageID{"missing"}}}, "missing"},
		{"cycle", []StageDescriptor{{ID: "a", Version: 1, Availability: StagePlanned, DependsOn: []StageID{"b"}}, {ID: "b", Version: 1, Availability: StagePlanned, DependsOn: []StageID{"a"}}}, "cycle"},
		{"available depends on planned", []StageDescriptor{{ID: "a", Version: 1, Availability: StageAvailable, DependsOn: []StageID{"b"}}, {ID: "b", Version: 1, Availability: StagePlanned}}, "unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (PipelineContract{SchemaVersion: SchemaVersion, Stages: test.stages}).Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}
