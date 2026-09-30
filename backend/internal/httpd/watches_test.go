package httpd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

type fakeWatches struct {
	mu          sync.Mutex
	st          *store.Store
	starts      []prwatch.StartRequest
	polls       []int64
	pollCtx     context.Context
	block       chan struct{}
	stops       []prwatch.StopOptions
	takeovers   []prwatch.TakeoverOptions
	takeoverErr error
	waits       []time.Duration
	merges      []prwatch.MergeOptions
	blockers    []string
	refused     bool
	nextBusy    bool
	now         time.Time
	interval    time.Duration
	state       agent.State
	sent        []string
	hooks       []string
	replies     []string
	sessions    int
	approval    approvalCalls
	mergeRules  []prwatch.MergeRulesChange
	sizes       []prwatch.TerminalSize

	authorRunning bool
	authorWork    *prwatch.WorkError
}

func (f *fakeWatches) Start(ctx context.Context, req prwatch.StartRequest) (store.Watch, error) {
	f.mu.Lock()
	f.starts = append(f.starts, req)
	f.mu.Unlock()
	if req.Target.Number == 99 {
		return store.Watch{}, prwatch.ErrNotOpen
	}
	if req.Target.Number == 98 {
		return store.Watch{}, prwatch.ErrNoCheckout
	}
	w, err := f.st.CreateWatch(ctx, store.Watch{
		Owner: req.Target.Owner, Name: req.Target.Name, Number: req.Target.Number,
		HeadRef: "fix", SourceDir: req.SourceDir, Provider: req.Provider, Model: req.Model,
		StartedAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
	})
	if errors.Is(err, store.ErrWatchExists) {
		existing, _ := f.st.FindActiveWatch(ctx, store.WatchKey{Owner: req.Target.Owner, Name: req.Target.Name, Number: req.Target.Number})
		return existing, err
	}
	return w, err
}

func (f *fakeWatches) Providers() []prwatch.Provider {
	out := prwatch.Catalog()
	out[0].Available = true
	return out
}

func (f *fakeWatches) Stop(ctx context.Context, id int64, o prwatch.StopOptions) (store.Watch, error) {
	f.mu.Lock()
	f.stops = append(f.stops, o)
	f.mu.Unlock()
	return f.st.StopWatch(ctx, id, store.StopUser, json.RawMessage(`{"prState":"open","headSha":"abc","checks":"green","activity":{},"messages":0,"reason":"user","worktreeRemoved":true}`), time.Now())
}

func (f *fakeWatches) Merge(ctx context.Context, id int64, o prwatch.MergeOptions) (store.Watch, error) {
	f.mu.Lock()
	f.merges = append(f.merges, o)
	blockers, refused := f.blockers, f.refused
	f.mu.Unlock()
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	switch {
	case w.Status != store.WatchActive:
		return store.Watch{}, prwatch.ErrWatchStopped
	case o.Method == "fast-forward":
		return store.Watch{}, prwatch.ErrBadMergeMethod
	case len(blockers) > 0:
		return store.Watch{}, fmt.Errorf("%w: %s", prwatch.ErrNotReady, strings.Join(blockers, "; "))
	case refused:
		return store.Watch{}, fmt.Errorf("%w: Pull Request is not mergeable", prwatch.ErrMergeRefused)
	}
	return f.st.StopWatch(ctx, id, store.StopMerged, json.RawMessage(`{"prState":"merged","headSha":"abc","checks":"green","activity":{},"messages":0,"reason":"merged","detail":"squash","worktreeRemoved":true}`), time.Now())
}

func (f *fakeWatches) Poll(ctx context.Context, id int64) error {
	f.mu.Lock()
	f.polls = append(f.polls, id)
	f.pollCtx = ctx
	block := f.block
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	return nil
}

func (f *fakeWatches) blockPolls() func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.block = make(chan struct{})
	block := f.block
	return sync.OnceFunc(func() { close(block) })
}

func newTestAPI(t *testing.T) (http.Handler, *store.Store, *fakeWatches) {
	t.Helper()
	return newTestAPIContext(t, context.Background())
}

func newTestAPIContext(t *testing.T, ctx context.Context) (http.Handler, *store.Store, *fakeWatches) {
	return newTestAPIWith(t, ctx, nil)
}

func newTestAPIWith(t *testing.T, ctx context.Context, tune func(*Deps)) (http.Handler, *store.Store, *fakeWatches) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "babysitter.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	bus := events.NewBus()
	st.SetPublisher(bus)
	fw := &fakeWatches{st: st, now: time.Now(), interval: time.Minute}
	deps := Deps{Context: ctx, Store: st, Bus: bus, Watches: fw, Log: testutil.Logger(t)}
	if tune != nil {
		tune(&deps)
	}
	return NewRouter(deps), st, fw
}

