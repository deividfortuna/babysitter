package autostart

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

var toggledOn = time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)

type fixture struct {
	t     *testing.T
	st    *store.Store
	repo  store.Repo
	now   time.Time
	mu    sync.Mutex
	reqs  []prwatch.StartRequest
	fail  error
	start *Starter
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "babysitter.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	repo, err := st.AddRepo(context.Background(), "acme", "billing")
	if err != nil {
		t.Fatal(err)
	}
	fx := &fixture{t: t, st: st, repo: repo, now: toggledOn.Add(time.Hour)}
	fx.start = fx.starter(testutil.Logger(t))
	return fx
}

func (fx *fixture) starter(log *slog.Logger) *Starter {
	return New(Deps{
		Store: fx.st, Start: fx.fakeStart, Log: log,
		Login: func(context.Context) (string, error) { return "alice", nil },
		Now:   func() time.Time { return fx.now },
	})
}

func (fx *fixture) fakeStart(ctx context.Context, req prwatch.StartRequest) (store.Watch, error) {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	if fx.fail != nil {
		return store.Watch{}, fx.fail
	}
	fx.reqs = append(fx.reqs, req)
	author := "alice"
	if req.AutoReason == store.AutoDependabot {
		author = "dependabot[bot]"
	}
	return fx.st.CreateWatch(ctx, store.Watch{
		Owner: req.Target.Owner, Name: req.Target.Name, Number: req.Target.Number, Author: author,
		StartedAt: fx.now, AutoReason: req.AutoReason,
	})
}

func (fx *fixture) configure(change func(*store.RepoConfig)) {
	fx.t.Helper()
	cfg := store.DefaultRepoConfig(fx.repo.ID)
	cfg.CheckoutDir = "/code/billing"
	change(&cfg)
	if _, err := fx.st.SaveRepoConfig(context.Background(), cfg); err != nil {
		fx.t.Fatal(err)
	}
}

func mineOn(c *store.RepoConfig) { c.OwnSince = &toggledOn }

func dependabotOn(c *store.RepoConfig) { c.DependabotSince = &toggledOn }

func (fx *fixture) pr(number int, change func(*store.PullRequest)) {
	fx.t.Helper()
	pr := store.PullRequest{
		RepoID: fx.repo.ID, Number: number, Title: "change", Author: "alice", State: store.StateOpen,
		CreatedAt: toggledOn.Add(time.Duration(number) * time.Minute), UpdatedAt: fx.now, SyncedAt: fx.now,
	}
	if change != nil {
		change(&pr)
	}
	if err := fx.st.UpsertPR(context.Background(), pr); err != nil {
		fx.t.Fatal(err)
	}
}

func bump(level dependabot.Level) func(*store.PullRequest) {
	return func(pr *store.PullRequest) {
		pr.Author, pr.UpdateType = "dependabot[bot]", level
	}
}

func (fx *fixture) run() {
	fx.t.Helper()
	fx.start.Run(context.Background())
}

func (fx *fixture) started() []int {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	out := []int{}
	for _, r := range fx.reqs {
		out = append(out, r.Target.Number)
	}
	return out
}

func (fx *fixture) queue() []int {
	fx.t.Helper()
	prs, err := Queue(context.Background(), fx.st, fx.repo)
	if err != nil {
		fx.t.Fatal(err)
	}
	out := []int{}
	for _, pr := range prs {
		out = append(out, pr.Number)
	}
	return out
}

func (fx *fixture) wantStarted(want ...int) {
	fx.t.Helper()
	if got := fx.started(); !slices.Equal(got, want) {
		fx.t.Fatalf("started %v, want %v", got, want)
	}
}

