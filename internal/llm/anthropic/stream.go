package anthropic

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Godric-W/Equus/internal/llm"
)

// eventStream consumes the Anthropic Messages SSE body. Events carry an
// "event:" name and a "data:" JSON payload; payload "type" matches the name.
type eventStream struct {
	body       io.ReadCloser
	reader     *bufio.Reader
	finished   bool
	stopReason string
	usage      *llm.TokenUsage
}

func newEventStream(body io.ReadCloser) *eventStream {
	return &eventStream{body: body, reader: bufio.NewReaderSize(body, 64<<10)}
}

func (stream *eventStream) Close() error { return stream.body.Close() }

// eventPayload is the subset of a streamed event we decode.
type eventPayload struct {
	Type  string `json:"type"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Message struct {
		Usage *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage *struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
	Index int `json:"index"`
}

func (stream *eventStream) Recv() (llm.StreamChunk, error) {
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
		// We only need the data lines; the event name is redundant with the
		// payload's own "type" field.
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}
		var event eventPayload
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return llm.StreamChunk{}, fmt.Errorf("decode stream event: %w", err)
		}
		chunk, terminal := decodeEvent(event)
		if terminal {
			stream.finished = true
		}
		if chunk != nil {
			return *chunk, nil
		}
		// No user-visible content (e.g. ping / block start); keep reading.
	}
}

// decodeEvent converts one Anthropic event into a neutral chunk. A nil chunk
// means the event carried no user-visible content; terminal reports stream end.
func decodeEvent(event eventPayload) (*llm.StreamChunk, bool) {
	switch event.Type {
	case "content_block_delta":
		chunk := llm.StreamChunk{}
		switch event.Delta.Type {
		case "text_delta":
			chunk.ContentDelta = event.Delta.Text
		case "input_json_delta":
			// Tool-call arguments arrive as JSON fragments.
			chunk.ToolCalls = []llm.ToolCall{{Arguments: json.RawMessage(event.Delta.PartialJSON)}}
		}
		return &chunk, false
	case "content_block_start":
		if event.ContentBlock.Type == "tool_use" {
			chunk := llm.StreamChunk{ToolCalls: []llm.ToolCall{{ID: event.ContentBlock.ID, Name: event.ContentBlock.Name}}}
			return &chunk, false
		}
		return nil, false
	case "message_delta":
		chunk := llm.StreamChunk{FinishReason: mapStopReason(event.Delta.StopReason), ProviderFinishReason: event.Delta.StopReason}
		if event.Usage != nil {
			chunk.TokenUsage = &llm.TokenUsage{
				InputTokens:  event.Usage.InputTokens,
				OutputTokens: event.Usage.OutputTokens,
				TotalTokens:  event.Usage.InputTokens + event.Usage.OutputTokens,
			}
		}
		return &chunk, false
	case "message_stop":
		return nil, true
	default:
		return nil, false
	}
}
