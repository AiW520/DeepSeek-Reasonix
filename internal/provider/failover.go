package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const DefaultFirstTokenTimeout = 45 * time.Second

type FailoverCandidate struct {
	Ref      string
	Provider Provider
}

type FailoverInfo struct {
	From    string
	To      string
	Reason  string
	Attempt int
	Max     int
}

type FailoverNotify func(FailoverInfo)

type failoverNotifyKey struct{}

func WithFailoverNotify(ctx context.Context, fn FailoverNotify) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, failoverNotifyKey{}, fn)
}

func failoverNotifyFromContext(ctx context.Context) FailoverNotify {
	fn, _ := ctx.Value(failoverNotifyKey{}).(FailoverNotify)
	return fn
}

type failoverProvider struct {
	primaryRef string
	primary    Provider
	fallbacks  []FailoverCandidate
	timeout    time.Duration
}

func NewFailoverProvider(primaryRef string, primary Provider, fallbacks []FailoverCandidate, timeout time.Duration) Provider {
	if primary == nil || len(fallbacks) == 0 {
		return primary
	}
	if timeout <= 0 {
		timeout = DefaultFirstTokenTimeout
	}
	return &failoverProvider{primaryRef: primaryRef, primary: primary, fallbacks: append([]FailoverCandidate(nil), fallbacks...), timeout: timeout}
}

func (p *failoverProvider) Name() string { return p.primary.Name() }

func (p *failoverProvider) Stream(ctx context.Context, req Request) (<-chan Chunk, error) {
	out := make(chan Chunk)
	go p.stream(ctx, req, out)
	return out, nil
}

func (p *failoverProvider) stream(ctx context.Context, req Request, out chan<- Chunk) {
	defer close(out)
	candidates := make([]FailoverCandidate, 0, len(p.fallbacks)+1)
	candidates = append(candidates, FailoverCandidate{Ref: p.primaryRef, Provider: p.primary})
	candidates = append(candidates, p.fallbacks...)
	var lastErr error
	for i, candidate := range candidates {
		if ctx.Err() != nil {
			return
		}
		attemptCtx, cancel := context.WithCancel(ctx)
		stream, err := candidate.Provider.Stream(attemptCtx, req)
		if err != nil {
			cancel()
			lastErr = err
			if ShouldFailover(err) && p.switchCandidate(ctx, candidates, i, failoverReason(err)) {
				continue
			}
			p.sendError(ctx, out, err)
			return
		}
		timer := time.NewTimer(p.timeout)
		committed := false
		for {
			select {
			case <-ctx.Done():
				cancel()
				stopTimer(timer)
				return
			case <-timer.C:
				cancel()
				go drainChunks(stream)
				lastErr = fmt.Errorf("first model output exceeded %s", p.timeout)
				if p.switchCandidate(ctx, candidates, i, "first_token_timeout") {
					goto nextCandidate
				}
				p.sendError(ctx, out, lastErr)
				return
			case chunk, ok := <-stream:
				if !ok {
					cancel()
					stopTimer(timer)
					lastErr = io.ErrUnexpectedEOF
					if !committed && p.switchCandidate(ctx, candidates, i, "premature_eof") {
						goto nextCandidate
					}
					p.sendError(ctx, out, lastErr)
					return
				}
				if chunk.Type == ChunkError {
					cancel()
					stopTimer(timer)
					lastErr = chunk.Err
					if !committed && ShouldFailover(chunk.Err) && p.switchCandidate(ctx, candidates, i, failoverReason(chunk.Err)) {
						goto nextCandidate
					}
					p.sendChunk(ctx, out, chunk)
					return
				}
				if !committed {
					committed = true
					stopTimer(timer)
				}
				if !p.sendChunk(ctx, out, chunk) {
					cancel()
					return
				}
				if chunk.Type == ChunkDone {
					cancel()
					return
				}
			}
		}
	nextCandidate:
		continue
	}
	if lastErr != nil {
		p.sendError(ctx, out, lastErr)
	}
}

func (p *failoverProvider) switchCandidate(ctx context.Context, candidates []FailoverCandidate, current int, reason string) bool {
	if current+1 >= len(candidates) || ctx.Err() != nil {
		return false
	}
	if notify := failoverNotifyFromContext(ctx); notify != nil {
		notify(FailoverInfo{From: candidates[current].Ref, To: candidates[current+1].Ref, Reason: reason, Attempt: current + 2, Max: len(candidates)})
	}
	return true
}

func (p *failoverProvider) sendChunk(ctx context.Context, out chan<- Chunk, chunk Chunk) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- chunk:
		return true
	}
}

func (p *failoverProvider) sendError(ctx context.Context, out chan<- Chunk, err error) {
	if err == nil {
		err = errors.New("provider failover exhausted")
	}
	p.sendChunk(ctx, out, Chunk{Type: ChunkError, Err: err})
}

func drainChunks(stream <-chan Chunk) {
	for range stream {
	}
}

func stopTimer(timer *time.Timer) {
	if timer != nil && !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func ShouldFailover(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || IsStreamInterrupted(err) || IsConnReset(err) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return RetryableStatus(apiErr.Status) || (apiErr.Status == http.StatusBadRequest && routeFailureText(apiErr.Body))
	}
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return authErr.Status == http.StatusForbidden && routeFailureText(authErr.Body)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return routeFailureText(err.Error())
}

func routeFailureText(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"no_available_channel", "upstream service", "upstream unavailable", "cannot be routed", "bad_response_error"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func failoverReason(err error) string {
	if err == nil {
		return "request_failed"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("http_%d", apiErr.Status)
	}
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return fmt.Sprintf("http_%d", authErr.Status)
	}
	if reason := StreamInterruptReason(err); reason != "" {
		return reason
	}
	if routeFailureText(err.Error()) {
		return "upstream_unavailable"
	}
	return "request_failed"
}

func (p *failoverProvider) RequiresToolCallReasoning() bool {
	return RequiresToolCallReasoning(p.primary)
}
func (p *failoverProvider) RequiresReasoningRoundTrip() bool {
	return RequiresReasoningRoundTrip(p.primary)
}
func (p *failoverProvider) WarnOnMissingToolCallReasoning() bool {
	return WarnOnMissingToolCallReasoning(p.primary)
}
func (p *failoverProvider) MissingToolCallReasoningWarningIdentity() string {
	return MissingToolCallReasoningWarningFingerprint(p.primary)
}
