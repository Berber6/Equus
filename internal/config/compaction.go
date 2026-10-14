package config

// CompactionConfig controls automatic context compaction.
type CompactionConfig struct {
	Enabled bool `yaml:"enabled"`
	// Threshold is the fraction of the context window at which compaction
	// triggers (0-1).
	Threshold float64 `yaml:"threshold"`
	// KeepRecentTurns is how many recent user turns stay verbatim.
	KeepRecentTurns int `yaml:"keep_recent_turns"`
}

// defaultCompaction returns the built-in compaction defaults.
func defaultCompaction() CompactionConfig {
	return CompactionConfig{
		Enabled:         true,
		Threshold:       0.8,
		KeepRecentTurns: 4,
	}
}
