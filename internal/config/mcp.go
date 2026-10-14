package config

import "time"

// MCPServerConfig describes one MCP server (stdio transport).
type MCPServerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Timeout time.Duration     `yaml:"timeout,omitempty"`
	// Enabled defaults to true when unset.
	Enabled *bool `yaml:"enabled,omitempty"`
}

// IsEnabled reports whether the server should be connected.
func (s MCPServerConfig) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

// TimeoutOrDefault returns the configured timeout or 30s.
func (s MCPServerConfig) TimeoutOrDefault() time.Duration {
	if s.Timeout <= 0 {
		return 30 * time.Second
	}
	return s.Timeout
}

// MCPConfig is the top-level mcp section.
type MCPConfig struct {
	Servers map[string]MCPServerConfig `yaml:"servers,omitempty"`
}

func defaultMCP() MCPConfig { return MCPConfig{} }
