// Package openai implements an llm.Client adapter for OpenAI-compatible
// Chat Completions endpoints (including OpenAI, DeepSeek, GLM, Qwen and any
// other server that speaks the /v1/chat/completions dialect).
package openai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Godric-W/Equus/internal/llm"
)

// Options configures the Chat Completions client.
type Options struct {
	ProviderName string
	APIKey       string
	BaseURL      string
	Model        string
	Timeout      time.Duration
}

// Client talks to an OpenAI-compatible Chat Completions endpoint.
type Client struct {
	options Options
	http    *http.Client
}

// NewClient builds a Chat Completions client. It returns an error when
// required transport fields are missing.
func NewClient(options Options) (*Client, error) {
	if strings.TrimSpace(options.APIKey) == "" {
		return nil, errors.New("openai client api key is empty")
	}
	if strings.TrimSpace(options.BaseURL) == "" {
		return nil, errors.New("openai client base url is empty")
	}
	if strings.TrimSpace(options.Model) == "" {
		return nil, errors.New("openai client model is empty")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return &Client{
		options: options,
		http:    &http.Client{Timeout: timeout},
	}, nil
}

func (client *Client) Model() llm.ModelInfo {
	return llm.ModelInfo{
		Provider:      client.options.ProviderName,
		Name:          client.options.Model,
		ContextWindow: 128_000,
	}
}

func (client *Client) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsStreaming:     true,
		SupportsDeveloperRole: true,
		SupportsToolCalls:     true,
	}
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
		return llm.Response{}, fmt.Errorf("openai complete request: %w", err)
	}
	defer httpResponse.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes))
	if err != nil {
		return llm.Response{}, fmt.Errorf("openai complete read body: %w", err)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return llm.Response{}, providerError(httpResponse.StatusCode, payload)
	}
	return decodeCompletionResponse(payload)
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
		return nil, fmt.Errorf("openai stream request: %w", err)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		defer httpResponse.Body.Close()
		payload, _ := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes))
		return nil, providerError(httpResponse.StatusCode, payload)
	}
	return newEventStream(httpResponse.Body), nil
}

func (client *Client) newHTTPRequest(ctx context.Context, body []byte) (*http.Request, error) {
	endpoint := strings.TrimRight(client.options.BaseURL, "/") + "/chat/completions"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+client.options.APIKey)
	return httpRequest, nil
}
