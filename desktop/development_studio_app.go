package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"reasonix/internal/agent"
	"reasonix/internal/boot"
	"reasonix/internal/config"
	"reasonix/internal/event"
	"reasonix/internal/fileutil"
	"reasonix/internal/permission"
)

const (
	developmentStudioChannel   = "development:studio"
	developmentStudioMaxEvents = 240
)

type DevelopmentAIConfig struct {
	Enabled         bool   `json:"enabled"`
	Model           string `json:"model"`
	Effort          string `json:"effort"`
	MaxSteps        int    `json:"maxSteps"`
	MaxOutputTokens int    `json:"maxOutputTokens"`
}

type DevelopmentStudioConfig struct {
	Enabled             bool                `json:"enabled"`
	TeachingMode        string              `json:"teachingMode"`
	Realtime            DevelopmentAIConfig `json:"realtime"`
	Final               DevelopmentAIConfig `json:"final"`
	MinReviewIntervalMS int                 `json:"minReviewIntervalMs"`
	TurnTokenBudget     int                 `json:"turnTokenBudget"`
}

type DevelopmentStudioEvent struct {
	ID       string   `json:"id"`
	TabID    string   `json:"tabId"`
	Sequence uint64   `json:"sequence"`
	At       int64    `json:"at"`
	Role     string   `json:"role"`
	Kind     string   `json:"kind"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail,omitempty"`
	Why      string   `json:"why,omitempty"`
	Target   string   `json:"target,omitempty"`
	Status   string   `json:"status,omitempty"`
	Severity string   `json:"severity,omitempty"`
	Model    string   `json:"model,omitempty"`
	Tokens   int      `json:"tokens,omitempty"`
	Cost     float64  `json:"cost,omitempty"`
	Currency string   `json:"currency,omitempty"`
	Paths    []string `json:"paths,omitempty"`
}

type DevelopmentAIRuntime struct {
	Role     string  `json:"role"`
	State    string  `json:"state"`
	Model    string  `json:"model,omitempty"`
	LastRun  int64   `json:"lastRun,omitempty"`
	Runs     int     `json:"runs"`
	Tokens   int     `json:"tokens"`
	Cost     float64 `json:"cost"`
	Currency string  `json:"currency,omitempty"`
}

type DevelopmentStudioSnapshot struct {
	Config   DevelopmentStudioConfig  `json:"config"`
	Paused   bool                     `json:"paused"`
	Lead     DevelopmentAIRuntime     `json:"lead"`
	Realtime DevelopmentAIRuntime     `json:"realtime"`
	Final    DevelopmentAIRuntime     `json:"final"`
	Events   []DevelopmentStudioEvent `json:"events"`
	Models   []ModelInfo              `json:"models"`
}

type developmentStudioTab struct {
	sequence        uint64
	events          []DevelopmentStudioEvent
	paused          bool
	turnActive      bool
	turnTokens      int
	realtimeRunning bool
	realtimePending bool
	realtimeTimer   *time.Timer
	finalRunning    bool
	finalTriggered  bool
	lastRealtimeRun time.Time
	lead            DevelopmentAIRuntime
	realtime        DevelopmentAIRuntime
	final           DevelopmentAIRuntime
	cancelRealtime  context.CancelFunc
	cancelFinal     context.CancelFunc
}

type developmentStudioManager struct {
	app               *App
	mu                sync.Mutex
	config            DevelopmentStudioConfig
	tabs              map[string]*developmentStudioTab
	reviewScheduler   *agent.SubagentScheduler
	reviewTranscripts *agent.SubagentStore
}

func defaultDevelopmentStudioConfig() DevelopmentStudioConfig {
	return DevelopmentStudioConfig{
		Enabled: true, TeachingMode: "engineer",
		Realtime:            DevelopmentAIConfig{Enabled: true, Effort: "high", MaxSteps: 6, MaxOutputTokens: 2200},
		Final:               DevelopmentAIConfig{Enabled: true, Effort: "xhigh", MaxSteps: 10, MaxOutputTokens: 4000},
		MinReviewIntervalMS: 4500,
		TurnTokenBudget:     60000,
	}
}

