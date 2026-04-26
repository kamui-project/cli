package iface

import "context"

// PATInfo is the user-facing view of a Personal Access Token (no plaintext).
type PATInfo struct {
	ID         string
	Name       string
	ExpiresAt  string
	CreatedAt  string
	LastUsedAt *string
}

// TokensService manages user-issued Personal Access Tokens.
type TokensService interface {
	// Create issues a new PAT and returns the plaintext token (shown only here)
	// along with its server-side ID.
	Create(ctx context.Context, name string, expiresInDays int) (token, id string, err error)

	// List returns the user's PATs. When includeOAuth is true, short-lived
	// CLI/MCP OAuth session tokens are also returned (default: hidden).
	List(ctx context.Context, includeOAuth bool) ([]PATInfo, error)

	// Delete revokes a PAT by its server-side ID.
	Delete(ctx context.Context, id string) error
}
