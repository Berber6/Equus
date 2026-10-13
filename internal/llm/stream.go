package llm

// Stream is the neutral streaming contract.
type Stream interface {
	// Recv blocks until the next chunk is available. It returns an io.EOF
	// error once the stream is fully consumed.
	Recv() (StreamChunk, error)
	// Close releases the underlying transport resources.
	Close() error
}

// StreamChunk is one incremental piece of a streaming response.
type StreamChunk struct {
	ID                   string
	ContentDelta         string
	ReasoningDelta       string
	FinishReason         FinishReason
	ProviderFinishReason string
	TokenUsage           *TokenUsage
	ToolCalls            []ToolCall
}

// Completed reports whether the chunk carries a terminal finish reason.
func (chunk StreamChunk) Completed() bool {
	return chunk.FinishReason != ""
}
