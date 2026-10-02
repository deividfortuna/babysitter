package daemon

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/runfile"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

type userAPI struct {
	mu   sync.Mutex
	seen []string
}

func (a *userAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seen = append(a.seen, r.Header.Get("If-None-Match"))
	w.Header().Set("ETag", `W/"one"`)
	if r.Header.Get("If-None-Match") == `W/"one"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"login":"octo"}`)
}

func (a *userAPI) validators() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.seen...)
}

func TestADaemonFindsTheAnswersOfTheDaemonBefore(t *testing.T) {
	t.Cleanup(func() { ghclient.KeepResponses(nil) })
	api := &userAPI{}
	srv := ghfake.Serve(t, api)
	dir := t.TempDir()
	cfg := Config{
		DataDir:    dir,
		Auth:       ghauth.New(dir, ghauth.WithFlag("token")),
		DBPath:     filepath.Join(dir, "babysitter.db"),
		AgentBin:   agentOff,
		CopilotBin: filepath.Join(dir, "no-copilot"),
		Interval:   time.Minute,
		Log:        testutil.Logger(t),
		NewClient: func(context.Context) (*github.Client, error) {
			base, err := ghclient.New("token", 0)
			if err != nil {
				return nil, err
			}
			return srv.NewClient(github.WithHTTPClient(base.Client()))
		},
	}
	run := func() {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ran := make(chan error, 1)
		go func() { ran <- Run(ctx, cfg) }()
		port := waitForPort(t, runfile.Path(dir))
		res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/viewer", port))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /viewer = %d, want 200", res.StatusCode)
		}
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

	run()
	ghclient.KeepResponses(nil)
	run()

	if got := api.validators(); len(got) != 2 || got[1] != `W/"one"` {
		t.Fatalf("validators sent = %q, want the second daemon to send the etag of the first", got)
	}
}
