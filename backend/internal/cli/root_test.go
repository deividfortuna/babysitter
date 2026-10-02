package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghauth"
)

func TestRootHasSubcommands(t *testing.T) {
	root := NewRootCmd()
	for _, path := range [][]string{
		{"whoami"},
		{"repos"},
		{"repo", "add"},
		{"repo", "remove"},
		{"repo", "list"},
		{"prs"},
		{"pr"},
		{"sync"},
		{"serve"},
		{"notify"},
		{"version"},
		{"service", "install"},
		{"service", "uninstall"},
		{"service", "start"},
		{"service", "stop"},
		{"service", "status"},
	} {
		cmd, _, err := root.Find(path)
		if err != nil || cmd.Name() != path[len(path)-1] {
			t.Errorf("command %v not found: %v", path, err)
		}
	}
}

func TestWhoamiWithoutToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("PATH", "")
	t.Setenv("BABYSITTER_DATA_DIR", t.TempDir())

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"whoami"})

	err := root.ExecuteContext(context.Background())
	if !errors.Is(err, ghauth.ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}

func TestInvalidOutputFormat(t *testing.T) {
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"whoami", "--output", "yaml"})

	err := root.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unknown output format") {
		t.Fatalf("err = %v, want unknown output format error", err)
	}
}
