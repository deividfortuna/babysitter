package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/runfile"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

// A daemon that was killed without cleanup leaves its run file, and
// Windows gives its pid to the next process, which can be the daemon
// that replaces it. That file is stale, not a sibling.
func TestRunReplacesARunFileThatNamesThisProcess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stale := runfile.Info{PID: os.Getpid(), Port: 1, StartedAt: time.Now().Add(-time.Hour)}
	if err := runfile.Write(runfile.Path(dir), stale); err != nil {
		t.Fatal(err)
	}
	gh := ghfake.New().Serve(t)
	cfg := Config{
		DataDir:    dir,
		DBPath:     filepath.Join(dir, "babysitter.db"),
		AgentBin:   agentOff,
		CopilotBin: filepath.Join(dir, "no-copilot"),
		Interval:   time.Minute,
		Log:        testutil.Logger(t),
		NewClient:  func(context.Context) (*github.Client, error) { return gh.NewClient() },
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ran := make(chan error, 1)
	go func() { ran <- Run(ctx, cfg) }()

	testutil.Eventually(t, func() bool {
		select {
		case err := <-ran:
			t.Fatalf("Run() returned %v, want the stale run file replaced", err)
		default:
		}
		info, err := runfile.Read(runfile.Path(dir))
		return err == nil && info != nil && info.Port != stale.Port
	}, "the daemon to replace the stale run file")

	cancel()
	select {
	case err := <-ran:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() never returned")
	}
}
