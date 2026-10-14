package openai

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Godric-W/Equus/internal/llm"
)

// eventStream consumes a Chat Completions SSE body and yields neutral chunks.
type eventStream struct {
	body     io.ReadCloser
	reader   *bufio.Reader
	finished bool
}

func newEventStream(body io.ReadCloser) *eventStream {
	return &eventStream{body: body, reader: bufio.NewReaderSize(body, 64<<10)}
}

func (stream *eventStream) Recv() (llm.StreamChunk, error) {
	if stream.finished {
		return llm.StreamChunk{}, io.EOF
	}
	for {
		line, err := stream.reader.ReadString('\n')
		if err != nil {
			stream.finished = true
			if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
				return llm.StreamChunk{}, io.EOF
			}
			return llm.StreamChunk{}, fmt.Errorf("read stream line: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "[DONE]" {
			stream.finished = true
			return llm.StreamChunk{}, io.EOF
		}
		chunk, decodeErr := decodeStreamChunk([]byte(payload))
		if decodeErr != nil {
			return llm.StreamChunk{}, decodeErr
		}
		// Note: with stream_options.include_usage, the terminal usage chunk is
		// sent AFTER the finish_reason chunk, so we must not stop on a chunk
		// merely because it carries a finish reason. The stream ends at [DONE]
		// or EOF (handled above).
		return chunk, nil
	}
}

func (stream *eventStream) Close() error {
	return stream.body.Close()
}

type streamChunkWire struct {
	ID      string `json:"id"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Role             string           `json:"role"`
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			ToolCalls        []streamToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
}

type streamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func decodeStreamChunk(payload []byte) (llm.StreamChunk, error) {
	var decoded streamChunkWire
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return llm.StreamChunk{}, fmt.Errorf("decode stream chunk: %w", err)
	}
	chunk := llm.StreamChunk{ID: decoded.ID}
	if len(decoded.Choices) > 0 {
		choice := decoded.Choices[0]
		chunk.ContentDelta = choice.Delta.Content
		chunk.ReasoningDelta = choice.Delta.ReasoningContent
		chunk.FinishReason = mapFinishReason(choice.FinishReason)
		chunk.ProviderFinishReason = choice.FinishReason
		for _, call := range choice.Delta.ToolCalls {
			chunk.ToolCalls = append(chunk.ToolCalls, llm.ToolCall{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: json.RawMessage(call.Function.Arguments),
			})
		}
	}
	if decoded.Usage != nil {
		chunk.TokenUsage = &llm.TokenUsage{
			InputTokens:  decoded.Usage.PromptTokens,
			OutputTokens: decoded.Usage.CompletionTokens,
			TotalTokens:  decoded.Usage.TotalTokens,
		}
	}
	return chunk, nil
}
