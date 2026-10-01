package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
)

var approvalFromBob = ghfake.Review{
	ID: 6, State: "APPROVED", Author: "bob", Body: "LGTM", SubmittedAt: ghfake.At("2026-09-02T00:00:00Z"),
	URL: "https://github.com/octo/hello/pull/3#pullrequestreview-6",
}

func (fx *fixture) good() {
	fx.update(func() { fx.pr.Reviews = []ghfake.Review{approvalFromBob} })
}

func (fx *fixture) agentIdle(w store.Watch) {
	fx.t.Helper()
	if _, err := fx.svc.Hook(context.Background(), w.ID, agent.EventStop, []byte(`{}`)); err != nil {
		fx.t.Fatalf("Hook(stop) error = %v", err)
	}
}

func (fx *fixture) readyToMerge(w store.Watch) {
	fx.t.Helper()
	fx.poll(w)
	fx.advance(time.Minute)
}

func (fx *fixture) watch(w store.Watch) store.Watch {
	fx.t.Helper()
	got, err := fx.st.GetWatch(context.Background(), w.ID)
	if err != nil {
		fx.t.Fatal(err)
	}
	return got
}

func TestAGoodPullRequestIsReadyAfterAWholeInterval(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	got := fx.watch(w)
	if got.ReadySince == nil || len(got.ReadyBlockers) != 0 {
		t.Fatalf("after the first good poll: since = %v, blockers = %v", got.ReadySince, got.ReadyBlockers)
	}
	if kinds := fx.kinds(w); slices.Contains(kinds, "merge_ready") {
		t.Fatalf("the first good poll already said ready: %v", kinds)
	}
	since := *got.ReadySince

	fx.poll(w)
	got = fx.watch(w)
	if got.ReadySince == nil || !got.ReadySince.Equal(since) {
		t.Fatalf("the second poll moved the clock: %v, want %v", got.ReadySince, since)
	}
	rows := fx.activity(w)
	last := rows[len(rows)-1]
	if last.Kind != store.ActivityMergeReady || last.Ref != "ready@abc" {
		t.Fatalf("last row = %+v, want merge_ready for abc", last)
	}
	if want := "ready to merge: 1 approval, 1 check green; merge from the app or `babysitter watch merge 1`"; last.Summary != want {
		t.Fatalf("summary = %q, want %q", last.Summary, want)
	}
	notes := fx.notes.messages()
	if len(notes) == 0 || !strings.Contains(notes[len(notes)-1], "ready to merge") {
		t.Fatalf("notifications = %v, want the readiness last", notes)
	}

	fx.poll(w)
	if n := countKind(fx.activity(w), store.ActivityMergeReady); n != 1 {
		t.Fatalf("merge_ready rows = %d, want one per head", n)
	}
}

func TestANewHeadStartsTheReadinessClockAgain(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)
	fx.poll(w)
	if n := countKind(fx.activity(w), store.ActivityMergeReady); n != 1 {
		t.Fatalf("merge_ready rows = %d, want 1", n)
	}
	before := *fx.watch(w).ReadySince

	fx.update(func() { fx.pr.HeadSHA = "def" })
	fx.poll(w)
	got := fx.watch(w)
	if got.ReadySince == nil || !got.ReadySince.After(before) {
		t.Fatalf("since = %v, want later than %v", got.ReadySince, before)
	}
	if n := countKind(fx.activity(w), store.ActivityMergeReady); n != 1 {
		t.Fatalf("the new head was called ready at once: %d rows", n)
	}
	fx.poll(w)
	rows := fx.activity(w)
	last := rows[len(rows)-1]
	if last.Kind != store.ActivityMergeReady || last.Ref != "ready@def" {
		t.Fatalf("last row = %+v, want merge_ready for def", last)
	}
}

