package httpd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (f *fakeWatches) Handback(ctx context.Context, id int64, o prwatch.HandbackOptions) (store.Watch, error) {
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	f.mu.Lock()
	running, work := f.authorRunning, f.authorWork
	f.mu.Unlock()
	switch {
	case w.TakenOverAt == nil:
		return store.Watch{}, prwatch.ErrNotTakenOver
	case running && !o.Force:
		return store.Watch{}, fmt.Errorf("%w (pid %d)", prwatch.ErrAuthorRunning, w.TakenOverPID)
	case work != nil && !o.Confirm:
		return store.Watch{}, work
	}
	return f.st.SetWatchTakeover(ctx, id, nil, 0)
}

func takenOver(t *testing.T, h http.Handler) {
	t.Helper()
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/takeover", `{"pid":4242}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("takeover: %d %s", rec.Code, rec.Body)
	}
}

func TestTheHandbackRouteGivesTheWatchBack(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches/1/handback", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("hand-back of no watch: %d %s", rec.Code, rec.Body)
	}
	takenOver(t, h)

	var out Watch
	if rec := call(t, h, http.MethodPost, "/watches/1/handback", "", &out); rec.Code != http.StatusOK || out.TakenOverAt != nil {
		t.Fatalf("hand-back: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/handback", "", nil); rec.Code != http.StatusConflict || errorCode(t, rec) != "not_taken_over" {
		t.Fatalf("a second hand-back: %d %s", rec.Code, rec.Body)
	}
}

func TestTheHandbackRouteListsTheWorkToConfirm(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)
	takenOver(t, h)
	fw.authorWork = &prwatch.WorkError{
		Commits: []gitrelease.Commit{{SHA: "5d0b7f1", Subject: "Start the sink"}, {SHA: "9a1c3e2", Subject: "Wait for the sink"}},
		Files:   []string{" M internal/webhook/deliver_test.go"},
	}

	rec := call(t, h, http.MethodPost, "/watches/1/handback", `{}`, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("hand-back with work: %d %s", rec.Code, rec.Body)
	}
	var refusal HandbackRefusal
	if err := json.Unmarshal(rec.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	want := []WorkCommit{{SHA: "5d0b7f1", Subject: "Start the sink"}, {SHA: "9a1c3e2", Subject: "Wait for the sink"}}
	if refusal.Error.Code != "unconfirmed_work" || !slices.Equal(refusal.Commits, want) || !slices.Equal(refusal.Files, []string{" M internal/webhook/deliver_test.go"}) {
		t.Fatalf("refusal = %+v", refusal)
	}

	if rec := call(t, h, http.MethodPost, "/watches/1/handback", `{"confirm":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("a confirmed hand-back: %d %s", rec.Code, rec.Body)
	}
}

func TestTheHandbackRouteRefusesWhileTheAgentOfTheAuthorRuns(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)
	takenOver(t, h)
	fw.authorRunning = true

	rec := call(t, h, http.MethodPost, "/watches/1/handback", `{"confirm":true}`, nil)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "author_running" {
		t.Fatalf("hand-back while the agent runs: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/handback", `{"force":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("a forced hand-back: %d %s", rec.Code, rec.Body)
	}
}
