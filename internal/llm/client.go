// Package llm defines the provider-neutral model domain used by Equus.
// It mirrors the core abstractions of the Amadeus project, reduced to the
// minimum needed for basic chat and tool calling.
package llm

import "context"

// Client is the transport-neutral contract every model adapter implements.
type Client interface {
	// Complete performs a blocking request and returns the full response.
	Complete(ctx context.Context, request Request) (Response, error)
	// Stream performs a streaming request and returns a Stream of chunks.
	Stream(ctx context.Context, request Request) (Stream, error)
	// Model reports the model identity used for requests.
	Model() ModelInfo
}

// ModelInfo describes the configured model.
type ModelInfo struct {
	Provider      string
	Name          string
	ContextWindow int64
}

// Capabilities reports what a concrete transport supports.
type Capabilities struct {
	SupportsStreaming     bool
	SupportsDeveloperRole bool
	SupportsToolCalls     bool
}
