package config

import (
	"errors"
	"fmt"
	"strings"
)

// Validate checks the effective configuration for completeness.
func Validate(configured Config) error {
	var problems []string
	if strings.TrimSpace(configured.Model) == "" {
		problems = append(problems, "model is empty")
	}
	if len(configured.ModelProviders) == 0 {
		problems = append(problems, "model_providers is empty")
	}
	provider, name, err := configured.ResolveProvider()
	if err != nil {
		problems = append(problems, err.Error())
	} else {
		if strings.TrimSpace(provider.APIKey) == "" {
			problems = append(problems, fmt.Sprintf("provider %q api_key is empty", name))
		}
		if strings.TrimSpace(provider.BaseURL) == "" {
			problems = append(problems, fmt.Sprintf("provider %q base_url is empty", name))
		}
	}
	switch configured.Logging.Level {
	case "", LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
	default:
		problems = append(problems, fmt.Sprintf("logging.level %q is invalid", configured.Logging.Level))
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New("invalid configuration: " + strings.Join(problems, "; "))
}

// Redact returns a copy safe for display: API keys are masked.
func Redact(configured Config) Config {
	redacted := configured
	redacted.ModelProviders = make(map[string]ModelProviderInfo, len(configured.ModelProviders))
	for name, info := range configured.ModelProviders {
		if info.APIKey != "" {
			info.APIKey = "********"
		}
		redacted.ModelProviders[name] = info
	}
	return redacted
}
