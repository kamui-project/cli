package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAPIURL(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid https", "https://api.kamui-platform.com", false},
		{"valid https with path", "https://api.kamui-platform.com/v1", false},
		{"empty", "", true},
		{"http scheme", "http://api.kamui-platform.com", true},
		{"file scheme", "file:///etc/passwd", true},
		{"javascript scheme", "javascript:alert(1)", true},
		{"leading dash", "-https://attacker.test", true},
		{"missing host", "https://", true},
		{"with userinfo", "https://user:pass@api.kamui-platform.com", true},
		{"garbage", "::not a url::", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAPIURL(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("validateAPIURL(%q) = nil, want error", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateAPIURL(%q) = %v, want nil", tc.input, err)
			}
		})
	}
}

func TestGetAPIURL_FallsBackOnInvalidStored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Config{APIURL: "http://attacker.test"}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}

	m := NewManagerWithPath(path)
	got, err := m.GetAPIURL()
	if err != nil {
		t.Fatalf("GetAPIURL: %v", err)
	}
	if got != DefaultAPIURL {
		t.Errorf("GetAPIURL = %q, want %q (fallback)", got, DefaultAPIURL)
	}
}

func TestGetAPIURL_EmptyReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	m := NewManagerWithPath(path)
	got, err := m.GetAPIURL()
	if err != nil {
		t.Fatalf("GetAPIURL: %v", err)
	}
	if got != DefaultAPIURL {
		t.Errorf("GetAPIURL = %q, want %q", got, DefaultAPIURL)
	}
}

func TestGetAPIURL_ValidStoredPassesThrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := Config{APIURL: "https://staging.kamui-platform.com"}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}

	m := NewManagerWithPath(path)
	got, err := m.GetAPIURL()
	if err != nil {
		t.Fatalf("GetAPIURL: %v", err)
	}
	if got != "https://staging.kamui-platform.com" {
		t.Errorf("GetAPIURL = %q, want pass-through", got)
	}
}