func developmentStudioConfigPath() string {
	return filepath.Join(config.ReasonixHomeDir(), "development-studio.json")
}

func newDevelopmentStudioManager(app *App) *developmentStudioManager {
	m := &developmentStudioManager{
		app:               app,
		config:            defaultDevelopmentStudioConfig(),
		tabs:              map[string]*developmentStudioTab{},
		reviewTranscripts: agent.NewSubagentStore(filepath.Join(config.SessionDir(), "subagents")),
	}
	if body, err := os.ReadFile(developmentStudioConfigPath()); err == nil {
		var saved DevelopmentStudioConfig
		if json.Unmarshal(body, &saved) == nil {
			m.config = normalizeDevelopmentStudioConfig(saved)
		}
	}
	return m
}

func (m *developmentStudioManager) reviewRuntime(cfg *config.Config) (*agent.SubagentScheduler, *agent.SubagentStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reviewScheduler == nil {
		maxReviewers, maxWriters := agent.NormalizeConcurrencyLimits(cfg.Agent.MaxSubagentConcurrency, cfg.Agent.MaxParallelWriters)
		m.reviewScheduler = agent.NewSubagentScheduler(maxReviewers, maxWriters)
	}
	if m.reviewTranscripts == nil {
		m.reviewTranscripts = agent.NewSubagentStore(filepath.Join(config.SessionDir(), "subagents"))
	}
	return m.reviewScheduler, m.reviewTranscripts
}

func normalizeDevelopmentStudioConfig(in DevelopmentStudioConfig) DevelopmentStudioConfig {
	d := defaultDevelopmentStudioConfig()
	d.Enabled = in.Enabled
	if in.TeachingMode == "beginner" || in.TeachingMode == "engineer" || in.TeachingMode == "expert" {
		d.TeachingMode = in.TeachingMode
	}
	d.Realtime.Enabled, d.Final.Enabled = in.Realtime.Enabled, in.Final.Enabled
	d.Realtime.Model, d.Final.Model = strings.TrimSpace(in.Realtime.Model), strings.TrimSpace(in.Final.Model)
	d.Realtime.Effort, d.Final.Effort = strings.TrimSpace(in.Realtime.Effort), strings.TrimSpace(in.Final.Effort)
	if in.Realtime.MaxSteps > 0 && in.Realtime.MaxSteps <= 20 {
		d.Realtime.MaxSteps = in.Realtime.MaxSteps
	}
	if in.Final.MaxSteps > 0 && in.Final.MaxSteps <= 20 {
		d.Final.MaxSteps = in.Final.MaxSteps
	}
	if in.Realtime.MaxOutputTokens >= 256 && in.Realtime.MaxOutputTokens <= 16000 {
		d.Realtime.MaxOutputTokens = in.Realtime.MaxOutputTokens
	}
	if in.Final.MaxOutputTokens >= 256 && in.Final.MaxOutputTokens <= 16000 {
		d.Final.MaxOutputTokens = in.Final.MaxOutputTokens
	}
	if in.MinReviewIntervalMS >= 1000 && in.MinReviewIntervalMS <= 60000 {
		d.MinReviewIntervalMS = in.MinReviewIntervalMS
	}
	if in.TurnTokenBudget >= 5000 && in.TurnTokenBudget <= 1000000 {
		d.TurnTokenBudget = in.TurnTokenBudget
	}
	return d
}

func (m *developmentStudioManager) tabLocked(tabID string) *developmentStudioTab {
	state := m.tabs[tabID]
	if state == nil {
		state = &developmentStudioTab{
			lead:     DevelopmentAIRuntime{Role: "lead", State: "idle"},
			realtime: DevelopmentAIRuntime{Role: "realtime", State: "idle"},
			final:    DevelopmentAIRuntime{Role: "final", State: "idle"},
		}
		m.tabs[tabID] = state
	}
	return state
}

