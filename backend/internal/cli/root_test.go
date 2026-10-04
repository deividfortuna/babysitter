package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghauth"
)

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
