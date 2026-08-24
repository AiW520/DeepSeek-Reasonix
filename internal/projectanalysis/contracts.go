package projectanalysis

import (
	"errors"
	"fmt"
)

const SchemaVersion = 1

type AnalysisMode string

const (
	AnalysisModeFull        AnalysisMode = "full"
	AnalysisModeIncremental AnalysisMode = "incremental"
)

type StageID string

const (
	StageReconnaissance  StageID = "reconnaissance"
	StageRepositoryIndex StageID = "repository_index"
	StageParse           StageID = "parse"
	StageSymbols         StageID = "symbols"
	StageDependencies    StageID = "dependencies"
	StageCallGraph       StageID = "call_graph"
	StageDataFlow        StageID = "data_flow"
	StageAPICatalog      StageID = "api_catalog"
	StageDatabaseSchema  StageID = "database_schema"
	StageArchitecture    StageID = "architecture"
	StageKnowledgeGraph  StageID = "knowledge_graph"
	StageAIReasoning     StageID = "ai_reasoning"
	StageLearning        StageID = "learning"
)

type StageAvailability string

const (
	StageAvailable StageAvailability = "available"
	StagePlanned   StageAvailability = "planned"
)

type StageDescriptor struct {
	ID           StageID           `json:"id"`
	Version      int               `json:"version"`
	Availability StageAvailability `json:"availability"`
	DependsOn    []StageID         `json:"dependsOn,omitempty"`
	Coverage     []string          `json:"coverage,omitempty"`
}

type PipelineContract struct {
	SchemaVersion int               `json:"schemaVersion"`
	Stages        []StageDescriptor `json:"stages"`
}

func DefaultPipelineContract() PipelineContract {
	planned := func(id StageID, dependencies ...StageID) StageDescriptor {
		return StageDescriptor{ID: id, Version: 1, Availability: StagePlanned, DependsOn: dependencies}
	}
	return PipelineContract{SchemaVersion: SchemaVersion, Stages: []StageDescriptor{
		{ID: StageReconnaissance, Version: 1, Availability: StageAvailable},
		{ID: StageRepositoryIndex, Version: 1, Availability: StageAvailable, DependsOn: []StageID{StageReconnaissance}},
		{ID: StageParse, Version: 1, Availability: StageAvailable, DependsOn: []StageID{StageRepositoryIndex}, Coverage: []string{"Go"}},
		{ID: StageSymbols, Version: 1, Availability: StageAvailable, DependsOn: []StageID{StageParse}, Coverage: []string{"Go"}},
		{ID: StageDependencies, Version: 1, Availability: StageAvailable, DependsOn: []StageID{StageParse, StageSymbols}, Coverage: []string{"Go"}},
		{ID: StageCallGraph, Version: 1, Availability: StageAvailable, DependsOn: []StageID{StageSymbols, StageDependencies}, Coverage: []string{"Go"}},
		{ID: StageDataFlow, Version: 1, Availability: StageAvailable, DependsOn: []StageID{StageSymbols, StageCallGraph}, Coverage: []string{"Go"}},
		planned(StageAPICatalog, StageSymbols, StageDependencies),
		planned(StageDatabaseSchema, StageParse, StageDependencies),
		planned(StageArchitecture, StageCallGraph, StageDataFlow, StageAPICatalog, StageDatabaseSchema),
		planned(StageKnowledgeGraph, StageArchitecture),
		planned(StageAIReasoning, StageKnowledgeGraph),
		planned(StageLearning, StageAIReasoning),
	}}
}

func (contract PipelineContract) Validate() error {
	if contract.SchemaVersion <= 0 {
		return errors.New("pipeline schema version must be positive")
	}
	if len(contract.Stages) == 0 {
		return errors.New("pipeline must contain at least one stage")
	}
	stages := make(map[StageID]StageDescriptor, len(contract.Stages))
	for _, stage := range contract.Stages {
		if stage.ID == "" {
			return errors.New("pipeline stage ID is required")
		}
		if _, exists := stages[stage.ID]; exists {
			return fmt.Errorf("duplicate pipeline stage %q", stage.ID)
		}
		if stage.Version <= 0 {
			return fmt.Errorf("pipeline stage %q version must be positive", stage.ID)
		}
		if stage.Availability != StageAvailable && stage.Availability != StagePlanned {
			return fmt.Errorf("pipeline stage %q has invalid availability %q", stage.ID, stage.Availability)
		}
		stages[stage.ID] = stage
	}
	for _, stage := range contract.Stages {
		seenDependencies := map[StageID]bool{}
		for _, dependency := range stage.DependsOn {
			if seenDependencies[dependency] {
				return fmt.Errorf("pipeline stage %q repeats dependency %q", stage.ID, dependency)
			}
			seenDependencies[dependency] = true
			upstream, exists := stages[dependency]
			if !exists {
				return fmt.Errorf("pipeline stage %q depends on missing stage %q", stage.ID, dependency)
			}
			if stage.Availability == StageAvailable && upstream.Availability != StageAvailable {
				return fmt.Errorf("available stage %q depends on unavailable stage %q", stage.ID, dependency)
			}
		}
	}
	visiting := map[StageID]bool{}
	visited := map[StageID]bool{}
	var visit func(StageID) error
	visit = func(id StageID) error {
		if visiting[id] {
			return fmt.Errorf("pipeline contains a cycle at stage %q", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range stages[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range stages {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
