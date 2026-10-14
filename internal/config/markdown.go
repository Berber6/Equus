package config

// MarkdownConfig controls assistant output rendering.
type MarkdownConfig struct {
	// Enabled renders assistant markdown (headings, lists, code) with styling.
	Enabled bool `yaml:"enabled"`
	// SyntaxHighlight highlights fenced code blocks with chroma.
	SyntaxHighlight bool `yaml:"syntax_highlight"`
}

func defaultMarkdown() MarkdownConfig {
	return MarkdownConfig{Enabled: true, SyntaxHighlight: true}
}
