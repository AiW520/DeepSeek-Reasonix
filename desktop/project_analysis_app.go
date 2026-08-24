package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"reasonix/internal/projectanalysis"
)

type ProjectAnalysisJobView struct {
	ID        string                  `json:"id"`
	Root      string                  `json:"root"`
	State     string                  `json:"state"`
	Phase     string                  `json:"phase"`
	Done      int                     `json:"done"`
	Total     int                     `json:"total"`
	StartedAt time.Time               `json:"startedAt"`
	Error     string                  `json:"error,omitempty"`
	Result    *projectanalysis.Result `json:"result,omitempty"`
}

type projectAnalysisJob struct {
	view   ProjectAnalysisJobView
	cancel context.CancelFunc
}

func (a *App) PickProjectAnalysisRoot() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "选择要逆向学习的项目",
		DefaultDirectory: dialogDefaultDirectory(storageDefaultWorkspace()),
	})
}

func (a *App) StartProjectAnalysis(root string) (ProjectAnalysisJobView, error) {
	clean, err := validateProjectAnalysisRoot(root)
	if err != nil {
		return ProjectAnalysisJobView{}, err
	}
	id := newProjectAnalysisID()
	ctx, cancel := context.WithCancel(a.bootContext())
	job := &projectAnalysisJob{view: ProjectAnalysisJobView{
		ID: id, Root: clean, State: "queued", Phase: "准备分析", StartedAt: time.Now(),
	}, cancel: cancel}
	a.projectAnalysisMu.Lock()
	if a.projectAnalysisJobs == nil {
		a.projectAnalysisJobs = make(map[string]*projectAnalysisJob)
	}
	a.pruneProjectAnalysisJobsLocked(32)
	a.projectAnalysisJobs[id] = job
	initial := cloneProjectAnalysisJobView(job.view)
	a.projectAnalysisMu.Unlock()

	go a.runProjectAnalysis(ctx, id, clean)
	return initial, nil
}

func (a *App) ProjectAnalysisJob(id string) (ProjectAnalysisJobView, error) {
	a.projectAnalysisMu.RLock()
	job := a.projectAnalysisJobs[strings.TrimSpace(id)]
	if job == nil {
		a.projectAnalysisMu.RUnlock()
		return ProjectAnalysisJobView{}, errors.New("analysis task not found")
	}
	view := cloneProjectAnalysisJobView(job.view)
	a.projectAnalysisMu.RUnlock()
	return view, nil
}

func (a *App) CancelProjectAnalysis(id string) error {
	a.projectAnalysisMu.RLock()
	job := a.projectAnalysisJobs[strings.TrimSpace(id)]
	if job == nil {
		a.projectAnalysisMu.RUnlock()
		return errors.New("analysis task not found")
	}
	cancel := job.cancel
	a.projectAnalysisMu.RUnlock()
	cancel()
	return nil
}

func (a *App) runProjectAnalysis(ctx context.Context, id, root string) {
	a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) {
		view.State = "running"
	})
	result, err := projectanalysis.Analyze(ctx, root, func(progress projectanalysis.Progress) {
		a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) {
			view.Phase, view.Done, view.Total = progress.Phase, progress.Done, progress.Total
		})
	})
	if err != nil {
		state := "failed"
		message := "分析失败"
		if errors.Is(err, context.Canceled) {
			state, message = "cancelled", "分析已取消"
		}
		a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) {
			view.State, view.Error = state, message
		})
		return
	}
	a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) { view.Phase = "建立版本化仓库索引" })
	reconnaissance, artifactErr := projectanalysis.BuildReconnaissanceArtifact(result)
	if artifactErr == nil {
		var repositoryIndex projectanalysis.ArtifactEnvelope
		repositoryIndex, artifactErr = projectanalysis.BuildRepositoryIndexArtifact(reconnaissance)
		if artifactErr == nil {
			a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) { view.Phase = "解析 Go AST" })
			var parsed projectanalysis.ArtifactEnvelope
			parsed, artifactErr = projectanalysis.ParseProject(ctx, root, repositoryIndex)
			if artifactErr == nil {
				var summary projectanalysis.ParseSummary
				summary, artifactErr = projectanalysis.SummarizeParseArtifact(parsed)
				if artifactErr == nil {
					result.Parse = &summary
					a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) { view.Phase = "解析 Go 符号与跨文件引用" })
					var symbols projectanalysis.ArtifactEnvelope
					symbols, artifactErr = projectanalysis.ResolveGoSymbols(ctx, root, parsed)
					if artifactErr == nil {
						var resolution projectanalysis.ResolutionSummary
						resolution, artifactErr = projectanalysis.SummarizeResolutionArtifact(symbols)
						if artifactErr == nil {
							result.Resolution = &resolution
							a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) { view.Phase = "建立 Go 依赖关系" })
							var dependencies projectanalysis.ArtifactEnvelope
							dependencies, artifactErr = projectanalysis.BuildGoDependencyArtifact(symbols)
							if artifactErr == nil {
								a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) { view.Phase = "建立 Go 调用图" })
								var callGraph projectanalysis.ArtifactEnvelope
								callGraph, artifactErr = projectanalysis.BuildGoCallGraph(symbols, dependencies)
								if artifactErr == nil {
									var callSummary projectanalysis.CallGraphSummary
									callSummary, artifactErr = projectanalysis.SummarizeCallGraph(callGraph)
									if artifactErr == nil {
										result.CallGraph = &callSummary
										a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) { view.Phase = "分析 Go 函数内数据流" })
										var dataFlow projectanalysis.ArtifactEnvelope
										dataFlow, artifactErr = projectanalysis.BuildGoDataFlow(ctx, root, symbols, callGraph)
										if artifactErr == nil {
											var dataFlowSummary projectanalysis.DataFlowSummary
											dataFlowSummary, artifactErr = projectanalysis.SummarizeDataFlow(dataFlow)
											if artifactErr == nil {
												result.DataFlow = &dataFlowSummary
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if artifactErr != nil {
		if errors.Is(artifactErr, context.Canceled) {
			a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) {
				view.State, view.Error = "cancelled", "分析已取消"
			})
			return
		}
		result.Errors = append(result.Errors, "正式解析阶段失败，侦察结果仍可用")
	}
	if _, err := projectanalysis.Store(ctx, result); err != nil {
		result.Errors = append(result.Errors, "分析缓存写入失败，结果仅在本次运行中可用")
	}
	a.updateProjectAnalysis(id, func(view *ProjectAnalysisJobView) {
		view.State, view.Phase, view.Result = "completed", "完成", &result
		view.Done = view.Total
	})
}

