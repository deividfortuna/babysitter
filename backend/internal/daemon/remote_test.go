package daemon

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/remote"
	"github.com/deividfortuna/babysitter/internal/runfile"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func waitForRemotePort(t *testing.T, runPath string) int {
	t.Helper()
	port := 0
	testutil.Eventually(t, func() bool {
		info, err := runfile.Read(runPath)
		if err != nil {
			t.Fatal(err)
		}
		if info != nil {
			port = info.RemotePort
		}
		return port != 0
	}, "the daemon to write the remote port to running.json")
	return port
}

func remoteStatus(t *testing.T, port int, token string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/v1/watches", port), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestTheRemoteListenerTakesARotatedTokenWithoutARestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	gh := ghfake.New().Serve(t)
	cfg := Config{
		DataDir:    dir,
		DBPath:     filepath.Join(dir, "babysitter.db"),
		AgentBin:   agentOff,
		CopilotBin: filepath.Join(dir, "no-copilot"),
		Interval:   time.Minute,
		Log:        testutil.Logger(t),
		NewClient:  func(context.Context) (*github.Client, error) { return gh.NewClient() },
		Auth:       ghauth.New(dir, ghauth.WithFlag("token")),
		RemoteAddr: "127.0.0.1:0",
		RemoteName: "rotation-test",
	}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan error, 1)
	go func() { ran <- Run(ctx, cfg) }()
	defer func() {
		cancel()
		<-ran
	}()

	port := waitForRemotePort(t, runfile.Path(dir))
	info, err := runfile.Read(runfile.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if info.RemoteHost != "127.0.0.1" {
		t.Fatalf("running.json names the remote host %q, want the bound 127.0.0.1", info.RemoteHost)
	}
	old, err := remote.Token(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := remoteStatus(t, port, old); got != http.StatusOK {
		t.Fatalf("the token before the rotation: status %d, want %d", got, http.StatusOK)
	}

	rotated, err := remote.RotateToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := remoteStatus(t, port, old); got != http.StatusUnauthorized {
		t.Fatalf("the old token after the rotation: status %d, want %d", got, http.StatusUnauthorized)
	}
	if got := remoteStatus(t, port, rotated); got != http.StatusOK {
		t.Fatalf("the rotated token: status %d, want %d", got, http.StatusOK)
	}
}
