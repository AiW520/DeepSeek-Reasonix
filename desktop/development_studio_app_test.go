package main

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/event"
)

func TestNormalizeDevelopmentStudioConfig(t *testing.T) {
	in := DevelopmentStudioConfig{
		Enabled:      true,
		TeachingMode: "expert",
		Realtime: DevelopmentAIConfig{
			Enabled: true, Model: "  provider/reviewer  ", Effort: " max ", MaxSteps: 99, MaxOutputTokens: 128,
		},
		Final: DevelopmentAIConfig{
			Enabled: true, Model: " provider/auditor ", Effort: " high ", MaxSteps: 12, MaxOutputTokens: 8192,
		},
		MinReviewIntervalMS: 100,
		TurnTokenBudget:     2000,
	}

	got := normalizeDevelopmentStudioConfig(in)
	if got.TeachingMode != "expert" || got.Realtime.Model != "provider/reviewer" || got.Realtime.Effort != "max" {
		t.Fatalf("valid values were not normalized: %+v", got)
	}
	if got.Realtime.MaxSteps != 6 || got.Realtime.MaxOutputTokens != 2200 {
		t.Fatalf("out-of-range realtime limits should fall back to defaults: %+v", got.Realtime)
	}
	if got.Final.MaxSteps != 12 || got.Final.MaxOutputTokens != 8192 {
		t.Fatalf("valid final limits changed: %+v", got.Final)
	}
	if got.MinReviewIntervalMS != 4500 || got.TurnTokenBudget != 60000 {
		t.Fatalf("out-of-range guardrails should fall back to defaults: %+v", got)
	}
}

func TestDevelopmentReviewSeverity(t *testing.T) {
	tests := []struct {
		name, report, role, severity, status string
	}{
		{name: "blocker", report: "阻断：认证绕过", role: "realtime", severity: "blocker", status: "failed"},
		{name: "high", report: "条件通过：需要补集成测试", role: "final", severity: "high", status: "warning"},
		{name: "final pass", report: "通过", role: "final", severity: "suggestion", status: "passed"},
		{name: "realtime advice", report: "建议补充边界用例", role: "realtime", severity: "suggestion", status: "reviewed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			severity, status := reviewSeverity(tt.report, tt.role)
			if severity != tt.severity || status != tt.status {
				t.Fatalf("got (%q, %q), want (%q, %q)", severity, status, tt.severity, tt.status)
			}
		})
	}
}

func TestDevelopmentStudioReviewIntervalSchedulesWakeup(t *testing.T) {
	m := &developmentStudioManager{config: defaultDevelopmentStudioConfig(), tabs: map[string]*developmentStudioTab{}}
	m.config.MinReviewIntervalMS = 60000
	state := m.tabLocked("tab-1")
	state.turnActive = true
	state.lastRealtimeRun = time.Now()

	m.queueRealtimeLocked("tab-1", state)
	if !state.realtimePending || state.realtimeTimer == nil {
		t.Fatalf("review inside the interval must be pending with a wakeup timer: %+v", state)
	}
	state.realtimeTimer.Stop()
	state.realtimeTimer = nil
}

func TestDevelopmentStudioTurnStartResetsFinalReview(t *testing.T) {
	m := &developmentStudioManager{config: defaultDevelopmentStudioConfig(), tabs: map[string]*developmentStudioTab{}}
	m.config.Enabled = false
	state := m.tabLocked("tab-1")
	state.finalTriggered = true

	m.Observe("tab-1", event.Event{Kind: event.TurnStarted})
	if state.finalTriggered {
		t.Fatal("a new turn must allow one new final review")
	}
	if !state.turnActive || state.lead.State != "running" || len(state.events) != 1 {
		t.Fatalf("turn start did not initialize the studio state: %+v", state)
	}
}

func TestDevelopmentStudioCancelAll(t *testing.T) {
	m := &developmentStudioManager{config: defaultDevelopmentStudioConfig(), tabs: map[string]*developmentStudioTab{}}
	state := m.tabLocked("tab-1")
	realtimeCtx, cancelRealtime := context.WithCancel(context.Background())
	finalCtx, cancelFinal := context.WithCancel(context.Background())
	state.cancelRealtime = cancelRealtime
	state.cancelFinal = cancelFinal
	state.realtimeTimer = time.AfterFunc(time.Hour, func() {})

	m.cancelAll()
	select {
	case <-realtimeCtx.Done():
	default:
		t.Fatal("realtime review was not cancelled")
	}
	select {
	case <-finalCtx.Done():
	default:
		t.Fatal("final review was not cancelled")
	}
	if state.realtimeTimer != nil {
		t.Fatal("review wakeup timer was not cleared")
	}
}

func TestDevelopmentStudioReviewRunnerUsesGovernedReadOnlySpec(t *testing.T) {
	m := newDevelopmentStudioManager(nil)
	if m.reviewScheduler != nil {
		t.Fatal("review scheduler must be lazy so App construction does not resolve credentials")
	}
	reviewScheduler, reviewTranscripts := m.reviewRuntime(config.Default())
	if reviewScheduler == nil || reviewTranscripts == nil {
		t.Fatal("development reviews must use the shared scheduler and transcript lifecycle")
	}
	total, writers := reviewScheduler.Limits()
	if total < 1 || writers < 1 || writers > total {
		t.Fatalf("invalid review scheduler limits: total=%d writers=%d", total, writers)
	}

	spec := developmentReviewProfileSpec("realtime", realtimeReviewPrompt, "review task", 7)
	if !spec.Grant.ReadOnly || !spec.Grant.AllowNoTools {
		t.Fatalf("review grant is not strictly read-only: %+v", spec.Grant)
	}
	if !spec.Context.Ephemeral {
		t.Fatalf("review context must be ephemeral: %+v", spec.Context)
	}
	if spec.Sched.MaxSteps != 7 || spec.Sched.RunInBackground {
		t.Fatalf("review scheduler policy changed: %+v", spec.Sched)
	}
	if spec.Worker.Name != "realtime-review" || spec.Worker.SystemPrompt != realtimeReviewPrompt {
		t.Fatalf("review worker identity changed: %+v", spec.Worker)
	}
}

func TestDevelopmentStudioEventHistoryIsBounded(t *testing.T) {
	m := &developmentStudioManager{tabs: map[string]*developmentStudioTab{}}
	state := m.tabLocked("tab-1")
	for i := 0; i < developmentStudioMaxEvents+5; i++ {
		m.publishLocked(state, DevelopmentStudioEvent{TabID: "tab-1", Title: "event"})
	}
	if len(state.events) != developmentStudioMaxEvents {
		t.Fatalf("got %d events, want %d", len(state.events), developmentStudioMaxEvents)
	}
	if state.events[0].Sequence != 6 {
		t.Fatalf("oldest retained event sequence = %d, want 6", state.events[0].Sequence)
	}
}
