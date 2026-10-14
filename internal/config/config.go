// Package config loads, validates and normalizes the Equus configuration.
package config

import "time"

// LogLevel names the supported logging levels.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// WireAPI selects the request dialect used by a provider adapter.
type WireAPI string

const (
	WireAPIChatCompletions WireAPI = "chat_completions"
)

// Config is the effective Equus configuration.
type Config struct {
	Model          string                       `yaml:"model"`
	ModelProvider  string                       `yaml:"model_provider"`
	ModelProviders map[string]ModelProviderInfo `yaml:"model_providers"`
	Logging        LoggingConfig                `yaml:"logging"`
	Approval       ApprovalConfig               `yaml:"approval"`
	Compaction     CompactionConfig             `yaml:"compaction"`
}

// ModelProviderInfo describes one model provider transport. The API key can
// be given inline (api_key), or loaded from a file (api_key_file) whose
// content is trimmed; api_key takes precedence when both are set.
type ModelProviderInfo struct {
	WireAPI    WireAPI       `yaml:"wire_api"`
	APIKey     string        `yaml:"api_key"`
	APIKeyFile string        `yaml:"api_key_file"`
	BaseURL    string        `yaml:"base_url"`
	Timeout    time.Duration `yaml:"timeout"`
}

// LoggingConfig controls runtime logging behavior.
type LoggingConfig struct {
	Level    LogLevel `yaml:"level"`
	TraceLLM bool     `yaml:"trace_llm"`
}

// Default returns the built-in configuration defaults.
func Default() Config {
	return Config{
		ModelProvider:  "openai",
		ModelProviders: make(map[string]ModelProviderInfo),
		Logging:        LoggingConfig{Level: LogLevelInfo},
		Approval:       defaultApproval(),
		Compaction:     defaultCompaction(),
	}
}

// ResolveProvider returns the provider selected by ModelProvider, falling
// back to the single configured provider when the alias is ambiguous.
func (configured Config) ResolveProvider() (ModelProviderInfo, string, error) {
	name := configured.ModelProvider
	if name == "" {
		if len(configured.ModelProviders) == 1 {
			for only := range configured.ModelProviders {
				name = only
			}
		} else {
			return ModelProviderInfo{}, "", errNoProviderSelected
		}
	}
	info, ok := configured.ModelProviders[name]
	if !ok {
		return ModelProviderInfo{}, name, errProviderUnknown(name)
	}
	return info, name, nil
}
