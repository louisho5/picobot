package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebToolRejectsUnsupportedScheme(t *testing.T) {
	tool := newWebToolWithOptions(true, 1024)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": "file:///etc/passwd",
	})
	if err == nil {
		t.Fatalf("expected error for unsupported scheme")
	}
	if !strings.Contains(err.Error(), "unsupported URL scheme") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebToolBlocksPrivateTargetsByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tool := newWebToolWithOptions(false, 1024)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": srv.URL,
	})
	if err == nil {
		t.Fatalf("expected private target to be blocked")
	}
	if !strings.Contains(err.Error(), "private/loopback") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebToolCanAllowPrivateTargetsForLocalDev(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tool := newWebToolWithOptions(true, 1024)

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": srv.URL,
	})
	if err != nil {
		t.Fatalf("expected fetch to succeed, got error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected body 'ok', got %q", out)
	}
}

func TestWebToolLimitsResponseBodySize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.Repeat("x", 128)))
	}))
	defer srv.Close()

	tool := newWebToolWithOptions(true, 32)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"url": srv.URL,
	})
	if err == nil {
		t.Fatalf("expected body size limit error")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("unexpected error: %v", err)
	}
}
