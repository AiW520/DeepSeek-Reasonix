package projectanalysis

import (
	"context"
	"errors"
	"testing"
)

type testStageWorker struct {
	stage StageID
	run   func(context.Context, StageRequest) (ArtifactEnvelope, error)
}

func (worker testStageWorker) Stage() StageID { return worker.stage }
func (worker testStageWorker) Run(ctx context.Context, request StageRequest) (ArtifactEnvelope, error) {
	return worker.run(ctx, request)
}

func TestPipelineExecutorRunsOnlyAvailableValidatedWorkers(t *testing.T) {
	output := validEnvelope()
	worker := testStageWorker{stage: StageReconnaissance, run: func(_ context.Context, request StageRequest) (ArtifactEnvelope, error) {
		output.Artifact.Snapshot = request.Snapshot
		return output, nil
	}}
	executor, err := NewPipelineExecutor(DefaultPipelineContract(), worker)
	if err != nil {
		t.Fatal(err)
	}
	request := StageRequest{Mode: AnalysisModeFull, Snapshot: output.Artifact.Snapshot, IdempotencyKey: "job-1:reconnaissance"}
	if _, err := executor.RunStage(context.Background(), StageReconnaissance, request); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.RunStage(context.Background(), StageAPICatalog, request); !errors.Is(err, ErrStageUnavailable) {
		t.Fatalf("planned stage error = %v", err)
	}
}

func TestPipelineExecutorHonorsCancellationBeforeWorker(t *testing.T) {
	called := false
	worker := testStageWorker{stage: StageReconnaissance, run: func(_ context.Context, _ StageRequest) (ArtifactEnvelope, error) {
		called = true
		return validEnvelope(), nil
	}}
	executor, err := NewPipelineExecutor(DefaultPipelineContract(), worker)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := StageRequest{Mode: AnalysisModeFull, Snapshot: validEnvelope().Artifact.Snapshot, IdempotencyKey: "job-1"}
	if _, err := executor.RunStage(ctx, StageReconnaissance, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("worker ran after cancellation")
	}
}
