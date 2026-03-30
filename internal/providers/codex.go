package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/local/picobot/internal/auth"
	"github.com/local/picobot/internal/config"
)

const (
	codexAPIURL       = "https://chatgpt.com/backend-api/codex/responses"
	codexDefaultModel = "openai-codex/gpt-5.4-mini"
)

// OpenAICodexProvider calls the OpenAI Codex Responses API using OAuth tokens.
// Unlike the standard OpenAI provider, it uses the Responses API SSE format
// and authenticates via OAuth rather than an API key.
type OpenAICodexProvider struct {
	tok     *config.CodexProviderConfig // current OAuth token; may be refreshed in Chat()
	cfgPath string                      // path to config.json; used to persist refreshed tokens
	Client  *http.Client
}

// NewOpenAICodexProvider creates a Codex provider from existing OAuth tokens.
// cfgPath is the path to config.json so that refreshed tokens can be written back.
func NewOpenAICodexProvider(tok *config.CodexProviderConfig, timeoutSecs int, cfgPath string) *OpenAICodexProvider {
	if timeoutSecs <= 0 {
		timeoutSecs = 120
	}
	return &OpenAICodexProvider{
		tok:     tok,
		cfgPath: cfgPath,
		Client:  &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second},
	}
}

func (p *OpenAICodexProvider) GetDefaultModel() string { return codexDefaultModel }

// Chat sends messages to the Codex Responses API and returns the response.
func (p *OpenAICodexProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string) (LLMResponse, error) {
	if model == "" {
		model = codexDefaultModel
	}
	// Strip the "openai-codex/" prefix for the API call
	apiModel := strings.TrimPrefix(model, "openai-codex/")

	// Refresh token if expiring soon
	if auth.NeedsRefresh(p.tok) {
		if p.tok == nil || p.tok.RefreshToken == "" {
			return LLMResponse{}, fmt.Errorf("codex token is missing or expired; run 'picobot provider login openai-codex' to authenticate")
		}
		newTok, err := auth.RefreshCodexToken(p.tok.RefreshToken)
		if err != nil {
			return LLMResponse{}, fmt.Errorf("refreshing Codex token: %w", err)
		}
		p.tok = newTok
		// Best-effort write back to config — log on error but don't abort the request.
		// Use LoadConfigFromFile (not LoadConfig) to avoid accidentally persisting
		// runtime env-var overrides (PICOBOT_MODEL etc.) into the config file.
		if p.cfgPath != "" {
			if cfg, err := config.LoadConfigFromFile(p.cfgPath); err == nil {
				cfg.Providers.Codex = p.tok
				if err := config.SaveConfig(cfg, p.cfgPath); err != nil {
					log.Printf("warning: could not persist refreshed Codex token to %s: %v", p.cfgPath, err)
				}
			}
		}
	}

	// Build the request body
	instructions, inputItems := convertMessagesToCodex(messages)
	body := map[string]interface{}{
		"model":               apiModel,
		"store":               false,
		"stream":              true,
		"instructions":        instructions,
		"input":               inputItems,
		"text":                map[string]interface{}{"verbosity": "medium"},
		"include":             []string{"reasoning.encrypted_content"},
		"tool_choice":         "auto",
		"parallel_tool_calls": true,
	}
	if len(tools) > 0 {
		body["tools"] = convertToolsToCodex(tools)
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return LLMResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", codexAPIURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return LLMResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+p.tok.AccessToken)
	req.Header.Set("chatgpt-account-id", p.tok.AccountID)
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("originator", "picobot")
	req.Header.Set("User-Agent", "picobot (go)")

	resp, err := p.Client.Do(req)
	if err != nil {
		return LLMResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		body := strings.TrimSpace(string(bodyBytes))
		if resp.StatusCode == 429 {
			return LLMResponse{}, fmt.Errorf("codex rate limit exceeded — please try again later")
		}
		return LLMResponse{}, fmt.Errorf("codex API error: %s - %s", resp.Status, body)
	}

	return consumeCodexSSE(resp.Body)
}

// ---------------------------------------------------------------------------
// Message conversion
// ---------------------------------------------------------------------------

// convertMessagesToCodex converts picobot messages to the Codex Responses API format.
// Returns the system instructions string and the input items array.
func convertMessagesToCodex(messages []Message) (instructions string, inputItems []interface{}) {
	for i, m := range messages {
		switch m.Role {
		case "system":
			instructions = m.Content // last system message wins

		case "user":
			inputItems = append(inputItems, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "input_text", "text": m.Content},
				},
			})

		case "assistant":
			if len(m.ToolCalls) > 0 {
				// Emit one function_call item per tool call
				for j, tc := range m.ToolCalls {
					argsBytes, _ := json.Marshal(tc.Arguments)
					inputItems = append(inputItems, map[string]interface{}{
						"type":      "function_call",
						"id":        fmt.Sprintf("fc_%d_%d", i, j),
						"call_id":   tc.ID,
						"name":      tc.Name,
						"arguments": string(argsBytes),
					})
				}
			}
			if m.Content != "" {
				inputItems = append(inputItems, map[string]interface{}{
					"type": "message",
					"role": "assistant",
					"content": []map[string]interface{}{
						{"type": "output_text", "text": m.Content},
					},
					"status": "completed",
					"id":     fmt.Sprintf("msg_%d", i),
				})
			}

		case "tool":
			inputItems = append(inputItems, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": m.ToolCallID,
				"output":  m.Content,
			})
		}
	}
	return instructions, inputItems
}

