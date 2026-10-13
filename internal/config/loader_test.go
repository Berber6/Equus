package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenFileMissing(t *testing.T) {
	loader := NewLoader(t.TempDir())
	configured, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if configured.ModelProvider != "openai" {
		t.Fatalf("default provider = %q", configured.ModelProvider)
	}
}

func TestLoadRejectsMultipleDocuments(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := os.WriteFile(path, []byte("model: a\n---\nmodel: b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileLoader(path).Load(); err == nil {
		t.Fatal("expected error for multiple YAML documents")
	}
}

func TestLoadAppliesPatchAndEnvExpansion(t *testing.T) {
	t.Setenv("EQUUS_TEST_KEY", "sk-test-123")
	path := filepath.Join(t.TempDir(), ConfigFileName)
	content := `
model: glm-5.3
model_providers:
  openai:
    wire_api: chat_completions
    api_key: ${EQUUS_TEST_KEY}
    base_url: http://localhost:9/v1
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	configured, err := NewFileLoader(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if configured.Model != "glm-5.3" {
		t.Fatalf("model = %q", configured.Model)
	}
	provider, name, err := configured.ResolveProvider()
	if err != nil {
		t.Fatal(err)
	}
	if name != "openai" || provider.APIKey != "sk-test-123" {
		t.Fatalf("provider = %q key = %q", name, provider.APIKey)
	}
}

func TestLoadFailsOnUnsetEnvReference(t *testing.T) {
	os.Unsetenv("EQUUS_TEST_MISSING")
	path := filepath.Join(t.TempDir(), ConfigFileName)
	content := "model_providers:\n  openai:\n    api_key: ${EQUUS_TEST_MISSING}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileLoader(path).Load(); err == nil {
		t.Fatal("expected error for unset env reference")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := os.WriteFile(path, []byte("modle: typo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileLoader(path).Load(); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestValidateDetectsMissingKey(t *testing.T) {
	configured := Default()
	configured.Model = "m"
	configured.ModelProviders["openai"] = ModelProviderInfo{BaseURL: "http://x/v1"}
	if err := Validate(configured); err == nil {
		t.Fatal("expected validation error for empty api key")
	}
}

func TestEnvAPIKeyOverride(t *testing.T) {
	t.Setenv(EnvAPIKey, "sk-override")
	configured, err := applyEnvironmentOverrides(Default())
	if err != nil {
		t.Fatal(err)
	}
	provider, _, err := configured.ResolveProvider()
	if err != nil {
		t.Fatal(err)
	}
	if provider.APIKey != "sk-override" {
		t.Fatalf("api key = %q", provider.APIKey)
	}
}
