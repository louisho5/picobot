package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockDDGServer returns an httptest server that serves the given JSON body.
func mockDDGServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
}

func TestWebSearch_AbstractAndAnswer(t *testing.T) {
	srv := mockDDGServer(t, `{
		"Heading": "Go (programming language)",
		"AbstractText": "Go is an open-source programming language.",
		"AbstractURL": "https://en.wikipedia.org/wiki/Go_(programming_language)",
		"Answer": "",
		"Definition": "",
		"DefinitionURL": "",
		"RelatedTopics": [],
		"Results": []
	}`)
	defer srv.Close()

	tool := &WebSearchTool{client: srv.Client(), baseURL: srv.URL}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "golang"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Go (programming language)") {
		t.Fatalf("expected heading in output, got: %s", out)
	}
	if !strings.Contains(out, "Go is an open-source programming language.") {
		t.Fatalf("expected abstract in output, got: %s", out)
	}
	if !strings.Contains(out, "https://en.wikipedia.org/wiki/Go_") {
		t.Fatalf("expected source URL in output, got: %s", out)
	}
}

func TestWebSearch_DirectAnswer(t *testing.T) {
	srv := mockDDGServer(t, `{
		"Heading": "",
		"AbstractText": "",
		"AbstractURL": "",
		"Answer": "42",
		"Definition": "",
		"DefinitionURL": "",
		"RelatedTopics": [],
		"Results": []
	}`)
	defer srv.Close()

	tool := &WebSearchTool{client: srv.Client(), baseURL: srv.URL}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "6 times 7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Answer: 42") {
		t.Fatalf("expected direct answer in output, got: %s", out)
	}
}

func TestWebSearch_RelatedTopics(t *testing.T) {
	srv := mockDDGServer(t, `{
		"Heading": "",
		"AbstractText": "",
		"AbstractURL": "",
		"Answer": "",
		"Definition": "",
		"DefinitionURL": "",
		"RelatedTopics": [
			{"Text": "Topic one", "FirstURL": "https://duckduckgo.com/topic1"},
			{"Text": "Topic two", "FirstURL": "https://duckduckgo.com/topic2"}
		],
		"Results": []
	}`)
	defer srv.Close()

	tool := &WebSearchTool{client: srv.Client(), baseURL: srv.URL}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "something"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Topic one") || !strings.Contains(out, "Topic two") {
		t.Fatalf("expected related topics in output, got: %s", out)
	}
}

func TestWebSearch_EmptyResponse(t *testing.T) {
	srv := mockDDGServer(t, `{
		"Heading": "",
		"AbstractText": "",
		"AbstractURL": "",
		"Answer": "",
		"Definition": "",
		"DefinitionURL": "",
		"RelatedTopics": [],
		"Results": []
	}`)
	defer srv.Close()

	tool := &WebSearchTool{client: srv.Client(), baseURL: srv.URL}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "xyzzy12345"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No instant answer found") {
		t.Fatalf("expected fallback message in output, got: %s", out)
	}
}

func TestWebSearch_MissingQuery(t *testing.T) {
	tool := NewWebSearchTool()
	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatalf("expected error for missing query")
	}
}

func TestWebSearch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	tool := &WebSearchTool{client: srv.Client(), baseURL: srv.URL}
	_, err := tool.Execute(context.Background(), map[string]interface{}{"query": "test"})
	if err == nil {
		t.Fatalf("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected 429 in error, got: %v", err)
	}
}

func TestWebSearch_GroupedTopics(t *testing.T) {
	srv := mockDDGServer(t, `{
		"Heading": "",
		"AbstractText": "",
		"AbstractURL": "",
		"Answer": "",
		"Definition": "",
		"DefinitionURL": "",
		"RelatedTopics": [
			{
				"Name": "See also",
				"Topics": [
					{"Text": "Nested topic", "FirstURL": "https://duckduckgo.com/nested"}
				]
			}
		],
		"Results": []
	}`)
	defer srv.Close()

	tool := &WebSearchTool{client: srv.Client(), baseURL: srv.URL}
	out, err := tool.Execute(context.Background(), map[string]interface{}{"query": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Nested topic") {
		t.Fatalf("expected nested grouped topic in output, got: %s", out)
	}
}
