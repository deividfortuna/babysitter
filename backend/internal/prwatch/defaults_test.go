package prwatch

import (
	"cmp"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

func ApprovalsFromBranch() Approvals { return Approvals{Set: true} }

func ApprovalsOf(n int) Approvals { return Approvals{Set: true, Count: &n} }

func (fx *fixture) settings(s store.Settings) {
	fx.t.Helper()
	s.PollInterval, s.WatchInterval, s.WatchMaxInterval, s.CheckMaxInterval = time.Minute, time.Minute, time.Minute, time.Minute
	s.ApprovalMode = cmp.Or(s.ApprovalMode, store.ApprovalAuto)
	s.Provider = cmp.Or(s.Provider, ProviderClaude)
	s.BranchUpdate = cmp.Or(s.BranchUpdate, store.BranchRebase)
	if _, err := fx.st.SaveSettings(context.Background(), s); err != nil {
		fx.t.Fatal(err)
	}
}

func TestStartWithScreenReaderLaunchesThePlainTextInterface(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.settings(store.Settings{ScreenReader: true})

	fx.start()

	h := fx.host.last()
	if h == nil {
		t.Fatal("no session started")
	}
	if !slices.Contains(h.spec.Argv, "--screen-reader") {
		t.Fatalf("the screen reader mode is off when the setting is on: %v", h.spec.Argv)
	}
}

type startedWith struct {
	Provider          string
	Model             string
	Effort            string
	MergeMethod       string
	ApprovalMode      store.ApprovalMode
	ApprovalsRequired int
	IncludeExisting   bool
	IncludeOwn        bool
	KeepWorktree      bool
	AutoApproveRebase bool
}

func startedWithOf(w store.Watch) startedWith {
	return startedWith{
		Provider: w.Provider, Model: w.Model, Effort: w.Effort, MergeMethod: w.MergeMethod,
		ApprovalMode: w.ApprovalMode, ApprovalsRequired: w.ApprovalsRequired,
		IncludeExisting: w.IncludeExisting, IncludeOwn: w.IncludeOwn, KeepWorktree: w.KeepWorktree, AutoApproveRebase: w.AutoApproveRebase,
	}
}

func TestStartTakesEachFieldFromTheLayerThatSetsIt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		settings store.Settings
		repo     *store.WatchOverrides
		request  StartRequest
		want     startedWith
	}{
		{
			name: "the settings without a repository row",
			settings: store.Settings{
				Model: "sonnet", Effort: "high", ApprovalsRequired: new(3), MergeMethod: "rebase", ApprovalMode: store.ApprovalManual,
				IncludeExisting: true, IncludeOwn: true, KeepWorktree: true, AutoApproveRebase: true,
			},
			want: startedWith{
				Provider: ProviderClaude, Model: "sonnet", Effort: "high", MergeMethod: "rebase", ApprovalMode: store.ApprovalManual, ApprovalsRequired: 3,
				IncludeExisting: true, IncludeOwn: true, KeepWorktree: true, AutoApproveRebase: true,
			},
		},
		{
			name: "the settings where the repository is silent",
			settings: store.Settings{
				Provider: ProviderCopilot, Model: "auto", ApprovalsRequired: new(3), MergeMethod: "rebase", ApprovalMode: store.ApprovalManual,
				IncludeExisting: true, KeepWorktree: true, AutoApproveRebase: true,
			},
			repo: &store.WatchOverrides{IncludeOwn: new(true)},
			want: startedWith{
				Provider: ProviderCopilot, Model: "auto", MergeMethod: "rebase", ApprovalMode: store.ApprovalManual, ApprovalsRequired: 3,
				IncludeExisting: true, IncludeOwn: true, KeepWorktree: true, AutoApproveRebase: true,
			},
		},
		{
			name: "the repository over the settings",
			settings: store.Settings{
				ApprovalsRequired: new(3), MergeMethod: "rebase", ApprovalMode: store.ApprovalManual,
				IncludeExisting: true, IncludeOwn: true, KeepWorktree: true, AutoApproveRebase: true,
			},
			repo: &store.WatchOverrides{
				Provider: ProviderCopilot, Model: "auto", MergeMethod: "squash", ApprovalMode: store.ApprovalAuto,
				ApprovalsSet: true, Approvals: new(1),
				IncludeExisting: new(false), IncludeOwn: new(false), KeepWorktree: new(false), AutoApproveRebase: new(false),
			},
			want: startedWith{Provider: ProviderCopilot, Model: "auto", MergeMethod: "squash", ApprovalMode: store.ApprovalAuto, ApprovalsRequired: 1},
		},
		{
			name: "the request over the settings",
			settings: store.Settings{
				MergeMethod: "rebase", ApprovalMode: store.ApprovalManual, IncludeExisting: true, IncludeOwn: true, AutoApproveRebase: true,
			},
			request: StartRequest{
				IncludeExisting: new(false), IncludeOwn: new(false), ApprovalsRequired: ApprovalsOf(0), MergeMethod: new("squash"),
				ApprovalMode: new(store.ApprovalAuto),
			},
			want: startedWith{Provider: ProviderClaude, MergeMethod: "squash", ApprovalMode: store.ApprovalAuto, AutoApproveRebase: true},
		},
		{
			name: "the request over the repository",
			repo: &store.WatchOverrides{
				Provider: ProviderCopilot, Model: "auto", MergeMethod: "squash", ApprovalMode: store.ApprovalManual,
				ApprovalsSet: true, Approvals: new(4), KeepWorktree: new(true), IncludeExisting: new(true),
			},
			request: StartRequest{
				Provider: ProviderClaude, MergeMethod: new("merge"), ApprovalMode: new(store.ApprovalAuto),
				ApprovalsRequired: ApprovalsOf(0), KeepWorktree: new(false), IncludeExisting: new(false),
			},
			want: startedWith{Provider: ProviderClaude, MergeMethod: "merge", ApprovalMode: store.ApprovalAuto},
		},
		{
			name:     "the rule of the branch over a setting that names a number",
			settings: store.Settings{ApprovalsRequired: new(5)},
			request:  StartRequest{ApprovalsRequired: ApprovalsFromBranch()},
			want:     startedWith{Provider: ProviderClaude, ApprovalMode: store.ApprovalAuto, ApprovalsRequired: 1},
		},
		{
			name:     "the repository default over a setting that names a method",
			settings: store.Settings{MergeMethod: "rebase"},
			request:  StartRequest{MergeMethod: new("")},
			want:     startedWith{Provider: ProviderClaude, ApprovalMode: store.ApprovalAuto, ApprovalsRequired: 1},
		},
		{
			name:    "a model without a provider on the provider of the chain",
			repo:    &store.WatchOverrides{Provider: ProviderCopilot, Model: "auto"},
			request: StartRequest{Model: "gpt-5.3-codex"},
			want:    startedWith{Provider: ProviderCopilot, Model: "gpt-5.3-codex", ApprovalMode: store.ApprovalAuto, ApprovalsRequired: 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			fx.settings(tc.settings)
			if tc.repo != nil {
				fx.repoOverrides(*tc.repo)
			}

			req := tc.request
			req.Target, req.SourceDir = snapshot.Target{Owner: "octo", Name: "hello", Number: 3}, fx.dir
			w, err := fx.svc.Start(context.Background(), req)
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			if got := startedWithOf(w); got != tc.want {
				t.Fatalf("watch = %+v, want %+v", got, tc.want)
			}
		})
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
