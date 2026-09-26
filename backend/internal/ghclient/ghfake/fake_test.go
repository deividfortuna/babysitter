package ghfake_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestAPullRequestReadsBackAsTheStateHoldsIt(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	pr := g.PR("octo/hello", 3)
	pr.Draft, pr.Labels, pr.Requested = true, []string{"bug"}, []string{"bob"}
	c := g.Client(t)

	got, _, err := c.PullRequests.Get(context.Background(), "Octo", "HELLO", 3)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetNumber() != 3 || got.GetState() != "open" || !got.GetDraft() || got.GetHead().GetSHA() != "abc" ||
		got.GetUser().GetLogin() != "alice" || got.GetBase().GetRef() != "main" || got.GetMergeableState() != "clean" ||
		len(got.Labels) != 1 || got.Labels[0].Name != "bug" || got.RequestedReviewers[0].GetLogin() != "bob" {
		t.Fatalf("pull request = %+v", got)
	}
	if n := g.Count(ghfake.RoutePull); n != 1 {
		t.Fatalf("Count(RoutePull) = %d, want 1", n)
	}
}

func TestAMergeClosesThePullRequest(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	pr := g.PR("octo/hello", 3)
	c := g.Client(t)
	ctx := context.Background()

	if _, _, err := c.PullRequests.Merge(ctx, "octo", "hello", 3, "", &github.PullRequestOptions{MergeMethod: "squash", SHA: "abc"}); err != nil {
		t.Fatal(err)
	}
	got, _, err := c.PullRequests.Get(ctx, "octo", "hello", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !got.GetMerged() || got.GetState() != "closed" {
		t.Fatalf("after the merge: merged %v, state %s", got.GetMerged(), got.GetState())
	}
	var merges []ghfake.Merge
	g.Update(func() { merges = slices.Clone(pr.Merges) })
	if len(merges) != 1 || merges[0].Method != "squash" || merges[0].SHA != "abc" {
		t.Fatalf("merges = %+v", merges)
	}
	if _, _, err := c.PullRequests.Merge(ctx, "octo", "hello", 3, "", nil); err == nil {
		t.Fatal("a second merge of a closed pull request passed")
	}
}

func TestAMergeOfAnOldHeadConflicts(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.PR("octo/hello", 3)
	c := g.Client(t)

	_, resp, err := c.PullRequests.Merge(context.Background(), "octo", "hello", 3, "", &github.PullRequestOptions{SHA: "old"})
	if err == nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("Merge() = %v, %v, want a 409", resp, err)
	}
}

func TestAPostedCommentShowsInTheNextList(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.PR("octo/hello", 3)
	c := g.Client(t)
	ctx := context.Background()

	posted, _, err := c.Issues.CreateComment(ctx, "octo", "hello", 3, github.IssueCommentRequest{Body: "on it"})
	if err != nil {
		t.Fatal(err)
	}
	list, _, err := c.Issues.ListComments(ctx, "octo", "hello", 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].GetID() != posted.GetID() || list[0].GetBody() != "on it" || list[0].GetUser().GetLogin() != "alice" {
		t.Fatalf("comments = %+v, want the posted one", list)
	}
	again, _, err := c.Issues.GetComment(ctx, "octo", "hello", posted.GetID())
	if err != nil || again.GetIssueURL() != "https://api.github.com/repos/octo/hello/issues/3" {
		t.Fatalf("GetComment() = %+v, %v", again, err)
	}
	if calls := g.Calls(ghfake.RouteComment); len(calls) != 1 || string(calls[0].Body) == "" {
		t.Fatalf("Calls(RouteComment) = %+v", calls)
	}
}

func TestAReplyToADeletedCommentIsNotFound(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.PR("octo/hello", 3)
	g.Repo("octo/hello").Deleted = []int64{31}
	c := g.Client(t)

	_, resp, err := c.PullRequests.CreateCommentInReplyTo(context.Background(), "octo", "hello", 3, "done", 31)
	if err == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("reply = %v, %v, want a 404", resp, err)
	}
	if _, _, err := c.PullRequests.GetComment(context.Background(), "octo", "hello", 31); err == nil {
		t.Fatal("GetComment() found a deleted comment")
	}
}

func TestReactorsAnswerInPlaceOfTheStateNewestFirst(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.PR("octo/hello", 3)
	g.Fail(ghfake.RoutePull, http.StatusBadGateway, "Bad Gateway")
	g.React(ghfake.RoutePull, func(a ghfake.Action) (ghfake.Response, bool) {
		return ghfake.Response{Status: http.StatusNotFound, Message: "gone"}, a.Vars["number"] == "3"
	})
	c := g.Client(t)

	var ghErr *github.ErrorResponse
	_, _, err := c.PullRequests.Get(context.Background(), "octo", "hello", 3)
	if !errors.As(err, &ghErr) || ghErr.Response.StatusCode != http.StatusNotFound || ghErr.Message != "gone" {
		t.Fatalf("Get(3) error = %v, want the 404 of the newest reactor", err)
	}
	_, _, err = c.PullRequests.Get(context.Background(), "octo", "hello", 4)
	if !errors.As(err, &ghErr) || ghErr.Response.StatusCode != http.StatusBadGateway {
		t.Fatalf("Get(4) error = %v, want the 502 of the older reactor", err)
	}
}

