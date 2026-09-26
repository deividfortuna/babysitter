package cli

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
	"github.com/deividfortuna/babysitter/internal/watcher"
)

func TestServeTakesTheIntervalOfTheSettings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "x.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	next := store.DefaultSettings()
	next.PollInterval = 45 * time.Second
	if _, err := st.SaveSettings(ctx, next); err != nil {
		t.Fatal(err)
	}

	got, err := serveInterval(ctx, st, false, defaultInterval)
	if err != nil || got != 45*time.Second {
		t.Fatalf("serveInterval(untyped) = %s, %v, want the stored 45s", got, err)
	}
	if got, err := serveInterval(ctx, st, true, 2*time.Minute); err != nil || got != 2*time.Minute {
		t.Fatalf("serveInterval(typed) = %s, %v, want the flag", got, err)
	}
	if got, err := serveInterval(ctx, st, true, time.Second); err != nil || got != store.MinInterval {
		t.Fatalf("serveInterval(1s) = %s, %v, want the floor %s", got, err, store.MinInterval)
	}
	if got, err := serveInterval(ctx, st, true, 48*time.Hour); err != nil || got != store.MaxInterval {
		t.Fatalf("serveInterval(48h) = %s, %v, want the roof %s", got, err, store.MaxInterval)
	}
}

func TestServeSaysWhenItPullsTheIntervalToTheBound(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := &lines{}
	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(log)
	root.SetArgs([]string{"--token", "x", "--db", filepath.Join(t.TempDir(), "x.db"), "serve", "--interval", "5s"})
	done := make(chan error, 1)
	go func() { done <- root.ExecuteContext(ctx) }()

	if !testutil.Within(testutil.Timeout, func() bool { return strings.Contains(log.text(), "polling every 10s") }) {
		t.Fatalf("log = %q, want the rate it settled on", log.text())
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve --interval 5s error = %v, want the run of a service that keeps polling", err)
	}
}

func TestServiceInstallRefusesAnIntervalOutsideTheBound(t *testing.T) {
	m := &fakeManager{}
	_, err := runService(t, m, "install", "--interval", "5s")
	if err == nil {
		t.Fatalf("install took 5s and wrote %v", m.cfg.Args)
	}
	if !strings.Contains(err.Error(), store.MinInterval.String()) {
		t.Fatalf("err = %v, want the bound of the settings in it", err)
	}
	if len(m.calls) != 0 {
		t.Fatalf("calls = %v, want nothing installed", m.calls)
	}
}

func TestServiceInstallLeavesTheIntervalToTheSettings(t *testing.T) {
	m := &fakeManager{}
	if _, err := runService(t, m, "install"); err != nil {
		t.Fatal(err)
	}
	for _, arg := range m.cfg.Args {
		if arg == "--interval" {
			t.Fatalf("args = %v, want no interval written into the service", m.cfg.Args)
		}
	}
}

func TestFollowIntervalTakesANewSetting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	w := watcher.New(st, nil, watcher.WithInterval(time.Minute))

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	go followInterval(runCtx, st, w, 5*time.Millisecond, testutil.Logger(t))

	next := store.DefaultSettings()
	next.PollInterval = 30 * time.Second
	if _, err := st.SaveSettings(ctx, next); err != nil {
		t.Fatal(err)
	}

	if testutil.Within(testutil.Timeout, func() bool { return w.Interval() == 30*time.Second }) {
		return
	}
	t.Fatalf("Interval() = %s, want the stored 30s", w.Interval())
}

func TestTheIntervalGoroutineIsGoneBeforeTheStoreCloses(t *testing.T) {
	t.Parallel()
	for range 50 {
		st, err := store.Open(filepath.Join(t.TempDir(), "x.db"))
		if err != nil {
			t.Fatal(err)
		}
		w := watcher.New(st, nil, watcher.WithInterval(store.MinInterval))
		ctx, stop := context.WithCancel(context.Background())
		log := &lines{}
		stopFollow := startFollowInterval(ctx, st, w, time.Microsecond, slog.New(slog.NewTextHandler(log, nil)))
		time.Sleep(time.Millisecond)

		stopFollow()
		stop()
		st.Close()

		if got := log.text(); strings.Contains(got, "err=") {
			t.Fatalf("the interval goroutine read the store serve had closed: %s", got)
		}
	}
}

type lines struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (l *lines) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lines) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}
