package boot

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/config"
	"reasonix/internal/netclient"
	"reasonix/internal/provider"
)

type failoverResolverProvider struct {
	name   string
	chunks []provider.Chunk
}

func (p failoverResolverProvider) Name() string { return p.name }
func (p failoverResolverProvider) Stream(context.Context, provider.Request) (<-chan provider.Chunk, error) {
	out := make(chan provider.Chunk, len(p.chunks))
	for _, chunk := range p.chunks {
		out <- chunk
	}
	close(out)
	return out, nil
}

type recordingFailoverResolver struct {
	mu         sync.Mutex
	selections []string
}

func (r *recordingFailoverResolver) Catalog() []provider.Descriptor { return nil }
func (r *recordingFailoverResolver) Resolve(selection provider.Selection) (provider.Provider, error) {
	r.mu.Lock()
	r.selections = append(r.selections, selection.Ref)
	r.mu.Unlock()
	if selection.Ref == "gateway/primary" {
		return failoverResolverProvider{name: "gateway", chunks: []provider.Chunk{{Type: provider.ChunkError, Err: &provider.APIError{
			Provider: "gateway", Status: http.StatusBadRequest, Body: `{"code":"no_available_channel"}`,
		}}}}, nil
	}
	return failoverResolverProvider{name: "gateway", chunks: []provider.Chunk{{Type: provider.ChunkText, Text: "fallback ok"}, {Type: provider.ChunkDone}}}, nil
}

func TestResolveProviderBuildsOrderedSameProviderFailover(t *testing.T) {
	cfg := &config.Config{Providers: []config.ProviderEntry{{
		Name: "gateway", Kind: "openai", Models: []string{"primary", "fallback"}, Default: "primary",
		FallbackModels: []string{"fallback"}, FirstTokenTimeoutSeconds: 1,
	}}}
	resolver := &recordingFailoverResolver{}
	resolved, err := resolveProvider(resolver, cfg, netclient.ProxySpec{}, provider.Selection{Ref: "gateway/primary"})
	if err != nil {
		t.Fatalf("resolveProvider: %v", err)
	}
	notified := make(chan provider.FailoverInfo, 1)
	ctx := provider.WithFailoverNotify(context.Background(), func(info provider.FailoverInfo) { notified <- info })
	stream, err := resolved.Stream(ctx, provider.Request{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var text strings.Builder
	for chunk := range stream {
		if chunk.Type == provider.ChunkError {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		if chunk.Type == provider.ChunkText {
			text.WriteString(chunk.Text)
		}
	}
	if text.String() != "fallback ok" {
		t.Fatalf("stream text = %q", text.String())
	}
	select {
	case info := <-notified:
		if info.From != "gateway/primary" || info.To != "gateway/fallback" {
			t.Fatalf("failover info = %+v", info)
		}
	case <-time.After(time.Second):
		t.Fatal("missing failover notification")
	}
	resolver.mu.Lock()
	selections := append([]string(nil), resolver.selections...)
	resolver.mu.Unlock()
	if want := []string{"gateway/primary", "gateway/fallback"}; !reflect.DeepEqual(selections, want) {
		t.Fatalf("resolver selections = %v, want %v", selections, want)
	}
}
