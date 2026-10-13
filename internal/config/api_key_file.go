package config

import (
	"fmt"
	"os"
	"strings"
)

// ResolveAPIKey returns the provider API key, reading it from api_key_file
// when no inline api_key is configured. Relative key file paths resolve
// against the Equus home directory.
func ResolveAPIKey(provider ModelProviderInfo, equusRoot string) (string, error) {
	if strings.TrimSpace(provider.APIKey) != "" {
		return strings.TrimSpace(provider.APIKey), nil
	}
	path := strings.TrimSpace(provider.APIKeyFile)
	if path == "" {
		return "", nil
	}
	if !strings.HasPrefix(path, "/") && equusRoot != "" {
		path = equusRoot + "/" + path
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read api_key_file %q: %w", provider.APIKeyFile, err)
	}
	key := strings.TrimSpace(string(content))
	if key == "" {
		return "", fmt.Errorf("api_key_file %q is empty", provider.APIKeyFile)
	}
	return key, nil
}
