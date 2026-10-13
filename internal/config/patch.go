package config

import (
	"fmt"
	"os"
	"time"
)

// lookupEnv is a seam for tests.
var lookupEnv = os.LookupEnv

func errEnvMissing(name string) error {
	return fmt.Errorf("environment variable %s is not set", name)
}

// configPatch is the optional overlay decoded from the YAML file.
type configPatch struct {
	Model          *string                      `yaml:"model"`
	ModelProvider  *string                      `yaml:"model_provider"`
	ModelProviders map[string]ModelProviderInfo `yaml:"model_providers"`
	Logging        *LoggingPatch                `yaml:"logging"`
}

type LoggingPatch struct {
	Level    *string `yaml:"level"`
	TraceLLM *bool   `yaml:"trace_llm"`
}

func (patch configPatch) apply(base Config) Config {
	if patch.Model != nil {
		base.Model = *patch.Model
	}
	if patch.ModelProvider != nil {
		base.ModelProvider = *patch.ModelProvider
	}
	if patch.ModelProviders != nil {
		merged := make(map[string]ModelProviderInfo, len(patch.ModelProviders))
		for name, info := range patch.ModelProviders {
			if info.WireAPI == "" {
				info.WireAPI = WireAPIChatCompletions
			}
			if info.Timeout == 0 {
				info.Timeout = 2 * time.Minute
			}
			merged[name] = info
		}
		base.ModelProviders = merged
	}
	if patch.Logging != nil {
		if patch.Logging.Level != nil {
			base.Logging.Level = LogLevel(*patch.Logging.Level)
		}
		if patch.Logging.TraceLLM != nil {
			base.Logging.TraceLLM = *patch.Logging.TraceLLM
		}
	}
	return base
}
