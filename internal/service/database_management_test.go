package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamui-project/kamui-cli/internal/api"
	"github.com/kamui-project/kamui-cli/internal/config"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

const dbManagementProject = "11111111-1111-4111-8111-111111111111"
const dbManagementOther = "22222222-2222-4222-8222-222222222222"
const dbManagementID = "33333333-3333-4333-8333-333333333333"

type managementAuth struct{ err error }

func (*managementAuth) Login(context.Context) error                    { return nil }
func (*managementAuth) Logout(context.Context) error                   { return nil }
func (*managementAuth) IsLoggedIn() bool                               { return true }
func (*managementAuth) GetAccessToken(context.Context) (string, error) { return "owner-token", nil }
func (a *managementAuth) EnsureAuthenticated(context.Context) error    { return a.err }

func databaseManagementService(t *testing.T, h http.HandlerFunc) *databaseService {
	t.Helper()
	server := httptest.NewTLSServer(h)
	t.Cleanup(server.Close)
	old := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = old })
	manager := config.NewManagerWithPath(filepath.Join(t.TempDir(), "config.json"))
	if err := manager.Save(&config.Config{APIURL: server.URL, AccessToken: "owner-token", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return NewDatabaseService(manager, &managementAuth{}, nil).(*databaseService)
}

func TestDatabaseManagementLifecycleAndBoundaries(t *testing.T) {
	ctx := context.Background()
	mode := "normal"
	creates, deletes, reads := 0, 0, 0
	svc := databaseManagementService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer owner-token" {
			t.Error("missing owner authentication")
		}
		switch r.URL.Path {
		case "/api/projects":
			projects := []api.DatabaseProject{{ID: dbManagementProject, Name: "my project"}}
			if mode == "ambiguous" {
				projects = append(projects, api.DatabaseProject{ID: dbManagementOther, Name: "my project"})
			}
			json.NewEncoder(w).Encode(projects)
		case "/api/projects/" + dbManagementProject:
			reads++
			p := api.DatabaseProject{ID: dbManagementProject, Name: "my project", Databases: []api.DatabaseMetadata{{ID: dbManagementID, Name: "data", SpecType: "nano", Status: "success"}}}
			if mode == "mismatch" {
				p.ID = dbManagementOther
			}
			if mode == "empty" {
				p.Databases = nil
			}
			json.NewEncoder(w).Encode(p)
		case "/api/databases":
			creates++
			var body api.CreateDatabaseRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.ProjectID != dbManagementProject || body.Name != "data" || body.SpecType != "nano" || body.RoutingMode != "shared" {
				t.Errorf("incorrect create: %+v", body)
			}
			if mode == "bad-create-id" {
				json.NewEncoder(w).Encode(map[string]string{"database_id": ""})
				return
			}
			json.NewEncoder(w).Encode(api.CreateDatabaseResponse{DatabaseID: dbManagementID})
		case "/api/database/" + dbManagementID:
			deletes++
			if r.Method != "DELETE" {
				t.Error("unexpected database method")
			}
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected path %q; must not fetch passwords or cross project", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	list, err := svc.List(ctx, "my project")
	if err != nil || len(list) != 1 || list[0].ProjectID != dbManagementProject {
		t.Fatalf("list: %+v %v", list, err)
	}
	got, err := svc.Get(ctx, dbManagementProject, "data")
	if err != nil || got.ID != dbManagementID {
		t.Fatalf("get by name: %+v %v", got, err)
	}
	got, err = svc.Get(ctx, dbManagementProject, dbManagementID)
	if err != nil || got.ID != dbManagementID {
		t.Fatalf("get by ID: %+v %v", got, err)
	}
	output, err := svc.Create(ctx, iface.CreateDatabaseInput{Project: "my project", Name: "data", SpecType: "nano", RoutingMode: "shared"})
	if err != nil || output.DatabaseID != dbManagementID || output.Status != "creation_requested" {
		t.Fatalf("create: %+v %v", output, err)
	}
	if err := svc.Delete(ctx, "my project", "data"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, dbManagementProject, dbManagementOther); err == nil {
		t.Fatal("accepted DB from another project")
	}
	if creates != 1 || deletes != 1 {
		t.Fatalf("unexpected mutations %d/%d", creates, deletes)
	}
	mode = "ambiguous"
	if err := svc.Delete(ctx, "my project", "data"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous project: %v", err)
	}
	mode = "mismatch"
	if err := svc.Delete(ctx, dbManagementProject, "data"); err == nil || !strings.Contains(err.Error(), "different project") {
		t.Fatalf("project mismatch: %v", err)
	}
	if creates != 1 || deletes != 1 {
		t.Fatal("mutation occurred after boundary failure")
	}
	mode = "empty"
	list, err = svc.List(ctx, dbManagementProject)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty list should be []: %+v %v", list, err)
	}
	mode = "bad-create-id"
	_, err = svc.Create(ctx, iface.CreateDatabaseInput{Project: dbManagementProject, Name: "data", SpecType: "nano", RoutingMode: "shared"})
	if err == nil || !strings.Contains(err.Error(), "before retrying") {
		t.Fatalf("ambiguous creation response: %v", err)
	}
	if reads == 0 {
		t.Fatal("missing project scoping reads")
	}
}

func TestDatabaseCreateRejectsInvalidInputBeforeHTTP(t *testing.T) {
	requests := 0
	svc := databaseManagementService(t, func(w http.ResponseWriter, r *http.Request) { requests++; http.Error(w, "unexpected", 500) })
	valid := iface.CreateDatabaseInput{Project: dbManagementProject, Name: "data", SpecType: "nano", RoutingMode: "shared"}
	for _, name := range []string{"", "Uppercase", "1data", "a/b", "a?b", "a\n", "data_user", "data_external", strings.Repeat("a", 31)} {
		input := valid
		input.Name = name
		if _, err := svc.Create(context.Background(), input); err == nil {
			t.Errorf("accepted invalid name %q", name)
		}
	}
	input := valid
	input.SpecType = "unlimited"
	if _, err := svc.Create(context.Background(), input); err == nil {
		t.Error("accepted invalid spec")
	}
	input = valid
	input.RoutingMode = "other"
	if _, err := svc.Create(context.Background(), input); err == nil {
		t.Error("accepted invalid routing")
	}
	input = valid
	input.Project = ""
	if _, err := svc.Create(context.Background(), input); err == nil {
		t.Error("accepted missing project")
	}
	if requests != 0 {
		t.Fatalf("invalid input sent %d requests", requests)
	}
	svc.authService = &managementAuth{err: errors.New("login required")}
	if _, err := svc.List(context.Background(), dbManagementProject); err == nil || err.Error() != "login required" {
		t.Fatalf("auth failure not propagated: %v", err)
	}
	if requests != 0 {
		t.Fatal("request made after auth failure")
	}
}
