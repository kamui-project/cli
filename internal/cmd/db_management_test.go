package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kamui-project/kamui-cli/internal/di"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

type managementDatabaseMock struct {
	iface.DatabaseService
	calls       int
	createInput iface.CreateDatabaseInput
	failure     error
}

func (m *managementDatabaseMock) List(context.Context, string) ([]iface.DatabaseInfo, error) {
	m.calls++
	return []iface.DatabaseInfo{{ID: "db-id", Name: "data\x1b[2J", ProjectName: "my project", SpecType: "nano", Status: "success"}}, m.failure
}
func (m *managementDatabaseMock) Get(_ context.Context, p, r string) (*iface.DatabaseInfo, error) {
	m.calls++
	return &iface.DatabaseInfo{ID: "db-id", Name: r, ProjectID: p}, m.failure
}
func (m *managementDatabaseMock) Create(_ context.Context, in iface.CreateDatabaseInput) (*iface.CreateDatabaseOutput, error) {
	m.calls++
	m.createInput = in
	return &iface.CreateDatabaseOutput{DatabaseID: "db-id", ProjectID: "project-id", Name: in.Name, Status: "creation_requested"}, m.failure
}
func (m *managementDatabaseMock) Delete(context.Context, string, string) error {
	m.calls++
	return m.failure
}

func runManagementCommand(t *testing.T, m *managementDatabaseMock, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand()
	container := di.NewContainerWithServices(nil, nil)
	container.SetDatabaseService(m)
	root.SetContainer(container)
	var out bytes.Buffer
	root.Command().SetOut(&out)
	root.Command().SetErr(&out)
	root.Command().SilenceErrors = true
	root.Command().SilenceUsage = true
	root.Command().SetArgs(args)
	err := root.Command().Execute()
	return out.String(), err
}
func TestDatabaseManagementJSONAndDefaults(t *testing.T) {
	for _, args := range [][]string{
		{"db", "list", "-p", "my project", "-o", "json"},
		{"db", "get", "data", "-p", "my project", "-o", "json"},
		{"db", "create", "--name", "data", "-p", "my project", "-o", "json"},
		{"db", "delete", "data", "-p", "my project", "--yes", "-o", "json"},
	} {
		m := &managementDatabaseMock{}
		out, err := runManagementCommand(t, m, args...)
		if err != nil || !json.Valid([]byte(out)) || m.calls != 1 {
			t.Fatalf("%v => %q %v calls=%d", args, out, err, m.calls)
		}
		if args[1] == "create" && (m.createInput.SpecType != "nano" || m.createInput.RoutingMode != "shared" || m.createInput.Project != "my project") {
			t.Fatalf("incorrect defaults: %+v", m.createInput)
		}
		if strings.Contains(out, "password") || strings.Contains(out, "access_token") {
			t.Fatal("unexpected credential field")
		}
	}
	m := &managementDatabaseMock{}
	out, err := runManagementCommand(t, m, "db", "list", "-p", "project")
	if err != nil || strings.ContainsRune(out, '\x1b') || !strings.Contains(out, "\\x1b") {
		t.Fatalf("unsafe terminal text %q %v", out, err)
	}
}
func TestDatabaseManagementValidationPreventsServiceCalls(t *testing.T) {
	for _, args := range [][]string{
		{"db", "list"},
		{"db", "list", "extra", "-p", "project"},
		{"db", "get", "-p", "project"},
		{"db", "create", "-p", "project"},
		{"db", "delete", "-p", "project", "--yes"},
		{"db", "list", "-p", "project", "-o", "yaml"},
	} {
		m := &managementDatabaseMock{}
		_, err := runManagementCommand(t, m, args...)
		if err == nil || m.calls != 0 {
			t.Fatalf("%v accepted or called service: %v %d", args, err, m.calls)
		}
	}
	// A pipe must never trigger an interactive deletion prompt or delete call.
	pipe, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	defer pipe.Close()
	original := os.Stdin
	os.Stdin = pipe
	defer func() { os.Stdin = original }()
	m := &managementDatabaseMock{}
	_, err = runManagementCommand(t, m, "db", "delete", "data", "-p", "project")
	if err == nil || err.Error() != "database deletion requires --yes when stdin is not a terminal" || m.calls != 0 {
		t.Fatalf("unconfirmed deletion: %v calls=%d", err, m.calls)
	}
	m = &managementDatabaseMock{failure: errors.New("Database is still in use by an app")}
	_, err = runManagementCommand(t, m, "db", "delete", "data", "-p", "project", "--yes")
	if err == nil || err.Error() != "Database is still in use by an app" || m.calls != 1 {
		t.Fatalf("in-use failure lost: %v", err)
	}
}
