package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

type failoverTestProvider struct {
	name   string
	stream func(context.Context) (<-chan Chunk, error)
}

func (p failoverTestProvider) Name() string { return p.name }
func (p failoverTestProvider) Stream(ctx context.Context, _ Request) (<-chan Chunk, error) {
	return p.stream(ctx)
}

func chunkStream(chunks ...Chunk) func(context.Context) (<-chan Chunk, error) {
	return func(context.Context) (<-chan Chunk, error) {
		out := make(chan Chunk, len(chunks))
		for _, chunk := range chunks {
			out <- chunk
		}
		close(out)
		return out, nil
	}
}

func TestFailoverProviderSwitchesOnFirstTokenTimeout(t *testing.T) {
	primary := failoverTestProvider{name: "gateway", stream: func(ctx context.Context) (<-chan Chunk, error) {
		out := make(chan Chunk)
		go func() {
			<-ctx.Done()
			close(out)
		}()
		return out, nil
	}}
	fallback := failoverTestProvider{name: "gateway", stream: chunkStream(
		Chunk{Type: ChunkText, Text: "OK"}, Chunk{Type: ChunkDone},
	)}
	prov := NewFailoverProvider("gateway/slow", primary, []FailoverCandidate{{Ref: "gateway/fast", Provider: fallback}}, 10*time.Millisecond)
	notified := make(chan FailoverInfo, 1)
	ctx := WithFailoverNotify(context.Background(), func(info FailoverInfo) { notified <- info })
	stream, err := prov.Stream(ctx, Request{})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for chunk := range stream {
		if chunk.Type == ChunkText {
			text.WriteString(chunk.Text)
		}
		if chunk.Type == ChunkError {
			t.Fatalf("unexpected error: %v", chunk.Err)
		}
	}
	if text.String() != "OK" {
		t.Fatalf("text = %q, want OK", text.String())
	}
	select {
	case info := <-notified:
		if info.From != "gateway/slow" || info.To != "gateway/fast" || info.Reason != "first_token_timeout" {
			t.Fatalf("notification = %+v", info)
		}
	default:
		t.Fatal("missing failover notification")
	}
}

func TestFailoverProviderSwitchesOnRouteFailure(t *testing.T) {
	primary := failoverTestProvider{name: "gateway", stream: chunkStream(Chunk{Type: ChunkError, Err: &APIError{
		Provider: "gateway", Status: http.StatusBadRequest, Body: `{"code":"no_available_channel"}`,
	}})}
	fallback := failoverTestProvider{name: "gateway", stream: chunkStream(Chunk{Type: ChunkText, Text: "OK"}, Chunk{Type: ChunkDone})}
	prov := NewFailoverProvider("gateway/a", primary, []FailoverCandidate{{Ref: "gateway/b", Provider: fallback}}, time.Second)
	stream, _ := prov.Stream(context.Background(), Request{})
	for chunk := range stream {
		if chunk.Type == ChunkError {
			t.Fatalf("unexpected error: %v", chunk.Err)
		}
	}
}

func TestFailoverProviderDoesNotSwitchAfterOutput(t *testing.T) {
	primaryErr := errors.New("stream broke")
	primary := failoverTestProvider{name: "gateway", stream: chunkStream(
		Chunk{Type: ChunkText, Text: "partial"}, Chunk{Type: ChunkError, Err: primaryErr},
	)}
	called := false
	fallback := failoverTestProvider{name: "gateway", stream: func(context.Context) (<-chan Chunk, error) {
		called = true
		return nil, errors.New("must not run")
	}}
	prov := NewFailoverProvider("gateway/a", primary, []FailoverCandidate{{Ref: "gateway/b", Provider: fallback}}, time.Second)
	stream, _ := prov.Stream(context.Background(), Request{})
	var gotErr error
	for chunk := range stream {
		if chunk.Type == ChunkError {
			gotErr = chunk.Err
		}
	}
	if !errors.Is(gotErr, primaryErr) || called {
		t.Fatalf("err=%v fallbackCalled=%v", gotErr, called)
	}
}

func TestShouldFailoverRejectsAuthenticationFailure(t *testing.T) {
	if ShouldFailover(&AuthError{Provider: "gateway", Status: http.StatusUnauthorized, HasKey: true}) {
		t.Fatal("401 must not switch models")
	}
	if !ShouldFailover(&AuthError{Provider: "gateway", Status: http.StatusForbidden, HasKey: true, Body: "upstream service exception"}) {
		t.Fatal("upstream 403 should switch models")
	}
}

func TestFailoverProviderDoesNotSwitchOnSynchronousAuthenticationFailure(t *testing.T) {
	primaryErr := &AuthError{Provider: "gateway", Status: http.StatusUnauthorized, HasKey: true}
	primary := failoverTestProvider{name: "gateway", stream: func(context.Context) (<-chan Chunk, error) {
		return nil, primaryErr
	}}
	called := false
	fallback := failoverTestProvider{name: "gateway", stream: func(context.Context) (<-chan Chunk, error) {
		called = true
		return nil, errors.New("must not run")
	}}
	prov := NewFailoverProvider("gateway/a", primary, []FailoverCandidate{{Ref: "gateway/b", Provider: fallback}}, time.Second)
	stream, err := prov.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	var gotErr error
	for chunk := range stream {
		if chunk.Type == ChunkError {
			gotErr = chunk.Err
		}
	}
	if !errors.Is(gotErr, primaryErr) || called {
		t.Fatalf("err=%v fallbackCalled=%v", gotErr, called)
	}
}
