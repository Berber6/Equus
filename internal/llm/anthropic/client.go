// Package anthropic implements an llm.Client adapter for the Anthropic
// Messages API (/v1/messages), used by Claude and Messages-compatible
// endpoints.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Godric-W/Equus/internal/llm"
)

// anthropicVersion is the Messages API version header value.
const anthropicVersion = "2023-06-01"

const maxResponseBytes = int64(32 << 20)

// Options configures the Messages client.
type Options struct {
	ProviderName string
	APIKey       string
	BaseURL      string
	Model        string
	Timeout      time.Duration
	MaxTokens    int
}

// Client talks to an Anthropic Messages endpoint.
type Client struct {
	options Options
	http    *http.Client
}

// NewClient builds a Messages client.
func NewClient(options Options) (*Client, error) {
	if strings.TrimSpace(options.APIKey) == "" {
		return nil, errors.New("anthropic client api key is empty")
	}
	if strings.TrimSpace(options.BaseURL) == "" {
		return nil, errors.New("anthropic client base url is empty")
	}
	if strings.TrimSpace(options.Model) == "" {
		return nil, errors.New("anthropic client model is empty")
	}
	if options.Timeout <= 0 {
		options.Timeout = 2 * time.Minute
	}
	if options.MaxTokens <= 0 {
		options.MaxTokens = 8192
	}
	return &Client{options: options, http: &http.Client{Timeout: options.Timeout}}, nil
}

func (client *Client) Model() llm.ModelInfo {
	return llm.ModelInfo{Provider: client.options.ProviderName, Name: client.options.Model, ContextWindow: 200_000}
}

func (client *Client) Capabilities() llm.Capabilities {
	return llm.Capabilities{SupportsStreaming: true, SupportsToolCalls: true}
}

func (client *Client) Complete(ctx context.Context, request llm.Request) (llm.Response, error) {
	body, err := client.newRequestBody(request, false)
	if err != nil {
		return llm.Response{}, err
	}
	httpRequest, err := client.newHTTPRequest(ctx, body)
	if err != nil {
		return llm.Response{}, err
	}
	httpResponse, err := client.http.Do(httpRequest)
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic complete request: %w", err)
	}
	defer httpResponse.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes))
	if err != nil {
		return llm.Response{}, fmt.Errorf("anthropic complete read body: %w", err)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return llm.Response{}, providerError(httpResponse.StatusCode, payload)
	}
	return decodeMessageResponse(payload)
}

func (client *Client) Stream(ctx context.Context, request llm.Request) (llm.Stream, error) {
	body, err := client.newRequestBody(request, true)
	if err != nil {
		return nil, err
	}
	httpRequest, err := client.newHTTPRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	httpResponse, err := client.http.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("anthropic stream request: %w", err)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		defer httpResponse.Body.Close()
		payload, _ := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes))
		return nil, providerError(httpResponse.StatusCode, payload)
	}
	return newEventStream(httpResponse.Body), nil
}

func (client *Client) newHTTPRequest(ctx context.Context, body []byte) (*http.Request, error) {
	endpoint := strings.TrimRight(client.options.BaseURL, "/") + "/messages"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("x-api-key", client.options.APIKey)
	httpRequest.Header.Set("anthropic-version", anthropicVersion)
	return httpRequest, nil
}

// providerError builds a non-retryable error for a rejected request.
func providerError(status int, payload []byte) error {
	return &ProviderError{StatusCode: status, Body: string(payload)}
}

// ProviderError is a terminal error returned by the endpoint (non-2xx).
type ProviderError struct {
	StatusCode int
	Body       string
}

func (err *ProviderError) Error() string {
	return fmt.Sprintf("anthropic provider error %d: %s", err.StatusCode, truncate(err.Body, 512))
}

// Retryable reports that a definitive rejection must not be retried.
func (err *ProviderError) Retryable() bool { return false }

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "..."
}

// marshal is a small helper shared by request builders.
func marshal(value any) ([]byte, error) { return json.Marshal(value) }
