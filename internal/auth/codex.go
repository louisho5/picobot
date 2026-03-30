// Package auth implements the OpenAI Codex OAuth PKCE flow for picobot.
// Tokens are stored in config.json alongside other provider credentials.
package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/local/picobot/internal/config"
)

const (
	codexClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	codexTokenURL     = "https://auth.openai.com/oauth/token"
	// Use localhost (not 127.0.0.1) to match the registered redirect URI in the Codex CLI.
	codexRedirectURI  = "http://localhost:1455/auth/callback"
	codexScopes       = "openid profile email offline_access"
	codexCallbackPort = "1455"
)

// LoginCodexInteractive runs the full PKCE OAuth flow interactively.
// It opens a browser (or prints a URL for headless environments), listens
// for the callback on port 1455, and exchanges the auth code for tokens.
// The caller is responsible for saving the returned config to disk.
//
// When manual is true the local callback server is skipped entirely and the
// user is asked to paste the redirect URL back into the terminal. Use this in
// Docker / headless environments where port 1455 cannot be reached from the
// host browser.
func LoginCodexInteractive(reader *bufio.Reader, manual bool) (*config.CodexProviderConfig, error) {
	// Generate PKCE verifier and challenge
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("generating code verifier: %w", err)
	}
	challenge := generateCodeChallenge(verifier)

	// Generate random state
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("generating state: %w", err)
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	// Build authorization URL
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", codexClientID)
	params.Set("redirect_uri", codexRedirectURI)
	params.Set("scope", codexScopes)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	// OpenAI-specific parameters — originator must be "opencode" (the Codex CLI value).
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")
	params.Set("originator", "opencode")

	authURL := codexAuthorizeURL + "?" + params.Encode()

	// Start a local callback server unless the caller requested manual mode.
	// In Docker / headless environments port 1455 is unreachable from the host
	// browser even if it binds successfully inside the container, so --manual
	// skips the server entirely and asks the user to paste the redirect URL.
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	useLocalServer := false
	if !manual {
		ln, listenErr := net.Listen("tcp", "localhost:"+codexCallbackPort)
		if listenErr == nil {
			useLocalServer = true
			mux := http.NewServeMux()
			srv := &http.Server{Handler: mux}
			mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("state") != state {
					http.Error(w, "invalid state", http.StatusBadRequest)
					errCh <- fmt.Errorf("state mismatch: OAuth flow may have been tampered with")
					return
				}
				code := q.Get("code")
				if code == "" {
					http.Error(w, "missing code", http.StatusBadRequest)
					errCh <- fmt.Errorf("no code in callback")
					return
				}
				fmt.Fprintln(w, "<html><body><h2>Authentication successful! You can close this tab.</h2></body></html>")
				codeCh <- code
				go srv.Shutdown(context.Background()) //nolint:errcheck
			})
			go func() {
				if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()
		}
	}

	// Always print the URL so users can open it manually if needed.
	fmt.Println("Open the URL below in your browser:")
	fmt.Println()
	fmt.Println("  " + authURL)
	fmt.Println()
	if !manual {
		openBrowser(authURL) //nolint:errcheck
	}

	var code string
	if useLocalServer {
		fmt.Println("Waiting for authentication callback on port " + codexCallbackPort + "...")
		select {
		case code = <-codeCh:
		case err := <-errCh:
			return nil, err
		case <-time.After(5 * time.Minute):
			return nil, fmt.Errorf("timed out waiting for OAuth callback")
		}
	} else {
		// Manual mode or port busy: ask user to paste the redirect URL.
		if !manual {
			fmt.Println("(Port " + codexCallbackPort + " is in use; using manual mode.)")
		}
		fmt.Println("After authenticating, paste the full redirect URL here:")
		fmt.Print("> ")
		raw, _ := reader.ReadString('\n')
		raw = strings.TrimSpace(raw)
		code, err = extractCodeFromRedirectURL(raw, state)
		if err != nil {
			return nil, err
		}
	}

	// Exchange code for tokens
	tok, err := exchangeCodeForToken(code, verifier)
	if err != nil {
		return nil, fmt.Errorf("exchanging code for token: %w", err)
	}
	return tok, nil
}

