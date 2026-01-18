// Package config provides configuration management for the Kamui CLI.
// It handles reading and writing credentials and settings to the config file.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultAPIURL is the default Kamui API endpoint
	DefaultAPIURL = "https://api.kamui-platform.com"

	// ConfigDirName is the name of the config directory
	ConfigDirName = ".kamui"

	// ConfigFileName is the name of the config file
	ConfigFileName = "config.json"
)

// Config represents the CLI configuration stored on disk
type Config struct {
	// AccessToken is the OAuth access token for API authentication
	AccessToken string `json:"access_token,omitempty"`

	// RefreshToken is the OAuth refresh token for obtaining new access tokens
	RefreshToken string `json:"refresh_token,omitempty"`

	// ExpiresAt is the expiration time of the access token
	ExpiresAt time.Time `json:"expires_at,omitempty"`

	// APIURL is the base URL of the Kamui API
	APIURL string `json:"api_url,omitempty"`

	// ClientID is the OAuth client ID from dynamic registration
	ClientID string `json:"client_id,omitempty"`

	// ClientSecret is the OAuth client secret from dynamic registration
	ClientSecret string `json:"client_secret,omitempty"`
}

// Manager handles configuration file operations
type Manager struct {
	configPath string
}

// NewManager creates a new configuration manager
func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configPath := filepath.Join(homeDir, ConfigDirName, ConfigFileName)
	return &Manager{configPath: configPath}, nil
}

// NewManagerWithPath creates a new configuration manager with a custom path
// This is useful for testing
func NewManagerWithPath(configPath string) *Manager {
	return &Manager{configPath: configPath}
}

// Load reads the configuration from disk
// Returns an empty config if the file doesn't exist
func (m *Manager) Load() (*Config, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Return default config if file doesn't exist
			return &Config{
				APIURL: DefaultAPIURL,
			}, nil
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Set default API URL if not specified
	if config.APIURL == "" {
		config.APIURL = DefaultAPIURL
	}

	return &config, nil
}

// Save writes the configuration to disk
func (m *Manager) Save(config *Config) error {
	// Ensure the config directory exists
	configDir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Write with restricted permissions (owner read/write only)
	return os.WriteFile(m.configPath, data, 0600)
}

// Clear removes all authentication data from the config
func (m *Manager) Clear() error {
	config, err := m.Load()
	if err != nil {
		return err
	}

	// Clear auth-related fields (keep client credentials for re-login)
	config.AccessToken = ""
	config.RefreshToken = ""
	config.ExpiresAt = time.Time{}

	return m.Save(config)
}

// Delete removes the config file entirely
func (m *Manager) Delete() error {
	err := os.Remove(m.configPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsLoggedIn checks if valid credentials are stored
func (m *Manager) IsLoggedIn() bool {
	config, err := m.Load()
	if err != nil {
		return false
	}

	if config.AccessToken == "" {
		return false
	}

	// Check if token is expired (with 1 minute buffer)
	if !config.ExpiresAt.IsZero() && time.Now().Add(time.Minute).After(config.ExpiresAt) {
		return false
	}

	return true
}

// GetAccessToken returns the current access token
// Returns an error if not logged in or token is expired
func (m *Manager) GetAccessToken() (string, error) {
	config, err := m.Load()
	if err != nil {
		return "", err
	}

	if config.AccessToken == "" {
		return "", errors.New("not logged in")
	}

	// Check if token is expired
	if !config.ExpiresAt.IsZero() && time.Now().After(config.ExpiresAt) {
		return "", errors.New("token expired")
	}

	return config.AccessToken, nil
}

// GetAPIURL returns the configured API URL
func (m *Manager) GetAPIURL() (string, error) {
	config, err := m.Load()
	if err != nil {
		return "", err
	}

	if config.APIURL == "" {
		return DefaultAPIURL, nil
	}

	return config.APIURL, nil
}

// GetClientCredentials returns the stored OAuth client credentials
// Returns empty strings if not registered
func (m *Manager) GetClientCredentials() (clientID, clientSecret string, err error) {
	config, err := m.Load()
	if err != nil {
		return "", "", err
	}

	return config.ClientID, config.ClientSecret, nil
}

// SaveClientCredentials saves OAuth client credentials to the config
func (m *Manager) SaveClientCredentials(clientID, clientSecret string) error {
	config, err := m.Load()
	if err != nil {
		return err
	}

	config.ClientID = clientID
	config.ClientSecret = clientSecret

	return m.Save(config)
}

// SaveTokens saves OAuth tokens to the config
func (m *Manager) SaveTokens(accessToken, refreshToken string, expiresIn int) error {
	config, err := m.Load()
	if err != nil {
		return err
	}

	config.AccessToken = accessToken
	config.RefreshToken = refreshToken

	if expiresIn > 0 {
		config.ExpiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	}

	return m.Save(config)
}

// ConfigPath returns the path to the config file
func (m *Manager) ConfigPath() string {
	return m.configPath
}
