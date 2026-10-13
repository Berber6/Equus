package llm

// FinishReason describes why the model stopped generating.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonCancelled     FinishReason = "cancelled"
	FinishReasonError         FinishReason = "error"
	FinishReasonUnknown       FinishReason = "unknown"
)

// TokenUsage reports token accounting for one request.
type TokenUsage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
}

// Response is the neutral result of one model request.
type Response struct {
	ID                   string
	Message              ResponseItem
	FinishReason         FinishReason
	ProviderFinishReason string
	TokenUsage           TokenUsage
}

func (reason FinishReason) Valid() bool {
	switch reason {
	case FinishReasonStop,
		FinishReasonLength,
		FinishReasonToolCalls,
		FinishReasonContentFilter,
		FinishReasonCancelled,
		FinishReasonError,
		FinishReasonUnknown:
		return true
	default:
		return false
	}
}