// convertToolsToCodex converts picobot tool definitions to Codex flat format.
func convertToolsToCodex(tools []ToolDefinition) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if params == nil {
			params = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
		}
		out = append(out, map[string]interface{}{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  params,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// SSE streaming
// ---------------------------------------------------------------------------

// toolCallBuffer accumulates streamed function call arguments.
type toolCallBuffer struct {
	itemID   string
	callID   string
	name     string
	argsJSON string
}

// consumeCodexSSE reads the SSE stream from the Codex Responses API and
// returns the assembled LLMResponse.
func consumeCodexSSE(body io.Reader) (LLMResponse, error) {
	var content strings.Builder
	var toolCalls []ToolCall
	buffers := map[string]*toolCallBuffer{} // keyed by call_id

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB buffer for large SSE payloads

	var dataBuf strings.Builder

	flush := func() error {
		data := strings.TrimSpace(dataBuf.String())
		dataBuf.Reset()
		if data == "" || data == "[DONE]" {
			return nil
		}
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil // skip malformed events
		}
		return handleCodexEvent(event, &content, buffers, &toolCalls)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// blank line → end of event
			if err := flush(); err != nil {
				return LLMResponse{}, err
			}
			continue
		}
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(strings.TrimSpace(after))
		}
		// ignore "event:", "id:", "retry:" lines
	}
	if err := scanner.Err(); err != nil {
		return LLMResponse{}, fmt.Errorf("reading Codex SSE stream: %w", err)
	}
	// Flush any remaining data
	if err := flush(); err != nil {
		return LLMResponse{}, err
	}

	hasToolCalls := len(toolCalls) > 0
	return LLMResponse{
		Content:      strings.TrimSpace(content.String()),
		HasToolCalls: hasToolCalls,
		ToolCalls:    toolCalls,
	}, nil
}

func handleCodexEvent(event map[string]interface{}, content *strings.Builder, buffers map[string]*toolCallBuffer, toolCalls *[]ToolCall) error {
	eventType, _ := event["type"].(string)

	switch eventType {
	case "response.output_text.delta":
		delta, _ := event["delta"].(string)
		content.WriteString(delta)

	case "response.output_item.added":
		item, _ := event["item"].(map[string]interface{})
		if item == nil {
			return nil
		}
		if item["type"] == "function_call" {
			callID, _ := item["call_id"].(string)
			if callID == "" {
				return nil
			}
			buffers[callID] = &toolCallBuffer{
				itemID: stringVal(item, "id"),
				callID: callID,
				name:   stringVal(item, "name"),
			}
		}

	case "response.function_call_arguments.delta":
		callID, _ := event["call_id"].(string)
		delta, _ := event["delta"].(string)
		if buf, ok := buffers[callID]; ok {
			buf.argsJSON += delta
		}

	case "response.function_call_arguments.done":
		callID, _ := event["call_id"].(string)
		finalArgs, _ := event["arguments"].(string)
		if buf, ok := buffers[callID]; ok {
			buf.argsJSON = finalArgs
		}

	case "response.output_item.done":
		item, _ := event["item"].(map[string]interface{})
		if item == nil {
			return nil
		}
		if item["type"] == "function_call" {
			callID, _ := item["call_id"].(string)
			buf, ok := buffers[callID]
			if !ok {
				return nil
			}
			argsRaw := buf.argsJSON
			if argsRaw == "" {
				if args, _ := item["arguments"].(string); args != "" {
					argsRaw = args
				}
			}
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(argsRaw), &parsed); err != nil {
				parsed = map[string]interface{}{"raw": argsRaw}
			}
			*toolCalls = append(*toolCalls, ToolCall{
				ID:        callID,
				Name:      buf.name,
				Arguments: parsed,
			})
		}

	case "error", "response.failed":
		msg, _ := event["message"].(string)
		if msg == "" {
			msg = "Codex response failed"
		}
		return fmt.Errorf("codex error: %s", msg)
	}

	return nil
}

func stringVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}
