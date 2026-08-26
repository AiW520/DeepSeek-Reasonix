package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestRenderTOMLPersistsOfflineEnvironment(t *testing.T) {
	cfg := Default()
	cfg.Environment.Offline = true

	for _, scope := range []RenderScope{RenderScopeFull, RenderScopeUser, RenderScopeProject} {
		rendered := RenderTOMLForScope(cfg, scope)
		if !strings.Contains(rendered, "offline = true") {
			t.Fatalf("%s-scope render missing offline declaration", scope)
		}

		back := Default()
		if _, err := toml.Decode(rendered, back); err != nil {
			t.Fatalf("%s-scope round-trip decode: %v", scope, err)
		}
		if !back.Environment.Offline {
			t.Fatalf("%s-scope round-trip lost offline declaration", scope)
		}
	}
}

func TestRenderTOMLPersistsProviderFailoverSettings(t *testing.T) {
	cfg := Default()
	cfg.Providers = []ProviderEntry{{
		Name: "gateway", Kind: "openai", BaseURL: "https://gateway.example/v1",
		Models: []string{"primary", "fallback"}, Default: "primary",
		FallbackModels: []string{"fallback"}, FirstTokenTimeoutSeconds: 45,
		APIKeyEnv: "GATEWAY_API_KEY",
	}}

	for _, scope := range []RenderScope{RenderScopeFull, RenderScopeUser, RenderScopeProject} {
		rendered := RenderTOMLForScope(cfg, scope)
		if !strings.Contains(rendered, `fallback_models = ["fallback"]`) || !strings.Contains(rendered, "first_token_timeout_seconds = 45") {
			t.Fatalf("%s-scope render missing provider failover settings:\n%s", scope, rendered)
		}
		back := Default()
		if _, err := toml.Decode(rendered, back); err != nil {
			t.Fatalf("%s-scope round-trip decode: %v", scope, err)
		}
		entry, ok := back.Provider("gateway")
		if !ok || !reflect.DeepEqual(entry.FallbackModels, []string{"fallback"}) || entry.FirstTokenTimeoutSeconds != 45 {
			t.Fatalf("%s-scope round-trip provider = %+v", scope, entry)
		}
	}
}
