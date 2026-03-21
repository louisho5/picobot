package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/local/picobot/internal/providers"
)

// LLMMemoryRanker uses an LLM provider to rank memories relative to a query.
// It falls back to a SimpleRanker if the provider fails or returns an unparsable response.
type LLMMemoryRanker struct {
	provider providers.LLMProvider
	model    string
	fallback *SimpleRanker
	logger   *log.Logger // optional per-instance logger for diagnostics
}

// NewLLMRanker constructs an LLMMemoryRanker using the given provider and model.
func NewLLMRanker(provider providers.LLMProvider, model string) *LLMMemoryRanker {
	return NewLLMRankerWithLogger(provider, model, nil)
}

// NewLLMRankerWithLogger constructs an LLMMemoryRanker with an optional logger.
func NewLLMRankerWithLogger(provider providers.LLMProvider, model string, logger *log.Logger) *LLMMemoryRanker {
	if model == "" && provider != nil {
		model = provider.GetDefaultModel()
	}
	return &LLMMemoryRanker{provider: provider, model: model, fallback: NewSimpleRanker(), logger: logger}
}

// logf logs using the instance logger if present, else falls back to package log.
func (r *LLMMemoryRanker) logf(format string, args ...interface{}) {
	if r.logger != nil {
		r.logger.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

// Rank implements the Ranker interface. It uses a background context for provider calls
// (this is acceptable for short operations; timeouts are applied by the provider implementation).
func (r *LLMMemoryRanker) Rank(query string, memories []MemoryItem, top int) []MemoryItem {
	if len(memories) == 0 || top <= 0 {
		return nil
	}
	// If provider is not available, use fallback.
	if r.provider == nil {
		return r.fallback.Rank(query, memories, top)
	}

	// Build a simple prompt listing memories with indices and expose a 'rank_memories' tool.
	var sb strings.Builder
	sb.WriteString("You are a ranking assistant. Given the query and a list of memories numbered 0..N-1, return only an ordered list of indices (most relevant first). Respond either by calling the tool 'rank_memories' with argument {\"indices\": [i, j, ...]} or by returning a JSON array like [i,j,...] in the assistant content. Do not return other text around the array; if you must, ensure the array appears in full (e.g. 'Result: [1,0]')." + "\n\n")
	sb.WriteString("Query: " + query + "\n\n")
	sb.WriteString("Memories (index: text):\n")
	for i, m := range memories {
		fmt.Fprintf(&sb, "%d: %s\n", i, m.Text)
	}

	messages := []providers.Message{{Role: "system", Content: sb.String()}, {Role: "user", Content: "Return an ordered list of indices ranked by relevance, or call the 'rank_memories' tool."}}

	// expose a tool definition to allow function-call style responses from providers
	rankTool := providers.ToolDefinition{
		Name:        "rank_memories",
		Description: "Return ranking indices for memories",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{"indices"},
			"properties": map[string]interface{}{
				"indices": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "number"}},
			},
		},
	}
	// diagnostic log
	r.logf("LLMMemoryRanker: sending ranking request for query=%q with %d memories", query, len(memories))
	resp, err := r.provider.Chat(context.Background(), messages, []providers.ToolDefinition{rankTool}, r.model)
	if err != nil {
		r.logf("LLMMemoryRanker provider error: %v", err)
		return r.fallback.Rank(query, memories, top)
	}
	// log response summary
	if resp.HasToolCalls {
		r.logf("LLMMemoryRanker: provider returned %d tool calls", len(resp.ToolCalls))
	} else {
		r.logf("LLMMemoryRanker: provider returned content=%q", strings.TrimSpace(resp.Content))
	}

	// Prefer function call results (ToolCalls) if present
	if resp.HasToolCalls && len(resp.ToolCalls) > 0 {
		for _, tc := range resp.ToolCalls {
			if tc.Name != "rank_memories" {
				continue
			}
			// expected argument: indices: [int]
			if raw, ok := tc.Arguments["indices"]; ok {
				if idxs, err := parseIndicesFromArgs(raw); err == nil {
					out := make([]MemoryItem, 0, top)
					seen := make(map[int]struct{})
					for _, idx := range idxs {
						if idx < 0 || idx >= len(memories) {
							continue
						}
						if _, ok := seen[idx]; ok {
							continue
						}
						out = append(out, memories[idx])
						seen[idx] = struct{}{}
						if len(out) >= top {
							break
						}
					}
					// pad if needed
					if len(out) < top {
						fallback := r.fallback.Rank(query, memories, len(memories))
						for _, m := range fallback {
							if len(out) >= top {
								break
							}
							skip := false
							for _, s := range out {
								if s.Text == m.Text && s.Kind == m.Kind {
									skip = true
									break
								}
							}
							if !skip {
								out = append(out, m)
							}
						}
					}
					return out
				}
			}
		}
	}

	// Attempt to parse JSON array of ints from resp.Content as a fallback
	var idxs []int
	body := strings.TrimSpace(resp.Content)
	if err := json.Unmarshal([]byte(body), &idxs); err != nil {
		// try to be forgiving: extract digits from content
		if err2 := parseIndicesFromText(body, &idxs); err2 != nil {
			r.logf("LLMMemoryRanker parse error: %v (content=%q)", err2, body)
			return r.fallback.Rank(query, memories, top)
		}
	}

	out := make([]MemoryItem, 0, top)
	seen := make(map[int]struct{})
	for _, idx := range idxs {
		if idx < 0 || idx >= len(memories) {
			continue
		}
		if _, ok := seen[idx]; ok {
			continue
		}
		out = append(out, memories[idx])
		seen[idx] = struct{}{}
		if len(out) >= top {
			break
		}
	}
	// If not enough returned, pad with fallback ordering excluding already seen
	if len(out) < top {
		fallback := r.fallback.Rank(query, memories, len(memories))
		for _, m := range fallback {
			if len(out) >= top {
				break
			}
			// check if already included
			skip := false
			for _, s := range out {
				if s.Text == m.Text && s.Kind == m.Kind {
					skip = true
					break
				}
			}
			if !skip {
				out = append(out, m)
			}
		}
	}
	return out
}

// parseIndicesFromText attempts to extract a JSON-like array of ints from arbitrary text.
func parseIndicesFromText(s string, out *[]int) error {
	// find the first [ ... ] substring
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start == -1 || end == -1 || start >= end {
		return ErrNoIndicesFound
	}
	sub := s[start : end+1]
	return json.Unmarshal([]byte(sub), out)
}

// parseIndicesFromArgs parses an arguments value that may be []int, []float64 or []interface{} and returns []int.
func parseIndicesFromArgs(v interface{}) ([]int, error) {
	switch t := v.(type) {
	case []int:
		return t, nil
	case []float64:
		out := make([]int, len(t))
		for i, f := range t {
			out[i] = int(f)
		}
		return out, nil
	case []interface{}:
		out := make([]int, 0, len(t))
		for _, it := range t {
			switch x := it.(type) {
			case float64:
				out = append(out, int(x))
			case int:
				out = append(out, x)
			case int64:
				out = append(out, int(x))
			default:
				// ignore non-numeric
			}
		}
		if len(out) == 0 {
			return nil, ErrNoIndicesFound
		}
		return out, nil
	default:
		return nil, ErrNoIndicesFound
	}
}
