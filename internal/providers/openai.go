package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// OpenAIProvider calls an OpenAI-compatible API (OpenAI, OpenRouter, or similar).
type OpenAIProvider struct {
	APIKey       string
	APIBase      string // e.g. https://api.openai.com/v1 or https://openrouter.ai/api/v1
	MaxTokens    int    // 0 means "let the API decide"
	Client       *http.Client
	RetryBackoff time.Duration
	MaxRetries   int
}

func NewOpenAIProvider(apiKey, apiBase string, timeoutSecs, maxTokens int) *OpenAIProvider {
	if apiBase == "" {
		apiBase = "https://api.openai.com/v1" // sensible default; can be overridden
	}
	if timeoutSecs <= 1 {
		timeoutSecs = 60 // default 60 seconds
	}
	return &OpenAIProvider{
		APIKey:    apiKey,
		APIBase:   strings.TrimRight(apiBase, "/"),
		MaxTokens: maxTokens,
		Client: &http.Client{
			Timeout: time.Duration(timeoutSecs) * time.Second,
		},
		RetryBackoff: 2 * time.Second,
		MaxRetries:   3,
	}
}

func (p *OpenAIProvider) GetDefaultModel() string { return "gpt-4o-mini" }

// Request/response shapes using the modern OpenAI "tools" format.
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []messageJSON `json:"messages"`
	Tools     []toolWrapper `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

// toolWrapper is the OpenAI tools array element: {"type": "function", "function": {...}}
type toolWrapper struct {
	Type     string      `json:"type"`
	Function functionDef `json:"function"`
}

type functionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type messageJSON struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCallJSON `json:"tool_calls,omitempty"`
}

type toolCallJSON struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function toolCallFunctionJSON `json:"function"`
}

type toolCallFunctionJSON struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type messageResponseJSON struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []toolCallJSON `json:"tool_calls,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message messageResponseJSON `json:"message"`
	} `json:"choices"`
}

// Chat calls an OpenAI-compatible chat completion endpoint and returns a simplified response.
func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (LLMResponse, error) {
	if model == "" {
		model = p.GetDefaultModel()
	}

	reqBody := chatRequest{Model: model, Messages: make([]messageJSON, 0, len(messages)), MaxTokens: p.MaxTokens}
	for _, m := range messages {
		mj := messageJSON{Role: m.Role, ToolCallID: m.ToolCallID}
		if len(m.ToolCalls) > 0 && m.Content == "" {
			mj.Content = nil
		} else {
			c := m.Content
			mj.Content = &c
		}
		// Convert provider ToolCall to JSON-serializable toolCallJSON
		for _, tc := range m.ToolCalls {
			argsBytes, _ := json.Marshal(tc.Arguments)
			mj.ToolCalls = append(mj.ToolCalls, toolCallJSON{
				ID:   tc.ID,
				Type: "function",
				Function: toolCallFunctionJSON{
					Name:      tc.Name,
					Arguments: string(argsBytes),
				},
			})
		}
		reqBody.Messages = append(reqBody.Messages, mj)
	}

	// Include tools in modern format if provided
	if len(tools) > 0 {
		reqBody.Tools = make([]toolWrapper, 0, len(tools))
		for _, t := range tools {
			params := t.Parameters
			if params == nil {
				params = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
			}
			reqBody.Tools = append(reqBody.Tools, toolWrapper{
				Type: "function",
				Function: functionDef{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  params,
				},
			})
		}
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return LLMResponse{}, err
	}

	url := fmt.Sprintf("%s/chat/completions", p.APIBase)

	var resp *http.Response
	var lastErr error
	maxRetries := p.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	backoff := p.RetryBackoff
	if backoff <= 0 {
		backoff = 2 * time.Second
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return LLMResponse{}, err
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(b)))
		if err != nil {
			return LLMResponse{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		if p.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.APIKey)
		}

		resp, err = p.Client.Do(req)

		shouldRetry := false
		if err != nil {
			if ctx.Err() != nil {
				return LLMResponse{}, err
			}
			shouldRetry = true
			lastErr = err
			log.Printf("OpenAI API attempt %d failed: %v. Retrying in %v...", attempt, err, backoff)
		} else {
			if resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode <= 504) {
				shouldRetry = true
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				bodyStr := strings.TrimSpace(string(bodyBytes))
				lastErr = fmt.Errorf("OpenAI API error: %s - %s", resp.Status, bodyStr)
				log.Printf("OpenAI API attempt %d failed with status %d: %q. Retrying in %v...", attempt, resp.StatusCode, bodyStr, backoff)
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				// Non-retryable status code (e.g. 400, 401, 403, 404)
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				bodyStr := strings.TrimSpace(string(bodyBytes))
				log.Printf("OpenAI API non-2xx (non-retryable): %s body=%q", resp.Status, bodyStr)
				if bodyStr == "" {
					return LLMResponse{}, fmt.Errorf("OpenAI API error: %s", resp.Status)
				}
				return LLMResponse{}, fmt.Errorf("OpenAI API error: %s - %s", resp.Status, bodyStr)
			}
		}

		if shouldRetry {
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return LLMResponse{}, ctx.Err()
				case <-time.After(backoff):
					backoff *= 2
					continue
				}
			} else {
				// exhausted retries
				if lastErr != nil {
					return LLMResponse{}, fmt.Errorf("OpenAI API exhausted %d retries. Last error: %w", maxRetries, lastErr)
				}
				return LLMResponse{}, fmt.Errorf("OpenAI API exhausted %d retries", maxRetries)
			}
		}

		// If success (2xx), break retry loop
		break
	}
	defer resp.Body.Close()

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return LLMResponse{}, err
	}

	if len(out.Choices) == 0 {
		return LLMResponse{}, errors.New("OpenAI API returned no choices")
	}

	msg := out.Choices[0].Message
	// If the model requested tool calls, parse them
	if len(msg.ToolCalls) > 0 {
		var tcs []ToolCall
		for _, tc := range msg.ToolCalls {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsed); err != nil {
				// skip unparseable tool calls
				continue
			}
			tcs = append(tcs, ToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: parsed})
		}
		if len(tcs) > 0 {
			return LLMResponse{Content: strings.TrimSpace(msg.Content), HasToolCalls: true, ToolCalls: tcs}, nil
		}
	}

	// No tool calls
	return LLMResponse{Content: strings.TrimSpace(msg.Content), HasToolCalls: false}, nil
}