func TestEveryBlockerKeepsThePullRequestFromReadiness(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		set  func(fx *fixture, w store.Watch)
		want string
	}{
		{"no approval", func(fx *fixture, _ store.Watch) {
			fx.update(func() { fx.pr.Reviews = nil })
		}, "no approval yet"},
		{"changes requested", func(fx *fixture, _ store.Watch) {
			fx.update(func() {
				fx.pr.Reviews = append(fx.pr.Reviews, ghfake.Review{ID: 7, State: "CHANGES_REQUESTED", Author: "carol", SubmittedAt: ghfake.At("2026-09-03T00:00:00Z"), Body: "no"})
			})
		}, "1 reviewer requested changes"},
		{"reviewer asked", func(fx *fixture, _ store.Watch) {
			fx.update(func() { fx.pr.Requested = []string{"dave"} })
		}, "waiting for a review from dave"},
		{"draft", func(fx *fixture, _ store.Watch) {
			fx.update(func() { fx.pr.Draft = true })
		}, "still a draft"},
		{"unresolved thread", func(fx *fixture, _ store.Watch) {
			fx.update(func() { fx.pr.Threads = threads(2, 0) })
		}, "2 review threads unresolved"},
		{"thread of the token answered by a reviewer", func(fx *fixture, _ store.Watch) {
			fx.update(func() { fx.pr.Threads = threads(1, 0, "alice", "bob") })
		}, "1 review thread unresolved"},
		{"threads unreadable", func(fx *fixture, _ store.Watch) {
			fx.api.GraphQLError("Resource not accessible by integration")
		}, "review threads unreadable: review threads octo/hello#3: Resource not accessible by integration"},
		{"behind", func(fx *fixture, _ store.Watch) {
			fx.update(func() { fx.pr.MergeableState = "behind" })
		}, "GitHub reports behind"},
		{"check failed", func(fx *fixture, _ store.Watch) {
			fx.failBuild("")
		}, "1 check failed"},
		{"agent working", func(fx *fixture, w store.Watch) {
			if _, err := fx.svc.Hook(context.Background(), w.ID, agent.EventUserPromptSubmit, []byte(`{}`)); err != nil {
				fx.t.Fatal(err)
			}
		}, "the agent is still working"},
		{"agent waits on a permission", func(fx *fixture, w store.Watch) {
			if _, err := fx.svc.Hook(context.Background(), w.ID, agent.EventPermissionRequest, []byte(`{"tool_name":"Bash"}`)); err != nil {
				fx.t.Fatal(err)
			}
		}, "the agent waits on a permission decision"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			fx.good()
			w := fx.start()
			fx.agentIdle(w)
			c.set(fx, w)
			fx.poll(w)
			got := fx.watch(w)
			info, err := fx.svc.Session(context.Background(), got)
			if err != nil {
				t.Fatal(err)
			}
			since, blockers := fx.svc.Readiness(got, info.State)
			if got.ReadySince != nil || since != nil {
				t.Fatalf("since = %v, %v, want nil", got.ReadySince, since)
			}
			if !slices.Contains(blockers, c.want) {
				t.Fatalf("blockers = %v, want %q among them", blockers, c.want)
			}
		})
	}
}

func TestABlockerCarriesNoTokenOfTheErrorBehindIt(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	token := "ghp_" + strings.Repeat("a", 36)
	fx.api.GraphQLError("Bad credentials for " + token)
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)
	got := fx.watch(w)
	if len(got.ReadyBlockers) == 0 {
		t.Fatal("the unreadable threads blocked nothing")
	}
	for _, blocker := range got.ReadyBlockers {
		if strings.Contains(blocker, token) {
			t.Fatalf("the blocker carries the token: %q", blocker)
		}
	}
	if !slices.Contains(got.ReadyBlockers, "review threads unreadable: review threads octo/hello#3: Bad credentials for [redacted]") {
		t.Fatalf("blockers = %v", got.ReadyBlockers)
	}
}

func TestAMessageTheAgentWasNotToldBlocksReadiness(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	if _, err := fx.svc.Hook(context.Background(), w.ID, agent.EventPermissionRequest, []byte(`{"tool_name":"Bash"}`)); err != nil {
		t.Fatal(err)
	}
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 31, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:30:00Z"), Body: "please rename x", URL: "https://github.com/octo/hello/pull/3#issuecomment-31"}}
	})
	fx.poll(w)
	got := fx.watch(w)
	if got.ReadySince != nil {
		t.Fatalf("since = %v, want nil", got.ReadySince)
	}
	if !slices.Contains(got.ReadyBlockers, "the agent was not told about 1 comment yet") {
		t.Fatalf("blockers = %v", got.ReadyBlockers)
	}
}

