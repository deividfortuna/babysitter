package daemon

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/runfile"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

type heldNotifier struct {
	got     chan struct{}
	release chan struct{}
}

func (n *heldNotifier) Send(ctx context.Context, _ notify.Notification) (notify.Result, error) {
	n.got <- struct{}{}
	select {
	case <-n.release:
		return notify.Result{Backend: "held"}, nil
	case <-ctx.Done():
		return notify.Result{}, ctx.Err()
	}
}

func waitForPort(t *testing.T, runPath string) int {
	t.Helper()
	port := 0
	testutil.Eventually(t, func() bool {
		info, err := runfile.Read(runPath)
		if err != nil {
			t.Fatal(err)
		}
		if info != nil {
			port = info.Port
		}
		return port != 0
	}, "the daemon to write running.json")
	return port
}

func TestRunWaitsForTheBannerOnShutdown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	notifier := &heldNotifier{got: make(chan struct{}, 1), release: make(chan struct{})}
	gh := ghfake.New().Serve(t)
	cfg := Config{
		DataDir:    dir,
		Auth:       ghauth.New(dir, ghauth.WithFlag("token")),
		DBPath:     filepath.Join(dir, "babysitter.db"),
		AgentBin:   agentOff,
		CopilotBin: filepath.Join(dir, "no-copilot"),
		Notifier:   notifier,
		Interval:   time.Minute,
		Log:        testutil.Logger(t),
		NewClient:  func(context.Context) (*github.Client, error) { return gh.NewClient() },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ran := make(chan error, 1)
	go func() { ran <- Run(ctx, cfg) }()

	port := waitForPort(t, runfile.Path(dir))
	body := bytes.NewBufferString(`{"title":"PR #42","body":"alice left a comment"}`)
	res, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/v1/notifications", port), "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST /notifications = %d, want %d", res.StatusCode, http.StatusCreated)
	}
	select {
	case <-notifier.got:
	case <-time.After(5 * time.Second):
		t.Fatal("the desktop never got the banner")
	}

	cancel()
	select {
	case err := <-ran:
		t.Fatalf("Run() returned %v while the desktop still held the banner", err)
	case <-time.After(300 * time.Millisecond):
	}

	close(notifier.release)
	select {
	case err := <-ran:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() never returned after the desktop answered")
	}
}

func TestRunGivesUpOnABannerAfterTheGrace(t *testing.T) {
	bannerGrace = 100 * time.Millisecond
	t.Cleanup(func() { bannerGrace = supervisorGrace })
	dir := t.TempDir()
	notifier := &heldNotifier{got: make(chan struct{}, 1), release: make(chan struct{})}
	gh := ghfake.New().Serve(t)
	cfg := Config{
		DataDir:    dir,
		Auth:       ghauth.New(dir, ghauth.WithFlag("token")),
		DBPath:     filepath.Join(dir, "babysitter.db"),
		AgentBin:   agentOff,
		CopilotBin: filepath.Join(dir, "no-copilot"),
		Notifier:   notifier,
		Interval:   time.Minute,
		Log:        testutil.Logger(t),
		NewClient:  func(context.Context) (*github.Client, error) { return gh.NewClient() },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ran := make(chan error, 1)
	go func() { ran <- Run(ctx, cfg) }()

	port := waitForPort(t, runfile.Path(dir))
	body := bytes.NewBufferString(`{"title":"PR #42","body":"alice left a comment"}`)
	res, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/api/v1/notifications", port), "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	<-notifier.got

	cancel()
	select {
	case err := <-ran:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() waited for a banner past the grace")
	}
}
