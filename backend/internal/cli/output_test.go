package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func runWithFakeAPI(t *testing.T, g *ghfake.GitHub, args ...string) string {
	t.Helper()
	out, err := runCLI(t, g, filepath.Join(t.TempDir(), "babysitter.db"), args...)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return out
}

func runCLI(t *testing.T, g *ghfake.GitHub, dbPath string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runCLIWriter(t, g, dbPath, &out, args...)
	return out.String(), err
}

func runCLIWriter(t *testing.T, g *ghfake.GitHub, dbPath string, out io.Writer, args ...string) error {
	t.Helper()
	srv := g.Serve(t)

	root := NewRootCmd(WithClientFactory(func(token string, timeout time.Duration) (*github.Client, error) {
		return srv.NewClient()
	}))
	root.SetOut(out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"--token", "x", "--db", dbPath}, args...))
	return root.ExecuteContext(context.Background())
}

func TestWhoamiJSON(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	*g.Viewer() = github.User{Login: new("octocat"), Name: new("The Octocat"), PublicRepos: new(2), Followers: new(3), Following: new(4)}

	out := runWithFakeAPI(t, g, "whoami", "-o", "json")

	var got userOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	want := userOutput{Login: "octocat", Name: "The Octocat", PublicRepos: 2, Followers: 3, Following: 4}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestWhoamiText(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	*g.Viewer() = github.User{Login: new("octocat"), Name: new("The Octocat")}

	out := runWithFakeAPI(t, g, "whoami")

	if !strings.Contains(out, "Login:        octocat\n") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestReposJSON(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	repo := g.Repo("a/one")
	repo.Visibility, repo.Stars, repo.UpdatedAt = "public", 5, ghfake.At("2026-09-01T10:00:00Z")

	out := runWithFakeAPI(t, g, "repos", "--output", "json")

	var got []repoOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if len(got) != 1 || got[0].FullName != "a/one" || got[0].Stars != 5 || got[0].URL != "https://github.com/a/one" {
		t.Fatalf("unexpected output: %+v", got)
	}
}

func TestReposText(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.Repo("a/one")

	out := runWithFakeAPI(t, g, "repos")

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "REPOSITORY") || !strings.HasPrefix(lines[1], "a/one") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}
