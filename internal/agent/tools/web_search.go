package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebSearchTool searches the web using DuckDuckGo's free Instant Answer API.
// No API key is required.
// Args: {"query": "search terms"}
type WebSearchTool struct {
	client  *http.Client
	baseURL string // overridable in tests
}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{
		client:  &http.Client{Timeout: 10 * time.Second},
		baseURL: "https://api.duckduckgo.com",
	}
}

func (t *WebSearchTool) Name() string { return "web_search" }
func (t *WebSearchTool) Description() string {
	return "Search the web using DuckDuckGo and return relevant results"
}

func (t *WebSearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query",
			},
		},
		"required": []string{"query"},
	}
}

// ddgResponse is the top-level DDG Instant Answer API JSON structure.
type ddgResponse struct {
	Heading       string     `json:"Heading"`
	AbstractText  string     `json:"AbstractText"`
	AbstractURL   string     `json:"AbstractURL"`
	Answer        string     `json:"Answer"`
	Definition    string     `json:"Definition"`
	DefinitionURL string     `json:"DefinitionURL"`
	RelatedTopics []ddgTopic `json:"RelatedTopics"`
	Results       []ddgTopic `json:"Results"`
}

// ddgTopic represents either a direct result or a grouped section.
// When grouped (e.g. "See also"), Name is set and Topics contains the items.
type ddgTopic struct {
	Text     string     `json:"Text"`
	FirstURL string     `json:"FirstURL"`
	Name     string     `json:"Name"`
	Topics   []ddgTopic `json:"Topics"`
}

func (t *WebSearchTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	q, ok := args["query"].(string)
	if !ok || strings.TrimSpace(q) == "" {
		return "", fmt.Errorf("web_search: 'query' argument required")
	}

	apiURL := t.baseURL + "/?q=" + url.QueryEscape(q) +
		"&format=json&no_html=1&skip_disambig=1&t=picobot"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "picobot/1.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("web_search: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("web_search: DuckDuckGo returned HTTP %d", resp.StatusCode)
	}

	var ddg ddgResponse
	if err := json.NewDecoder(resp.Body).Decode(&ddg); err != nil {
		return "", fmt.Errorf("web_search: failed to decode response: %w", err)
	}

	return formatDDGResponse(q, &ddg), nil
}

// formatDDGResponse builds a clean, LLM-friendly text from the DDG API response.
func formatDDGResponse(query string, r *ddgResponse) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "DuckDuckGo search results for %q:\n", query)

	hasContent := false

	if r.Answer != "" {
		fmt.Fprintf(&sb, "\nAnswer: %s\n", r.Answer)
		hasContent = true
	}

	if r.AbstractText != "" {
		if r.Heading != "" {
			fmt.Fprintf(&sb, "\n%s\n", r.Heading)
		}
		fmt.Fprintf(&sb, "%s\n", r.AbstractText)
		if r.AbstractURL != "" {
			fmt.Fprintf(&sb, "Source: %s\n", r.AbstractURL)
		}
		hasContent = true
	}

	if r.Definition != "" {
		fmt.Fprintf(&sb, "\nDefinition: %s\n", r.Definition)
		if r.DefinitionURL != "" {
			fmt.Fprintf(&sb, "Source: %s\n", r.DefinitionURL)
		}
		hasContent = true
	}

	// Flatten related topics: accept direct items and one level of grouped sections.
	var topics []ddgTopic
	for _, rt := range r.RelatedTopics {
		if rt.Text != "" && rt.FirstURL != "" {
			topics = append(topics, rt)
		} else if len(rt.Topics) > 0 {
			for _, sub := range rt.Topics {
				if sub.Text != "" && sub.FirstURL != "" {
					topics = append(topics, sub)
				}
			}
		}
	}
	for _, res := range r.Results {
		if res.Text != "" && res.FirstURL != "" {
			topics = append(topics, res)
		}
	}

	const maxTopics = 5
	if len(topics) > maxTopics {
		topics = topics[:maxTopics]
	}
	if len(topics) > 0 {
		fmt.Fprintf(&sb, "\nRelated results:\n")
		for i, topic := range topics {
			fmt.Fprintf(&sb, "%d. %s\n   %s\n", i+1, topic.Text, topic.FirstURL)
		}
		hasContent = true
	}

	if !hasContent {
		fmt.Fprintf(&sb, "\nNo instant answer found. Try the 'web' tool to visit a specific URL.\n")
	}

	return sb.String()
}
