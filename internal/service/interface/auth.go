// Package iface defines service interfaces for the Kamui CLI.
// These interfaces enable dependency injection and mocking for tests.
package iface

import (
	"context"
)

// AuthService defines the interface for authentication operations
type AuthService interface {
	// Login performs OAuth authentication and saves credentials
	Login(ctx context.Context) error

	// Logout clears stored credentials
	Logout(ctx context.Context) error

	// IsLoggedIn checks if the user is currently authenticated
	IsLoggedIn() bool

	// GetAccessToken returns the current access token, refreshing if needed
	GetAccessToken(ctx context.Context) (string, error)

	// EnsureAuthenticated checks login status and refreshes token if needed
	EnsureAuthenticated(ctx context.Context) error
}

