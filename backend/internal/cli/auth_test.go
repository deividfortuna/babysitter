package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func runAuth(t *testing.T, g *ghfake.GitHub, dir string, args ...string) (string, error) {
	t.Helper()
	srv := g.Serve(t)
	root := NewRootCmd(
		WithClientFactory(func(string, time.Duration) (*github.Client, error) { return srv.NewClient() }),
		WithAuthOptions(
			ghauth.WithApp(ghauth.App{ClientID: "Iv1.test", Slug: "babysitter"}),
			ghauth.WithOAuth(ghauth.OAuth{BaseURL: srv.URL, PollUnit: time.Millisecond}),
			ghauth.WithGH(func(context.Context) (string, error) { return "", errors.New("gh is not installed") }),
		),
	)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append(args, "--data-dir", dir))
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func approveOnPoll(g *ghfake.GitHub) {
	g.React(ghfake.RouteOAuthGrant, func(ghfake.Action) (ghfake.Response, bool) {
		g.ApproveDevice()
		return ghfake.Response{}, false
	})
}

func TestAuthLoginStatusLogout(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	g := ghfake.New()
	g.Install("alice")
	approveOnPoll(g)
	dir := t.TempDir()

	out, err := runAuth(t, g, dir, "auth", "login")
	if err != nil {
		t.Fatalf("auth login: %v", err)
	}
	for _, want := range []string{"enter the code ABCD-0001", "Signed in to GitHub as alice", "https://github.com/apps/babysitter/installations/new"} {
		if !strings.Contains(out, want) {
			t.Errorf("auth login printed %q, want %q in it", out, want)
		}
	}

	out, err = runAuth(t, g, dir, "auth", "status", "-o", "json")
	if err != nil {
		t.Fatalf("auth status: %v", err)
	}
	var st authOutput
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if st.Origin != "app" || !st.SignedIn || st.Login != "alice" || len(st.Installations) != 1 || st.Installations[0] != "alice" {
		t.Fatalf("status = %+v", st)
	}

	if _, err := runAuth(t, g, dir, "auth", "logout"); err != nil {
		t.Fatalf("auth logout: %v", err)
	}
	out, err = runAuth(t, g, dir, "auth", "status")
	if err != nil {
		t.Fatalf("auth status: %v", err)
	}
	if !strings.Contains(out, "not signed in") || !strings.Contains(out, "Token from:    none") {
		t.Fatalf("status after logout = %q", out)
	}
}

func TestAuthLoginTellsWhenATokenComesFirst(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_from_env")
	g := ghfake.New()
	approveOnPoll(g)

	out, err := runAuth(t, g, t.TempDir(), "auth", "login")
	if err != nil {
		t.Fatalf("auth login: %v", err)
	}
	if !strings.Contains(out, "the GITHUB_TOKEN environment variable comes before the app") {
		t.Fatalf("auth login printed %q, want the note on GITHUB_TOKEN", out)
	}
}

func TestAuthLoginRefused(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	g := ghfake.New()
	g.React(ghfake.RouteOAuthGrant, func(ghfake.Action) (ghfake.Response, bool) {
		g.DenyDevice()
		return ghfake.Response{}, false
	})

	_, err := runAuth(t, g, t.TempDir(), "auth", "login")

	if !errors.Is(err, ghauth.ErrDenied) {
		t.Fatalf("err = %v, want ErrDenied", err)
	}
}

func TestCommandsUseTheAppAfterLogin(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	g := ghfake.New()
	approveOnPoll(g)
	dir := t.TempDir()
	t.Setenv("BABYSITTER_DATA_DIR", dir)
	if _, err := runAuth(t, g, dir, "auth", "login"); err != nil {
		t.Fatal(err)
	}
	var tokens []string
	srv := g.Serve(t)
	root := NewRootCmd(
		WithClientFactory(func(token string, _ time.Duration) (*github.Client, error) {
			tokens = append(tokens, token)
			return srv.NewClient()
		}),
		WithAuthOptions(ghauth.WithGH(func(context.Context) (string, error) { return "gho_from_gh", nil })),
	)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"whoami"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if len(tokens) != 1 || !strings.HasPrefix(tokens[0], "ghu_") {
		t.Fatalf("whoami used %v, want the token of the app", tokens)
	}
}
