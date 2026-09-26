package httpd

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
)

const branchRule = 2

func (f *fakeWatches) SetMergeRules(ctx context.Context, id int64, c prwatch.MergeRulesChange) (store.Watch, error) {
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	negative := c.ApprovalsRequired.Count != nil && *c.ApprovalsRequired.Count < 0
	unknownMethod := c.MergeMethod != nil && *c.MergeMethod == "fast-forward"
	switch {
	case negative:
		return store.Watch{}, prwatch.ErrBadApprovals
	case w.Status != store.WatchActive:
		return store.Watch{}, prwatch.ErrWatchStopped
	case unknownMethod:
		return store.Watch{}, prwatch.ErrBadMergeMethod
	}
	f.mu.Lock()
	f.mergeRules = append(f.mergeRules, c)
	f.mu.Unlock()
	approvals, method := w.ApprovalsRequired, w.MergeMethod
	if c.ApprovalsRequired.Set {
		approvals = branchRule
	}
	if n := c.ApprovalsRequired.Count; n != nil {
		approvals = *n
	}
	if c.MergeMethod != nil {
		method = *c.MergeMethod
	}
	return f.st.SetWatchMergeRules(ctx, id, approvals, method)
}

func TestAPatchChangesTheMergeRulesOfAWatch(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	if _, err := st.CreateWatch(context.Background(), store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now(), ApprovalsRequired: 1, MergeMethod: "squash"}); err != nil {
		t.Fatal(err)
	}

	var got Watch
	if rec := call(t, h, http.MethodPatch, "/watches/1", `{"approvalsRequired":0,"mergeMethod":"rebase"}`, &got); rec.Code != http.StatusOK ||
		got.ApprovalsRequired != 0 || got.MergeMethod != "rebase" {
		t.Fatalf("set both: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPatch, "/watches/1", `{"approvalsRequired":null}`, &got); rec.Code != http.StatusOK ||
		got.ApprovalsRequired != branchRule || got.MergeMethod != "rebase" {
		t.Fatalf("the rule of the base branch: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPatch, "/watches/1", `{"mergeMethod":""}`, &got); rec.Code != http.StatusOK ||
		got.ApprovalsRequired != branchRule || got.MergeMethod != "" {
		t.Fatalf("the repository default: %d %s", rec.Code, rec.Body)
	}

	fw.mu.Lock()
	changes := fw.mergeRules
	fw.mu.Unlock()
	if len(changes) != 3 || changes[2].ApprovalsRequired.Set || changes[1].MergeMethod != nil {
		t.Fatalf("changes = %+v, want the fields each body left out unset", changes)
	}
}

func TestAPatchRefusesWhatAWatchCannotTake(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	ctx := context.Background()
	if _, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	stopped, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 4, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.StopWatch(ctx, stopped.ID, store.StopUser, nil, time.Now()); err != nil {
		t.Fatal(err)
	}

	cases := map[string]struct {
		path, body string
		code       int
	}{
		"approvals below zero":   {"/watches/1", `{"approvalsRequired":-1}`, http.StatusBadRequest},
		"merge method unknown":   {"/watches/1", `{"mergeMethod":"fast-forward"}`, http.StatusBadRequest},
		"body not JSON":          {"/watches/1", `approvals`, http.StatusBadRequest},
		"body not an object":     {"/watches/1", `[1]`, http.StatusBadRequest},
		"approvals not a number": {"/watches/1", `{"approvalsRequired":"two"}`, http.StatusBadRequest},
		"no such watch":          {"/watches/99", `{"approvalsRequired":1}`, http.StatusNotFound},
		"watch stopped":          {"/watches/2", `{"approvalsRequired":1}`, http.StatusConflict},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if rec := call(t, h, http.MethodPatch, tc.path, tc.body, nil); rec.Code != tc.code {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.code, rec.Body)
			}
		})
	}
}

func TestAPatchRefusesAFieldOfAWatchThatCannotChange(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	if _, err := st.CreateWatch(context.Background(), store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now(), ApprovalsRequired: 1, MergeMethod: "squash"}); err != nil {
		t.Fatal(err)
	}

	for _, body := range []string{
		`{"status":"stopped"}`,
		`{"provider":"copilot"}`,
		`{"approvalsRequired":0,"number":7}`,
	} {
		rec := call(t, h, http.MethodPatch, "/watches/1", body, nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"field_not_changeable"`) ||
			!strings.Contains(rec.Body.String(), "only approvalsRequired and mergeMethod can") {
			t.Fatalf("PATCH %s = %d %s", body, rec.Code, rec.Body)
		}
	}

	fw.mu.Lock()
	changes := fw.mergeRules
	fw.mu.Unlock()
	if len(changes) != 0 {
		t.Fatalf("changes = %+v, want none", changes)
	}
	w, err := st.GetWatch(context.Background(), 1)
	if err != nil || w.ApprovalsRequired != 1 || w.Status != store.WatchActive {
		t.Fatalf("watch = %+v, %v; want it as it was", w, err)
	}
}

func TestTheChangeableFieldsAreTheFieldsOfTheBody(t *testing.T) {
	t.Parallel()
	body := reflect.TypeFor[UpdateWatchRequest]()
	var names []string
	for f := range body.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		names = append(names, name)
	}
	if !slices.Equal(names, changeableWatchFields) {
		t.Fatalf("fields of UpdateWatchRequest = %v, changeableWatchFields = %v", names, changeableWatchFields)
	}
}