func TestANewPullRequestOfTheAuthorStartsWithTheOverrides(t *testing.T) {
	fx := newFixture(t)
	two, yes := 2, true
	fx.configure(func(c *store.RepoConfig) {
		mineOn(c)
		c.Overrides = store.WatchOverrides{
			Provider: "copilot", Model: "gpt-5", ApprovalMode: store.ApprovalManual, MergeMethod: "squash",
			ApprovalsSet: true, Approvals: &two, IncludeExisting: &yes,
		}
	})
	fx.pr(1, nil)
	fx.pr(2, func(pr *store.PullRequest) { pr.Author, pr.Assignees = "bob", []string{"Alice"} })
	fx.pr(3, func(pr *store.PullRequest) { pr.Author = "bob" })

	fx.run()

	fx.wantStarted(1, 2)
	mine, assigned := fx.reqs[0], fx.reqs[1]
	if mine.AutoReason != store.AutoMine || assigned.AutoReason != store.AutoAssigned {
		t.Errorf("reasons = %s, %s, want mine and assigned", mine.AutoReason, assigned.AutoReason)
	}
	if mine.SourceDir != "/code/billing" || mine.Provider != "copilot" || mine.Model != "gpt-5" ||
		*mine.ApprovalMode != store.ApprovalManual || *mine.MergeMethod != "squash" ||
		!mine.ApprovalsRequired.Set || *mine.ApprovalsRequired.Count != 2 || !*mine.IncludeExisting {
		t.Errorf("request = %+v, want the overrides of the repository", mine)
	}
	if mine.MergeWhenReady != nil {
		t.Errorf("MergeWhenReady = %v, want the setting of the daemon for a pull request of the author", *mine.MergeWhenReady)
	}
}

func TestAnEmptyOverrideTakesTheSettingOfTheDaemon(t *testing.T) {
	fx := newFixture(t)
	fx.configure(mineOn)
	fx.pr(1, nil)
	fx.run()
	req := fx.reqs[0]
	if req.Provider != "" || req.ApprovalMode != nil || req.MergeMethod != nil || req.ApprovalsRequired.Set || req.IncludeExisting != nil {
		t.Fatalf("request = %+v, want no override", req)
	}
}

func TestAPullRequestFromBeforeTheToggleNeverStarts(t *testing.T) {
	fx := newFixture(t)
	fx.configure(mineOn)
	fx.pr(1, func(pr *store.PullRequest) { pr.CreatedAt = toggledOn.Add(-time.Minute) })
	fx.run()
	fx.run()
	fx.wantStarted()
}