func (a *App) updateProjectAnalysis(id string, mutate func(*ProjectAnalysisJobView)) {
	a.projectAnalysisMu.Lock()
	if job := a.projectAnalysisJobs[id]; job != nil {
		mutate(&job.view)
	}
	a.projectAnalysisMu.Unlock()
}

func (a *App) pruneProjectAnalysisJobsLocked(limit int) {
	for len(a.projectAnalysisJobs) >= limit {
		var oldestID string
		var oldest time.Time
		for id, job := range a.projectAnalysisJobs {
			if job.view.State == "queued" || job.view.State == "running" {
				continue
			}
			if oldestID == "" || job.view.StartedAt.Before(oldest) {
				oldestID, oldest = id, job.view.StartedAt
			}
		}
		if oldestID == "" {
			return
		}
		delete(a.projectAnalysisJobs, oldestID)
	}
}

func validateProjectAnalysisRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("请选择项目目录")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", errors.New("无法解析项目目录")
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", errors.New("项目目录不存在或不可访问")
	}
	return filepath.Clean(abs), nil
}

func newProjectAnalysisID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "analysis-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("analysis-%d", time.Now().UnixNano())
}

func cloneProjectAnalysisJobView(view ProjectAnalysisJobView) ProjectAnalysisJobView {
	copy := view
	if view.Result != nil {
		result := *view.Result
		result.Files = append([]projectanalysis.File(nil), view.Result.Files...)
		result.Symbols = append([]projectanalysis.Symbol(nil), view.Result.Symbols...)
		result.Dependencies = append([]projectanalysis.Dependency(nil), view.Result.Dependencies...)
		result.Evidence = append([]projectanalysis.Evidence(nil), view.Result.Evidence...)
		result.Languages = append([]string(nil), view.Result.Languages...)
		result.Frameworks = append([]string(nil), view.Result.Frameworks...)
		result.SensitiveFiles = append([]string(nil), view.Result.SensitiveFiles...)
		result.Errors = append([]string(nil), view.Result.Errors...)
		if view.Result.Parse != nil {
			parse := *view.Result.Parse
			parse.UnsupportedFiles = append([]string(nil), view.Result.Parse.UnsupportedFiles...)
			parse.BlockedFiles = append([]string(nil), view.Result.Parse.BlockedFiles...)
			parse.StaleFiles = append([]string(nil), view.Result.Parse.StaleFiles...)
			parse.ParserLanguages = append([]string(nil), view.Result.Parse.ParserLanguages...)
			result.Parse = &parse
		}
		if view.Result.Resolution != nil {
			resolution := *view.Result.Resolution
			resolution.SupportedLanguages = append([]string(nil), view.Result.Resolution.SupportedLanguages...)
			result.Resolution = &resolution
		}
		if view.Result.CallGraph != nil {
			callGraph := *view.Result.CallGraph
			callGraph.SupportedLanguages = append([]string(nil), view.Result.CallGraph.SupportedLanguages...)
			result.CallGraph = &callGraph
		}
		if view.Result.DataFlow != nil {
			dataFlow := *view.Result.DataFlow
			dataFlow.SupportedLanguages = append([]string(nil), view.Result.DataFlow.SupportedLanguages...)
			result.DataFlow = &dataFlow
		}
		copy.Result = &result
	}
	return copy
}