func TestTheBlockerNamesWhatTheMessageOfThePollLeftUntold(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	ctx := context.Background()
	if _, err := fx.svc.Hook(ctx, w.ID, agent.EventPermissionRequest, []byte(`{"tool_name":"Bash"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.st.InsertActivity(ctx, store.Activity{
		WatchID: w.ID, Kind: store.ActivityCheckFailed, Ref: "build@old", At: fx.clock(),
		Summary: "build failed on old", Payload: json.RawMessage(`{"check":"build","sha":"old"}`),
	}); err != nil {
		t.Fatal(err)
	}
	fx.update(func() {
		fx.pr.IssueComments = []ghfake.Comment{{ID: 31, Author: "bob", CreatedAt: ghfake.At("2026-09-07T12:30:00Z"), Body: "please rename x", URL: "https://c/31"}}
	})

	fx.poll(w)

	got := fx.watch(w)
	if !slices.Contains(got.ReadyBlockers, "the agent was not told about 1 comment yet") {
		t.Fatalf("blockers = %v, want the comment alone", got.ReadyBlockers)
	}
	todo, err := fx.st.UnnudgedActionable(ctx, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(todo) != 1 || todo[0].Kind != store.ActivityComment {
		t.Fatalf("unnudged = %+v, want the comment alone", todo)
	}
}

func TestTheBaseBranchRuleSetsTheApprovals(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() { fx.repo.Approvals["main"] = 0 })
	w := fx.start()
	if w.ApprovalsRequired != 0 {
		t.Fatalf("watch asks for %d approvals", w.ApprovalsRequired)
	}
	fx.agentIdle(w)
	fx.poll(w)
	if got := fx.watch(w); got.ReadySince == nil || len(got.ReadyBlockers) != 0 {
		t.Fatalf("watch = since %v, blockers %v", got.ReadySince, got.ReadyBlockers)
	}

	fx.update(func() { fx.repo.Approvals["main"] = 2 })
	fx.openPR(4)
	second := fx.startRequest()
	second.Target.Number = 4
	if w, err := fx.svc.Start(context.Background(), second); err != nil || w.ApprovalsRequired != 2 {
		t.Fatalf("watch = %+v, %v", w, err)
	}

	fx.openPR(5)
	third := fx.startRequest()
	third.Target.Number, third.ApprovalsRequired = 5, ApprovalsOf(0)
	if w, err := fx.svc.Start(context.Background(), third); err != nil || w.ApprovalsRequired != 0 {
		t.Fatalf("watch = %+v, %v", w, err)
	}
}

func TestTheStartRequestSetsApprovalsAndMethod(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	req := fx.startRequest()
	req.ApprovalsRequired, req.MergeMethod = ApprovalsOf(2), new("Rebase")
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.ApprovalsRequired != 2 || w.MergeMethod != "rebase" {
		t.Fatalf("watch = approvals %d, method %q", w.ApprovalsRequired, w.MergeMethod)
	}
	fx.agentIdle(w)
	fx.poll(w)
	if got := fx.watch(w); got.ReadySince != nil || !slices.Contains(got.ReadyBlockers, "1 of 2 approvals") {
		t.Fatalf("watch = since %v, blockers %v", got.ReadySince, got.ReadyBlockers)
	}

	req.MergeMethod = new("fast-forward")
	if _, err := fx.svc.Start(context.Background(), req); !errorsIs(err, ErrBadMergeMethod) {
		t.Fatalf("Start() error = %v, want ErrBadMergeMethod", err)
	}
}

func countKind(rows []store.Activity, kind store.ActivityKind) int {
	n := 0
	for _, a := range rows {
		if a.Kind == kind {
			n++
		}
	}
	return n
}

func TestReadinessFollowsTheAgentBetweenPolls(t *testing.T) {
	t.Parallel()
	since := time.Date(2026, 9, 7, 12, 6, 0, 0, time.UTC)
	settledAt := since.Add(time.Minute)
	ready := store.Watch{ReadySince: &since, ReadyBlockers: []string{}}
	if got, blockers := Readiness(ready, agent.StateIdle, settledAt, time.Minute); got != &since || len(blockers) != 0 {
		t.Fatalf("idle: since = %v, blockers = %v", got, blockers)
	}
	if got, blockers := Readiness(ready, agent.StateActive, settledAt, time.Minute); got != nil || strings.Join(blockers, "; ") != "the agent is still working" {
		t.Fatalf("active: since = %v, blockers = %v", got, blockers)
	}
	blocked := store.Watch{ReadyBlockers: []string{"no approval yet"}}
	if got, blockers := Readiness(blocked, agent.StateWaitingInput, settledAt, time.Minute); got != nil || strings.Join(blockers, "; ") != "no approval yet; the agent asks you a question" {
		t.Fatalf("blocked: since = %v, blockers = %v", got, blockers)
	}
	if _, blockers := Readiness(store.Watch{}, agent.StateNone, settledAt, time.Minute); blockers == nil || len(blockers) != 0 {
		t.Fatalf("none: blockers = %#v, want an empty list", blockers)
	}
}

func TestWorkThatDidNotGoOutBlocksTheMerge(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.rel.commit("abc", "w1")
	fx.rel.set(func(f *fakeRelease) {
		f.pushErr = errors.New("could not read Username for 'https://github.com': terminal prompts disabled")
	})
	fx.hook(w, agent.EventStop, `{}`)
	fx.readyToMerge(w)

	_, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{})
	if !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "proposal 1") {
		t.Fatalf("Merge() error = %v, want ErrNotReady naming proposal 1", err)
	}
	if got := fx.watch(w); got.Status != store.WatchActive || len(fx.merges()) != 0 {
		t.Fatalf("watch = %s, merges %v", got.Status, fx.merges())
	}

	fx.rel.set(func(f *fakeRelease) { f.pushErr = nil })
	if _, err := fx.svc.Retry(context.Background(), w.ID, 1); err != nil {
		t.Fatal(err)
	}
	fx.readyToMerge(w)
	if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{}); err != nil {
		t.Fatalf("Merge() after the retry error = %v", err)
	}
}
