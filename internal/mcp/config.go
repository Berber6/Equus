// Package mcp connects to Model Context Protocol servers over stdio and exposes
// their tools to the agent.
package mcp

import "time"

// ServerConfig describes one MCP server.
type ServerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Timeout time.Duration     `yaml:"timeout,omitempty"`
	// Enabled defaults to true when unset.
	Enabled *bool `yaml:"enabled,omitempty"`
}

// Config is the top-level mcp section.
type Config struct {
	Servers map[string]ServerConfig `yaml:"servers,omitempty"`
}

func (c ServerConfig) enabled() bool {
	return c.Enabled == nil || *c.Enabled
}

func (c ServerConfig) timeout() time.Duration {
	if c.Timeout <= 0 {
		return 30 * time.Second
	}
	return c.Timeout
}
