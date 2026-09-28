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
	s.Provider = cmp.Or(s.Provider, ProviderClaude)
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

func (fx *fixture) repoOverrides(o store.WatchOverrides) {
	fx.t.Helper()
	ctx := context.Background()
	repo, err := fx.st.AddRepo(ctx, "octo", "hello")
	if err != nil {
		fx.t.Fatal(err)
	}
	cfg := store.DefaultRepoConfig(repo.ID)
	cfg.Overrides = o
	if _, err := fx.st.SaveRepoConfig(ctx, cfg); err != nil {
		fx.t.Fatal(err)
	}
}

func TestStartTakesTheRepositoryOverTheSettings(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	approvals := 3
	fx.settings(store.Settings{
		ApprovalsRequired: &approvals, MergeMethod: "rebase", ApprovalMode: store.ApprovalManual,
		IncludeExisting: true, IncludeOwn: true, KeepWorktree: true, AutoApproveRebase: true,
	})
	fx.repoOverrides(store.WatchOverrides{
		Provider: ProviderCopilot, Model: "auto", MergeMethod: "squash", ApprovalMode: store.ApprovalAuto,
		ApprovalsSet: true, Approvals: new(1),
		IncludeExisting: new(false), IncludeOwn: new(false), KeepWorktree: new(false), AutoApproveRebase: new(false),
	})

	w := fx.start()

	if w.Provider != ProviderCopilot || w.Model != "auto" || w.MergeMethod != "squash" || w.ApprovalMode != store.ApprovalAuto ||
		w.ApprovalsRequired != 1 || w.IncludeExisting || w.IncludeOwn || w.KeepWorktree || w.AutoApproveRebase {
		t.Fatalf("watch = %+v, want what the repository says", w)
	}
}

func TestStartTakesTheSettingsWhereTheRepositoryIsSilent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	approvals := 3
	fx.settings(store.Settings{
		Provider: ProviderCopilot, Model: "auto", ApprovalsRequired: &approvals, MergeMethod: "rebase",
		ApprovalMode: store.ApprovalManual, IncludeExisting: true, KeepWorktree: true, AutoApproveRebase: true,
	})
	fx.repoOverrides(store.WatchOverrides{IncludeOwn: new(true)})

	w := fx.start()

	if w.Provider != ProviderCopilot || w.Model != "auto" || w.MergeMethod != "rebase" || w.ApprovalMode != store.ApprovalManual ||
		w.ApprovalsRequired != 3 || !w.IncludeExisting || !w.IncludeOwn || !w.KeepWorktree || !w.AutoApproveRebase {
		t.Fatalf("watch = %+v, want the settings with the one field the repository sets", w)
	}
}

func TestStartTakesTheRequestOverTheRepository(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{})
	fx.repoOverrides(store.WatchOverrides{
		Provider: ProviderCopilot, Model: "auto", MergeMethod: "squash", ApprovalMode: store.ApprovalManual,
		ApprovalsSet: true, Approvals: new(4), KeepWorktree: new(true), IncludeExisting: new(true),
	})

	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir,
		Provider: ProviderClaude, MergeMethod: new("merge"), ApprovalMode: new(store.ApprovalAuto),
		ApprovalsRequired: ApprovalsOf(0), KeepWorktree: new(false), IncludeExisting: new(false),
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.Provider != ProviderClaude || w.Model != "" || w.MergeMethod != "merge" || w.ApprovalMode != store.ApprovalAuto ||
		w.ApprovalsRequired != 0 || w.KeepWorktree || w.IncludeExisting {
		t.Fatalf("watch = %+v, want what the request asked for", w)
	}
}

func TestAModelWithoutAProviderRunsOnTheProviderOfTheChain(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{})
	fx.repoOverrides(store.WatchOverrides{Provider: ProviderCopilot, Model: "auto"})

	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir,
		Model: "gpt-5.3-codex",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.Provider != ProviderCopilot || w.Model != "gpt-5.3-codex" {
		t.Fatalf("agent = %s %s, want copilot with the model of the request", w.Provider, w.Model)
	}
}

func TestTheEffortComesFromTheLayerThatGivesTheModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		asked StartRequest
		model string
		want  string
	}{
		{"nothing asked takes the effort of the repository", StartRequest{}, "opus", "max"},
		{"an effort alone keeps the model of the repository", StartRequest{Effort: "low"}, "opus", "low"},
		{"a new model takes its own default effort", StartRequest{Model: "sonnet"}, "sonnet", ""},
		{"a new model takes the effort of the request", StartRequest{Model: "sonnet", Effort: "medium"}, "sonnet", "medium"},
		{"a new provider takes nothing of the repository", StartRequest{Provider: ProviderClaude}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			fx.settings(store.Settings{Model: "sonnet", Effort: "high"})
			fx.repoOverrides(store.WatchOverrides{Provider: ProviderClaude, Model: "opus", Effort: "max"})

			req := tc.asked
			req.Target, req.SourceDir = snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, fx.dir
			w, err := fx.svc.Start(context.Background(), req)
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			if w.Model != tc.model || w.Effort != tc.want {
				t.Fatalf("agent = %q at %q effort, want %q at %q", w.Model, w.Effort, tc.model, tc.want)
			}
		})
	}
}

func TestTheSettingsGiveTheEffortWithoutARepositoryAgent(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{Model: "sonnet", Effort: "high"})

	w, err := fx.svc.Start(context.Background(), StartRequest{
		Target: snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, SourceDir: fx.dir,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if w.Model != "sonnet" || w.Effort != "high" {
		t.Fatalf("agent = %q at %q effort, want the sonnet at high effort of the settings", w.Model, w.Effort)
	}
}

func TestAStoppedWatchKeepsTheWorktreeRuleItStartedWith(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{KeepWorktree: true})
	w := fx.start()
	fx.settings(store.Settings{KeepWorktree: false})
	before := len(fx.git.removedDirs())

	if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if got := fx.git.removedDirs(); len(got) != before {
		t.Fatalf("the worktree was removed (%v), want it kept as the watch took at start", got[before:])
	}
}