func (m *developmentStudioManager) publishLocked(state *developmentStudioTab, ev DevelopmentStudioEvent) {
	state.sequence++
	ev.Sequence = state.sequence
	ev.ID = fmt.Sprintf("%s-%d", ev.TabID, ev.Sequence)
	if ev.At == 0 {
		ev.At = time.Now().UnixMilli()
	}
	state.events = append(state.events, ev)
	if len(state.events) > developmentStudioMaxEvents {
		state.events = append([]DevelopmentStudioEvent(nil), state.events[len(state.events)-developmentStudioMaxEvents:]...)
	}
	if m.app != nil && m.app.ctx != nil {
		m.app.runtimeEvents.Emit(m.app.ctx, developmentStudioChannel, ev)
	}
}

func (m *developmentStudioManager) Observe(tabID string, ev event.Event) {
	if strings.TrimSpace(tabID) == "" {
		return
	}
	m.mu.Lock()
	state := m.tabLocked(tabID)
	cfg := m.config
	if ev.Kind == event.Usage && ev.Usage != nil && ev.Source != "realtime-review" && ev.Source != "final-review" {
		state.lead.Tokens += ev.Usage.TotalTokens
		state.lead.Model = ev.ModelRef
	}
	switch ev.Kind {
	case event.TurnStarted:
		state.turnActive, state.turnTokens = true, 0
		state.finalTriggered = false
		state.lead.State = "running"
		m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "lead", Kind: "turn_started", Title: "主开发 AI 已开始本轮开发", Detail: "正在理解需求、规划实现并准备执行工具。", Why: "先建立目标和验证边界，减少开发过程中的返工。", Status: "running"})
	case event.TurnPhase:
		title, detail := teachingPhaseCopy(string(ev.PhaseName))
		m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "lead", Kind: "phase", Title: title, Detail: detail, Why: "每个阶段都需要可验证产物，不能只依赖最终回答。", Status: "running"})
	case event.ToolDispatch:
		if ev.Tool.Partial {
			break
		}
		title, detail, why := teachingToolCopy(ev.Tool.Name, false, "")
		m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "lead", Kind: "activity", Title: title, Detail: detail, Why: why, Target: strings.Join(ev.Tool.WorkspacePaths, ", "), Status: "running", Paths: append([]string(nil), ev.Tool.WorkspacePaths...)})
	case event.ToolResult:
		title, detail, why := teachingToolCopy(ev.Tool.Name, true, ev.Tool.Err)
		status := "passed"
		if ev.Tool.Err != "" {
			status = "failed"
		}
		m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "lead", Kind: classifyDevelopmentTool(ev.Tool.Name), Title: title, Detail: detail, Why: why, Target: strings.Join(ev.Tool.WorkspacePaths, ", "), Status: status, Paths: append([]string(nil), ev.Tool.WorkspacePaths...)})
		trigger := ev.Tool.WorkspaceMutation || ev.Tool.Diff != "" || ev.Tool.Err != "" || classifyDevelopmentTool(ev.Tool.Name) == "test"
		if trigger && cfg.Enabled && cfg.Realtime.Enabled && !state.paused {
			m.queueRealtimeLocked(tabID, state)
		}
	case event.WorkspaceChanged:
		paths := []string{}
		if ev.Workspace != nil {
			for _, change := range ev.Workspace.Changes {
				if change.Path != "" {
					paths = append(paths, change.Path)
				}
			}
		}
		m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "lead", Kind: "change", Title: "工作区变更已捕获", Detail: fmt.Sprintf("检测到 %d 个文件变化，已加入学习与审查上下文。", len(paths)), Why: "文件级证据能让审查结论定位到真实改动。", Target: strings.Join(paths, ", "), Status: "passed", Paths: paths})
	case event.TurnDone:
		state.turnActive = false
		if ev.Err != nil {
			state.lead.State = "error"
		} else {
			state.lead.State = "done"
		}
		m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "lead", Kind: "turn_done", Title: "主开发阶段已结束", Detail: "代码、测试与过程证据已封存，最终验收 AI 开始独立检查。", Why: "最终验收必须与开发过程解耦，避免主开发 AI 自证偏差。", Status: state.lead.State})
		if cfg.Enabled && cfg.Final.Enabled && !state.paused && !state.finalTriggered {
			m.startFinalLocked(tabID, state)
		}
	}
	m.mu.Unlock()
}

