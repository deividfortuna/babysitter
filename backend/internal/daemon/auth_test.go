package daemon

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/httpd"
)

func TestSignInFailure(t *testing.T) {
	t.Parallel()
	cases := map[error]string{
		ghauth.ErrCodeExpired:             httpd.SignInCodeExpired,
		ghauth.ErrDenied:                  httpd.SignInDenied,
		errors.New("GitHub answered 502"): httpd.SignInFailed,
	}
	for err, want := range cases {
		if got := signInFailure(err); got != want {
			t.Errorf("signInFailure(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestStatusSaysWhenTheInstallationsCannotBeRead(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	dir := t.TempDir()
	signIn := `{"login":"alice","accessToken":"ghu_signedin0000000000000000"}`
	if err := os.WriteFile(filepath.Join(dir, "github-app.json"), []byte(signIn), 0o600); err != nil {
		t.Fatal(err)
	}
	g := ghfake.New()
	g.Fail(ghfake.RouteInstallations, http.StatusBadGateway, "bad gateway")
	srv := g.Serve(t)
	auth := ghauth.New(dir, ghauth.WithGH(func(context.Context) (string, error) { return "", errors.New("no gh") }))
	c := &authController{
		auth:      auth,
		signIns:   auth.SignIns(t.Context(), nil),
		newClient: func(context.Context) (*github.Client, error) { return srv.NewClient() },
	}

	st := c.Status(context.Background())

	if st.State != string(ghauth.StateConnected) {
		t.Fatalf("State = %q, want connected", st.State)
	}
	if st.InstallsError == "" {
		t.Fatal("InstallsError is empty, want the reason the installations are unknown")
	}
	if len(st.Installations) != 0 {
		t.Fatalf("Installations = %v, want none", st.Installations)
	}
	if st.Error != "" {
		t.Fatalf("Error = %q, want the failure only in installationsError", st.Error)
	}
}

func TestTheDaemonDoesNotStartWithoutTheGitConfigOfTheSessions(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{DataDir: dir, Auth: ghauth.New(dir, ghauth.WithGH(func(context.Context) (string, error) { return "", errors.New("no gh") }))}

	if _, err := newAuthController(t.Context(), cfg, events.NewBus(), slog.New(slog.DiscardHandler), ""); err == nil {
		t.Fatal("err = nil, want the failed write of the git config of the sessions")
	}
}

func TestAFailedWriteOfTheGitConfigIsTriedAgain(t *testing.T) {
	t.Parallel()
	changes := make(chan struct{}, 1)
	written := make(chan struct{})
	failures := 2
	write := func() error {
		if failures > 0 {
			failures--
			return errors.New("disk full")
		}
		close(written)
		return nil
	}
	go keepGitConfig(t.Context(), changes, write, time.Millisecond, slog.New(slog.DiscardHandler))

	changes <- struct{}{}

	select {
	case <-written:
	case <-time.After(5 * time.Second):
		t.Fatal("the git config was not written again after the failure")
	}
}

func TestAFailedRunStopsWritingTheGitConfigOfTheSessions(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	dir := t.TempDir()
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	gh := ghfake.New().Serve(t)
	auth := ghauth.New(dir, ghauth.WithGH(func(context.Context) (string, error) { return "", errors.New("no gh") }))
	cfg := Config{
		DataDir:    dir,
		Auth:       auth,
		DBPath:     filepath.Join(dir, "babysitter.db"),
		Port:       taken.Addr().(*net.TCPAddr).Port,
		AgentBin:   agentOff,
		CopilotBin: filepath.Join(dir, "no-copilot"),
		Interval:   time.Minute,
		Log:        slog.New(slog.DiscardHandler),
		NewClient:  func(context.Context) (*github.Client, error) { return gh.NewClient() },
	}

	if err := Run(t.Context(), cfg); err == nil {
		t.Fatal("Run() on a port in use = nil, want an error")
	}
	signIn := `{"login":"alice","accessToken":"ghu_app"}`
	if err := os.WriteFile(filepath.Join(dir, "github-app.json"), []byte(signIn), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Credential(t.Context()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	got, err := os.ReadFile(agent.AppGitConfigPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("app.gitconfig = %q after Run returned, want nothing written by a daemon that stopped", got)
	}
}
