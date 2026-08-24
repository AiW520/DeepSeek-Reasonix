package projectanalysis

import (
	"context"
	"errors"
	"fmt"
)

type RepositoryIndexWorker struct{}

func (RepositoryIndexWorker) Stage() StageID { return StageRepositoryIndex }

func (RepositoryIndexWorker) Run(_ context.Context, request StageRequest) (ArtifactEnvelope, error) {
	input, err := stageInput(request.Inputs, StageReconnaissance)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	return BuildRepositoryIndexArtifact(input)
}

type ParseWorker struct {
	Root string
}

type SymbolWorker struct {
	Root string
}

func (SymbolWorker) Stage() StageID { return StageSymbols }

func (worker SymbolWorker) Run(ctx context.Context, request StageRequest) (ArtifactEnvelope, error) {
	if worker.Root == "" {
		return ArtifactEnvelope{}, errors.New("symbol worker root is required")
	}
	input, err := stageInput(request.Inputs, StageParse)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	return ResolveGoSymbols(ctx, worker.Root, input)
}

type DependencyWorker struct{}

func (DependencyWorker) Stage() StageID { return StageDependencies }

func (DependencyWorker) Run(_ context.Context, request StageRequest) (ArtifactEnvelope, error) {
	if _, err := stageInput(request.Inputs, StageParse); err != nil {
		return ArtifactEnvelope{}, err
	}
	symbols, err := stageInput(request.Inputs, StageSymbols)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	return BuildGoDependencyArtifact(symbols)
}

type CallGraphWorker struct{}

func (CallGraphWorker) Stage() StageID { return StageCallGraph }

func (CallGraphWorker) Run(_ context.Context, request StageRequest) (ArtifactEnvelope, error) {
	symbols, err := stageInput(request.Inputs, StageSymbols)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	dependencies, err := stageInput(request.Inputs, StageDependencies)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	return BuildGoCallGraph(symbols, dependencies)
}

type DataFlowWorker struct {
	Root string
}

func (DataFlowWorker) Stage() StageID { return StageDataFlow }

func (worker DataFlowWorker) Run(ctx context.Context, request StageRequest) (ArtifactEnvelope, error) {
	if worker.Root == "" {
		return ArtifactEnvelope{}, errors.New("data flow worker root is required")
	}
	symbols, err := stageInput(request.Inputs, StageSymbols)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	callGraph, err := stageInput(request.Inputs, StageCallGraph)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	return BuildGoDataFlow(ctx, worker.Root, symbols, callGraph)
}

func (ParseWorker) Stage() StageID { return StageParse }

func (worker ParseWorker) Run(ctx context.Context, request StageRequest) (ArtifactEnvelope, error) {
	if worker.Root == "" {
		return ArtifactEnvelope{}, errors.New("parse worker root is required")
	}
	input, err := stageInput(request.Inputs, StageRepositoryIndex)
	if err != nil {
		return ArtifactEnvelope{}, err
	}
	return ParseProject(ctx, worker.Root, input)
}

func stageInput(inputs []ArtifactEnvelope, stage StageID) (ArtifactEnvelope, error) {
	for _, input := range inputs {
		if input.Artifact.Stage == stage {
			return input, nil
		}
	}
	return ArtifactEnvelope{}, fmt.Errorf("stage input %q is required", stage)
}
