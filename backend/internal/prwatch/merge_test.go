package prwatch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func TestMergeRefusesWhileSomethingBlocks(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.agentIdle(w)

	_, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{})
	if !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "no approval yet") {
		t.Fatalf("Merge() error = %v, want ErrNotReady naming the approval", err)
	}
	got := fx.watch(w)
	if got.Status != store.WatchActive || !slices.Contains(got.ReadyBlockers, "no approval yet") {
		t.Fatalf("watch = %s, blockers %v", got.Status, got.ReadyBlockers)
	}
	if n := len(fx.merges()); n != 0 {
		t.Fatalf("GitHub got %d merge calls", n)
	}
}

func TestMergeMergesAndStopsTheWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.agentIdle(w)
	fx.readyToMerge(w)

	got, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{})
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	if len(fx.merges()) != 1 || fx.merges()[0].Method != "squash" || fx.merges()[0].SHA != "abc" {
		t.Fatalf("merge calls = %v, want one squash of abc", fx.merges())
	}
	if got.Status != store.WatchStopped || got.StopReason != store.StopMerged {
		t.Fatalf("watch = %s (%s)", got.Status, got.StopReason)
	}
	var sum Summary
	if err := json.Unmarshal(got.Summary, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.PRState != store.StateMerged || sum.Detail != "squash" {
		t.Fatalf("summary = %+v", sum)
	}
	kinds := fx.kinds(w)
	if want := "watch_started,session_started,nudged,merge_ready,merged,watch_stopped"; strings.Join(kinds, ",") != want {
		t.Fatalf("kinds = %v, want %s", kinds, want)
	}
	rows := fx.activity(w)
	if merged := rows[4]; merged.Summary != "merged by babysitter (squash)" || merged.Actor != "alice" {
		t.Fatalf("merged row = %+v", merged)
	}
	if removed := fx.git.removedDirs(); len(removed) == 0 || removed[len(removed)-1] != got.WorktreeDir {
		t.Fatalf("removed = %v, want %s last", removed, got.WorktreeDir)
	}
	if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{}); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("second Merge() error = %v, want ErrWatchStopped", err)
	}
}

func TestMergeTakesTheMethodOfTheRequestThenTheWatchThenTheRepository(t *testing.T) {
	t.Parallel()
	t.Run("request", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		fx.good()
		req := fx.startRequest()
		req.MergeMethod = new("rebase")
		w, err := fx.svc.Start(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		fx.agentIdle(w)
		fx.readyToMerge(w)
		if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{Method: "merge"}); err != nil {
			t.Fatalf("Merge() error = %v", err)
		}
		if fx.merges()[0].Method != "merge" {
			t.Fatalf("merge calls = %v", fx.merges())
		}
	})
	t.Run("watch", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		fx.good()
		req := fx.startRequest()
		req.MergeMethod = new("rebase")
		w, err := fx.svc.Start(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		fx.agentIdle(w)
		fx.readyToMerge(w)
		if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{}); err != nil {
			t.Fatalf("Merge() error = %v", err)
		}
		if fx.merges()[0].Method != "rebase" {
			t.Fatalf("merge calls = %v", fx.merges())
		}
	})
	t.Run("repository", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		fx.good()
		fx.update(func() { fx.repo.MergeMethods = []string{"rebase"} })
		w := fx.start()
		fx.agentIdle(w)
		fx.readyToMerge(w)
		if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{}); err != nil {
			t.Fatalf("Merge() error = %v", err)
		}
		if fx.merges()[0].Method != "rebase" {
			t.Fatalf("merge calls = %v", fx.merges())
		}
	})
	t.Run("not allowed", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		fx.good()
		fx.update(func() { fx.repo.MergeMethods = []string{"rebase"} })
		w := fx.start()
		fx.agentIdle(w)
		fx.readyToMerge(w)
		_, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{Method: "squash"})
		if !errors.Is(err, ErrMergeRefused) || !strings.Contains(err.Error(), "only rebase") {
			t.Fatalf("Merge() error = %v, want ErrMergeRefused naming rebase", err)
		}
		if len(fx.merges()) != 0 || fx.watch(w).Status != store.WatchActive {
			t.Fatalf("merge calls = %v, watch %s", fx.merges(), fx.watch(w).Status)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		w := fx.start()
		if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{Method: "fast-forward"}); !errors.Is(err, ErrBadMergeMethod) {
			t.Fatalf("Merge() error = %v, want ErrBadMergeMethod", err)
		}
	})
}

