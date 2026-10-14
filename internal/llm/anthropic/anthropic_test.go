package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Godric-W/Equus/internal/llm"
)

const completionPayload = `{
	"id": "msg_1",
	"model": "claude",
	"content": [
		{"type": "text", "text": "你好，我是 Equus。"},
		{"type": "tool_use", "id": "tool_1", "name": "read_file", "input": {"path": "a.txt"}}
	],
	"stop_reason": "tool_use",
	"usage": {"input_tokens": 12, "output_tokens": 8}
}`

func TestCompleteDecodesMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/messages" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("x-api-key") != "sk-test" {
			t.Errorf("x-api-key = %q", request.Header.Get("x-api-key"))
		}
		if request.Header.Get("anthropic-version") == "" {
			t.Error("anthropic-version header missing")
		}
		var body map[string]any
		json.NewDecoder(request.Body).Decode(&body)
		if body["system"] != "You are Equus." {
			t.Errorf("system = %v", body["system"])
		}
		if body["max_tokens"] == nil {
			t.Error("max_tokens missing")
		}
		writer.Write([]byte(completionPayload))
	}))
	defer server.Close()

	client, err := NewClient(Options{APIKey: "sk-test", BaseURL: server.URL, Model: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	request := llm.NewRequest("claude", []llm.ResponseItem{llm.UserMessage("hi")})
	request.Prompt.BaseInstructions = "You are Equus."
	response, err := client.Complete(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Message.Content != "你好，我是 Equus。" {
		t.Fatalf("content = %q", response.Message.Content)
	}
	if response.FinishReason != llm.FinishReasonToolCalls {
		t.Fatalf("finish = %q", response.FinishReason)
	}
	if len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].Name != "read_file" {
		t.Fatalf("tool calls = %+v", response.Message.ToolCalls)
	}
	if string(response.Message.ToolCalls[0].Arguments) != `{"path": "a.txt"}` {
		t.Fatalf("tool args = %s", response.Message.ToolCalls[0].Arguments)
	}
	if response.TokenUsage.InputTokens != 12 || response.TokenUsage.TotalTokens != 20 {
		t.Fatalf("usage = %+v", response.TokenUsage)
	}
}

func TestToolResultFoldsIntoUserMessage(t *testing.T) {
	request := llm.Request{Prompt: llm.Prompt{Input: []llm.ResponseItem{
		llm.UserMessage("do it"),
		llm.AssistantToolCallMessage("", llm.ToolCall{ID: "t1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)}),
		llm.ToolResultMessage("t1", "file contents"),
	}}}
	client, _ := NewClient(Options{APIKey: "k", BaseURL: "http://x", Model: "claude"})
	body, err := client.newRequestBody(request, false)
	if err != nil {
		t.Fatal(err)
	}
	var decoded messageRequest
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	// user, assistant(tool_use), user(tool_result)
	if len(decoded.Messages) != 3 {
		t.Fatalf("messages = %d: %+v", len(decoded.Messages), decoded.Messages)
	}
	last := decoded.Messages[2]
	if last.Role != "user" || last.Content[0].Type != "tool_result" || last.Content[0].ToolUseID != "t1" {
		t.Fatalf("tool result not folded into user message: %+v", last)
	}
}

func TestStreamDecodesTextAndToolArgs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		for _, event := range []string{
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"你"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"好"}}`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"read_file"}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"pa"}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"th\":\"a\"}"}}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`,
			`data: {"type":"message_stop"}`,
		} {
			writer.Write([]byte(event + "\n\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client, _ := NewClient(Options{APIKey: "sk", BaseURL: server.URL, Model: "claude"})
	stream, err := client.Stream(context.Background(), llm.NewRequest("claude", []llm.ResponseItem{llm.UserMessage("hi")}))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var content strings.Builder
	var calls []llm.ToolCall
	var finish llm.FinishReason
	for {
		chunk, err := stream.Recv()
		if err != nil {
			break
		}
		content.WriteString(chunk.ContentDelta)
		calls = append(calls, chunk.ToolCalls...)
		if chunk.FinishReason != "" {
			finish = chunk.FinishReason
		}
	}
	if content.String() != "你好" {
		t.Fatalf("content = %q", content.String())
	}
	if finish != llm.FinishReasonToolCalls {
		t.Fatalf("finish = %q", finish)
	}
	joined := ""
	for _, c := range calls {
		joined += c.ID + c.Name + string(c.Arguments)
	}
	if joined != "t1read_file{\"path\":\"a\"}" {
		t.Fatalf("joined = %q", joined)
	}
}

func TestProviderErrorNotRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()
	client, _ := NewClient(Options{APIKey: "sk", BaseURL: server.URL, Model: "claude"})
	_, err := client.Complete(context.Background(), llm.NewRequest("claude", []llm.ResponseItem{llm.UserMessage("hi")}))
	if err == nil || llm.IsRetryable(err) {
		t.Fatalf("expected non-retryable provider error, got %v", err)
	}
}