func teachingPhaseCopy(phase string) (string, string) {
	switch phase {
	case "checking":
		return "正在检查实现边界", "核对改动是否覆盖需求，并确认没有遗漏调用方。"
	case "verifying":
		return "正在执行验证", "运行测试、类型检查或构建，收集可重复的通过证据。"
	case "reviewing":
		return "正在复核交付质量", "检查回归风险、安全边界和维护成本。"
	default:
		return "正在开发核心实现", "主 AI 正在阅读、修改代码并逐步形成可运行结果。"
	}
}

func classifyDevelopmentTool(name string) string {
	n := strings.ToLower(name)
	if n == "bash" || strings.Contains(n, "test") || strings.Contains(n, "build") {
		return "test"
	}
	if strings.Contains(n, "edit") || strings.Contains(n, "write") || strings.Contains(n, "patch") {
		return "change"
	}
	return "activity"
}

func teachingToolCopy(name string, done bool, toolErr string) (string, string, string) {
	label := strings.TrimSpace(name)
	if label == "" {
		label = "开发工具"
	}
	if !done {
		return "正在执行 " + label, "工具已启动，等待产生可验证结果。", "开发过程以工具结果和文件差异作为事实依据。"
	}
	if toolErr != "" {
		return label + " 执行未通过", "工具返回错误，实时审查 AI 将判断影响并给出处理建议。", "尽早暴露失败可以避免错误继续扩散。"
	}
	return label + " 已完成", "工具执行成功，结果已进入开发证据流。", "成功结果仍需结合差异与测试进行交叉验证。"
}

func (m *developmentStudioManager) queueRealtimeLocked(tabID string, state *developmentStudioTab) {
	if state.turnTokens >= m.config.TurnTokenBudget {
		m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "realtime", Kind: "budget", Title: "实时审查已达到本轮 Token 上限", Detail: "可在开发实况设置中提高上限或等待下一轮。", Severity: "observe", Status: "paused"})
		return
	}
	if state.realtimeRunning {
		state.realtimePending = true
		return
	}
	interval := time.Duration(m.config.MinReviewIntervalMS) * time.Millisecond
	if remaining := interval - time.Since(state.lastRealtimeRun); remaining > 0 {
		state.realtimePending = true
		if state.realtimeTimer == nil {
			state.realtimeTimer = time.AfterFunc(remaining, func() { m.wakeRealtime(tabID) })
		}
		return
	}
	m.startRealtimeLocked(tabID, state)
}

func (m *developmentStudioManager) wakeRealtime(tabID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.tabLocked(tabID)
	state.realtimeTimer = nil
	if !state.realtimePending || state.realtimeRunning || !state.turnActive || state.paused || !m.config.Enabled || !m.config.Realtime.Enabled {
		return
	}
	m.startRealtimeLocked(tabID, state)
}

func (m *developmentStudioManager) startRealtimeLocked(tabID string, state *developmentStudioTab) {
	ctx, cancel := context.WithCancel(m.app.bootContext())
	state.cancelRealtime, state.realtimeRunning, state.realtimePending = cancel, true, false
	if state.realtimeTimer != nil {
		state.realtimeTimer.Stop()
		state.realtimeTimer = nil
	}
	state.realtime.State, state.realtime.Model = "running", m.config.Realtime.Model
	state.lastRealtimeRun = time.Now()
	m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "realtime", Kind: "status", Title: "实时审查 AI 正在检查最新改动", Detail: "只读查看当前差异、接口影响和最近的失败证据。", Status: "running", Model: m.config.Realtime.Model})
	cfg := m.config.Realtime
	go m.runReview(ctx, tabID, "realtime", cfg)
}