func TestMergeRecordsARefusalAndKeepsTheWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	fx.update(func() { fx.pr.RefuseMerge = http.StatusMethodNotAllowed })
	w := fx.start()
	fx.agentIdle(w)
	fx.readyToMerge(w)

	_, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{})
	if !errors.Is(err, ErrMergeRefused) || !strings.Contains(err.Error(), "not mergeable") {
		t.Fatalf("Merge() error = %v, want ErrMergeRefused with the message of GitHub", err)
	}
	got := fx.watch(w)
	if got.Status != store.WatchActive {
		t.Fatalf("watch = %s, want active", got.Status)
	}
	rows := fx.activity(w)
	last := rows[len(rows)-1]
	if last.Kind != store.ActivityMergeFailed || !strings.HasPrefix(last.Summary, "GitHub refused the merge: ") {
		t.Fatalf("last row = %+v", last)
	}
	notes := fx.notes.messages()
	if len(notes) == 0 || !strings.Contains(notes[len(notes)-1], "refused the merge") {
		t.Fatalf("notifications = %v", notes)
	}
}

func TestMergeSeesWhatHappenedSinceTheLastPoll(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.agentIdle(w)
	fx.update(func() { fx.pr.Merged, fx.pr.State = true, "closed" })

	got, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{})
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	if got.Status != store.WatchStopped || got.StopReason != store.StopMerged || len(fx.merges()) != 0 {
		t.Fatalf("watch = %s (%s), merge calls %v", got.Status, got.StopReason, fx.merges())
	}
}

func TestMergeWaitsForTheReadinessToSettle(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)

	if since, blockers := fx.svc.Readiness(fx.watch(w), agent.StateIdle); since != nil || len(blockers) != 0 {
		t.Fatalf("after the first good poll: since = %v, blockers = %v", since, blockers)
	}
	_, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{})
	if !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "the readiness has not stood a whole interval yet") {
		t.Fatalf("Merge() error = %v, want ErrNotReady naming the readiness", err)
	}
	if n := len(fx.merges()); n != 0 {
		t.Fatalf("GitHub got %d merge calls before the readiness settled", n)
	}

	fx.advance(time.Minute)
	merged, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{})
	if err != nil || merged.Status != store.WatchStopped {
		t.Fatalf("Merge() once the readiness settled = %+v, %v", merged.Status, err)
	}
	if n := len(fx.merges()); n != 1 {
		t.Fatalf("GitHub got %d merge calls", n)
	}
}

func TestMergeCountsTheChecksOfTheSnapshotItCollected(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.good()
	w := fx.start()
	fx.agentIdle(w)
	fx.readyToMerge(w)

	fx.update(func() {
		fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Name: "build", Status: "completed", Conclusion: "success", CheckSuiteID: 501}, {ID: 2, Name: "lint", Status: "completed", Conclusion: "success", CheckSuiteID: 501}, {ID: 3, Name: "test", Status: "completed", Conclusion: "success", CheckSuiteID: 501}}
	})
	if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{}); err != nil {
		t.Fatalf("Merge() error = %v", err)
	}
	var ready store.Activity
	for _, a := range fx.activity(w) {
		if a.Kind == store.ActivityMergeReady {
			ready = a
		}
	}
	if !strings.Contains(ready.Summary, "3 checks green") {
		t.Fatalf("merge_ready summary = %q, want the three checks the merge collected", ready.Summary)
	}
}

func TestPollStopsAWatchWhoseMergedRowAnEarlierPassWrote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := newFixture(t)
	w := fx.start()
	fx.update(func() { fx.pr.Merged, fx.pr.State = true, "closed" })
	if _, _, err := fx.st.InsertActivity(ctx, store.Activity{
		WatchID: w.ID, Kind: store.ActivityMerged, Ref: "merged", At: fx.clock(), Summary: "merged",
	}); err != nil {
		t.Fatal(err)
	}

	fx.poll(w)
	if got := fx.watch(w); got.Status != store.WatchStopped || got.StopReason != store.StopMerged {
		t.Fatalf("watch = %s (%s), want it stopped as merged", got.Status, got.StopReason)
	}
}
