package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ConfigFileName is the file searched inside the Equus home directory.
const ConfigFileName = "config.yaml"

// Loader resolves the effective configuration from the Equus home directory.
type Loader struct {
	path     string
	required bool
}

// NewLoader builds a loader rooted at the given Equus home directory.
func NewLoader(equusRoot string) Loader {
	if strings.TrimSpace(equusRoot) == "" {
		return Loader{}
	}
	return Loader{path: filepath.Join(equusRoot, ConfigFileName)}
}

// NewFileLoader builds a loader that requires the exact file to exist.
func NewFileLoader(path string) Loader {
	return Loader{path: path, required: true}
}

// ConfigPath reports the file the loader reads.
func (loader Loader) ConfigPath() string {
	return loader.path
}

// Load reads and validates the configuration file, applying environment
// overrides. A missing optional file yields the defaults.
func (loader Loader) Load() (Config, error) {
	if strings.TrimSpace(loader.path) == "" {
		return Config{}, errors.New("config path is empty")
	}
	file, err := os.Open(loader.path)
	if errors.Is(err, os.ErrNotExist) && !loader.required {
		return applyEnvironmentOverrides(Default())
	}
	if err != nil {
		return Config{}, fmt.Errorf("open config %q: %w", loader.path, err)
	}
	defer file.Close()

	base := Default()
	patched, err := decodeYAMLPatch(file, base)
	if err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", loader.path, err)
	}
	overridden, err := applyEnvironmentOverrides(patched)
	if err != nil {
		return Config{}, err
	}
	return loader.resolveAPIKeys(overridden)
}

// resolveAPIKeys reads provider keys from api_key_file entries whose inline
// api_key is empty, yielding a Config where APIKey is directly usable.
func (loader Loader) resolveAPIKeys(configured Config) (Config, error) {
	equusRoot := ""
	if loader.path != "" {
		equusRoot = filepath.Dir(loader.path)
	}
	resolved := configured
	resolved.ModelProviders = make(map[string]ModelProviderInfo, len(configured.ModelProviders))
	for name, provider := range configured.ModelProviders {
		key, err := ResolveAPIKey(provider, equusRoot)
		if err != nil {
			return Config{}, fmt.Errorf("provider %q: %w", name, err)
		}
		provider.APIKey = key
		resolved.ModelProviders[name] = provider
	}
	return resolved, nil
}

// applyEnvironmentOverrides injects environment variables above the values
// read from the file, mirroring Amadeus' precedence model.
func applyEnvironmentOverrides(configured Config) (Config, error) {
	if model, ok := os.LookupEnv(EnvModel); ok && strings.TrimSpace(model) != "" {
		configured.Model = strings.TrimSpace(model)
	}
	if provider, ok := os.LookupEnv(EnvModelProvider); ok && strings.TrimSpace(provider) != "" {
		configured.ModelProvider = strings.TrimSpace(provider)
	}
	if key, ok := os.LookupEnv(EnvAPIKey); ok && strings.TrimSpace(key) != "" {
		name := configured.ModelProvider
		if name == "" && len(configured.ModelProviders) == 1 {
			for only := range configured.ModelProviders {
				name = only
			}
		}
		if name == "" {
			return Config{}, fmt.Errorf("%s is set but no model provider is selected", EnvAPIKey)
		}
		if _, exists := configured.ModelProviders[name]; !exists {
			configured.ModelProviders[name] = defaultProviderInfo()
		}
		updated := configured.ModelProviders[name]
		updated.APIKey = strings.TrimSpace(key)
		configured.ModelProviders[name] = updated
	}
	if baseURL, ok := os.LookupEnv(EnvBaseURL); ok && strings.TrimSpace(baseURL) != "" {
		name := configured.ModelProvider
		if name == "" && len(configured.ModelProviders) == 1 {
			for only := range configured.ModelProviders {
				name = only
			}
		}
		if name == "" {
			return Config{}, fmt.Errorf("%s is set but no model provider is selected", EnvBaseURL)
		}
		if _, exists := configured.ModelProviders[name]; !exists {
			configured.ModelProviders[name] = defaultProviderInfo()
		}
		updated := configured.ModelProviders[name]
		updated.BaseURL = strings.TrimSpace(baseURL)
		configured.ModelProviders[name] = updated
	}
	return configured, nil
}

func defaultProviderInfo() ModelProviderInfo {
	return ModelProviderInfo{
		WireAPI: WireAPIChatCompletions,
		Timeout: 2 * time.Minute,
	}
}
