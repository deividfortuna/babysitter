package watcher

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

type fixture struct {
	gh    *ghfake.GitHub
	store *store.Store
	w     *Watcher
	slept []time.Duration
	repo  store.Repo
	now   time.Time
}

func (fx *fixture) advance(d time.Duration) { fx.now = fx.now.Add(d) }

func newFixture(t *testing.T) *fixture {
	t.Helper()
	gh := ghfake.New()
	gh.Repo("o/r")
	srv := gh.Serve(t)

	st, err := store.Open(filepath.Join(t.TempDir(), "babysitter.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	repo, err := st.AddRepo(context.Background(), "o", "r")
	if err != nil {
		t.Fatal(err)
	}

	fx := &fixture{gh: gh, store: st, repo: repo, now: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)}
	newClient := func(ctx context.Context) (*github.Client, error) {
		return srv.NewClient()
	}
	fx.w = New(st, newClient,
		WithLogger(testutil.Logger(t)),
		WithClock(func() time.Time { return fx.now }, func(ctx context.Context, d time.Duration) error {
			fx.slept = append(fx.slept, d)
			return nil
		}),
	)
	return fx
}

// seedOpenPR puts the open pull request o/r#1 in the fake: by alice, from
// feat at sha1 into main, approved by bob, with one green check run and no
// commit status.
func (fx *fixture) seedOpenPR() *ghfake.PR {
	pr := fx.gh.PR("o/r", 1)
	fx.gh.Update(func() {
		pr.ID, pr.Title, pr.Author = 11, "One", "alice"
		pr.HeadRef, pr.HeadSHA, pr.BaseRef = "feat", "sha1", "main"
		pr.CreatedAt = ghfake.At("2026-09-01T00:00:00Z")
		pr.UpdatedAt = ghfake.At("2026-09-02T00:00:00Z")
		pr.MergeableState, pr.Additions, pr.Deletions = "clean", 5, 2
		pr.Requested, pr.Labels = []string{"bob"}, []string{"bug"}
		pr.Reviews = []ghfake.Review{{ID: 1, State: "APPROVED", Author: "bob", SubmittedAt: pr.UpdatedAt}}
		pr.CheckRuns = []ghfake.CheckRun{greenRun(1)}
		pr.Statuses, pr.StatusState = nil, "pending"
	})
	return pr
}

// update changes the pull request while the fake serves.
func (fx *fixture) update(pr *ghfake.PR, fn func(pr *ghfake.PR)) {
	fx.gh.Update(func() { fn(pr) })
}

func greenRun(id int64) ghfake.CheckRun {
	return ghfake.CheckRun{ID: id, Status: "completed", Conclusion: "success"}
}

func runningRun(id int64) ghfake.CheckRun {
	return ghfake.CheckRun{ID: id, Status: "in_progress"}
}

func (fx *fixture) passesThatReadChecks(t *testing.T, sha string, every time.Duration, passes int) []int {
	t.Helper()
	var read []int
	for pass := range passes {
		fx.gh.Reset()
		if err := fx.w.SyncAll(t.Context()); err != nil {
			t.Fatal(err)
		}
		if fx.gh.CountPath("/repos/o/r/commits/"+sha+"/check-runs") > 0 {
			read = append(read, pass)
		}
		fx.advance(every)
	}
	return read
}

func (fx *fixture) seedPendingPR() *ghfake.PR {
	pr := fx.seedOpenPR()
	fx.update(pr, func(pr *ghfake.PR) { pr.CheckRuns = []ghfake.CheckRun{runningRun(1)} })
	return pr
}

// paths lists the paths the fake received, for a failure message.
func (fx *fixture) paths() []string {
	var out []string
	for _, a := range fx.gh.Actions() {
		out = append(out, a.Path)
	}
	return out
}

func TestSyncNewAndUnchangedPR(t *testing.T) {
	fx := newFixture(t)
	fx.seedOpenPR()
	ctx := context.Background()

	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatalf("SyncAll() error = %v", err)
	}
	for path, want := range map[string]int{
		"/repos/o/r/pulls": 1, "/repos/o/r/pulls/1": 1, "/repos/o/r/pulls/1/reviews": 1,
		"/repos/o/r/commits/sha1/check-runs": 1, "/repos/o/r/commits/sha1/status": 1,
	} {
		if got := fx.gh.CountPath(path); got != want {
			t.Errorf("%s called %d times, want %d", path, got, want)
		}
	}
	prs, _ := fx.store.ListPRs(ctx, store.ListPRsOptions{})
	if len(prs) != 1 {
		t.Fatalf("stored %d PRs, want 1", len(prs))
	}
	pr := prs[0]
	if pr.Title != "One" || pr.Author != "alice" || pr.HeadSHA != "sha1" || pr.MergeableState != "clean" ||
		pr.Additions != 5 || pr.Deletions != 2 || pr.ReviewDecision != store.ReviewApproved || pr.Approvals != 1 ||
		pr.CIStatus != checks.CISuccess || len(pr.Labels) != 1 || pr.Labels[0] != "bug" ||
		len(pr.RequestedReviewers) != 1 || pr.RequestedReviewers[0] != "bob" {
		t.Fatalf("unexpected PR %+v", pr)
	}
	repo, _ := fx.store.GetRepo(ctx, "o", "r")
	if repo.LastSyncedAt == nil || repo.LastError != "" {
		t.Fatalf("repo sync state %+v", repo)
	}

	fx.gh.Reset()
	fx.advance(time.Minute)
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatalf("second SyncAll() error = %v", err)
	}
	if fx.gh.CountPath("/repos/o/r/pulls") != 1 || fx.gh.Total() != 1 {
		t.Fatalf("second pass made %d calls, want only the list", fx.gh.Total())
	}
	prs, _ = fx.store.ListPRs(ctx, store.ListPRsOptions{})
	if prs[0].CIStatus != checks.CISuccess || prs[0].MergeableState != "clean" || prs[0].ReviewDecision != store.ReviewApproved {
		t.Fatalf("unexpected PR after second pass %+v", prs[0])
	}
	if len(fx.slept) != 0 {
		t.Fatalf("slept %v, want no sleeps", fx.slept)
	}
}

