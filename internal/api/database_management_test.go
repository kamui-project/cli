package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDatabaseManagementRequestsAndRedirects(t *testing.T) {
	for _, action := range []string{"list", "get", "create", "delete"} {
		t.Run(action, func(t *testing.T) {
			downstream := 0
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { downstream++ }))
			defer target.Close()
			requests := 0
			redirect := false
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Header.Get("Authorization") != "Bearer owner-token" || r.Header.Get("X-Kamui-Client-Type") != "cli" {
					t.Error("missing CLI authentication")
				}
				path, method := "/api/projects", "GET"
				switch action {
				case "get":
					path = "/api/projects/project-id"
				case "create":
					path, method = "/api/databases", "POST"
				case "delete":
					path, method = "/api/database/database-id", "DELETE"
				}
				if r.URL.Path != path || r.Method != method {
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				}
				if redirect {
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
					return
				}
				switch action {
				case "create":
					var body CreateDatabaseRequest
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body != (CreateDatabaseRequest{"project-id", "data", "nano", "shared"}) {
						t.Errorf("incorrect create payload: %+v", body)
					}
					w.WriteHeader(201)
					w.Write([]byte(`{"database_id":"created-id"}`))
				case "list":
					w.Write([]byte(`[]`))
				case "get":
					w.Write([]byte(`{"id":"project-id","database":[]}`))
				case "delete":
					w.WriteHeader(204)
				}
			}))
			defer s.Close()
			c := NewClient(s.URL, "owner-token")
			run := func() error {
				switch action {
				case "list":
					_, err := c.ListDatabaseProjects(context.Background())
					return err
				case "get":
					_, err := c.GetDatabaseProject(context.Background(), "project-id")
					return err
				case "create":
					_, err := c.CreateDatabase(context.Background(), CreateDatabaseRequest{"project-id", "data", "nano", "shared"})
					return err
				default:
					return c.DeleteDatabase(context.Background(), "database-id")
				}
			}
			if err := run(); err != nil {
				t.Fatal(err)
			}
			redirect = true
			if err := run(); err == nil || !strings.Contains(err.Error(), "redirects are not allowed") {
				t.Fatalf("redirect not rejected: %v", err)
			}
			if downstream != 0 || requests != 2 {
				t.Fatalf("unexpected retries or credential forwarding: %d/%d", requests, downstream)
			}
		})
	}
}

func TestDatabaseDeletePreservesInUseError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"message":"Database is still in use by an app"}`))
	}))
	defer s.Close()
	err := NewClient(s.URL, "token").DeleteDatabase(context.Background(), "id")
	if err == nil || err.Error() != "API error (status 400): Database is still in use by an app" {
		t.Fatalf("lost API error: %v", err)
	}
}
