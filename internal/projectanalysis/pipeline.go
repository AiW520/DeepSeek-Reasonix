package projectanalysis

import (
	"context"
	"errors"
	"fmt"
)

var ErrStageUnavailable = errors.New("analysis stage is not available")

type StageRequest struct {
	Mode           AnalysisMode       `json:"mode"`
	Snapshot       ProjectSnapshot    `json:"snapshot"`
	Inputs         []ArtifactEnvelope `json:"inputs,omitempty"`
	Changes        ChangeSet          `json:"changes,omitempty"`
	IdempotencyKey string             `json:"idempotencyKey"`
}

type StageWorker interface {
	Stage() StageID
	Run(context.Context, StageRequest) (ArtifactEnvelope, error)
}

type PipelineExecutor struct {
	contract PipelineContract
	stages   map[StageID]StageDescriptor
	workers  map[StageID]StageWorker
}

func NewPipelineExecutor(contract PipelineContract, workers ...StageWorker) (*PipelineExecutor, error) {
	if err := contract.Validate(); err != nil {
		return nil, err
	}
	executor := &PipelineExecutor{
		contract: contract,
		stages:   make(map[StageID]StageDescriptor, len(contract.Stages)),
		workers:  make(map[StageID]StageWorker, len(workers)),
	}
	for _, stage := range contract.Stages {
		executor.stages[stage.ID] = stage
	}
	for _, worker := range workers {
		if worker == nil || worker.Stage() == "" {
			return nil, errors.New("pipeline worker and stage ID are required")
		}
		if _, exists := executor.stages[worker.Stage()]; !exists {
			return nil, fmt.Errorf("worker registered for unknown stage %q", worker.Stage())
		}
		if _, exists := executor.workers[worker.Stage()]; exists {
			return nil, fmt.Errorf("duplicate worker for stage %q", worker.Stage())
		}
		executor.workers[worker.Stage()] = worker
	}
	return executor, nil
}

func (executor *PipelineExecutor) RunStage(ctx context.Context, stageID StageID, request StageRequest) (ArtifactEnvelope, error) {
	if executor == nil {
		return ArtifactEnvelope{}, errors.New("pipeline executor is nil")
	}
	stage, exists := executor.stages[stageID]
	if !exists {
		return ArtifactEnvelope{}, fmt.Errorf("unknown analysis stage %q", stageID)
	}
	if stage.Availability != StageAvailable {
		return ArtifactEnvelope{}, fmt.Errorf("%w: %s", ErrStageUnavailable, stageID)
	}
	worker := executor.workers[stageID]
	if worker == nil {
		return ArtifactEnvelope{}, fmt.Errorf("available analysis stage %q has no worker", stageID)
	}
	if request.Mode != AnalysisModeFull && request.Mode != AnalysisModeIncremental {
		return ArtifactEnvelope{}, fmt.Errorf("invalid analysis mode %q", request.Mode)
	}
	if request.Snapshot.ProjectID == "" || request.Snapshot.Revision.Provider == "" || request.Snapshot.Revision.Value == "" {
		return ArtifactEnvelope{}, errors.New("stage request requires a versioned project snapshot")
	}
	if request.IdempotencyKey == "" {
		return ArtifactEnvelope{}, errors.New("stage request idempotency key is required")
	}
	if err := validateStageInputs(stage, request.Inputs); err != nil {
		return ArtifactEnvelope{}, err
	}
	if err := ctx.Err(); err != nil {
		return ArtifactEnvelope{}, err
	}
	output, err := worker.Run(ctx, request)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	if output.Artifact.Stage != stageID || output.StageVersion != stage.Version {
		return ArtifactEnvelope{}, fmt.Errorf("worker %q returned mismatched stage or version", stageID)
	}
	if output.Artifact.Snapshot.ProjectID != request.Snapshot.ProjectID || output.Artifact.Snapshot.Revision != request.Snapshot.Revision {
		return ArtifactEnvelope{}, fmt.Errorf("worker %q returned an artifact for a different project snapshot", stageID)
	}
	if err := output.Validate(); err != nil {
		return ArtifactEnvelope{}, fmt.Errorf("worker %q returned an invalid artifact: %w", stageID, err)
	}
	return output, nil
}

func validateStageInputs(stage StageDescriptor, inputs []ArtifactEnvelope) error {
	byStage := make(map[StageID]ArtifactEnvelope, len(inputs))
	for _, input := range inputs {
		if err := input.Validate(); err != nil {
			return fmt.Errorf("invalid stage input: %w", err)
		}
		if _, exists := byStage[input.Artifact.Stage]; exists {
			return fmt.Errorf("duplicate input for stage %q", input.Artifact.Stage)
		}
		byStage[input.Artifact.Stage] = input
	}
	for _, dependency := range stage.DependsOn {
		if _, exists := byStage[dependency]; !exists {
			return fmt.Errorf("stage %q requires input from stage %q", stage.ID, dependency)
		}
	}
	return nil
}