func TestSyncReadsPendingChecksAgainAfterAMinute(t *testing.T) {
	fx := newFixture(t)
	pr := fx.seedOpenPR()
	fx.update(pr, func(pr *ghfake.PR) { pr.CheckRuns = []ghfake.CheckRun{runningRun(1)} })
	ctx := context.Background()
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	prs, _ := fx.store.ListPRs(ctx, store.ListPRsOptions{})
	if prs[0].CIStatus != checks.CIPending {
		t.Fatalf("CI status = %q, want pending", prs[0].CIStatus)
	}

	fx.gh.Reset()
	fx.advance(time.Minute)
	fx.update(pr, func(pr *ghfake.PR) { pr.CheckRuns = []ghfake.CheckRun{greenRun(1)} })
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	if fx.gh.CountPath("/repos/o/r/commits/sha1/check-runs") != 1 || fx.gh.CountPath("/repos/o/r/commits/sha1/status") != 1 {
		t.Fatalf("checks that still run were not read again: %d calls", fx.gh.Total())
	}
	prs, _ = fx.store.ListPRs(ctx, store.ListPRsOptions{})
	if prs[0].CIStatus != checks.CISuccess {
		t.Fatalf("CI status = %q, want success", prs[0].CIStatus)
	}
}

func TestSyncBacksOffTheChecksOfAPendingPR(t *testing.T) {
	fx := newFixture(t)
	fx.w.SetLongestCheckWait(8 * time.Minute)
	fx.seedPendingPR()

	if got, want := fx.passesThatReadChecks(t, "sha1", time.Minute, 32), []int{0, 1, 3, 7, 15, 23, 31}; !slices.Equal(got, want) {
		t.Fatalf("checks read at minutes %v, want %v", got, want)
	}
}

func TestSyncStartsTheBackOffAtAPollIntervalShorterThanAMinute(t *testing.T) {
	fx := newFixture(t)
	WithInterval(20 * time.Second)(fx.w)
	fx.seedPendingPR()

	if got, want := fx.passesThatReadChecks(t, "sha1", 20*time.Second, 16), []int{0, 1, 3, 7, 15}; !slices.Equal(got, want) {
		t.Fatalf("checks read on passes %v, want %v: waits of 20s, 40s, 80s and 160s", got, want)
	}
}