func call(t *testing.T, h http.Handler, method, path, body string, out any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, Prefix+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, Prefix+path, nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if out != nil && rec.Code < 300 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec
}

func TestStartWatchWithoutASourceDirLeavesTheCheckoutToTheService(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)

	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#4"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start without a source dir: %d %s", rec.Code, rec.Body)
	}
	if len(fw.starts) != 1 || fw.starts[0].SourceDir != "" {
		t.Fatalf("starts = %+v", fw.starts)
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#98"}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "watch_rejected") {
		t.Fatalf("start that needs a checkout: %d %s", rec.Code, rec.Body)
	}
}

func TestWatchRoutes(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	ctx := context.Background()

	var list WatchList
	if rec := call(t, h, http.MethodGet, "/watches", "", &list); rec.Code != http.StatusOK || len(list.Watches) != 0 {
		t.Fatalf("empty list: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches?status=bogus", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status: %d", rec.Code)
	}

	var w Watch
	rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","repo":"","sourceDir":"/src","includeExisting":true}`, &w)
	if rec.Code != http.StatusCreated || w.ID == 0 || w.Repo != "octo/hello" || w.Number != 3 || w.Status != "active" || w.CheckStates == nil {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	if len(fw.starts) != 1 || fw.starts[0].SourceDir != "/src" || fw.starts[0].IncludeExisting == nil || !*fw.starts[0].IncludeExisting {
		t.Fatalf("starts = %+v", fw.starts)
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","repo":"","sourceDir":"/src","includeExisting":false}`, nil); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "watch_exists") {
		t.Fatalf("duplicate start: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#99","repo":"","sourceDir":"/src","includeExisting":false}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "watch_rejected") {
		t.Fatalf("rejected start: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"nonsense","repo":"","sourceDir":"/src","includeExisting":false}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_target") {
		t.Fatalf("bad target: %d %s", rec.Code, rec.Body)
	}

	var got Watch
	if rec := call(t, h, http.MethodGet, "/watches/1", "", &got); rec.Code != http.StatusOK || got.ID != w.ID {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches/42", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: %d", rec.Code)
	}
	if rec := call(t, h, http.MethodGet, "/watches/x", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("get bad id: %d", rec.Code)
	}

	if _, _, err := st.InsertActivity(ctx, store.Activity{WatchID: w.ID, Kind: store.ActivityComment, Ref: "1", At: time.Now(), Actor: "bob", Summary: "hi", Payload: json.RawMessage(`{"body":"hi"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.InsertActivity(ctx, store.Activity{WatchID: w.ID, Kind: store.ActivityCheckFailed, Ref: "build@abc", At: time.Now(), Summary: "build failed"}); err != nil {
		t.Fatal(err)
	}
	var act ActivityList
	if rec := call(t, h, http.MethodGet, "/watches/1/activity", "", &act); rec.Code != http.StatusOK || len(act.Activity) != 2 || act.Activity[0].Payload["body"] != "hi" || act.Activity[1].Payload == nil {
		t.Fatalf("activity: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches/1/activity?since=1&limit=5", "", &act); rec.Code != http.StatusOK || len(act.Activity) != 1 || act.Activity[0].Kind != "check_failed" {
		t.Fatalf("activity since: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches/1/activity?since=x", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("activity bad since: %d", rec.Code)
	}
	if rec := call(t, h, http.MethodGet, "/watches/9/activity", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("activity missing watch: %d", rec.Code)
	}

	if rec := call(t, h, http.MethodPost, "/watches/1/poll", "", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("poll: %d %s", rec.Code, rec.Body)
	}
	testutil.Within(testutil.Timeout, func() bool { return len(fw.polled()) > 0 })
	if p := fw.polled(); len(p) != 1 || p[0] != 1 {
		t.Fatalf("polls = %v", fw.polled())
	}
	if rec := call(t, h, http.MethodPost, "/watches/9/poll", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("poll missing: %d", rec.Code)
	}

	var stopped Watch
	if rec := call(t, h, http.MethodPost, "/watches/1/stop", "", &stopped); rec.Code != http.StatusOK || stopped.Status != "stopped" || stopped.Summary == nil || stopped.Summary.Checks != "green" || !stopped.Summary.WorktreeRemoved {
		t.Fatalf("stop: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/stop", `{"keepWorktree":true}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("stop keeping the worktree: %d %s", rec.Code, rec.Body)
	}
	if got := fw.stopped(); len(got) != 2 || got[0].KeepWorktree != nil || got[1].KeepWorktree == nil || !*got[1].KeepWorktree {
		t.Fatalf("stops = %+v", got)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/stop", `{"bogus":true}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("stop with an unknown field: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/9/stop", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("stop missing: %d", rec.Code)
	}
	if rec := call(t, h, http.MethodGet, "/watches?status=all", "", &list); rec.Code != http.StatusOK || len(list.Watches) != 1 || list.Watches[0].Status != "stopped" {
		t.Fatalf("list all: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches", "", &list); rec.Code != http.StatusOK || len(list.Watches) != 0 {
		t.Fatalf("list active: %d %s", rec.Code, rec.Body)
	}
}

func TestWatchRoutesWithoutService(t *testing.T) {
	t.Parallel()
	st, err := store.Open(filepath.Join(t.TempDir(), "babysitter.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := NewRouter(Deps{Log: testutil.Logger(t), Store: st, Bus: events.NewBus()})
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","repo":"","sourceDir":"/src","includeExisting":false}`, nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("start without service: %d %s", rec.Code, rec.Body)
	}
}

func (f *fakeWatches) started() []prwatch.StartRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]prwatch.StartRequest(nil), f.starts...)
}

func (f *fakeWatches) stopped() []prwatch.StopOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]prwatch.StopOptions(nil), f.stops...)
}

