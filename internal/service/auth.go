package service

import (
	"context"
	"fmt"

	"github.com/kamui-project/kamui-cli/internal/auth"
	"github.com/kamui-project/kamui-cli/internal/config"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

// authService implements iface.AuthService
type authService struct {
	configManager *config.Manager
}

// NewAuthService creates a new authentication service
func NewAuthService(configManager *config.Manager) iface.AuthService {
	return &authService{
		configManager: configManager,
	}
}

// Login performs OAuth authentication and saves credentials
func (s *authService) Login(ctx context.Context) error {
	// Check if already logged in
	if s.IsLoggedIn() {
		return fmt.Errorf("already logged in. Use 'kamui logout' first to log out")
	}

	// Get API URL from config
	apiURL, err := s.configManager.GetAPIURL()
	if err != nil {
		return fmt.Errorf("failed to get API URL: %w", err)
	}

	// Create OAuth flow
	oauthFlow := auth.NewOAuthFlow(apiURL)

	// Check for existing client credentials
	clientID, clientSecret, err := s.configManager.GetClientCredentials()
	if err != nil {
		return fmt.Errorf("failed to get client credentials: %w", err)
	}

	// If we have stored credentials, use them
	if clientID != "" {
		oauthFlow.SetClientCredentials(clientID, clientSecret)
	}

	// Perform OAuth flow (will register if no credentials)
	result, err := oauthFlow.Login(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save client credentials if newly registered
	creds := oauthFlow.GetClientCredentials()
	if creds != nil && clientID == "" {
		if err := s.configManager.SaveClientCredentials(creds.ClientID, creds.ClientSecret); err != nil {
			return fmt.Errorf("failed to save client credentials: %w", err)
		}
	}

	// Save tokens
	if err := s.configManager.SaveTokens(result.AccessToken, result.RefreshToken, result.ExpiresIn); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	return nil
}

// Logout clears stored credentials
func (s *authService) Logout(ctx context.Context) error {
	// Check if we have any tokens (even expired ones)
	cfg, err := s.configManager.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.AccessToken == "" && cfg.RefreshToken == "" {
		return fmt.Errorf("not logged in")
	}

	if err := s.configManager.Clear(); err != nil {
		return fmt.Errorf("failed to clear credentials: %w", err)
	}

	return nil
}

// IsLoggedIn checks if the user is currently authenticated
// Note: This only checks if tokens exist, not if they're valid
func (s *authService) IsLoggedIn() bool {
	cfg, err := s.configManager.Load()
	if err != nil {
		return false
	}

	// Consider logged in if we have either access token or refresh token
	return cfg.AccessToken != "" || cfg.RefreshToken != ""
}

// EnsureAuthenticated checks login status and refreshes token if needed
func (s *authService) EnsureAuthenticated(ctx context.Context) error {
	cfg, err := s.configManager.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Check if we have any tokens
	if cfg.AccessToken == "" && cfg.RefreshToken == "" {
		return fmt.Errorf("not logged in. Please run 'kamui login' first")
	}

	// Check if access token is still valid
	if s.configManager.IsLoggedIn() {
		return nil // Token is valid
	}

	// Token expired, try to refresh
	if cfg.RefreshToken == "" {
		return fmt.Errorf("session expired. Please run 'kamui login' again")
	}

	apiURL, err := s.configManager.GetAPIURL()
	if err != nil {
		return fmt.Errorf("failed to get API URL: %w", err)
	}

	oauthFlow := auth.NewOAuthFlow(apiURL)
	oauthFlow.SetClientCredentials(cfg.ClientID, cfg.ClientSecret)

	result, err := oauthFlow.RefreshTokens(ctx, cfg.RefreshToken)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w. Please run 'kamui login' again", err)
	}

	// Save new tokens
	if err := s.configManager.SaveTokens(result.AccessToken, result.RefreshToken, result.ExpiresIn); err != nil {
		return fmt.Errorf("failed to save refreshed tokens: %w", err)
	}

	return nil
}

// GetAccessToken returns the current access token, refreshing if needed
func (s *authService) GetAccessToken(ctx context.Context) (string, error) {
	if err := s.EnsureAuthenticated(ctx); err != nil {
		return "", err
	}

	return s.configManager.GetAccessToken()
}