func TestSyncKeepsTheBackOffWhenAPassStartsALittleEarly(t *testing.T) {
	fx := newFixture(t)
	fx.seedPendingPR()

	if got, want := fx.passesThatReadChecks(t, "sha1", time.Minute-100*time.Millisecond, 16), []int{0, 1, 3, 7, 15}; !slices.Equal(got, want) {
		t.Fatalf("checks read on passes %v, want %v", got, want)
	}
}

func TestSyncReadsPendingChecksAfterAKickAndStartsTheBackOffAgain(t *testing.T) {
	fx := newFixture(t)
	fx.seedPendingPR()
	fx.passesThatReadChecks(t, "sha1", time.Minute, 5)

	fx.w.forgetPendingChecks()

	if got, want := fx.passesThatReadChecks(t, "sha1", time.Minute, 4), []int{0, 1, 3}; !slices.Equal(got, want) {
		t.Fatalf("checks read on passes %v after the kick, want %v", got, want)
	}
}

func TestSyncStartsTheBackOffAgainOnANewHead(t *testing.T) {
	fx := newFixture(t)
	pr := fx.seedPendingPR()
	fx.passesThatReadChecks(t, "sha1", time.Minute, 10)

	fx.update(pr, func(pr *ghfake.PR) {
		pr.HeadSHA = "sha2"
		pr.CheckRuns = []ghfake.CheckRun{runningRun(2)}
	})

	if got, want := fx.passesThatReadChecks(t, "sha2", time.Minute, 4), []int{0, 1, 3}; !slices.Equal(got, want) {
		t.Fatalf("checks of the new head read at minutes %v, want %v", got, want)
	}
}

func TestSyncReadsChecksAgainAfterTheTTL(t *testing.T) {
	fx := newFixture(t)
	WithCheckTTL(10 * time.Minute)(fx.w)
	fx.seedOpenPR()
	ctx := context.Background()
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}

	fx.gh.Reset()
	fx.advance(9 * time.Minute)
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	if got := fx.gh.CountPath("/repos/o/r/commits/sha1/check-runs"); got != 0 {
		t.Fatalf("checks read %d times inside the TTL, want none", got)
	}

	fx.advance(2 * time.Minute)
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	if fx.gh.CountPath("/repos/o/r/commits/sha1/check-runs") != 1 || fx.gh.CountPath("/repos/o/r/commits/sha1/status") != 1 {
		t.Fatalf("checks not read again after the TTL: %d calls", fx.gh.Total())
	}
}

func TestSyncReadsTheChecksOfANewHead(t *testing.T) {
	fx := newFixture(t)
	pr := fx.seedOpenPR()
	ctx := context.Background()
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}

	fx.gh.Reset()
	fx.advance(time.Minute)
	fx.update(pr, func(pr *ghfake.PR) {
		pr.HeadSHA = "sha2"
		pr.CheckRuns = []ghfake.CheckRun{runningRun(2)}
	})
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	if fx.gh.CountPath("/repos/o/r/commits/sha2/check-runs") != 1 || fx.gh.CountPath("/repos/o/r/commits/sha2/status") != 1 {
		t.Fatalf("the checks of the new head were not read: %d calls", fx.gh.Total())
	}
	prs, _ := fx.store.ListPRs(ctx, store.ListPRsOptions{})
	if prs[0].HeadSHA != "sha2" || prs[0].CIStatus != checks.CIPending {
		t.Fatalf("unexpected PR after the push %+v", prs[0])
	}
}

func TestSyncForgetsTheChecksOfAHeadThatMoved(t *testing.T) {
	fx := newFixture(t)
	pr := fx.seedOpenPR()
	ctx := context.Background()
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	fx.update(pr, func(pr *ghfake.PR) { pr.HeadSHA = "sha2" })
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}

	fx.w.checksMu.Lock()
	held := len(fx.w.checked)
	fx.w.checksMu.Unlock()
	if held != 1 {
		t.Fatalf("the watcher holds %d head commits, want only the current one", held)
	}
}