func (f *fakeWatches) polled() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.polls...)
}

func (f *fakeWatches) Send(ctx context.Context, id int64, message string) (store.Activity, error) {
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return store.Activity{}, err
	}
	if w.Status != store.WatchActive {
		return store.Activity{}, prwatch.ErrWatchStopped
	}
	if w.TakenOverAt != nil {
		return store.Activity{}, prwatch.ErrTakenOver
	}
	if message == "busy" {
		return store.Activity{}, prwatch.ErrAgentBusy
	}
	if message == "pending" {
		return store.Activity{}, fmt.Errorf("%w: proposal 1", prwatch.ErrProposalPending)
	}
	if message == "updating" {
		return store.Activity{}, prwatch.ErrBranchUpdating
	}
	f.mu.Lock()
	f.sent = append(f.sent, message)
	f.mu.Unlock()
	row, _, err := f.st.InsertActivity(ctx, store.Activity{WatchID: id, Kind: store.ActivityNudged, Ref: message, At: time.Now(), Summary: "you told the agent: " + message})
	return row, err
}

func (f *fakeWatches) Next(ctx context.Context, id int64, wait time.Duration) (prwatch.NextMessage, error) {
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return prwatch.NextMessage{}, err
	}
	if w.Provider != prwatch.ProviderSelf {
		return prwatch.NextMessage{}, prwatch.ErrHostedWatch
	}
	if f.nextBusy {
		return prwatch.NextMessage{}, prwatch.ErrNextInFlight
	}
	f.mu.Lock()
	f.waits = append(f.waits, wait)
	first := len(f.waits) == 1
	f.mu.Unlock()
	if !first {
		return prwatch.NextMessage{Watch: w}, nil
	}
	row, _, err := f.st.InsertActivity(ctx, store.Activity{WatchID: id, Kind: store.ActivityNudged, Ref: "next", At: time.Now(), Summary: "told the agent about 1 comment", Payload: []byte(`{"message":"bob asks for a test"}`)})
	return prwatch.NextMessage{Watch: w, Message: &row}, err
}

func (f *fakeWatches) Reply(ctx context.Context, id int64, req prwatch.ReplyRequest) (prwatch.ReplyOutcome, error) {
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return prwatch.ReplyOutcome{}, err
	}
	if w.Status != store.WatchActive {
		return prwatch.ReplyOutcome{}, prwatch.ErrWatchStopped
	}
	if req.InReplyTo == 404 {
		return prwatch.ReplyOutcome{}, prwatch.ErrNoSuchComment
	}
	f.mu.Lock()
	f.replies = append(f.replies, fmt.Sprintf("%d:%s", req.InReplyTo, req.Body))
	f.mu.Unlock()
	if w.Provider != prwatch.ProviderSelf {
		return prwatch.ReplyOutcome{Proposal: 1}, nil
	}
	row, _, err := f.st.InsertActivity(ctx, store.Activity{WatchID: id, Kind: store.ActivityReplied, Ref: req.Body, At: time.Now(), Summary: "the agent commented: " + req.Body, URL: "https://c/40"})
	return prwatch.ReplyOutcome{Posted: &row}, err
}

