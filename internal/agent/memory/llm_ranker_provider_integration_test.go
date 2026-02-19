package memory

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/local/picobot/internal/providers"
)

func TestLLMRankerWithOpenAIFunctionCall(t *testing.T) {
	// server returns tool_calls style response for rank_memories
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{
		  "choices": [
		    {
		      "message": {
		        "role": "assistant",
		        "content": "",
		        "tool_calls": [
		          {
		            "id": "call_rank_1",
		            "type": "function",
		            "function": {
		              "name": "rank_memories",
		              "arguments": "{\"indices\": [1, 0]}"
		            }
		          }
		        ]
		      }
		    }
		  ]
		}`))
	}))
	defer h.Close()

	p := providers.NewOpenAIProvider("test-key", h.URL, 60)
	p.Client = &http.Client{Timeout: 5 * time.Second}

	mems := []MemoryItem{{Kind: "short", Text: "buy milk"}, {Kind: "short", Text: "call mom"}}
	r := NewLLMRanker(p, "model-x")
	res := r.Rank("milk", mems, 2)
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].Text != "call mom" {
		t.Fatalf("expected first result to be 'call mom', got %q", res[0].Text)
	}
}