func TestObserveSeesEveryRequest(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	var seen []string
	g.Observe(func(a ghfake.Action) { seen = append(seen, a.Route) })
	c := g.Client(t)

	if _, _, err := c.Users.Get(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	_, _, _ = c.Repositories.Get(context.Background(), "octo", "none")
	if !slices.Equal(seen, []string{ghfake.RouteUser, ghfake.RouteRepo}) {
		t.Fatalf("seen = %v", seen)
	}
}

func TestTheRateLimitHeadersGoOnEveryResponse(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	reset := time.Date(2026, 9, 7, 13, 0, 0, 0, time.UTC)
	g.SetRate(&ghfake.Rate{Limit: 5000, Remaining: 3, Reset: reset})
	c := g.Client(t)

	_, resp, err := c.Users.Get(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Rate.Remaining != 3 || !resp.Rate.Reset.Time.Equal(reset) {
		t.Fatalf("rate = %+v", resp.Rate)
	}
}

func TestChecksRunsAndLogsFollowTheHead(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	pr := g.PR("octo/hello", 3)
	pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Name: "build", Status: "completed", Conclusion: "failure", CheckSuiteID: 501}}
	g.Repo("octo/hello").Runs = []*ghfake.Run{{
		ID: 77, Name: "CI", CheckSuiteID: 501, Status: "completed", Conclusion: "failure",
		Jobs: []*ghfake.Job{{ID: 9, Name: "build", Status: "completed", Conclusion: "failure", Log: "boom"}},
	}}
	c := g.Client(t)
	ctx := context.Background()

	checks, _, err := c.Checks.ListCheckRunsForRef(ctx, "octo", "hello", "abc", nil)
	if err != nil || checks.GetTotal() != 1 || checks.CheckRuns[0].GetCheckSuite().GetID() != 501 {
		t.Fatalf("check runs = %+v, %v", checks, err)
	}
	if other, _, _ := c.Checks.ListCheckRunsForRef(ctx, "octo", "hello", "old", nil); other.GetTotal() != 0 {
		t.Fatalf("check runs of another commit = %d, want 0", other.GetTotal())
	}
	runs, _, err := c.Actions.ListRepositoryWorkflowRuns(ctx, "octo", "hello", &github.ListWorkflowRunsOptions{HeadSHA: "abc"})
	if err != nil || runs.GetTotalCount() != 1 || runs.WorkflowRuns[0].GetHeadSHA() != "abc" {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	jobs, _, err := c.Actions.ListWorkflowJobs(ctx, "octo", "hello", 77, nil)
	if err != nil || jobs.GetTotalCount() != 1 || jobs.Jobs[0].GetRunID() != 77 {
		t.Fatalf("jobs = %+v, %v", jobs, err)
	}
	u, _, err := c.Actions.GetWorkflowJobLogs(ctx, "octo", "hello", 9, 0)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if body, _ := io.ReadAll(resp.Body); string(body) != "boom" {
		t.Fatalf("log = %q", body)
	}
}

func TestTheReviewStateComesFromGraphQL(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	pr := g.PR("octo/hello", 3)
	pr.RequestedTeams = []string{"core"}
	pr.Threads = []ghfake.Thread{
		{Authors: []string{"bob", "renovate[bot]"}, LastCommentID: 31},
		{Resolved: true, Authors: []string{"carol"}, LastCommentID: 40},
	}
	c := g.Client(t)

	state, _, err := ghclient.FetchReviewState(context.Background(), c, "octo", "hello", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(state.Requested, []string{"core"}) || len(state.Threads) != 2 ||
		state.Threads[0].LastAuthor != "renovate[bot]" || state.Threads[0].LastCommentID != 31 || !state.Threads[1].Resolved {
		t.Fatalf("review state = %+v", state)
	}

	g.GraphQLError("Resource not accessible by integration")
	if _, _, err := ghclient.FetchReviewState(context.Background(), c, "octo", "hello", 3); err == nil {
		t.Fatal("ReviewStateOf() passed on a GraphQL error")
	}
}

func TestServeStartsAnyHandler(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"login":"octocat"}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	u, _, err := c.Users.Get(context.Background(), "")
	if err != nil || u.GetLogin() != "octocat" {
		t.Fatalf("Users.Get() = %+v, %v", u, err)
	}
}
