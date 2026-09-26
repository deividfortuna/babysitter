package httpd

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (f *fakeWatches) Takeover(ctx context.Context, id int64, o prwatch.TakeoverOptions) (prwatch.Takeover, error) {
	f.mu.Lock()
	f.takeovers = append(f.takeovers, o)
	refusal := f.takeoverErr
	f.mu.Unlock()
	if refusal != nil {
		return prwatch.Takeover{}, refusal
	}
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return prwatch.Takeover{}, err
	}
	switch {
	case w.Status != store.WatchActive:
		return prwatch.Takeover{}, prwatch.ErrWatchStopped
	case w.Provider == prwatch.ProviderSelf:
		return prwatch.Takeover{}, prwatch.ErrSelfWatch
	case w.TakenOverAt != nil:
		return prwatch.Takeover{}, prwatch.ErrTakenOver
	}
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	if w, err = f.st.SetWatchTakeover(ctx, id, &at, o.PID); err != nil {
		return prwatch.Takeover{}, err
	}
	return prwatch.Takeover{
		Watch: w, WorktreeDir: "/data/worktrees/octo-hello-3", WorkBranch: "babysitter/fix", HeadRef: "fix",
		Argv: []string{"claude", "--resume", "3f0c"}, Declined: []int{4},
	}, nil
}

func TestTheTakeoverRouteGivesTheCommandOfTheAuthor(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}

	var out TakeoverResponse
	if rec := call(t, h, http.MethodPost, "/watches/1/takeover", `{"pid":4242}`, &out); rec.Code != http.StatusOK {
		t.Fatalf("takeover: %d %s", rec.Code, rec.Body)
	}
	if out.Watch.TakenOverAt == nil || out.WorktreeDir != "/data/worktrees/octo-hello-3" || out.WorkBranch != "babysitter/fix" || out.HeadRef != "fix" {
		t.Fatalf("takeover = %+v", out)
	}
	if !slices.Equal(out.Argv, []string{"claude", "--resume", "3f0c"}) || !slices.Equal(out.Declined, []int{4}) || out.NewConversation {
		t.Fatalf("takeover = %+v", out)
	}

	if rec := call(t, h, http.MethodPost, "/watches/1/takeover", `{"pid":4242}`, nil); rec.Code != http.StatusConflict || errorCode(t, rec) != "taken_over" {
		t.Fatalf("a second takeover: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/send", `{"message":"rename it"}`, nil); rec.Code != http.StatusConflict || errorCode(t, rec) != "taken_over" {
		t.Fatalf("a send while taken over: %d %s", rec.Code, rec.Body)
	}
}

func TestTheTakeoverRouteTellsTheServiceTheAuthorRunsAShell(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}

	if rec := call(t, h, http.MethodPost, "/watches/1/takeover", `{"pid":4242,"shell":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("takeover: %d %s", rec.Code, rec.Body)
	}

	fw.mu.Lock()
	defer fw.mu.Unlock()
	if want := []prwatch.TakeoverOptions{{PID: 4242, Shell: true}}; !slices.Equal(fw.takeovers, want) {
		t.Fatalf("takeovers = %+v, want %+v", fw.takeovers, want)
	}
}

func TestTheTakeoverRouteRefusesWhileTheSessionOfTheDaemonRuns(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	fw.mu.Lock()
	fw.takeoverErr = fmt.Errorf("%w (pid 51920): %w", prwatch.ErrSessionRunning, context.DeadlineExceeded)
	fw.mu.Unlock()

	if rec := call(t, h, http.MethodPost, "/watches/1/takeover", `{"pid":4242}`, nil); rec.Code != http.StatusConflict || errorCode(t, rec) != "session_running" {
		t.Fatalf("takeover while the session runs: %d %s", rec.Code, rec.Body)
	}
}

func TestTheTakeoverRouteRefusesWithAReason(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches/7/takeover", `{"pid":4242}`, nil); rec.Code != http.StatusNotFound || errorCode(t, rec) != "watch_not_found" {
		t.Fatalf("takeover of no watch: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src","provider":"self"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/takeover", `{"pid":4242}`, nil); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "self_watch" {
		t.Fatalf("takeover of a self watch: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/takeover", `{}`, nil); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "bad_request" {
		t.Fatalf("takeover with no pid: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/stop", "", nil); rec.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/takeover", `{"pid":4242}`, nil); rec.Code != http.StatusConflict || errorCode(t, rec) != "watch_stopped" {
		t.Fatalf("takeover of a stopped watch: %d %s", rec.Code, rec.Body)
	}
}
