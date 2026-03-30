package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/local/picobot/internal/config"
)

// buildJWT creates a minimal, unsigned JWT with the given claims payload.
func buildJWT(payload map[string]interface{}) string {
	b, _ := json.Marshal(payload)
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256"}`))
	mid := base64.RawURLEncoding.EncodeToString(b)
	return header + "." + mid + ".fakesig"
}

// TestExtractAccountIDFromNamespace verifies the primary claim path:
// https://api.openai.com/auth → chatgpt_account_id in the id_token.
func TestExtractAccountIDFromNamespace(t *testing.T) {
	idToken := buildJWT(map[string]interface{}{
		"sub": "user-sub-123",
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": "acct-namespace",
		},
	})

	got := ExtractAccountID(idToken, "")
	if got != "acct-namespace" {
		t.Errorf("expected %q, got %q", "acct-namespace", got)
	}
}

// TestExtractAccountIDTopLevel verifies the top-level chatgpt_account_id fallback.
func TestExtractAccountIDTopLevel(t *testing.T) {
	idToken := buildJWT(map[string]interface{}{
		"chatgpt_account_id": "acct-toplevel",
		"sub":                "user-sub",
	})

	got := ExtractAccountID(idToken, "")
	if got != "acct-toplevel" {
		t.Errorf("expected %q, got %q", "acct-toplevel", got)
	}
}

// TestExtractAccountIDFromAccessTokenFallback verifies that when the id_token
// is empty the access_token is tried next.
func TestExtractAccountIDFromAccessTokenFallback(t *testing.T) {
	accessToken := buildJWT(map[string]interface{}{
		"https://api.openai.com/auth": map[string]interface{}{
			"chatgpt_account_id": "acct-from-access",
		},
	})

	got := ExtractAccountID("", accessToken)
	if got != "acct-from-access" {
		t.Errorf("expected %q, got %q", "acct-from-access", got)
	}
}

// TestExtractAccountIDSubFallback verifies sub is used when no account ID claim exists.
func TestExtractAccountIDSubFallback(t *testing.T) {
	idToken := buildJWT(map[string]interface{}{
		"sub": "fallback-sub",
	})

	got := ExtractAccountID(idToken, "")
	if got != "fallback-sub" {
		t.Errorf("expected sub fallback %q, got %q", "fallback-sub", got)
	}
}

// TestExtractAccountIDEmptyOnInvalid verifies that malformed inputs return "".
func TestExtractAccountIDEmptyOnInvalid(t *testing.T) {
	cases := []struct{ id, access string }{
		{"", ""},
		{"notajwt", ""},
		{"only.two", ""},
	}
	for _, tc := range cases {
		got := ExtractAccountID(tc.id, tc.access)
		if got != "" {
			t.Errorf("id=%q access=%q: expected empty string, got %q", tc.id, tc.access, got)
		}
	}
}

// TestNeedsRefreshExpired verifies that a token expiring in the past needs refresh.
func TestNeedsRefreshExpired(t *testing.T) {
	tok := &config.CodexProviderConfig{
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(-1 * time.Minute),
	}
	if !NeedsRefresh(tok) {
		t.Error("expected NeedsRefresh=true for expired token")
	}
}

// TestNeedsRefreshSoon verifies that a token expiring within 60 seconds needs refresh.
func TestNeedsRefreshSoon(t *testing.T) {
	tok := &config.CodexProviderConfig{
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(30 * time.Second),
	}
	if !NeedsRefresh(tok) {
		t.Error("expected NeedsRefresh=true for token expiring in 30s")
	}
}

// TestNeedsRefreshValid verifies that a token with plenty of lifetime is not refreshed.
func TestNeedsRefreshValid(t *testing.T) {
	tok := &config.CodexProviderConfig{
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}
	if NeedsRefresh(tok) {
		t.Error("expected NeedsRefresh=false for token with 10 minutes remaining")
	}
}

// TestNeedsRefreshNilToken verifies that a nil token is treated as needing refresh.
func TestNeedsRefreshNilToken(t *testing.T) {
	if !NeedsRefresh(nil) {
		t.Error("expected NeedsRefresh=true for nil token")
	}
}