func (m *developmentStudioManager) startFinalLocked(tabID string, state *developmentStudioTab) {
	if state.finalRunning {
		return
	}
	ctx, cancel := context.WithCancel(m.app.bootContext())
	state.cancelFinal, state.finalRunning = cancel, true
	state.finalTriggered = true
	state.final.State, state.final.Model = "running", m.config.Final.Model
	m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "final", Kind: "status", Title: "最终验收 AI 已启动", Detail: "独立审查完整项目差异、测试证据与未解决问题。", Status: "running", Model: m.config.Final.Model})
	cfg := m.config.Final
	go m.runReview(ctx, tabID, "final", cfg)
}

type developmentReviewUsage struct {
	tokens   int
	cost     float64
	currency string
}

func (m *developmentStudioManager) runReview(ctx context.Context, tabID, role string, aiCfg DevelopmentAIConfig) {
	report, usage, model, err := m.executeReadOnlyReview(ctx, tabID, role, aiCfg)
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.tabLocked(tabID)
	runtime := &state.realtime
	if role == "final" {
		runtime = &state.final
		state.finalRunning = false
		state.cancelFinal = nil
	} else {
		state.realtimeRunning = false
		state.cancelRealtime = nil
	}
	runtime.Runs++
	runtime.LastRun = time.Now().UnixMilli()
	runtime.Tokens += usage.tokens
	runtime.Cost += usage.cost
	runtime.Currency = usage.currency
	state.turnTokens += usage.tokens
	if err != nil {
		runtime.State = "error"
		m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: role, Kind: "error", Title: map[bool]string{true: "最终验收未完成", false: "实时审查未完成"}[role == "final"], Detail: err.Error(), Severity: "observe", Status: "error", Model: model})
	} else {
		severity, status := reviewSeverity(report, role)
		runtime.State = "done"
		title := "实时审查 AI 已返回结论"
		kind := "finding"
		if role == "final" {
			title, kind = "最终验收 AI 已给出结论", "verdict"
		}
		m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: role, Kind: kind, Title: title, Detail: report, Severity: severity, Status: status, Model: model, Tokens: usage.tokens, Cost: usage.cost, Currency: usage.currency})
	}
	if role == "realtime" && state.realtimePending && state.turnActive && !state.paused && m.config.Enabled && m.config.Realtime.Enabled {
		m.queueRealtimeLocked(tabID, state)
	}
}

func reviewSeverity(report, role string) (string, string) {
	n := strings.ToLower(report)
	if strings.Contains(n, "不通过") || strings.Contains(n, "阻断") || strings.Contains(n, "block") || strings.Contains(n, "critical") {
		return "blocker", "failed"
	}
	if strings.Contains(n, "高风险") || strings.Contains(n, "high risk") || strings.Contains(n, "条件通过") {
		return "high", "warning"
	}
	if role == "final" {
		return "suggestion", "passed"
	}
	return "suggestion", "reviewed"
}

