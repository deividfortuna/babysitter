package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
)

// newFakePR returns a fake GitHub with octo/hello#3, opened from the fork
// alice/hello: blocked while GitHub computes whether it merges, with a
// pending, an approving and a commenting review, two conversation comments,
// two inline comments, a failing check and the workflow runs of a failing
// head.
func newFakePR() (*ghfake.GitHub, *ghfake.PR) {
	g := ghfake.New()
	pr := g.PR("octo/hello", 3)
	pr.Mergeable, pr.MergeableState, pr.HeadRepo = nil, "blocked", "alice/hello"
	pr.Reviews = []ghfake.Review{
		{ID: 5, State: "PENDING", Author: "dave", Body: "draft"},
		{
			ID: 6, State: "APPROVED", Author: "bob", Body: "LGTM", Association: "MEMBER",
			SubmittedAt: ghfake.At("2026-09-02T00:00:00Z"), URL: "https://github.com/octo/hello/pull/3#pullrequestreview-6",
		},
		{
			ID: 7, State: "COMMENTED", Author: "bob",
			SubmittedAt: ghfake.At("2026-09-02T00:00:00Z"), URL: "https://github.com/octo/hello/pull/3#pullrequestreview-7",
		},
	}
	pr.IssueComments = []ghfake.Comment{
		{ID: 20, Author: "carol", Body: "second", Association: "NONE", CreatedAt: ghfake.At("2026-09-03T00:00:00Z")},
		{ID: 10, Author: "carol", Body: "first", Association: "NONE", CreatedAt: ghfake.At("2026-09-01T00:00:00Z")},
	}
	pr.ReviewComments = []ghfake.ReviewComment{
		{
			ID: 30, Author: "dave", Body: "pending inline", CreatedAt: ghfake.At("2026-09-02T00:00:00Z"),
			ReviewID: 5, Path: "a.go", Line: 1,
		},
		{
			ID: 31, Author: "bob", Body: "rename this", Association: "MEMBER", CreatedAt: ghfake.At("2026-09-02T00:00:00Z"),
			ReviewID: 7, Path: "internal/x.go", OriginalLine: 12, Side: "RIGHT", CommitID: "abc", OriginalCommitID: "old",
		},
	}
	pr.CheckRuns = []ghfake.CheckRun{
		{ID: 1, Name: "test", Status: "completed", Conclusion: "failure", URL: "https://github.com/octo/hello/runs/1"},
		{ID: 2, Name: "build", Status: "completed", Conclusion: "success"},
	}
	failingRuns(g)
	return g, pr
}

// newGreenPR returns a fake GitHub with octo/hello#3 mergeable, approved by
// bob, with no comment and a green head.
func newGreenPR() (*ghfake.GitHub, *ghfake.PR) {
	g := ghfake.New()
	pr := g.PR("octo/hello", 3)
	pr.HeadRepo = "alice/hello"
	pr.Reviews = []ghfake.Review{{ID: 6, State: "APPROVED", Author: "bob", Body: "LGTM", SubmittedAt: ghfake.At("2026-09-02T00:00:00Z")}}
	pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Name: "test", Status: "completed", Conclusion: "success"}}
	g.Repo("octo/hello").Runs = []*ghfake.Run{
		{ID: 99, Name: "ci", WorkflowID: 1, HeadSHA: "abc", Status: "completed", Conclusion: "cancelled", RefuseJobs: true},
		{ID: 101, Name: "ci", WorkflowID: 1, HeadSHA: "abc", Status: "completed", Conclusion: "success", RefuseJobs: true},
	}
	return g, pr
}