func TestSyncMergedPR(t *testing.T) {
	fx := newFixture(t)
	pr := fx.seedOpenPR()
	ctx := context.Background()
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}

	fx.update(pr, func(pr *ghfake.PR) {
		merged := ghfake.At("2026-09-03T00:00:00Z")
		pr.State, pr.Merged, pr.MergedAt, pr.UpdatedAt = "closed", true, merged, merged
	})
	fx.gh.Reset()
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	if fx.gh.CountPath("/repos/o/r/pulls/1") != 1 {
		t.Fatalf("calls = %v", fx.paths())
	}
	open, _ := fx.store.ListPRs(ctx, store.ListPRsOptions{})
	if len(open) != 0 {
		t.Fatalf("open PRs = %+v, want none", open)
	}
	all, _ := fx.store.ListPRs(ctx, store.ListPRsOptions{State: "all"})
	if len(all) != 1 || all[0].State != store.StateMerged || all[0].MergedAt == nil {
		t.Fatalf("all PRs = %+v", all)
	}

	fx.gh.Reset()
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	if fx.gh.CountPath("/repos/o/r/pulls/1") != 0 {
		t.Fatalf("merged PR fetched again: %v", fx.paths())
	}
}

func TestSyncDeletedPRIsClosed(t *testing.T) {
	fx := newFixture(t)
	fx.seedOpenPR()
	ctx := context.Background()
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	fx.gh.DeletePR("o/r", 1)
	if err := fx.w.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	all, _ := fx.store.ListPRs(ctx, store.ListPRsOptions{State: "all"})
	if len(all) != 1 || all[0].State != store.StateClosed || all[0].ClosedAt == nil {
		t.Fatalf("all PRs = %+v", all)
	}
}

func TestSyncOneRepoFailureDoesNotStopOthers(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	if _, err := fx.store.AddRepo(ctx, "o", "bad"); err != nil {
		t.Fatal(err)
	}
	fx.gh.React(ghfake.RoutePulls, func(a ghfake.Action) (ghfake.Response, bool) {
		return ghfake.Response{Status: http.StatusInternalServerError, Message: "boom"}, a.Vars["repo"] == "bad"
	})

	err := fx.w.SyncAll(ctx)
	if err == nil || !strings.Contains(err.Error(), "o/bad") {
		t.Fatalf("SyncAll() error = %v, want o/bad failure", err)
	}
	good, _ := fx.store.GetRepo(ctx, "o", "r")
	bad, _ := fx.store.GetRepo(ctx, "o", "bad")
	if good.LastSyncedAt == nil || good.LastError != "" {
		t.Fatalf("good repo %+v", good)
	}
	if bad.LastSyncedAt == nil || !strings.Contains(bad.LastError, "boom") {
		t.Fatalf("bad repo %+v", bad)
	}
}

func TestSyncStoresTheFailureWithoutTheToken(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()
	token := "ghp_" + strings.Repeat("a", 36)
	fx.gh.Fail(ghfake.RoutePulls, http.StatusUnauthorized, "Bad credentials for "+token)

	err := fx.w.SyncAll(ctx)
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("SyncAll() error = %v", err)
	}
	repo, _ := fx.store.GetRepo(ctx, "o", "r")
	if repo.LastError == "" || strings.Contains(repo.LastError, token) {
		t.Fatalf("the repository row carries the token: %q", repo.LastError)
	}
}

func TestSyncWaitsWhenRateLimitIsLow(t *testing.T) {
	fx := newFixture(t)
	reset := time.Date(2026, 9, 7, 12, 30, 0, 0, time.UTC)
	fx.gh.SetRate(&ghfake.Rate{Limit: 5000, Remaining: 3, Reset: reset})

	if err := fx.w.SyncAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(fx.slept) != 1 || fx.slept[0] != 30*time.Minute+time.Second {
		t.Fatalf("slept = %v, want one sleep of 30m1s", fx.slept)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	fx := newFixture(t)
	WithInterval(time.Millisecond)(fx.w)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- fx.w.Run(ctx) }()

	if !testutil.Within(testutil.Timeout, func() bool { return fx.gh.CountPath("/repos/o/r/pulls") >= 2 }) {
		cancel()
		t.Fatalf("list called %d times in %s, want at least 2", fx.gh.CountPath("/repos/o/r/pulls"), testutil.Timeout)
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run() did not return after the context was cancelled")
	}
}
