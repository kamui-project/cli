package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kamui-project/kamui-cli/internal/di"
	iface "github.com/kamui-project/kamui-cli/internal/service/interface"
)

func TestDeleteWithoutConsentRejectsNonTerminalBeforeServiceCalls(t *testing.T) {
	for _, group := range []string{"apps", "projects"} {
		for _, scenario := range []struct {
			name  string
			input string
			flags []string
		}{
			{name: "closed stdin"},
			{name: "piped yes is not consent", input: "y\n"},
			{name: "explicit false is not consent", input: "y\n", flags: []string{"--yes=false"}},
			{name: "json refusal stays on stderr", flags: []string{"-o", "json"}},
		} {
			t.Run(group+"/"+scenario.name, func(t *testing.T) {
				stdin, input, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { stdin.Close() })
				if _, err := io.WriteString(input, scenario.input); err != nil {
					t.Fatal(err)
				}
				input.Close()
				stdout, output, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { stdout.Close(); output.Close() })
				originalIn, originalOut := os.Stdin, os.Stdout
				os.Stdin, os.Stdout = stdin, output
				t.Cleanup(func() { os.Stdin, os.Stdout = originalIn, originalOut })

				calls := 0
				projects := &MockProjectService{
					ListProjectsFunc:  func(context.Context) ([]iface.Project, error) { calls++; return nil, nil },
					DeleteProjectFunc: func(context.Context, string) error { calls++; return nil },
				}
				apps := &MockAppService{
					GetAppFunc:    func(context.Context, string) (*iface.AppDetail, error) { calls++; return nil, nil },
					DeleteAppFunc: func(context.Context, string) error { calls++; return nil },
				}
				root := NewRootCommand()
				root.SetContainer(di.NewContainerWithAllServices(&MockAuthService{}, projects, apps))
				var commandErr bytes.Buffer
				root.Command().SetErr(&commandErr)
				args := append([]string{group, "delete", "target-id"}, scenario.flags...)
				root.Command().SetArgs(args)
				err = root.Command().Execute()
				output.Close()
				captured, readErr := io.ReadAll(stdout)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if err == nil || calls != 0 {
					t.Fatalf("unapproved non-terminal deletion: error=%v service calls=%d", err, calls)
				}
				if len(captured) != 0 {
					t.Fatalf("refusal printed to stdout: %q", captured)
				}
				diagnostic := strings.ToLower(strings.SplitN(commandErr.String(), "Usage:", 2)[0])
				for _, required := range []string{"deletion", "--yes", "terminal"} {
					if !strings.Contains(diagnostic, required) {
						t.Errorf("stderr diagnostic lacks %q: %s", required, diagnostic)
					}
				}
			})
		}
	}
}
