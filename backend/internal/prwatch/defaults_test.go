package prwatch

import (
	"cmp"
	"context"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

func ApprovalsFromBranch() Approvals { return Approvals{Set: true} }

func ApprovalsOf(n int) Approvals { return Approvals{Set: true, Count: &n} }

func (fx *fixture) settings(s store.Settings) {
	fx.t.Helper()
	s.PollInterval, s.WatchInterval = time.Minute, time.Minute
	s.ApprovalMode = cmp.Or(s.ApprovalMode, store.ApprovalAuto)
	if _, err := fx.st.SaveSettings(context.Background(), s); err != nil {
		fx.t.Fatal(err)
	}
}

func TestStartTakesTheDefaultsOfTheSettings(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	approvals := 3
	fx.settings(store.Settings{IncludeExisting: true, IncludeOwn: true, ApprovalsRequired: &approvals, MergeMethod: "rebase"})

	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !w.IncludeExisting || !w.IncludeOwn || w.MergeMethod != "rebase" || w.ApprovalsRequired != 3 {
		t.Fatalf("watch = %+v, want the defaults of the settings", w)
	}
}

func TestStartTakesTheRepositoryDefaultOverASettingThatNamesAMethod(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{MergeMethod: "rebase"})

	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir,
		MergeMethod: new(""),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.MergeMethod != "" {
		t.Fatalf("merge method = %q, want the repository default the request asked for", w.MergeMethod)
	}
}

func TestStartTakesTheRuleOfTheBranchOverASettingThatNamesANumber(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.repo.Approvals["main"] = 2
	approvals := 5
	fx.settings(store.Settings{ApprovalsRequired: &approvals})

	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir,
		ApprovalsRequired: ApprovalsFromBranch(),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.ApprovalsRequired != 2 {
		t.Fatalf("approvals = %d, want the 2 the base branch asks for", w.ApprovalsRequired)
	}
}

func TestStartKeepsWhatTheRequestAsksFor(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{IncludeExisting: true, IncludeOwn: true, MergeMethod: "rebase"})

	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir,
		IncludeExisting: new(false), IncludeOwn: new(false), ApprovalsRequired: ApprovalsOf(0), MergeMethod: new("squash"),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.IncludeExisting || w.IncludeOwn || w.MergeMethod != "squash" || w.ApprovalsRequired != 0 {
		t.Fatalf("watch = %+v, want what the request asked for", w)
	}
}

func TestEveryStopTakesTheWorktreeDefaultOfTheSettings(t *testing.T) {
	t.Parallel()
	cases := map[string]func(fx *fixture, w store.Watch){
		"the author stops the watch": func(fx *fixture, w store.Watch) {
			if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{}); err != nil {
				fx.t.Fatalf("Stop() error = %v", err)
			}
		},
		"the daemon merges the pull request": func(fx *fixture, w store.Watch) {
			fx.agentIdle(w)
			fx.readyToMerge(w)
			if _, err := fx.svc.Merge(context.Background(), w.ID, MergeOptions{}); err != nil {
				fx.t.Fatalf("Merge() error = %v", err)
			}
		},
		"the pull request is closed": func(fx *fixture, w store.Watch) {
			fx.update(func() { fx.pr.State = "closed" })
			fx.poll(w)
		},
	}
	for name, stop := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			fx.good()
			fx.settings(store.Settings{KeepWorktree: true})
			w := fx.start()
			before := len(fx.git.removedDirs())

			stop(fx, w)

			if got := fx.watch(w); got.Status != store.WatchStopped {
				t.Fatalf("watch = %s, want it stopped", got.Status)
			}
			if got := fx.git.removedDirs(); len(got) != before {
				t.Fatalf("the worktree was removed (%v), want it kept as the settings ask", got[before:])
			}
		})
	}
}

func TestAStopThatAsksForTheWorktreeBeatsTheSettings(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{KeepWorktree: true})
	w := fx.start()
	before := len(fx.git.removedDirs())

	if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{KeepWorktree: new(false)}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if got := fx.git.removedDirs(); len(got) != before+1 {
		t.Fatalf("removed = %v, want the worktree removed as the stop asked", got)
	}
}
