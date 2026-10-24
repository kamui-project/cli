package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDatabasePermitUsesOwnerAuthenticationAndNeverFallsBack(t *testing.T) {
	requests := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/database/test-db/access-permit" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer owner-token" || r.Header.Get("X-Kamui-Client-Type") != "cli" {
			t.Error("incorrect permit request")
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer s.Close()
	c := NewClient(s.URL, "owner-token")
	if permit, err := c.GetDatabaseAccessPermit(context.Background(), "test-db"); err == nil || permit != nil {
		t.Fatal("unavailable permit accepted")
	}
	if requests != 1 {
		t.Fatal("unexpected fallback request")
	}
}
