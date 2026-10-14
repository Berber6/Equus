package openai

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
	"id": "chatcmpl-1",
	"model": "glm-5.3",
	"choices": [{
		"index": 0,
		"finish_reason": "stop",
		"message": {"role": "assistant", "content": "你好，我是 Equus。"}
	}],
	"usage": {"prompt_tokens": 12, "completion_tokens": 8, "total_tokens": 20}
}`

func TestCompleteDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("authorization = %q", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Write([]byte(completionPayload))
	}))
	defer server.Close()

	client, err := NewClient(Options{APIKey: "sk-test", BaseURL: server.URL, Model: "glm-5.3"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Complete(context.Background(), llm.NewRequest("glm-5.3", []llm.ResponseItem{llm.UserMessage("你好")}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Message.Content != "你好，我是 Equus。" {
		t.Fatalf("content = %q", response.Message.Content)
	}
	if response.FinishReason != llm.FinishReasonStop {
		t.Fatalf("finish reason = %q", response.FinishReason)
	}
	if response.TokenUsage.TotalTokens != 20 {
		t.Fatalf("total tokens = %d", response.TokenUsage.TotalTokens)
	}
}

func TestStreamDecodesChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		for _, event := range []string{
			`data: {"id":"c1","choices":[{"index":0,"delta":{"role":"assistant","content":"你"}}]}`,
			`data: {"id":"c1","choices":[{"index":0,"delta":{"content":"好"}}]}`,
			`data: {"id":"c1","choices":[{"index":0,"delta":{"content":"","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"pa"}}]}}]}`,
			`data: {"id":"c1","choices":[{"index":0,"delta":{"content":"","tool_calls":[{"index":0,"function":{"arguments":"th\":\"a.txt\"}"}}]}}]}`,
			`data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
			`data: [DONE]`,
		} {
			writer.Write([]byte(event + "\n\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client, err := NewClient(Options{APIKey: "sk-test", BaseURL: server.URL, Model: "glm-5.3"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Stream(context.Background(), llm.NewRequest("glm-5.3", []llm.ResponseItem{llm.UserMessage("读文件")}))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var content strings.Builder
	var toolCalls []llm.ToolCall
	var sawUsage bool
	for {
		chunk, err := stream.Recv()
		if err != nil {
			break
		}
		content.WriteString(chunk.ContentDelta)
		toolCalls = append(toolCalls, chunk.ToolCalls...)
		if chunk.TokenUsage != nil {
			sawUsage = true
		}
		if chunk.Completed() {
			break
		}
	}
	if content.String() != "你好" {
		t.Fatalf("content = %q", content.String())
	}
	if !sawUsage {
		t.Fatal("usage chunk missing")
	}
	if len(toolCalls) != 2 {
		t.Fatalf("tool call fragments = %d", len(toolCalls))
	}
	joined := toolCalls[0].ID + toolCalls[0].Name + string(toolCalls[0].Arguments) + string(toolCalls[1].Arguments)
	if joined != `call_1read_file{"path":"a.txt"}` {
		t.Fatalf("joined = %q", joined)
	}
}

func TestCompleteSurfacesProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer server.Close()

	client, err := NewClient(Options{APIKey: "sk-test", BaseURL: server.URL, Model: "glm-5.3"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), llm.NewRequest("glm-5.3", []llm.ResponseItem{llm.UserMessage("hi")})); err == nil {
		t.Fatal("expected provider error")
	}
}

func TestProviderErrorIsNotRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		writer.Write([]byte(`{"error":{"message":"Invalid token"}}`))
	}))
	defer server.Close()

	client, err := NewClient(Options{APIKey: "sk-test", BaseURL: server.URL, Model: "glm-5.3"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Stream(context.Background(), llm.NewRequest("glm-5.3", []llm.ResponseItem{llm.UserMessage("hi")}))
	if err == nil {
		t.Fatal("expected provider error")
	}
	if llm.IsRetryable(err) {
		t.Fatalf("provider rejection must not be retryable: %v", err)
	}
}

func TestRequestIncludesSystemAndTools(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		decoder := json.NewDecoder(request.Body)
		decoder.Decode(&body)
		writer.Header().Set("Content-Type", "application/json")
		writer.Write([]byte(completionPayload))
	}))
	defer server.Close()

	client, err := NewClient(Options{APIKey: "sk-test", BaseURL: server.URL, Model: "glm-5.3"})
	if err != nil {
		t.Fatal(err)
	}
	request := llm.NewRequest("glm-5.3", []llm.ResponseItem{llm.UserMessage("hi")})
	request.Prompt.BaseInstructions = "You are Equus."
	request.Prompt.Tools = []llm.ToolSpec{{
		Name: "read_file", Description: "read",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}}
	if _, err := client.Complete(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	messages := body["messages"].([]any)
	first := messages[0].(map[string]any)
	if first["role"] != "system" || first["content"] != "You are Equus." {
		t.Fatalf("first message = %v", first)
	}
	tools := body["tools"].([]any)
	tool := tools[0].(map[string]any)["function"].(map[string]any)
	if tool["name"] != "read_file" {
		t.Fatalf("tool = %v", tool)
	}
}

// TestStreamUsageAfterFinishChunk ensures the terminal usage chunk (sent
// after the finish_reason chunk when stream_options.include_usage is set) is
// delivered to the caller — the agent relies on it for token accounting.
func TestStreamUsageAfterFinishChunk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		for _, event := range []string{
			`data: {"id":"c1","choices":[{"index":0,"delta":{"content":"hi"}}]}`,
			`data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`data: {"id":"c1","choices":[{"index":0,"delta":{}}],"usage":{"prompt_tokens":31,"completion_tokens":5,"total_tokens":36}}`,
			`data: [DONE]`,
		} {
			writer.Write([]byte(event + "\n\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client, err := NewClient(Options{APIKey: "sk-test", BaseURL: server.URL, Model: "glm-5.3"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Stream(context.Background(), llm.NewRequest("glm-5.3", []llm.ResponseItem{llm.UserMessage("hi")}))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	var usage *llm.TokenUsage
	for {
		chunk, err := stream.Recv()
		if err != nil {
			break
		}
		if chunk.TokenUsage != nil {
			usage = chunk.TokenUsage
		}
	}
	if usage == nil || usage.InputTokens != 31 {
		t.Fatalf("usage after finish chunk missed: %+v", usage)
	}
}

// TestStreamRequestsUsage verifies include_usage is set on streamed requests.
func TestStreamRequestsUsage(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		json.NewDecoder(request.Body).Decode(&body)
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	client, _ := NewClient(Options{APIKey: "sk-test", BaseURL: server.URL, Model: "glm-5.3"})
	stream, err := client.Stream(context.Background(), llm.NewRequest("glm-5.3", []llm.ResponseItem{llm.UserMessage("hi")}))
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	opts, ok := body["stream_options"].(map[string]any)
	if !ok || opts["include_usage"] != true {
		t.Fatalf("stream_options.include_usage missing: %v", body["stream_options"])
	}
}

func TestResponsesCompleteDecodes(t *testing.T) {
	payload := `{
		"id": "resp_1",
		"status": "completed",
		"output": [
			{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "hi there"}]},
			{"type": "function_call", "call_id": "c1", "name": "read_file", "arguments": "{\"path\":\"a\"}"}
		],
		"usage": {"input_tokens": 9, "output_tokens": 4, "total_tokens": 13}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" {
			t.Errorf("path = %q", request.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(request.Body).Decode(&body)
		if body["instructions"] != "You are Equus." {
			t.Errorf("instructions = %v", body["instructions"])
		}
		writer.Write([]byte(payload))
	}))
	defer server.Close()

	client, err := NewClient(Options{APIKey: "sk", BaseURL: server.URL, Model: "gpt", WireAPI: "responses"})
	if err != nil {
		t.Fatal(err)
	}
	request := llm.NewRequest("gpt", []llm.ResponseItem{llm.UserMessage("hi")})
	request.Prompt.BaseInstructions = "You are Equus."
	response, err := client.Complete(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Message.Content != "hi there" {
		t.Fatalf("content = %q", response.Message.Content)
	}
	if len(response.Message.ToolCalls) != 1 || response.Message.ToolCalls[0].ID != "c1" {
		t.Fatalf("tool calls = %+v", response.Message.ToolCalls)
	}
	if response.FinishReason != llm.FinishReasonToolCalls {
		t.Fatalf("finish = %q", response.FinishReason)
	}
	if response.TokenUsage.TotalTokens != 13 {
		t.Fatalf("usage = %+v", response.TokenUsage)
	}
}

func TestResponsesStreamDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		flusher := writer.(http.Flusher)
		for _, event := range []string{
			`event: response.output_text.delta`,
			`data: {"type":"response.output_text.delta","delta":"hel"}`,
			`data: {"type":"response.output_text.delta","delta":"lo"}`,
			`data: {"type":"response.output_item.added","item":{"type":"function_call","call_id":"c1","name":"read_file"}}`,
			`data: {"type":"response.function_call_arguments.delta","delta":"{\"pa"}`,
			`data: {"type":"response.function_call_arguments.delta","delta":"th\":\"a\"}"}`,
			`data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":6,"total_tokens":16}}}`,
		} {
			writer.Write([]byte(event + "\n\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	client, _ := NewClient(Options{APIKey: "sk", BaseURL: server.URL, Model: "gpt", WireAPI: "responses"})
	stream, err := client.Stream(context.Background(), llm.NewRequest("gpt", []llm.ResponseItem{llm.UserMessage("hi")}))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var content strings.Builder
	var calls []llm.ToolCall
	var usage *llm.TokenUsage
	for {
		chunk, err := stream.Recv()
		if err != nil {
			break
		}
		content.WriteString(chunk.ContentDelta)
		calls = append(calls, chunk.ToolCalls...)
		if chunk.TokenUsage != nil {
			usage = chunk.TokenUsage
		}
	}
	if content.String() != "hello" {
		t.Fatalf("content = %q", content.String())
	}
	joined := ""
	for _, c := range calls {
		joined += c.ID + c.Name + string(c.Arguments)
	}
	if joined != "c1read_file{\"path\":\"a\"}" {
		t.Fatalf("joined = %q", joined)
	}
	if usage == nil || usage.TotalTokens != 16 {
		t.Fatalf("usage = %+v", usage)
	}
}