func (f *fakeWatches) Retry(ctx context.Context, id int64, number int) (store.Proposal, error) {
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return store.Proposal{}, err
	}
	switch {
	case w.Provider == prwatch.ProviderSelf:
		return store.Proposal{}, prwatch.ErrSelfWatch
	case number == 2:
		return store.Proposal{}, prwatch.ErrNothingToRetry
	case number != 1:
		return store.Proposal{}, store.ErrProposalNotFound
	}
	return store.Proposal{WatchID: id, Number: 1, Status: store.ProposalReleased, HeadSHA: "abc", BaseSHA: "abc", WorkSHA: "w1", HasPush: true}, nil
}

func (f *fakeWatches) Output(ctx context.Context, id int64, lines int) (string, error) {
	if _, err := f.st.GetWatch(ctx, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("prompt ❯ (%d lines)\n", lines), nil
}

func (f *fakeWatches) View(ctx context.Context, id int64) (*snapshot.Snapshot, error) {
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if w.Number == 4 {
		return nil, prwatch.ErrNoSnapshot
	}
	return &snapshot.Snapshot{
		SnapshotAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC),
		PR: snapshot.PR{
			Repo: w.Repo(), Number: w.Number, Title: "Fix the thing", State: store.StateOpen, BaseBranch: "main", HeadBranch: "fix",
			Labels: []string{"bug"}, Body: "Fixes the retry loop.",
			Reviewers: []snapshot.Reviewer{{Login: "bob", State: "APPROVED"}},
		},
		Checks: snapshot.Checks{Status: "success", PassedCount: 2},
	}, nil
}

func (f *fakeWatches) Diff(ctx context.Context, id int64) (prwatch.PullDiff, error) {
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return prwatch.PullDiff{}, err
	}
	if w.Status == store.WatchStopped {
		return prwatch.PullDiff{}, prwatch.ErrWatchStopped
	}
	return prwatch.PullDiff{Base: "m1", Head: "abc", Diff: "diff --git a/x.go b/x.go\n"}, nil
}

func (f *fakeWatches) Resize(ctx context.Context, id int64, size prwatch.TerminalSize) error {
	if _, err := f.st.GetWatch(ctx, id); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizes = append(f.sizes, size)
	return nil
}

func (f *fakeWatches) resizes() []prwatch.TerminalSize {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]prwatch.TerminalSize(nil), f.sizes...)
}

func (f *fakeWatches) Session(_ context.Context, w store.Watch) (prwatch.SessionInfo, error) {
	f.mu.Lock()
	f.sessions++
	f.mu.Unlock()
	if w.Status != store.WatchActive {
		return prwatch.SessionInfo{State: agent.StateNone}, nil
	}
	f.mu.Lock()
	state := f.state
	f.mu.Unlock()
	if state == "" {
		state = agent.StateIdle
	}
	started := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	return prwatch.SessionInfo{State: state, PID: 4242, StartedAt: &started, AgentSession: w.AgentSession, LogPath: "/data/sessions/1.log"}, nil
}

func (f *fakeWatches) Readiness(w store.Watch, state agent.State) (*time.Time, []string) {
	f.mu.Lock()
	now, interval := f.now, f.interval
	f.mu.Unlock()
	return prwatch.Readiness(w, state, now, interval)
}

func (f *fakeWatches) Hook(_ context.Context, id int64, event string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hooks = append(f.hooks, fmt.Sprintf("%d %s %s", id, event, payload))
	return nil
}

