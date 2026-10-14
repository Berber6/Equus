package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withoutSSRFGuard bypasses the public-host guard so tests can reach loopback
// httptest servers; the guard itself is covered by TestWebFetchRejectsNonPublicHosts.
func withoutSSRFGuard(t *testing.T) {
	t.Helper()
	original := webFetchGuard
	webFetchGuard = func(c *http.Client) *http.Client { return c }
	t.Cleanup(func() { webFetchGuard = original })
}

func TestWebFetchRejectsNonPublicHosts(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1/",
		"http://localhost/",
		"http://10.0.0.1/",
		"http://192.168.1.1/",
		"http://169.254.1.1/",
		"http://[::1]/",
	} {
		if isPublicHost(strings.TrimPrefix(strings.TrimPrefix(raw, "http://"), "https://")) {
			t.Errorf("host from %q should not be public", raw)
		}
	}
}

func TestWebFetchRejectsBadScheme(t *testing.T) {
	tool := WebFetchTool{}
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"ftp://example.com"}`)); err == nil {
		t.Fatal("expected scheme error")
	}
}

func TestWebFetchStripsHTML(t *testing.T) {
	withoutSSRFGuard(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><style>.x{}</style></head><body><h1>Title</h1><script>var x=1;</script><p>Hello &amp; welcome</p></body></html>`))
	}))
	defer server.Close()

	tool := WebFetchTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"url":"`+server.URL+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Title") || !strings.Contains(result, "Hello & welcome") {
		t.Fatalf("content = %q", result)
	}
	if strings.Contains(result, "var x=1") || strings.Contains(result, "<h1>") {
		t.Fatalf("html not stripped: %q", result)
	}
	if !strings.Contains(result, "status: 200") {
		t.Fatalf("missing status: %q", result)
	}
}

func TestWebFetchApprovalKind(t *testing.T) {
	tool := WebFetchTool{}
	if tool.ApprovalKind() != approvalKindNetwork {
		t.Fatalf("kind = %q", tool.ApprovalKind())
	}
	summary, ok := tool.ApprovalSummary(json.RawMessage(`{"url":"https://example.com/x"}`))
	if !ok || summary != "https://example.com/x" {
		t.Fatalf("summary = %q %v", summary, ok)
	}
}
