package llm

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestMessageConstructors(t *testing.T) {
	if m := SystemMessage("s"); m.Role != RoleSystem || m.Content != "s" {
		t.Fatalf("system = %+v", m)
	}
	if m := UserMessage("u"); m.Role != RoleUser || m.Content != "u" {
		t.Fatalf("user = %+v", m)
	}
	if m := AssistantMessage("a"); m.Role != RoleAssistant || m.Content != "a" {
		t.Fatalf("assistant = %+v", m)
	}
	if m := ToolResultMessage("id1", "r"); m.Role != RoleTool || m.ToolCallID != "id1" || m.Content != "r" {
		t.Fatalf("tool = %+v", m)
	}
}

func TestAssistantToolCallMessageClonesCalls(t *testing.T) {
	calls := []ToolCall{{ID: "c1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a"}`)}}
	message := AssistantToolCallMessage("", calls...)
	// Mutating the input slice must not affect the message.
	calls[0].Arguments[7] = 'X'
	if string(message.ToolCalls[0].Arguments) != `{"path":"a"}` {
		t.Fatalf("arguments mutated: %s", message.ToolCalls[0].Arguments)
	}
}

func TestRequestConversationItemsIsCopy(t *testing.T) {
	request := NewRequest("m", []ResponseItem{UserMessage("hi")})
	items := request.ConversationItems()
	items[0].Content = "changed"
	again := request.ConversationItems()
	if again[0].Content != "hi" {
		t.Fatalf("ConversationItems not a copy: %q", again[0].Content)
	}
}

func TestRequestToolSpecsIsCopy(t *testing.T) {
	request := Request{Prompt: Prompt{Tools: []ToolSpec{{Name: "t", InputSchema: json.RawMessage(`{"x":1}`)}}}}
	specs := request.ToolSpecs()
	specs[0].InputSchema[1] = 'Y'
	again := request.ToolSpecs()
	if string(again[0].InputSchema) != `{"x":1}` {
		t.Fatalf("ToolSpecs not a copy: %s", again[0].InputSchema)
	}
}

func TestRoleValidAndDisplay(t *testing.T) {
	valid := []Role{RoleSystem, RoleDeveloper, RoleUser, RoleAssistant, RoleTool}
	for _, role := range valid {
		if !role.Valid() {
			t.Errorf("%q should be valid", role)
		}
	}
	if Role("bogus").Valid() {
		t.Error("bogus role should be invalid")
	}
	if RoleUser.Display() != "you" || RoleAssistant.Display() != "equus" || RoleTool.Display() != "tool" {
		t.Errorf("unexpected display values")
	}
}

func TestFinishReasonValid(t *testing.T) {
	for _, reason := range []FinishReason{FinishReasonStop, FinishReasonLength, FinishReasonToolCalls, FinishReasonUnknown} {
		if !reason.Valid() {
			t.Errorf("%q should be valid", reason)
		}
	}
	if FinishReason("nope").Valid() {
		t.Error("bogus reason should be invalid")
	}
}

type retryableErr struct{ retryable bool }

func (e retryableErr) Error() string   { return "x" }
func (e retryableErr) Retryable() bool { return e.retryable }

func TestIsRetryable(t *testing.T) {
	if IsRetryable(nil) {
		t.Error("nil must not be retryable")
	}
	if !IsRetryable(errors.New("plain")) {
		t.Error("errors without RetryableError default to retryable")
	}
	if IsRetryable(retryableErr{retryable: false}) {
		t.Error("Retryable()==false must not be retryable")
	}
	if !IsRetryable(retryableErr{retryable: true}) {
		t.Error("Retryable()==true must be retryable")
	}
}

func TestStreamChunkCompleted(t *testing.T) {
	if (StreamChunk{}).Completed() {
		t.Error("empty chunk should not be completed")
	}
	if !(StreamChunk{FinishReason: FinishReasonStop}).Completed() {
		t.Error("chunk with finish reason should be completed")
	}
}
