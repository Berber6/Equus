// Package llm defines the provider-neutral model domain used by Equus,
// reduced to the minimum needed for basic chat and tool calling.
package llm

import (
	"context"
	"errors"
)

// Client is the transport-neutral contract every model adapter implements.
type Client interface {
	// Complete performs a blocking request and returns the full response.
	Complete(ctx context.Context, request Request) (Response, error)
	// Stream performs a streaming request and returns a Stream of chunks.
	Stream(ctx context.Context, request Request) (Stream, error)
	// Model reports the model identity used for requests.
	Model() ModelInfo
}

// RetryableError is implemented by errors that are worth retrying in another
// form (e.g. falling back from streaming to a blocking request). Transport
// failures such as a dropped connection satisfy it; a definitive rejection
// from the provider does not, and callers should surface those directly.
type RetryableError interface {
	error
	// Retryable reports whether a fallback attempt could plausibly succeed.
	Retryable() bool
}

// IsRetryable reports whether err is worth retrying in another form. Errors
// that do not implement RetryableError are treated as retryable, preserving
// the long-standing "stream failed, try blocking" fallback for unknown
// failures.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var retryable RetryableError
	if errors.As(err, &retryable) {
		return retryable.Retryable()
	}
	return true
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
