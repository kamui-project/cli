package service

import (
	"context"
	"errors"
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
	// Reject only if the current access token is still valid.
	// Expired sessions are allowed to re-login directly without `kamui logout`.
	if s.configManager.IsLoggedIn() {
		return fmt.Errorf("already logged in. Use 'kamui logout' first to log out")
	}

	// Get API URL from config
	apiURL, err := s.configManager.GetAPIURL()
	if err != nil {
		return fmt.Errorf("failed to get API URL: %w", err)
	}

	// Create OAuth flow. Login() always performs a fresh DCR — we no longer
	// reuse stored client credentials across logins.
	oauthFlow := auth.NewOAuthFlow(apiURL)

	result, err := oauthFlow.Login(ctx)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Save the freshly-minted client credentials. They're needed for token
	// refresh (until the next login overwrites them).
	creds := oauthFlow.GetClientCredentials()
	if creds != nil {
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

// Logout revokes server-side tokens (RFC 7009) then clears local credentials.
// Server-side revoke is best-effort: if the network or server is unavailable,
// local credentials are still cleared (logout MUST work offline).
func (s *authService) Logout(ctx context.Context) error {
	cfg, err := s.configManager.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.AccessToken == "" && cfg.RefreshToken == "" {
		return fmt.Errorf("not logged in")
	}

	// Best-effort server-side revocation. We need client credentials to
	// authenticate the revoke call; if they're missing (e.g. partially
	// corrupted config) just skip and clear local state.
	if cfg.ClientID != "" && cfg.ClientSecret != "" {
		oauthFlow := auth.NewOAuthFlow(cfg.APIURL)
		oauthFlow.SetClientCredentials(cfg.ClientID, cfg.ClientSecret)

		if cfg.AccessToken != "" {
			if err := oauthFlow.Revoke(ctx, cfg.AccessToken, "access_token"); err != nil {
				// Don't abort logout; just inform the user.
				fmt.Printf("Warning: server-side access token revoke failed (%v). Local credentials will still be cleared.\n", err)
			}
		}
		if cfg.RefreshToken != "" {
			if err := oauthFlow.Revoke(ctx, cfg.RefreshToken, "refresh_token"); err != nil {
				fmt.Printf("Warning: server-side refresh token revoke failed (%v). Local credentials will still be cleared.\n", err)
			}
		}
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
		if errors.Is(err, auth.ErrRefreshTokenInvalid) {
			// Refresh token was rejected by the server: drop local tokens
			// so the user can simply run `kamui login` again.
			if clearErr := s.configManager.Clear(); clearErr != nil {
				return fmt.Errorf("session expired and failed to clear local credentials: %w", clearErr)
			}
			return fmt.Errorf("session expired. Please run 'kamui login' again")
		}
		return fmt.Errorf("failed to refresh token: %w", err)
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