// RefreshCodexToken exchanges a refresh token for a new set of credentials.
func RefreshCodexToken(refreshToken string) (*config.CodexProviderConfig, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", codexClientID)
	form.Set("refresh_token", refreshToken)

	return doTokenRequest(form)
}

// ExtractAccountID decodes the chatgpt_account_id from OAuth tokens.
// It checks the id_token first (primary source), then the access_token as
// fallback, looking in the https://api.openai.com/auth claim namespace and
// falling back to the top-level sub claim.
//
// No signature verification is performed — we only read the payload.
func ExtractAccountID(idToken, accessToken string) string {
	// Try id_token first (primary source per the Codex CLI implementation),
	// then access_token as fallback.
	for _, token := range []string{idToken, accessToken} {
		if id := extractFromJWT(token); id != "" {
			return id
		}
	}
	return ""
}

// NeedsRefresh reports whether the token expires within the next 60 seconds.
func NeedsRefresh(tok *config.CodexProviderConfig) bool {
	if tok == nil || tok.AccessToken == "" {
		return true
	}
	return time.Until(tok.ExpiresAt) < 60*time.Second
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func openBrowser(urlStr string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", urlStr)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", urlStr)
	default:
		cmd = exec.Command("xdg-open", urlStr)
	}
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

func extractCodeFromRedirectURL(rawURL, expectedState string) (string, error) {
	// Accept either the full URL or just the query string
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://localhost/?" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing redirect URL: %w", err)
	}
	q := u.Query()
	if st := q.Get("state"); st != "" && st != expectedState {
		return "", fmt.Errorf("state mismatch in pasted URL")
	}
	code := q.Get("code")
	if code == "" {
		return "", fmt.Errorf("no 'code' parameter found in the pasted URL")
	}
	return code, nil
}

func exchangeCodeForToken(code, verifier string) (*config.CodexProviderConfig, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", codexClientID)
	form.Set("code", code)
	form.Set("redirect_uri", codexRedirectURI)
	form.Set("code_verifier", verifier)

	return doTokenRequest(form)
}

// tokenResponse is the JSON shape returned by the OpenAI OAuth token endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"` // carries chatgpt_account_id in https://api.openai.com/auth namespace
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func doTokenRequest(form url.Values) (*config.CodexProviderConfig, error) {
	resp, err := http.PostForm(codexTokenURL, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("token error %s: %s", tr.Error, tr.ErrorDesc)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("token response contained no access_token (HTTP %d)", resp.StatusCode)
	}

	expiresAt := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	accountID := ExtractAccountID(tr.IDToken, tr.AccessToken)

	return &config.CodexProviderConfig{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    expiresAt,
		AccountID:    accountID,
	}, nil
}

// extractFromJWT decodes the payload of a JWT and returns the chatgpt_account_id.
// Checks the https://api.openai.com/auth namespace first, then top-level
// chatgpt_account_id, then sub as a last resort.
func extractFromJWT(token string) string {
	if token == "" {
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	// JWT payload is base64url-encoded without padding
	// Add padding back before decoding
	seg := parts[1]
	if pad := len(seg) % 4; pad != 0 {
		seg += strings.Repeat("=", 4-pad)
	}
	payload, err := base64.URLEncoding.DecodeString(seg)
	if err != nil {
		// Try without padding as fallback
		payload, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}

	// 1. https://api.openai.com/auth namespace → chatgpt_account_id
	if ns, ok := claims["https://api.openai.com/auth"].(map[string]interface{}); ok {
		if v, ok := ns["chatgpt_account_id"].(string); ok && v != "" {
			return v
		}
	}

	// 2. Top-level chatgpt_account_id
	if v, ok := claims["chatgpt_account_id"].(string); ok && v != "" {
		return v
	}

	// 3. Top-level account_id (older tokens)
	if v, ok := claims["account_id"].(string); ok && v != "" {
		return v
	}

	// 4. sub as last resort
	if v, ok := claims["sub"].(string); ok && v != "" {
		return v
	}

	return ""
}
