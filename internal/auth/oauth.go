// Package auth provides OAuth authentication functionality for the CLI.
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/browser"
)

const (
	// DefaultCallbackPort is the default port for the local OAuth callback server
	DefaultCallbackPort = 9876

	// DefaultClientName is the default name for dynamic client registration
	DefaultClientName = "Kamui CLI"
)

// OAuthResult contains the result of an OAuth flow
type OAuthResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	Scope        string
}

// ClientCredentials contains OAuth client credentials
type ClientCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// TokenResponse represents the response from the OAuth token endpoint
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// RegistrationResponse represents the response from dynamic client registration
type RegistrationResponse struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// OAuthFlow handles the OAuth authentication flow
type OAuthFlow struct {
	apiURL       string
	clientID     string
	clientSecret string
	callbackPort int
}

// NewOAuthFlow creates a new OAuth flow handler
func NewOAuthFlow(apiURL string) *OAuthFlow {
	return &OAuthFlow{
		apiURL:       apiURL,
		clientID:     "",
		clientSecret: "",
		callbackPort: DefaultCallbackPort,
	}
}

// SetClientCredentials sets the OAuth client credentials
func (o *OAuthFlow) SetClientCredentials(clientID, clientSecret string) {
	o.clientID = clientID
	o.clientSecret = clientSecret
}

// RegisterClient performs OAuth Dynamic Client Registration (RFC 7591)
// This should be called before Login if no client credentials are stored
func (o *OAuthFlow) RegisterClient(ctx context.Context, redirectURI string) (*ClientCredentials, error) {
	registerURL := o.apiURL + "/oauth/register"

	reqBody := map[string]interface{}{
		"client_name":   DefaultClientName,
		"redirect_uris": []string{redirectURI},
		"grant_types":   []string{"authorization_code", "refresh_token"},
		"scope":         "full",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal registration request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client registration failed with status %d", resp.StatusCode)
	}

	var regResp RegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		return nil, fmt.Errorf("failed to parse registration response: %w", err)
	}

	return &ClientCredentials{
		ClientID:     regResp.ClientID,
		ClientSecret: regResp.ClientSecret,
	}, nil
}

// Login performs the OAuth login flow
// It starts a local server, opens the browser for authentication,
// and waits for the callback with the authorization code
func (o *OAuthFlow) Login(ctx context.Context) (*OAuthResult, error) {
	// Find an available port first (needed for redirect URI)
	port, err := o.findAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to find available port: %w", err)
	}

	redirectURI := fmt.Sprintf("http://localhost:%d/callback", port)

	// If no client credentials, register first
	if o.clientID == "" {
		fmt.Println("Registering CLI with Kamui Platform...")
		creds, err := o.RegisterClient(ctx, redirectURI)
		if err != nil {
			return nil, fmt.Errorf("failed to register client: %w", err)
		}
		o.clientID = creds.ClientID
		o.clientSecret = creds.ClientSecret
	}

	// Generate a random state parameter
	state, err := generateRandomState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	// Channel to receive the authorization code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Start local server
	server := o.startCallbackServer(port, state, codeChan, errChan)
	defer server.Shutdown(context.Background())

	// Build authorization URL
	authURL := o.buildAuthURL(redirectURI, state)

	// Open browser
	fmt.Println("Opening browser for authentication...")
	fmt.Printf("If the browser doesn't open, please visit:\n%s\n\n", authURL)

	if err := browser.OpenURL(authURL); err != nil {
		fmt.Printf("Failed to open browser automatically: %v\n", err)
	}

	fmt.Println("Waiting for authentication...")

	// Wait for the callback or timeout
	select {
	case code := <-codeChan:
		// Exchange the code for tokens
		return o.exchangeCodeForTokens(ctx, code, redirectURI)
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authentication timed out")
	}
}

// GetClientCredentials returns the current client credentials
func (o *OAuthFlow) GetClientCredentials() *ClientCredentials {
	if o.clientID == "" {
		return nil
	}
	return &ClientCredentials{
		ClientID:     o.clientID,
		ClientSecret: o.clientSecret,
	}
}

// RefreshTokens exchanges a refresh token for new tokens
func (o *OAuthFlow) RefreshTokens(ctx context.Context, refreshToken string) (*OAuthResult, error) {
	tokenURL := o.apiURL + "/oauth/token"

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", o.clientID)
	if o.clientSecret != "" {
		data.Set("client_secret", o.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &OAuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		Scope:        tokenResp.Scope,
	}, nil
}

// findAvailablePort finds an available port starting from the default
func (o *OAuthFlow) findAvailablePort() (int, error) {
	for port := o.callbackPort; port < o.callbackPort+10; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			listener.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found")
}

// startCallbackServer starts the local OAuth callback server
func (o *OAuthFlow) startCallbackServer(port int, expectedState string, codeChan chan<- string, errChan chan<- error) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		// Check state parameter
		state := r.URL.Query().Get("state")
		if state != expectedState {
			errChan <- fmt.Errorf("state mismatch")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}

		// Check for errors
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errDesc := r.URL.Query().Get("error_description")
			errChan <- fmt.Errorf("OAuth error: %s - %s", errMsg, errDesc)
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, successHTML("Authentication failed. You can close this window."))
			return
		}

		// Get authorization code
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no authorization code received")
			http.Error(w, "No code received", http.StatusBadRequest)
			return
		}

		// Send success response to browser
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, successHTML("Authentication successful! You can close this window."))

		// Send code to channel
		codeChan <- code
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go server.ListenAndServe()

	return server
}

// buildAuthURL builds the OAuth authorization URL
func (o *OAuthFlow) buildAuthURL(redirectURI, state string) string {
	params := url.Values{}
	params.Set("client_id", o.clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "full")
	params.Set("state", state)

	return fmt.Sprintf("%s/oauth/authorize?%s", o.apiURL, params.Encode())
}

// exchangeCodeForTokens exchanges the authorization code for tokens
func (o *OAuthFlow) exchangeCodeForTokens(ctx context.Context, code, redirectURI string) (*OAuthResult, error) {
	tokenURL := o.apiURL + "/oauth/token"

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", o.clientID)
	if o.clientSecret != "" {
		data.Set("client_secret", o.clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &OAuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
		Scope:        tokenResp.Scope,
	}, nil
}

// generateRandomState generates a cryptographically secure random state string
func generateRandomState() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// successHTML returns the HTML page shown after successful authentication
func successHTML(message string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Kamui CLI</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background-color: #f5f5f5;
        }
        .container {
            text-align: center;
            padding: 40px;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        h1 { color: #333; margin-bottom: 10px; }
        p { color: #666; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Kamui CLI</h1>
        <p>%s</p>
    </div>
</body>
</html>`, message)
}
