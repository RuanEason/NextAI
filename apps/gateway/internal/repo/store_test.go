package repo

import (
	"os"
	"path/filepath"
	"testing"

	"copaw-next/apps/gateway/internal/provider"
)

func TestLoadMigratesLegacyActiveCustomProviderToOpenAI(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	raw := `{
  "providers": {
    "demo": {"enabled": true},
    "openai": {"enabled": true},
    "custom-openai": {
      "api_key": "sk-legacy",
      "base_url": "http://127.0.0.1:19002/v1",
      "display_name": "Legacy Gateway",
      "enabled": true,
      "headers": {"X-Test": "1"},
      "timeout_ms": 12000,
      "model_aliases": {"fast": "gpt-4o-mini"}
    }
  },
  "active_llm": {"provider_id": "custom-openai", "model": "legacy-model"}
}`
	if err := os.WriteFile(statePath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write state failed: %v", err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}

	store.Read(func(st *State) {
		if len(st.Providers) != 2 {
			t.Fatalf("expected only builtin providers, got=%d", len(st.Providers))
		}
		if _, ok := st.Providers["demo"]; !ok {
			t.Fatalf("demo provider should exist")
		}
		openai, ok := st.Providers["openai"]
		if !ok {
			t.Fatalf("openai provider should exist")
		}
		if _, ok := st.Providers["custom-openai"]; ok {
			t.Fatalf("custom provider should be removed after migration")
		}

		if openai.DisplayName != "Legacy Gateway" {
			t.Fatalf("expected display_name migrated, got=%q", openai.DisplayName)
		}
		if openai.APIKey != "sk-legacy" {
			t.Fatalf("expected api_key migrated, got=%q", openai.APIKey)
		}
		if openai.BaseURL != "http://127.0.0.1:19002/v1" {
			t.Fatalf("expected base_url migrated, got=%q", openai.BaseURL)
		}
		if openai.TimeoutMS != 12000 {
			t.Fatalf("expected timeout_ms migrated, got=%d", openai.TimeoutMS)
		}
		if openai.ModelAliases["fast"] != "gpt-4o-mini" {
			t.Fatalf("expected model_aliases migrated, got=%v", openai.ModelAliases)
		}

		if st.ActiveLLM.ProviderID != "openai" {
			t.Fatalf("expected active provider migrated to openai, got=%q", st.ActiveLLM.ProviderID)
		}
		if st.ActiveLLM.Model != provider.DefaultModelID("openai") {
			t.Fatalf("expected active model fallback to openai default, got=%q", st.ActiveLLM.Model)
		}
	})
}
