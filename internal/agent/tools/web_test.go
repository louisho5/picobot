package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
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
	if !strings.Contains(err.Error(), "private/special-use") {
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

func TestWebToolRejectsSpecialUseDirectIPRanges(t *testing.T) {
	tool := newWebToolWithOptions(false, 1024)

	cases := []string{
		"http://100.64.0.1",
		"http://198.18.0.1",
		"http://192.0.2.10",
		"http://203.0.113.10",
		"http://[2001:db8::1]",
	}

	for _, rawURL := range cases {
		_, err := tool.Execute(context.Background(), map[string]interface{}{"url": rawURL})
		if err == nil {
			t.Fatalf("expected %s to be blocked", rawURL)
		}
		if !strings.Contains(err.Error(), "private/special-use") {
			t.Fatalf("unexpected error for %s: %v", rawURL, err)
		}
	}
}

func TestIsPrivateOrLocalIP_SpecialUseCoverage(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{ip: "100.64.0.1", blocked: true},
		{ip: "198.18.0.1", blocked: true},
		{ip: "192.0.2.1", blocked: true},
		{ip: "2001:db8::1", blocked: true},
		{ip: "1.1.1.1", blocked: false},
		{ip: "2606:4700:4700::1111", blocked: false},
	}

	for _, tc := range tests {
		ip := netip.MustParseAddr(tc.ip)
		got := isPrivateOrLocalIP(ip)
		if got != tc.blocked {
			t.Fatalf("ip=%s blocked=%v got=%v", tc.ip, tc.blocked, got)
		}
	}
}