func (m *developmentStudioManager) executeReadOnlyReview(ctx context.Context, tabID, role string, aiCfg DevelopmentAIConfig) (string, developmentReviewUsage, string, error) {
	root := ""
	m.app.mu.RLock()
	if tab := m.app.tabByIDLocked(tabID); tab != nil {
		root = tab.WorkspaceRoot
	}
	m.app.mu.RUnlock()
	cfg, err := config.LoadForRoot(root)
	if err != nil {
		return "", developmentReviewUsage{}, "", err
	}
	modelRef := strings.TrimSpace(aiCfg.Model)
	if modelRef == "" {
		modelRef = strings.TrimSpace(cfg.Agent.SubagentModel)
	}
	if modelRef == "" {
		modelRef = cfg.DefaultModel
	}
	entry, ok := cfg.ResolveModel(modelRef)
	if !ok {
		return "", developmentReviewUsage{}, modelRef, fmt.Errorf("unknown review model %q", modelRef)
	}
	me := *entry
	if effort := strings.TrimSpace(aiCfg.Effort); effort != "" && effort != "auto" {
		normalized, normalizeErr := config.NormalizeEffort(&me, effort)
		if normalizeErr != nil {
			return "", developmentReviewUsage{}, modelRef, normalizeErr
		}
		me.Effort = normalized
		if me.Kind == "anthropic" && me.Effort != "" && strings.TrimSpace(me.Thinking) == "" {
			me.Thinking = "adaptive"
		}
	}
	prov, err := boot.NewProviderWithProxy(&me, cfg.NetworkProxySpec())
	if err != nil {
		return "", developmentReviewUsage{}, modelRef, err
	}
	usage := developmentReviewUsage{currency: me.Price.Currency}
	sink := event.FuncSink(func(ev event.Event) {
		if ev.Kind == event.Usage && ev.Usage != nil {
			usage.tokens += ev.Usage.TotalTokens
			usage.cost += me.Price.Cost(ev.Usage)
		}
	})
	systemPrompt := realtimeReviewPrompt
	task := "Review the current working tree changes and the latest development state. Return concise Chinese findings only."
	if role == "final" {
		systemPrompt, task = finalReviewPrompt, "Independently audit the entire current project delivery against correctness, security, maintainability and test evidence. Return the final acceptance report in Chinese."
	}
	reviewScheduler, reviewTranscripts := m.reviewRuntime(cfg)
	modelIdentity := me.Name + "/" + me.Model
	taskTool := agent.NewTaskToolWithOptions(agent.TaskToolOptions{
		Provider:       prov,
		Pricing:        me.Price,
		ParentRegistry: trySubagentToolRegistry(cfg, root, nil),
		MaxSteps:       aiCfg.MaxSteps,
		ContextWindow:  me.ContextWindow,
		RecentKeep:     cfg.Agent.RecentKeep,
		CompactRatio:   cfg.Agent.CompactRatio,
		Temperature:    cfg.Agent.Temperature,
		ArchiveDir:     config.ArchiveDir(),
		SysPrompt:      systemPrompt,
		Gate:           developmentReviewPermissionGate(cfg),
	}).
		WithTranscripts(reviewTranscripts, root, modelIdentity, me.Effort).
		WithMaxSubagentDepth(cfg.Agent.MaxSubagentDepth).
		WithScheduler(reviewScheduler)
	callID := fmt.Sprintf("development-%s-review-%d", role, time.Now().UnixNano())
	ctx = agent.WithToolCallContext(ctx, callID, sink, nil, false)
	result, err := taskTool.RunProfileSpec(ctx, developmentReviewProfileSpec(role, systemPrompt, task, aiCfg.MaxSteps))
	return strings.TrimSpace(result), usage, me.Name + "/" + me.Model, err
}

func developmentReviewProfileSpec(role, systemPrompt, task string, maxSteps int) agent.ProfileExecSpec {
	return agent.ProfileExecSpec{
		Task: agent.TaskSpec{
			Objective:   task,
			Description: role + " development review",
		},
		Worker: agent.WorkerSpec{
			Kind:         "task",
			Name:         role + "-review",
			SystemPrompt: systemPrompt,
		},
		Grant: agent.CapabilityGrant{
			ReadOnly:     true,
			AllowNoTools: true,
		},
		Context: agent.ContextRequest{Ephemeral: true},
		Sched:   agent.SchedulerPolicy{MaxSteps: maxSteps},
	}
}

func developmentReviewPermissionGate(cfg *config.Config) agent.Gate {
	return trySubagentPermissionGate(permission.New(cfg.Permissions.Mode, cfg.Permissions.Allow, cfg.Permissions.Ask, cfg.Permissions.Deny).WithAllowDynamicBashFallback(cfg.Permissions.AllowDynamicBash))
}