func TestAPullRequestWhoseWatchStoppedNeverStartsAgain(t *testing.T) {
	fx := newFixture(t)
	fx.configure(mineOn)
	fx.pr(1, nil)
	fx.run()
	w, err := fx.st.FindActiveWatch(context.Background(), store.WatchKey{Owner: "acme", Name: "billing", Number: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.st.StopWatch(context.Background(), w.ID, store.StopUser, nil, fx.now); err != nil {
		t.Fatal(err)
	}
	fx.run()
	fx.run()
	fx.wantStarted(1)
}

func TestAPullRequestWatchedByHandBeforeTheToggleDoesNotStart(t *testing.T) {
	fx := newFixture(t)
	fx.configure(mineOn)
	fx.pr(1, nil)
	if _, err := fx.st.CreateWatch(context.Background(), store.Watch{Owner: "acme", Name: "billing", Number: 1, StartedAt: fx.now}); err != nil {
		t.Fatal(err)
	}
	fx.run()
	fx.wantStarted()
}

func TestADraftStartsWhenItIsReady(t *testing.T) {
	fx := newFixture(t)
	fx.configure(mineOn)
	fx.pr(1, func(pr *store.PullRequest) { pr.Draft = true })
	fx.run()
	fx.wantStarted()

	fx.pr(1, nil)
	fx.run()
	fx.wantStarted(1)
}

func TestIncludeDraftsStartsADraft(t *testing.T) {
	fx := newFixture(t)
	fx.configure(func(c *store.RepoConfig) {
		mineOn(c)
		c.IncludeDrafts = true
	})
	fx.pr(1, func(pr *store.PullRequest) { pr.Draft = true })
	fx.run()
	fx.wantStarted(1)
}

func TestAPullRequestFromAForkIsSkippedAndLoggedOnce(t *testing.T) {
	fx := newFixture(t)
	var logs bytes.Buffer
	fx.start = fx.starter(slog.New(slog.NewTextHandler(&logs, nil)))
	fx.configure(mineOn)
	fx.pr(7, func(pr *store.PullRequest) {
		pr.Author, pr.Assignees, pr.Fork = "teammate", []string{"alice"}, true
	})
	fx.run()
	fx.run()
	fx.wantStarted()
	if n := strings.Count(logs.String(), "acme/billing#7"); n != 1 {
		t.Fatalf("the log names acme/billing#7 %d times, want once:\n%s", n, logs.String())
	}
}

func TestNothingStartsWithoutAToggle(t *testing.T) {
	fx := newFixture(t)
	fx.configure(func(*store.RepoConfig) {})
	fx.pr(1, nil)
	fx.pr(2, bump(dependabot.Patch))
	fx.run()
	fx.wantStarted()
}

func TestDependabotStartsUpToTheLimitAndQueuesTheRestOldestFirst(t *testing.T) {
	fx := newFixture(t)
	fx.configure(dependabotOn)
	fx.pr(10, bump(dependabot.Patch))
	fx.pr(11, bump(dependabot.Minor))
	fx.pr(12, func(pr *store.PullRequest) {
		bump(dependabot.Major)(pr)
		pr.CreatedAt = toggledOn.Add(time.Second)
	})
	fx.pr(13, bump(dependabot.Major))
	fx.pr(14, nil)

	fx.run()

	fx.wantStarted(12)
	if got := fx.queue(); !slices.Equal(got, []int{10, 11, 13}) {
		t.Fatalf("queue = %v, want 10, 11, 13", got)
	}
}

func TestTheNextPullRequestOfTheQueueStartsWhenAPlaceIsFree(t *testing.T) {
	fx := newFixture(t)
	fx.configure(dependabotOn)
	fx.pr(10, bump(dependabot.Patch))
	fx.pr(11, bump(dependabot.Patch))
	fx.run()
	w, err := fx.st.FindActiveWatch(context.Background(), store.WatchKey{Owner: "acme", Name: "billing", Number: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.st.StopWatch(context.Background(), w.ID, store.StopMerged, nil, fx.now); err != nil {
		t.Fatal(err)
	}
	fx.run()
	fx.wantStarted(10, 11)
	if got := fx.queue(); len(got) != 0 {
		t.Fatalf("queue = %v, want empty", got)
	}
}

func TestAWatchStartedByHandCountsTowardTheLimit(t *testing.T) {
	fx := newFixture(t)
	fx.configure(dependabotOn)
	fx.pr(9, func(pr *store.PullRequest) {
		bump(dependabot.Patch)(pr)
		pr.CreatedAt = toggledOn.Add(-time.Hour)
	})
	fx.pr(10, bump(dependabot.Patch))
	if _, err := fx.st.CreateWatch(context.Background(), store.Watch{Owner: "acme", Name: "billing", Number: 9, Author: "dependabot[bot]", StartedAt: fx.now}); err != nil {
		t.Fatal(err)
	}
	fx.run()
	fx.wantStarted()
	if got := fx.queue(); !slices.Equal(got, []int{10}) {
		t.Fatalf("queue = %v, want 10", got)
	}
}

func TestAPullRequestLeavesTheQueueWhenTheAuthorStartsItByHand(t *testing.T) {
	fx := newFixture(t)
	fx.configure(dependabotOn)
	fx.pr(10, bump(dependabot.Patch))
	fx.pr(11, bump(dependabot.Patch))
	fx.run()
	if _, err := fx.st.CreateWatch(context.Background(), store.Watch{Owner: "acme", Name: "billing", Number: 11, Author: "dependabot[bot]", StartedAt: fx.now}); err != nil {
		t.Fatal(err)
	}
	if got := fx.queue(); len(got) != 0 {
		t.Fatalf("queue = %v, want empty", got)
	}
}

func TestTheScopeSetsMergeWhenReadyOnADependabotWatch(t *testing.T) {
	fx := newFixture(t)
	fx.configure(func(c *store.RepoConfig) {
		dependabotOn(c)
		c.DependabotScope = dependabot.Minor
		c.DependabotLimit = 2
	})
	fx.pr(10, bump(dependabot.Minor))
	fx.pr(11, bump(dependabot.Major))
	fx.run()
	fx.wantStarted(10, 11)
	minor, major := fx.reqs[0], fx.reqs[1]
	if !*minor.MergeWhenReady || minor.UpdateType != dependabot.Minor {
		t.Errorf("minor update: MergeWhenReady %v, update %s, want on and minor", *minor.MergeWhenReady, minor.UpdateType)
	}
	if *major.MergeWhenReady || major.UpdateType != dependabot.Major {
		t.Errorf("major update: MergeWhenReady %v, update %s, want off and major", *major.MergeWhenReady, major.UpdateType)
	}
}

func TestTheToggleGoesOffTheWatchesGoOnAndTheQueueIsEmpty(t *testing.T) {
	fx := newFixture(t)
	fx.configure(func(c *store.RepoConfig) {
		dependabotOn(c)
		c.DependabotLimit = 2
	})
	for n := 10; n < 14; n++ {
		fx.pr(n, bump(dependabot.Patch))
	}
	fx.run()
	fx.configure(func(*store.RepoConfig) {})
	fx.run()
	fx.wantStarted(10, 11)
	if got := fx.queue(); len(got) != 0 {
		t.Fatalf("queue = %v, want empty", got)
	}
	active, err := fx.st.ListWatches(context.Background(), store.ListWatchesOptions{Status: store.WatchActive})
	if err != nil || len(active) != 2 {
		t.Fatalf("active watches = %d, %v, want 2", len(active), err)
	}
}

func TestTheQueueNeverHoldsAPullRequestOfTheAuthor(t *testing.T) {
	fx := newFixture(t)
	fx.configure(func(c *store.RepoConfig) {
		mineOn(c)
		dependabotOn(c)
	})
	fx.pr(1, nil)
	fx.pr(2, nil)
	fx.pr(10, bump(dependabot.Patch))
	fx.pr(11, bump(dependabot.Patch))
	fx.run()
	fx.wantStarted(1, 2, 10)
	if got := fx.queue(); !slices.Equal(got, []int{11}) {
		t.Fatalf("queue = %v, want 11", got)
	}
}

func TestTwoDaemonsStartOneWatchAndTheOtherWritesADebugLine(t *testing.T) {
	fx := newFixture(t)
	fx.configure(mineOn)
	fx.pr(1, nil)
	var logs bytes.Buffer
	other := fx.starter(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err := fx.st.ClaimAutoStart(context.Background(), fx.repo.ID, 1, fx.now); err != nil {
		t.Fatal(err)
	}
	other.startOne(context.Background(), fx.repo, store.DefaultRepoConfig(fx.repo.ID), candidate{pr: store.PullRequest{Number: 1}, reason: store.AutoMine})
	fx.wantStarted()
	if !strings.Contains(logs.String(), "level=DEBUG") || strings.Contains(logs.String(), "level=ERROR") || strings.Contains(logs.String(), "level=WARN") {
		t.Fatalf("the daemon that lost wrote:\n%s\nwant one debug line", logs.String())
	}
}

func TestALostRaceOnTheUniqueIndexIsADebugLine(t *testing.T) {
	fx := newFixture(t)
	fx.configure(mineOn)
	fx.pr(1, nil)
	var logs bytes.Buffer
	fx.start = fx.starter(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	fx.fail = store.ErrWatchExists
	fx.run()
	if !strings.Contains(logs.String(), "already watched") || strings.Contains(logs.String(), "level=WARN") {
		t.Fatalf("log:\n%s\nwant a debug line only", logs.String())
	}
}

func TestAFailedStartTriesAgainOnTheNextPass(t *testing.T) {
	fx := newFixture(t)
	fx.configure(mineOn)
	fx.pr(1, nil)
	fx.fail = errors.New("GitHub is down")
	fx.run()
	fx.wantStarted()
	fx.fail = nil
	fx.run()
	fx.wantStarted(1)
}

func TestTheLoginIsReadOnceForManyPasses(t *testing.T) {
	fx := newFixture(t)
	calls := 0
	fx.start = New(Deps{
		Store: fx.st, Start: fx.fakeStart, Log: testutil.Logger(t), Now: func() time.Time { return fx.now },
		Login: func(context.Context) (string, error) {
			calls++
			return "alice", nil
		},
	})
	fx.configure(mineOn)
	fx.run()
	fx.run()
	fx.now = fx.now.Add(loginTTL)
	fx.run()
	if calls != 2 {
		t.Fatalf("the login was read %d times, want 2", calls)
	}
}