// failingRuns gives octo/hello the workflow runs of a failing head. Run 99
// is superseded by run 100, 101 passed, 102 ran on an older head, and 103
// waits for approval: the code must not read the jobs of any of them.
//
// GitHub filters the runs by head_sha, so the state never lists run 102
// for abc. The reactor lists every run whatever head it ran on, to keep
// the filter of the code on the head under test, and refuses a query for
// another head than abc.
func failingRuns(g *ghfake.GitHub) {
	runs := []*ghfake.Run{
		{ID: 99, Name: "ci", WorkflowID: 1, HeadSHA: "abc", Status: "completed", Conclusion: "cancelled", RefuseJobs: true},
		{
			ID: 100, Name: "ci", WorkflowID: 1, HeadSHA: "abc", Status: "completed", Conclusion: "failure",
			URL: "https://github.com/octo/hello/actions/runs/100",
			Jobs: []*ghfake.Job{
				{ID: 1000, Name: "test", Status: "completed", Conclusion: "failure", URL: "https://github.com/octo/hello/actions/runs/100/job/1000"},
				{ID: 1001, Name: "build", Status: "completed", Conclusion: "success"},
			},
		},
		{ID: 101, Name: "lint", HeadSHA: "abc", Status: "completed", Conclusion: "success", RefuseJobs: true},
		{ID: 102, Name: "ci", HeadSHA: "old", Status: "completed", Conclusion: "failure", RefuseJobs: true},
		{
			ID: 103, Name: "deploy", HeadSHA: "abc", Status: "action_required", Conclusion: "action_required",
			URL: "https://github.com/octo/hello/actions/runs/103", RefuseJobs: true,
		},
		{
			ID: 104, Name: "release", HeadSHA: "abc", Status: "waiting", URL: "https://github.com/octo/hello/actions/runs/104",
			Jobs: []*ghfake.Job{
				{ID: 1040, Name: "test", Status: "completed", Conclusion: "failure", URL: "https://github.com/octo/hello/actions/runs/104/job/1040"},
				{ID: 1041, Name: "deploy", Status: "waiting"},
			},
		},
	}
	g.Repo("octo/hello").Runs = runs
	g.React(ghfake.RouteWorkflowRuns, func(a ghfake.Action) (ghfake.Response, bool) {
		if a.Query.Get("head_sha") != "abc" {
			return ghfake.Response{Status: http.StatusBadRequest, Message: "want head_sha=abc"}, true
		}
		out := &github.WorkflowRuns{TotalCount: new(len(runs))}
		for _, r := range runs {
			out.WorkflowRuns = append(out.WorkflowRuns, &github.WorkflowRun{
				ID: new(r.ID), Name: new(r.Name), WorkflowID: new(r.WorkflowID), HeadSHA: new(r.HeadSHA),
				Status: new(r.Status), Conclusion: new(r.Conclusion), HTMLURL: new(r.URL),
			})
		}
		body, err := json.Marshal(out)
		if err != nil {
			return ghfake.Response{Status: http.StatusInternalServerError, Message: err.Error()}, true
		}
		return ghfake.Response{Body: string(body)}, true
	})
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "babysitter.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestCollectFailingPR(t *testing.T) {
	t.Parallel()
	g, _ := newFakePR()
	c, st := g.Client(t), openStore(t)
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	opts := Options{MaxFlakyRetries: 3, Now: func() time.Time { return now }}
	target := Target{Owner: "octo", Name: "hello", Number: 3}

	s, err := Collect(context.Background(), c, st, target, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !s.SnapshotAt.Equal(now) {
		t.Errorf("snapshot_at = %v", s.SnapshotAt)
	}
	pr := s.PR
	if pr.Repo != "octo/hello" || pr.Number != 3 || pr.Title != "Fix the thing" || pr.Author != "alice" ||
		pr.State != "open" || pr.Merged || pr.Closed || pr.HeadSHA != "abc" || pr.HeadBranch != "fix" || pr.HeadRepo != "alice/hello" || pr.BaseBranch != "main" ||
		pr.Mergeable != nil || pr.MergeableState != "blocked" || pr.ReviewDecision != "approved" {
		t.Errorf("pr = %+v", pr)
	}
	ch := s.Checks
	if ch.Status != "failure" || ch.PassedCount != 1 || ch.FailedCount != 1 || ch.PendingCount != 0 || !ch.AllTerminal || len(ch.Items) != 2 {
		t.Errorf("checks = %+v", ch)
	}
	if len(s.FailedRuns) != 1 || s.FailedRuns[0].RunID != 100 || s.FailedRuns[0].WorkflowName != "ci" {
		t.Errorf("failed_runs = %+v", s.FailedRuns)
	}
	if len(s.AwaitingApproval) != 2 || s.AwaitingApproval[0].RunID != 103 || s.AwaitingApproval[0].WorkflowName != "deploy" ||
		s.AwaitingApproval[1].RunID != 104 || s.AwaitingApproval[1].Status != "waiting" {
		t.Errorf("awaiting_approval = %+v", s.AwaitingApproval)
	}
	if len(s.FailedJobs) != 2 || s.FailedJobs[0].JobID != 1000 || s.FailedJobs[0].JobName != "test" ||
		s.FailedJobs[0].LogsEndpoint != "repos/octo/hello/actions/jobs/1000/logs" ||
		s.FailedJobs[1].JobID != 1040 || s.FailedJobs[1].RunStatus != "waiting" {
		t.Errorf("failed_jobs = %+v", s.FailedJobs)
	}
	var got []string
	for _, it := range s.NewReviewItems {
		got = append(got, fmt.Sprintf("%s:%d", it.Kind, it.ID))
	}
	if want := "issue_comment:10,review:6,review_comment:31,issue_comment:20"; strings.Join(got, ",") != want {
		t.Errorf("new_review_items = %v, want %s", got, want)
	}
	if it := s.NewReviewItems[2]; it.Path != "internal/x.go" || it.Line == nil || *it.Line != 12 || it.Author != "bob" || it.AuthorAssociation != "MEMBER" {
		t.Errorf("review comment = %+v", it)
	}
	if it := s.NewReviewItems[2]; it.Side != "RIGHT" || it.CommitID != "old" {
		t.Errorf("an outdated comment keeps the commit of its original line: side %q, commit %q", it.Side, it.CommitID)
	}
	if want := "process_review_comment,diagnose_ci_failure,retry_failed_checks,stop_action_required"; strings.Join(s.Actions, ",") != want {
		t.Errorf("actions = %v, want %s", s.Actions, want)
	}
	if s.RetryState != (RetryState{0, 3}) {
		t.Errorf("retry_state = %+v", s.RetryState)
	}

	s, err = Collect(context.Background(), c, st, target, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.NewReviewItems) != 4 {
		t.Errorf("uncommitted second new_review_items = %+v", s.NewReviewItems)
	}
	if err := s.Commit(context.Background(), st); err != nil {
		t.Fatal(err)
	}

	s, err = Collect(context.Background(), c, st, Target{Owner: "Octo", Name: "HELLO", Number: 3}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if s.PR.Repo != "octo/hello" || len(s.NewReviewItems) != 0 {
		t.Errorf("second snapshot: repo = %q, new_review_items = %+v", s.PR.Repo, s.NewReviewItems)
	}
	if want := "diagnose_ci_failure,retry_failed_checks,stop_action_required"; strings.Join(s.Actions, ",") != want {
		t.Errorf("second actions = %v, want %s", s.Actions, want)
	}

	key := store.WatchKey{Owner: "octo", Name: "hello", Number: 3}
	record := opts
	record.RecordRetry = true
	s, err = Collect(context.Background(), c, st, target, record)
	if err != nil {
		t.Fatal(err)
	}
	if s.RetryState.CurrentSHARetriesUsed != 1 {
		t.Errorf("uncommitted retry_state = %+v", s.RetryState)
	}
	if n, _ := st.RetryCount(context.Background(), key, "abc"); n != 0 {
		t.Errorf("uncommitted retry was stored: %d", n)
	}
	for want := 1; want <= 2; want++ {
		s, err = Collect(context.Background(), c, st, target, record)
		if err != nil {
			t.Fatal(err)
		}
		if s.RetryState.CurrentSHARetriesUsed != want || !strings.Contains(strings.Join(s.Actions, ","), "retry_failed_checks") {
			t.Errorf("retry %d: actions = %v, retry_state = %+v", want, s.Actions, s.RetryState)
		}
		if err := s.Commit(context.Background(), st); err != nil {
			t.Fatal(err)
		}
	}
	s, err = Collect(context.Background(), c, st, target, record)
	if err != nil {
		t.Fatal(err)
	}
	if want := "diagnose_ci_failure,stop_exhausted_retries,stop_action_required"; strings.Join(s.Actions, ",") != want || s.RetryState.CurrentSHARetriesUsed != 3 {
		t.Errorf("exhausted actions = %v, retry_state = %+v", s.Actions, s.RetryState)
	}
	if err := s.Commit(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.RetryCount(context.Background(), key, "abc"); n != 3 {
		t.Errorf("stored retries = %d, want 3", n)
	}
}

func TestCollectReadyAndMerged(t *testing.T) {
	t.Parallel()
	g, pr := newGreenPR()
	c, st := g.Client(t), openStore(t)
	target := Target{Owner: "octo", Name: "hello", Number: 3}

	s, err := Collect(context.Background(), c, st, target, Options{MaxFlakyRetries: 3})
	if err != nil {
		t.Fatal(err)
	}
	if want := "process_review_comment"; strings.Join(s.Actions, ",") != want {
		t.Fatalf("first actions = %v, want %s", s.Actions, want)
	}
	if err := s.Commit(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	s, err = Collect(context.Background(), c, st, target, Options{MaxFlakyRetries: 3})
	if err != nil {
		t.Fatal(err)
	}
	if want := "ready_to_merge"; strings.Join(s.Actions, ",") != want {
		t.Fatalf("second actions = %v, want %s", s.Actions, want)
	}
	if s.PR.Mergeable == nil || !*s.PR.Mergeable || s.Checks.Status != "success" || len(s.FailedJobs) != 0 {
		t.Fatalf("snapshot = %+v", s)
	}

	g.Update(func() {
		pr.State, pr.Merged, pr.Mergeable, pr.MergeableState = "closed", true, nil, "unknown"
	})
	s, err = Collect(context.Background(), c, st, target, Options{MaxFlakyRetries: 3})
	if err != nil {
		t.Fatal(err)
	}
	if want := "stop_pr_closed"; strings.Join(s.Actions, ",") != want || s.PR.State != "merged" || !s.PR.Merged || !s.PR.Closed {
		t.Fatalf("merged = %+v, actions %v", s.PR, s.Actions)
	}
}

func TestCollectResolvesFromGit(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	g, pr := newFakePR()
	// The branch fix of the clone below pushes to octo/hello, not to a fork.
	pr.HeadRepo = "octo/hello"
	c, st := g.Client(t), openStore(t)
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"remote", "add", "origin", "https://github.com/octo/hello.git"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	_, err := Collect(context.Background(), c, st, Target{}, Options{Dir: dir})
	if err == nil || !strings.Contains(err.Error(), `no pull request for branch "main" on octo/hello`) {
		t.Fatalf("Collect on main err = %v", err)
	}

	cmd := exec.Command("git", "checkout", "-q", "-b", "fix")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	s, err := Collect(context.Background(), c, st, Target{}, Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if s.PR.Number != 3 || s.PR.Repo != "octo/hello" {
		t.Fatalf("pr = %+v", s.PR)
	}

	s, err = Collect(context.Background(), c, st, Target{Number: 3}, Options{Dir: dir})
	if err != nil || s.PR.Repo != "octo/hello" {
		t.Fatalf("number only: %+v, %v", s, err)
	}

	_, err = Collect(context.Background(), c, st, Target{Number: 3}, Options{Dir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "resolve repository from git") {
		t.Fatalf("outside repo err = %v", err)
	}
}

func gitInit(t *testing.T, dir string, remotes map[string]string) {
	t.Helper()
	args := [][]string{
		{"init", "-q", "-b", "main"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"},
		{"checkout", "-q", "-b", "fix"},
	}
	for name, url := range remotes {
		args = append(args, []string{"remote", "add", name, url})
	}
	for _, a := range args {
		cmd := exec.Command("git", a...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
}

func TestResolveFork(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	g := ghfake.New()
	g.Repo("octo/hello")
	g.PR("up/hello", 7).HeadRepo = "octo/hello"
	c := g.Client(t)
	ctx := context.Background()
	want := Target{Owner: "up", Name: "hello", Number: 7}

	dir := t.TempDir()
	gitInit(t, dir, map[string]string{"origin": "git@github.com:octo/hello.git", "upstream": "https://github.com/up/hello.git"})
	if got, err := resolve(ctx, c, Target{}, Options{Dir: dir}); err != nil || got != want {
		t.Fatalf("resolve with upstream = %+v, %v, want %+v", got, err, want)
	}
	if got, err := resolve(ctx, c, Target{Owner: "up", Name: "hello"}, Options{Dir: dir}); err != nil || got != want {
		t.Fatalf("resolve with --repo = %+v, %v, want %+v", got, err, want)
	}
	if got, err := resolve(ctx, c, want, Options{Dir: t.TempDir()}); err != nil || got != want {
		t.Fatalf("resolve full target = %+v, %v, want %+v", got, err, want)
	}

	dir = t.TempDir()
	gitInit(t, dir, map[string]string{"origin": "git@github.com:octo/hello.git"})
	if _, err := resolve(ctx, c, Target{}, Options{Dir: dir}); err == nil || !strings.Contains(err.Error(), `no pull request for branch "fix" on octo/hello`) {
		t.Fatalf("resolve without upstream err = %v", err)
	}
}

func TestCollectAfterCall(t *testing.T) {
	t.Parallel()
	g, _ := newFakePR()
	c, st := g.Client(t), openStore(t)
	target := Target{Owner: "octo", Name: "hello", Number: 3}

	var mu sync.Mutex
	calls := 0
	opts := Options{AfterCall: func(_ context.Context, resp *github.Response, err error) error {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if resp == nil {
			t.Error("AfterCall got a nil response")
		}
		return err
	}}
	if _, err := Collect(context.Background(), c, st, target, opts); err != nil {
		t.Fatal(err)
	}
	if calls < 7 {
		t.Fatalf("AfterCall ran %d times, want at least 7", calls)
	}

	stop := errors.New("stop here")
	opts.AfterCall = func(context.Context, *github.Response, error) error { return stop }
	if _, err := Collect(context.Background(), c, st, target, opts); !errors.Is(err, stop) {
		t.Fatalf("Collect() error = %v, want %v", err, stop)
	}
}

func TestReviewerLoginsDropsALoginThatIsDrafting(t *testing.T) {
	t.Parallel()
	review := func(id int64, login, state string) *github.PullRequestReview {
		return &github.PullRequestReview{
			ID:    new(id),
			State: new(state),
			User:  &github.User{Login: new(login)},
		}
	}
	got := indexReviews([]*github.PullRequestReview{
		review(1, "bob", "APPROVED"),
		review(2, "bob", "PENDING"),
		review(3, "carol", "COMMENTED"),
		review(4, "Carol", "PENDING"),
		review(5, "dave", "APPROVED"),
	}).behindHead("alice", "head")
	if strings.Join(got, ",") != "dave" {
		t.Fatalf("behindHead() = %v, want dave alone", got)
	}
}

func TestReviewerLogins(t *testing.T) {
	t.Parallel()
	review := func(id int64, login, state string) *github.PullRequestReview {
		return &github.PullRequestReview{
			ID:    new(id),
			State: new(state),
			User:  &github.User{Login: new(login)},
		}
	}
	got := indexReviews([]*github.PullRequestReview{
		review(1, "bob", "APPROVED"),
		review(2, "bob", "COMMENTED"),
		review(3, "dave", "PENDING"),
		review(4, "Alice", "CHANGES_REQUESTED"),
		review(5, "carol", "DISMISSED"),
	}).behindHead("alice", "head")
	if strings.Join(got, ",") != "bob,carol" {
		t.Fatalf("behindHead() = %v, want bob and carol once each", got)
	}
}

func TestCollectLeavesTheIgnoredAuthorOutOfTheSeenState(t *testing.T) {
	t.Parallel()
	g, _ := newFakePR()
	c, st := g.Client(t), openStore(t)
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	target := Target{Owner: "octo", Name: "hello", Number: 3}
	key := store.WatchKey{Owner: "octo", Name: "hello", Number: 3}

	opts := Options{MaxFlakyRetries: 3, Now: func() time.Time { return now }, IgnoreAuthor: "Carol"}
	s, err := Collect(context.Background(), c, st, target, opts)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range s.NewReviewItems {
		got = append(got, fmt.Sprintf("%s:%d", it.Kind, it.ID))
	}
	if want := "review:6,review_comment:31"; strings.Join(got, ",") != want {
		t.Fatalf("new_review_items = %v, want %s", got, want)
	}

	if err := s.Commit(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	seen, err := st.SeenReviewItems(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{10, 20} {
		if seen[store.SeenItem{Kind: store.KindIssueComment, ID: id}] {
			t.Errorf("issue comment %d of the ignored author was recorded as seen", id)
		}
	}

	opts.IgnoreAuthor = ""
	s, err = Collect(context.Background(), c, st, target, opts)
	if err != nil {
		t.Fatal(err)
	}
	got = nil
	for _, it := range s.NewReviewItems {
		got = append(got, fmt.Sprintf("%s:%d", it.Kind, it.ID))
	}
	if want := "issue_comment:10,issue_comment:20"; strings.Join(got, ",") != want {
		t.Fatalf("second new_review_items = %v, want %s", got, want)
	}
}

func TestReviewerLoginsDropsALoginThatReviewedTheHead(t *testing.T) {
	t.Parallel()
	review := func(id int64, login, state, sha string) *github.PullRequestReview {
		r := &github.PullRequestReview{
			ID:    new(id),
			State: new(state),
			User:  &github.User{Login: new(login)},
		}
		if sha != "" {
			r.CommitID = new(sha)
		}
		return r
	}
	got := indexReviews([]*github.PullRequestReview{
		review(1, "bob", "CHANGES_REQUESTED", "old"),
		review(2, "bob", "APPROVED", "head"),
		review(3, "carol", "APPROVED", "head"),
		review(4, "Carol", "CHANGES_REQUESTED", "old"),
		review(5, "dave", "COMMENTED", "old"),
		review(6, "erin", "COMMENTED", ""),
	}).behindHead("alice", "head")
	if strings.Join(got, ",") != "carol,dave,erin" {
		t.Fatalf("behindHead() = %v, want carol, dave and erin", got)
	}
}

func TestReviewerLoginsKeepsAReviewerWhoAsksForChangesOnTheHead(t *testing.T) {
	t.Parallel()
	review := func(id int64, login, state, commit string) *github.PullRequestReview {
		return &github.PullRequestReview{
			ID:       new(id),
			State:    new(state),
			CommitID: new(commit),
			User:     &github.User{Login: new(login)},
		}
	}
	got := indexReviews([]*github.PullRequestReview{
		review(1, "bob", "CHANGES_REQUESTED", "head"),
		review(2, "carol", "APPROVED", "head"),
	}).behindHead("alice", "head")
	if strings.Join(got, ",") != "bob" {
		t.Fatalf("behindHead() = %v, want bob alone", got)
	}
}

func TestCollectCarriesTheTeamAndBotReviewRequests(t *testing.T) {
	t.Parallel()
	g, pr := newFakePR()
	pr.RequestedTeams = []string{"platform-reviewers", "copilot-pull-request-reviewer"}
	c, st := g.Client(t), openStore(t)
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	opts := Options{MaxFlakyRetries: 3, Now: func() time.Time { return now }}

	s, err := Collect(context.Background(), c, st, Target{Owner: "octo", Name: "hello", Number: 3}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.PR.RequestedReviewers, ",") != "copilot-pull-request-reviewer,platform-reviewers" {
		t.Fatalf("requested reviewers = %v", s.PR.RequestedReviewers)
	}
}

func TestCollectCountsTheThreadsOfTheIgnoredAuthorOut(t *testing.T) {
	t.Parallel()
	g, pr := newFakePR()
	pr.Threads = []ghfake.Thread{{Authors: []string{"carol"}}, {Authors: []string{"bob"}}}
	c, st := g.Client(t), openStore(t)
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	opts := Options{MaxFlakyRetries: 3, Now: func() time.Time { return now }, IgnoreAuthor: "Carol"}

	s, err := Collect(context.Background(), c, st, Target{Owner: "octo", Name: "hello", Number: 3}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if s.Threads.Unresolved != 1 {
		t.Fatalf("unresolved = %d, want the thread of carol left out", s.Threads.Unresolved)
	}
}

func TestCollectCountsTheThreadsTheTokenLoginAnswered(t *testing.T) {
	t.Parallel()
	g, pr := newFakePR()
	pr.Threads = []ghfake.Thread{
		{Authors: []string{"bob", "carol"}, LastCommentID: 100},
		{Authors: []string{"bob"}, LastCommentID: 200},
		{Authors: []string{"carol", "bob"}, LastCommentID: 300},
		{Authors: []string{"dave", "carol"}, LastCommentID: 400},
	}
	c, st := g.Client(t), openStore(t)
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	opts := Options{MaxFlakyRetries: 3, Now: func() time.Time { return now }, TokenLogin: "Carol"}

	s, err := Collect(context.Background(), c, st, Target{Owner: "octo", Name: "hello", Number: 3}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if s.Threads.Unresolved != 4 || s.Threads.Unanswered != 2 {
		t.Fatalf("threads = %+v, want 4 unresolved and the 2 carol did not write last in unanswered", s.Threads)
	}
	if strings.Join(s.Threads.Reviewers, ",") != "bob" {
		t.Fatalf("reviewers = %v, want bob alone: dave wrote in an answered thread but still drafts his review", s.Threads.Reviewers)
	}
	if s.Threads.LastAnswer != 400 {
		t.Fatalf("last answer = %d, want 400, the last comment carol wrote", s.Threads.LastAnswer)
	}
}

func TestRequestableLeavesOutWhoCannotBeAsked(t *testing.T) {
	t.Parallel()
	review := func(login, state string) *github.PullRequestReview {
		return &github.PullRequestReview{State: new(state), User: &github.User{Login: new(login)}}
	}
	reviews := []*github.PullRequestReview{
		review("alice", "COMMENTED"),
		review("bob", "COMMENTED"),
		review("carol", "COMMENTED"),
		review("carol", "PENDING"),
		review("Copilot-Pull-Request-Reviewer[bot]", "COMMENTED"),
	}
	logins := []string{"alice", "bob", "carol", "copilot-pull-request-reviewer[bot]", "dave", "github-advanced-security[bot]"}

	got := indexReviews(reviews).requestable(logins, "Alice")
	if strings.Join(got, ",") != "bob,copilot-pull-request-reviewer[bot]" {
		t.Fatalf("requestable() = %v, want bob and copilot: alice wrote the pull request, carol drafts a review, dave and the scanner never reviewed", got)
	}
}

func TestRequestableNamesEachLoginOnceInOrder(t *testing.T) {
	t.Parallel()
	review := func(login string) *github.PullRequestReview {
		return &github.PullRequestReview{State: new("COMMENTED"), User: &github.User{Login: new(login)}}
	}
	reviews := []*github.PullRequestReview{review("bob"), review("carol")}

	got := indexReviews(reviews).requestable([]string{"carol", "Bob", "bob", "carol"}, "alice")
	if strings.Join(got, ",") != "Bob,carol" {
		t.Fatalf("requestable() = %v, want Bob and carol once each, in order", got)
	}
}

func TestWithRequestsMergesTheLoginsOnce(t *testing.T) {
	t.Parallel()
	got := withRequests([]string{"carol", "bob"}, []string{"", "Bob", "platform-reviewers"})
	if strings.Join(got, ",") != "bob,carol,platform-reviewers" {
		t.Fatalf("withRequests() = %v, want bob, carol and the team once each, in order", got)
	}
}

func TestReviewerLoginsKeepsAChangeRequestACommentFollowed(t *testing.T) {
	t.Parallel()
	review := func(id int64, login, state, commit string) *github.PullRequestReview {
		return &github.PullRequestReview{
			ID:       new(id),
			State:    new(state),
			CommitID: new(commit),
			User:     &github.User{Login: new(login)},
		}
	}
	for _, state := range []string{"COMMENTED", "DISMISSED"} {
		got := indexReviews([]*github.PullRequestReview{
			review(1, "bob", "CHANGES_REQUESTED", "head"),
			review(2, "bob", state, "head"),
		}).behindHead("alice", "head")
		if strings.Join(got, ",") != "bob" {
			t.Fatalf("behindHead() with a %s review after the change request = %v, want bob", state, got)
		}
	}
	got := indexReviews([]*github.PullRequestReview{
		review(1, "bob", "APPROVED", "head"),
		review(2, "bob", "COMMENTED", "head"),
	}).behindHead("alice", "head")
	if len(got) != 0 {
		t.Fatalf("behindHead() after an approval = %v, want nobody", got)
	}
}