const realtimeReviewPrompt = `You are the real-time reviewer in a three-AI development studio. You are strictly read-only. Inspect the current working tree and recent evidence. Report only new, actionable issues. Use four levels: 阻断, 高风险, 建议, 观察. Include file:line when possible. Never edit files and never expose hidden chain-of-thought.`
const finalReviewPrompt = `You are the independent final acceptance AI in a three-AI development studio. You are strictly read-only and did not author the implementation. Audit the complete working tree, requirements evidence, tests, security, regressions and maintainability. End with exactly one verdict: 通过, 条件通过, or 不通过. Include file:line findings and remaining verification gaps. Never edit files and never expose hidden chain-of-thought.`

func (a *App) DevelopmentStudio(tabID string) DevelopmentStudioSnapshot {
	if a.developmentStudio == nil {
		a.developmentStudio = newDevelopmentStudioManager(a)
	}
	m := a.developmentStudio
	m.mu.Lock()
	state := m.tabLocked(tabID)
	snapshot := DevelopmentStudioSnapshot{Config: m.config, Paused: state.paused, Lead: state.lead, Realtime: state.realtime, Final: state.final, Events: append([]DevelopmentStudioEvent(nil), state.events...)}
	m.mu.Unlock()
	snapshot.Models = a.ModelsForTab(tabID)
	return snapshot
}

func (a *App) SaveDevelopmentStudioConfig(input DevelopmentStudioConfig) error {
	if a.developmentStudio == nil {
		a.developmentStudio = newDevelopmentStudioManager(a)
	}
	next := normalizeDevelopmentStudioConfig(input)
	body, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := fileutil.AtomicWriteFile(developmentStudioConfigPath(), body, 0o600); err != nil {
		return err
	}
	a.developmentStudio.mu.Lock()
	a.developmentStudio.config = next
	a.developmentStudio.mu.Unlock()
	return nil
}

func (a *App) PauseDevelopmentStudio(tabID string, paused bool) {
	if a.developmentStudio == nil {
		return
	}
	m := a.developmentStudio
	m.mu.Lock()
	state := m.tabLocked(tabID)
	state.paused = paused
	if paused {
		state.realtimePending = false
		if state.realtimeTimer != nil {
			state.realtimeTimer.Stop()
			state.realtimeTimer = nil
		}
		if state.cancelRealtime != nil {
			state.cancelRealtime()
		}
		if state.cancelFinal != nil {
			state.cancelFinal()
		}
	}
	m.publishLocked(state, DevelopmentStudioEvent{TabID: tabID, Role: "system", Kind: "status", Title: map[bool]string{true: "三 AI 协作已暂停", false: "三 AI 协作已恢复"}[paused], Status: map[bool]string{true: "paused", false: "running"}[paused]})
	m.mu.Unlock()
}

func (m *developmentStudioManager) cancelAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, state := range m.tabs {
		if state.realtimeTimer != nil {
			state.realtimeTimer.Stop()
			state.realtimeTimer = nil
		}
		if state.cancelRealtime != nil {
			state.cancelRealtime()
		}
		if state.cancelFinal != nil {
			state.cancelFinal()
		}
	}
}

func (a *App) ClearDevelopmentStudio(tabID string) {
	if a.developmentStudio == nil {
		return
	}
	a.developmentStudio.mu.Lock()
	state := a.developmentStudio.tabLocked(tabID)
	state.events = nil
	state.sequence = 0
	a.developmentStudio.mu.Unlock()
}

func (a *App) RunFinalDevelopmentReview(tabID string) error {
	if a.developmentStudio == nil {
		return fmt.Errorf("development studio is unavailable")
	}
	m := a.developmentStudio
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.tabLocked(tabID)
	if state.finalRunning {
		return fmt.Errorf("final review is already running")
	}
	if state.paused {
		return fmt.Errorf("development studio is paused")
	}
	m.startFinalLocked(tabID, state)
	return nil
}