func TestSessionRoutes(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	ctx := context.Background()
	w, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now(), AgentSession: "sess-1"})
	if err != nil {
		t.Fatal(err)
	}

	var got Watch
	if rec := call(t, h, http.MethodGet, "/watches/1", "", &got); rec.Code != http.StatusOK || got.Session.State != agent.StateIdle || got.Session.PID != 4242 || got.AgentSession != "sess-1" || got.Session.LogPath != "/data/sessions/1.log" {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}

	var row Activity
	if rec := call(t, h, http.MethodPost, "/watches/1/send", `{"message":"look at the tests"}`, &row); rec.Code != http.StatusCreated || row.Kind != store.ActivityNudged || row.Summary != "you told the agent: look at the tests" {
		t.Fatalf("send: %d %s", rec.Code, rec.Body)
	}
	if got := fw.sentMessages(); len(got) != 1 || got[0] != "look at the tests" {
		t.Fatalf("sent = %v", got)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/send", `{"message":""}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("send empty: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/send", `{"message":"busy"}`, nil); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "agent_busy") {
		t.Fatalf("send busy: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/9/send", `{"message":"hi"}`, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("send missing: %d %s", rec.Code, rec.Body)
	}

	var out SessionOutput
	if rec := call(t, h, http.MethodGet, "/watches/1/output?lines=5", "", &out); rec.Code != http.StatusOK || out.Output != "prompt ❯ (5 lines)\n" {
		t.Fatalf("output: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches/1/output", "", &out); rec.Code != http.StatusOK || out.Output != "prompt ❯ (200 lines)\n" {
		t.Fatalf("output default: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches/1/output?lines=x", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("output bad lines: %d", rec.Code)
	}
	if rec := call(t, h, http.MethodGet, "/watches/9/output", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("output missing: %d", rec.Code)
	}

	if rec := call(t, h, http.MethodPost, "/watches/1/hook", `{"event":"stop","payload":{"session_id":"s"}}`, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("hook: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/hook", `{"event":"bogus","payload":{}}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("hook bad event: %d %s", rec.Code, rec.Body)
	}
	if got := fw.hookEvents(); len(got) != 1 || got[0] != `1 stop {"session_id":"s"}` {
		t.Fatalf("hooks = %v", got)
	}

	if _, err := st.StopWatch(ctx, w.ID, store.StopUser, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/send", `{"message":"hi"}`, nil); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "watch_stopped") {
		t.Fatalf("send to a stopped watch: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches/1", "", &got); rec.Code != http.StatusOK || got.Session.State != agent.StateNone {
		t.Fatalf("get stopped: %d %s", rec.Code, rec.Body)
	}
}

func TestNextRoute(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	ctx := context.Background()
	if _, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now(), Provider: prwatch.ProviderSelf}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 4, StartedAt: time.Now(), Provider: prwatch.ProviderClaude}); err != nil {
		t.Fatal(err)
	}

	var got NextMessage
	if rec := call(t, h, http.MethodPost, "/watches/1/next?wait=5m", "", &got); rec.Code != http.StatusOK || got.Watch.ID != 1 || got.Message == nil || got.Message.Payload["message"] != "bob asks for a test" {
		t.Fatalf("next: %d %s", rec.Code, rec.Body)
	}
	got = NextMessage{}
	if rec := call(t, h, http.MethodPost, "/watches/1/next", "", &got); rec.Code != http.StatusOK || got.Watch.ID != 1 || got.Message != nil {
		t.Fatalf("next without a message: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/next?wait=2h", "", &got); rec.Code != http.StatusOK {
		t.Fatalf("next with a long wait: %d %s", rec.Code, rec.Body)
	}
	fw.mu.Lock()
	waits := append([]time.Duration(nil), fw.waits...)
	fw.mu.Unlock()
	if len(waits) != 3 || waits[0] != 5*time.Minute || waits[1] != 0 || waits[2] != MaxNextWait {
		t.Fatalf("a wait past the cap was not cut down to it: %v", waits)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/next?wait=soon", "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("next with a bad wait: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/2/next", "", nil); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "hosted_watch" {
		t.Fatalf("next on a hosted watch: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/9/next", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("next missing: %d %s", rec.Code, rec.Body)
	}
	fw.nextBusy = true
	if rec := call(t, h, http.MethodPost, "/watches/1/next", "", nil); rec.Code != http.StatusConflict || errorCode(t, rec) != "next_in_flight" {
		t.Fatalf("next with a caller already waiting: %d %s", rec.Code, rec.Body)
	}
}

func TestSessionRoutesWithoutService(t *testing.T) {
	t.Parallel()
	st, err := store.Open(filepath.Join(t.TempDir(), "babysitter.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.CreateWatch(context.Background(), store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	h := NewRouter(Deps{Log: testutil.Logger(t), Store: st, Bus: events.NewBus()})
	var got Watch
	if rec := call(t, h, http.MethodGet, "/watches/1", "", &got); rec.Code != http.StatusOK || got.Session.State != agent.StateNone {
		t.Fatalf("get without service: %d %s", rec.Code, rec.Body)
	}
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/watches/1/send", `{"message":"hi"}`},
		{http.MethodPost, "/watches/1/next", ""},
		{http.MethodGet, "/watches/1/output", ""},
		{http.MethodGet, "/watches/1/view", ""},
		{http.MethodGet, "/watches/1/diff", ""},
		{http.MethodPost, "/watches/1/hook", `{"event":"stop","payload":{}}`},
		{http.MethodPost, "/watches/1/resize", `{"rows":40,"cols":120}`},
	} {
		if rec := call(t, h, c.method, c.path, c.body, nil); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s %s without service: %d %s", c.method, c.path, rec.Code, rec.Body)
		}
	}
}

func TestViewAndDiffRoutes(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	for _, w := range []store.Watch{
		{Owner: "octo", Name: "hello", Number: 3, Provider: prwatch.ProviderClaude, StartedAt: time.Now()},
		{Owner: "octo", Name: "hello", Number: 4, Provider: prwatch.ProviderSelf, StartedAt: time.Now()},
	} {
		if _, err := st.CreateWatch(context.Background(), w); err != nil {
			t.Fatal(err)
		}
	}

	var view PullRequestView
	if rec := call(t, h, http.MethodGet, "/watches/1/view", "", &view); rec.Code != http.StatusOK {
		t.Fatalf("view: %d %s", rec.Code, rec.Body)
	}
	if view.Repo != "octo/hello" || strings.Join(view.Labels, ",") != "bug" || view.Body != "Fixes the retry loop." || view.Checks.Passed != 2 {
		t.Fatalf("view = %+v", view)
	}
	if len(view.Reviewers) != 1 || view.Reviewers[0] != (PullRequestReview{Login: "bob", State: "APPROVED"}) || view.Assignees == nil {
		t.Fatalf("view reviewers %+v, assignees %v: want bob and an empty list, not null", view.Reviewers, view.Assignees)
	}
	if rec := call(t, h, http.MethodGet, "/watches/2/view", "", nil); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "no_snapshot") {
		t.Fatalf("view before a poll: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches/9/view", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("view missing: %d", rec.Code)
	}

	var diff PullRequestDiff
	if rec := call(t, h, http.MethodGet, "/watches/1/diff", "", &diff); rec.Code != http.StatusOK || diff != (PullRequestDiff{Base: "m1", Head: "abc", Diff: "diff --git a/x.go b/x.go\n"}) {
		t.Fatalf("diff: %d %s", rec.Code, rec.Body)
	}
	if _, err := st.StopWatch(context.Background(), 2, store.StopUser, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if rec := call(t, h, http.MethodGet, "/watches/2/diff", "", nil); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "watch_stopped") {
		t.Fatalf("diff of a stopped watch: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodGet, "/watches/9/diff", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("diff missing: %d", rec.Code)
	}
}

func TestResizeRoute(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	if _, err := st.CreateWatch(context.Background(), store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if rec := call(t, h, http.MethodPost, "/watches/1/resize", `{"rows":52,"cols":214}`, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("resize: %d %s", rec.Code, rec.Body)
	}
	if got := fw.resizes(); len(got) != 1 || got[0] != (prwatch.TerminalSize{Rows: 52, Cols: 214}) {
		t.Fatalf("sizes = %+v", got)
	}

	for _, body := range []string{
		`{"rows":0,"cols":120}`,
		`{"rows":40,"cols":0}`,
		`{"rows":40,"cols":1001}`,
		`{"rows":501,"cols":120}`,
		`{"rows":-1,"cols":120}`,
		`{"rows":40,"cols":70000}`,
		`{"rows":40}`,
		`not json`,
	} {
		if rec := call(t, h, http.MethodPost, "/watches/1/resize", body, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("resize %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := call(t, h, http.MethodPost, "/watches/9/resize", `{"rows":40,"cols":120}`, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("resize missing: %d %s", rec.Code, rec.Body)
	}
	if got := fw.resizes(); len(got) != 1 {
		t.Fatalf("a refused size reached the service: %+v", got)
	}
}

func (f *fakeWatches) sentMessages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

func (f *fakeWatches) hookEvents() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hooks...)
}

func TestPollRequestsFoldIntoOneQueuedPoll(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)
	release := fw.blockPolls()
	defer release()

	var w Watch
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#7","repo":"","sourceDir":"/src","includeExisting":false}`, &w); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	path := fmt.Sprintf("/watches/%d/poll", w.ID)
	if rec := call(t, h, http.MethodPost, path, "", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("first poll: %d %s", rec.Code, rec.Body)
	}
	waitForPolls(t, fw, 1)

	for i := range 4 {
		if rec := call(t, h, http.MethodPost, path, "", nil); rec.Code != http.StatusAccepted {
			t.Fatalf("poll %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	if n := settledPolls(fw, 1); n != 1 {
		t.Fatalf("%d polls ran while one was in flight, want 1", n)
	}

	release()
	waitForPolls(t, fw, 2)
	if n := settledPolls(fw, 2); n != 2 {
		t.Fatalf("%d polls ran in total, want the running one plus the folded one", n)
	}
	if got := fw.polled(); got[0] != w.ID || got[1] != w.ID {
		t.Fatalf("polls = %v", got)
	}
}

func (f *fakeWatches) lastPollContext() context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pollCtx
}

func TestAPollStopsWhenTheDaemonShutsDown(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, _, fw := newTestAPIContext(t, ctx)
	release := fw.blockPolls()
	defer release()

	var w Watch
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#8","repo":"","sourceDir":"/src","includeExisting":false}`, &w); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, fmt.Sprintf("/watches/%d/poll", w.ID), "", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("poll: %d %s", rec.Code, rec.Body)
	}
	waitForPolls(t, fw, 1)

	pollCtx := fw.lastPollContext()
	if pollCtx.Err() != nil {
		t.Fatalf("the poll started under a context that is already done: %v", pollCtx.Err())
	}
	cancel()
	select {
	case <-pollCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the daemon shut down and the poll kept its context")
	}
}

func waitForPolls(t *testing.T, fw *fakeWatches, n int) {
	t.Helper()
	if testutil.Within(testutil.Timeout, func() bool { return len(fw.polled()) >= n }) {
		return
	}
	t.Fatalf("%d polls ran, want %d", len(fw.polled()), n)
}

func settledPolls(fw *fakeWatches, want int) int {
	testutil.Within(200*time.Millisecond, func() bool { return len(fw.polled()) > want })
	return len(fw.polled())
}

func TestAnOversizedBodyClosesTheConnection(t *testing.T) {
	t.Parallel()
	h, _, _ := newTestAPI(t)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	body := `{"message":"` + strings.Repeat("a", 1<<20) + `"}`
	req, err := http.NewRequest(http.MethodPost, srv.URL+Prefix+"/watches/1/send", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if !resp.Close {
		t.Fatalf("the server kept the connection open after a body over the limit: Connection=%q", resp.Header.Get("Connection"))
	}
}

func TestStartWatchRejectsNegativeApprovals(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src","approvalsRequired":-1}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "approvalsRequired must be 0 or more") {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src"}`, nil); rec.Code != http.StatusCreated || len(fw.starts) != 1 || fw.starts[0].ApprovalsRequired.Set {
		t.Fatalf("start without a count: %d %s, starts %+v", rec.Code, rec.Body, fw.starts)
	}
}

func TestMergeWatch(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src","approvalsRequired":2,"mergeMethod":"rebase"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	if got := fw.starts[0]; got.ApprovalsRequired.Count == nil || *got.ApprovalsRequired.Count != 2 || got.MergeMethod == nil || *got.MergeMethod != "rebase" {
		t.Fatalf("start request = %+v", got)
	}
	var w Watch
	if rec := call(t, h, http.MethodGet, "/watches/1", "", &w); rec.Code != http.StatusOK || w.ReadySince != nil || w.ReadyBlockers == nil || len(w.ReadyBlockers) != 0 {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}

	fw.mu.Lock()
	fw.blockers = []string{"no approval yet", "2 checks pending"}
	fw.mu.Unlock()
	if rec := call(t, h, http.MethodPost, "/watches/1/merge", "", nil); rec.Code != http.StatusConflict || errorCode(t, rec) != "not_ready" || !strings.Contains(rec.Body.String(), "no approval yet; 2 checks pending") {
		t.Fatalf("merge while blocked: %d %s", rec.Code, rec.Body)
	}
	fw.mu.Lock()
	fw.blockers, fw.refused = nil, true
	fw.mu.Unlock()
	if rec := call(t, h, http.MethodPost, "/watches/1/merge", "", nil); rec.Code != http.StatusUnprocessableEntity || errorCode(t, rec) != "merge_refused" {
		t.Fatalf("merge refused: %d %s", rec.Code, rec.Body)
	}
	fw.mu.Lock()
	fw.refused = false
	fw.mu.Unlock()
	if rec := call(t, h, http.MethodPost, "/watches/1/merge", `{"method":"fast-forward"}`, nil); rec.Code != http.StatusBadRequest || errorCode(t, rec) != "bad_merge_method" {
		t.Fatalf("merge with a bad method: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/merge", `{"bogus":true}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("merge with an unknown field: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/9/merge", "", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("merge missing: %d", rec.Code)
	}

	var merged Watch
	if rec := call(t, h, http.MethodPost, "/watches/1/merge", `{"method":"squash"}`, &merged); rec.Code != http.StatusOK || merged.Status != "stopped" || merged.StopReason != store.StopMerged || merged.Summary == nil || merged.Summary.Detail != "squash" {
		t.Fatalf("merge: %d %s", rec.Code, rec.Body)
	}
	if n := len(fw.merges); n != 5 || fw.merges[n-1].Method != "squash" {
		t.Fatalf("merges = %+v", fw.merges)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/merge", "", nil); rec.Code != http.StatusConflict || errorCode(t, rec) != "watch_stopped" {
		t.Fatalf("merge a stopped watch: %d %s", rec.Code, rec.Body)
	}
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var apiErr APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode error %q: %v", rec.Body.String(), err)
	}
	return apiErr.Error.Code
}

func TestAWatchReadsTheAgentStateIntoItsReadiness(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	since := time.Date(2026, 9, 7, 12, 6, 0, 0, time.UTC)
	if err := st.SetWatchReadiness(context.Background(), 1, &since, nil); err != nil {
		t.Fatal(err)
	}
	var w Watch
	if rec := call(t, h, http.MethodGet, "/watches/1", "", &w); rec.Code != http.StatusOK || w.ReadySince == nil || len(w.ReadyBlockers) != 0 {
		t.Fatalf("idle: %d %s", rec.Code, rec.Body)
	}
	fw.mu.Lock()
	fw.state = agent.StateActive
	fw.mu.Unlock()
	var busy Watch
	if rec := call(t, h, http.MethodGet, "/watches/1", "", &busy); rec.Code != http.StatusOK || busy.ReadySince != nil || strings.Join(busy.ReadyBlockers, "; ") != "the agent is still working" {
		t.Fatalf("active: %d %s", rec.Code, rec.Body)
	}
}

func TestNextHidesAReadinessThatHasNotStoodAnInterval(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src","provider":"self"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	since := time.Date(2026, 9, 7, 12, 6, 0, 0, time.UTC)
	if err := st.SetWatchReadiness(context.Background(), 1, &since, nil); err != nil {
		t.Fatal(err)
	}
	fw.mu.Lock()
	fw.now, fw.interval = since.Add(30*time.Second), time.Minute
	fw.mu.Unlock()

	var early NextMessage
	if rec := call(t, h, http.MethodPost, "/watches/1/next", "", &early); rec.Code != http.StatusOK || early.Watch.ReadySince != nil {
		t.Fatalf("half an interval in: %d %s", rec.Code, rec.Body)
	}

	fw.mu.Lock()
	fw.now = since.Add(time.Minute)
	fw.mu.Unlock()
	var settled NextMessage
	if rec := call(t, h, http.MethodPost, "/watches/1/next", "", &settled); rec.Code != http.StatusOK || settled.Watch.ReadySince == nil {
		t.Fatalf("a whole interval in: %d %s", rec.Code, rec.Body)
	}
}

func TestReplyWatch(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	ctx := context.Background()
	if _, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	var out ReplyResult
	if rec := call(t, h, http.MethodPost, "/watches/1/reply", `{"inReplyTo":31,"body":"done, see 1a2b3c"}`, &out); rec.Code != http.StatusCreated || out.Posted != nil || out.Proposal != 1 {
		t.Fatalf("reply: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/reply", `{"body":"on the pull request"}`, &out); rec.Code != http.StatusCreated {
		t.Fatalf("comment: %d %s", rec.Code, rec.Body)
	}
	if got := fw.replies; len(got) != 2 || got[0] != "31:done, see 1a2b3c" || got[1] != "0:on the pull request" {
		t.Fatalf("replies = %v", got)
	}
	self, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 4, Provider: prwatch.ProviderSelf, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	out = ReplyResult{}
	if rec := call(t, h, http.MethodPost, fmt.Sprintf("/watches/%d/reply", self.ID), `{"body":"posted at once"}`, &out); rec.Code != http.StatusCreated ||
		out.Posted == nil || out.Posted.Kind != store.ActivityReplied || out.Posted.URL != "https://c/40" || out.Proposal != 0 {
		t.Fatalf("self reply: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/reply", `{"inReplyTo":31}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("reply without a body: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/reply", `{"inReplyTo":404,"body":"hi"}`, nil); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "no_such_comment") {
		t.Fatalf("reply to a missing comment: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/9/reply", `{"body":"hi"}`, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("reply on a missing watch: %d %s", rec.Code, rec.Body)
	}
}

func TestRetryProposal(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	ctx := context.Background()
	if _, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	self, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 4, Provider: prwatch.ProviderSelf, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var p Proposal
	if rec := call(t, h, http.MethodPost, "/watches/1/proposals/1/retry", "", &p); rec.Code != http.StatusOK ||
		p.Number != 1 || p.Status != store.ProposalReleased || p.WorkSHA != "w1" || !p.HasPush {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body)
	}
	for _, tc := range []struct {
		path string
		code int
		word string
	}{
		{"/watches/1/proposals/2/retry", http.StatusConflict, "nothing_to_retry"},
		{"/watches/1/proposals/7/retry", http.StatusNotFound, "proposal_not_found"},
		{"/watches/9/proposals/1/retry", http.StatusNotFound, "watch_not_found"},
		{fmt.Sprintf("/watches/%d/proposals/1/retry", self.ID), http.StatusBadRequest, "self_watch"},
		{"/watches/1/proposals/x/retry", http.StatusBadRequest, "bad_request"},
	} {
		if rec := call(t, h, http.MethodPost, tc.path, "", nil); rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.word) {
			t.Errorf("%s: %d %s, want %d %s", tc.path, rec.Code, rec.Body, tc.code, tc.word)
		}
	}
}
