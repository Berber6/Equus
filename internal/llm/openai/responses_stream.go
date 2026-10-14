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

// responsesStream consumes the Responses API SSE body.
type responsesStream struct {
	body     io.ReadCloser
	reader   *bufio.Reader
	finished bool
}

func newResponsesStream(body io.ReadCloser) *responsesStream {
	return &responsesStream{body: body, reader: bufio.NewReaderSize(body, 64<<10)}
}

func (stream *responsesStream) Close() error { return stream.body.Close() }

// responsesEvent is the subset of a streamed Responses event we decode.
type responsesEvent struct {
	Type   string `json:"type"`
	Delta  string `json:"delta"`
	ItemID string `json:"item_id"`
	Item   struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Name   string `json:"name"`
	} `json:"item"`
	Response struct {
		Usage *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
	} `json:"response"`
}

func (stream *responsesStream) Recv() (llm.StreamChunk, error) {
	if stream.finished {
		return llm.StreamChunk{}, io.EOF
	}
	for {
		line, err := stream.reader.ReadString('\n')
		if err != nil {
			stream.finished = true
			if errors.Is(err, io.EOF) {
				return llm.StreamChunk{}, io.EOF
			}
			return llm.StreamChunk{}, fmt.Errorf("read stream line: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "" || payload == "[DONE]" {
			if payload == "[DONE]" {
				stream.finished = true
				return llm.StreamChunk{}, io.EOF
			}
			continue
		}
		var event responsesEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return llm.StreamChunk{}, fmt.Errorf("decode stream event: %w", err)
		}
		chunk, terminal := decodeResponsesEvent(event)
		if terminal {
			stream.finished = true
		}
		if chunk != nil {
			return *chunk, nil
		}
	}
}

func decodeResponsesEvent(event responsesEvent) (*llm.StreamChunk, bool) {
	switch event.Type {
	case "response.output_text.delta":
		return &llm.StreamChunk{ContentDelta: event.Delta}, false
	case "response.function_call_arguments.delta":
		return &llm.StreamChunk{ToolCalls: []llm.ToolCall{{Arguments: json.RawMessage(event.Delta)}}}, false
	case "response.output_item.added":
		if event.Item.Type == "function_call" {
			return &llm.StreamChunk{ToolCalls: []llm.ToolCall{{ID: event.Item.CallID, Name: event.Item.Name}}}, false
		}
		return nil, false
	case "response.completed":
		chunk := llm.StreamChunk{FinishReason: llm.FinishReasonStop}
		if event.Response.Usage != nil {
			chunk.TokenUsage = &llm.TokenUsage{
				InputTokens:  event.Response.Usage.InputTokens,
				OutputTokens: event.Response.Usage.OutputTokens,
				TotalTokens:  event.Response.Usage.TotalTokens,
			}
		}
		return &chunk, true
	default:
		return nil, false
	}
}
