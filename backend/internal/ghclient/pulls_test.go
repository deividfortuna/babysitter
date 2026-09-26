package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestListOpenPullsFollowsPagination(t *testing.T) {
	t.Parallel()
	var (
		calls int
		base  string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("state") != "open" {
			http.Error(w, "want state=open", http.StatusBadRequest)
			return
		}
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/api/v3/repos/o/r/pulls?page=2>; rel="next"`, base))
			fmt.Fprint(w, `[{"number":1},{"number":2}]`)
		case "2":
			fmt.Fprint(w, `[{"number":3}]`)
		}
	})
	srv := ghfake.Serve(t, mux)
	base = srv.URL
	c := srv.Client(t)

	prs, resp, err := ListOpenPulls(context.Background(), c, "o", "r")
	if err != nil {
		t.Fatalf("ListOpenPulls() error = %v", err)
	}
	if len(prs) != 3 || prs[2].GetNumber() != 3 {
		t.Fatalf("prs = %v", prs)
	}
	if calls != 2 || resp == nil {
		t.Fatalf("calls = %d, resp = %v", calls, resp)
	}
}

func TestGetPullAndReviewsAndChecks(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	p := g.PR("o/r", 5)
	p.Additions, p.Deletions = 3, 1
	p.Reviews = []ghfake.Review{{ID: 1, State: "APPROVED", Author: "bob"}}
	p.CheckRuns = []ghfake.CheckRun{{ID: 9, Status: "completed", Conclusion: "success"}}
	p.Statuses = []ghfake.Status{{Context: "ci", State: "success"}}
	c := g.Client(t)
	ctx := context.Background()

	pr, _, err := GetPull(ctx, c, "o", "r", 5)
	if err != nil {
		t.Fatalf("GetPull() error = %v", err)
	}
	if pr.GetMergeableState() != "clean" || pr.GetAdditions() != 3 {
		t.Fatalf("pr = %+v", pr)
	}
	reviews, _, err := ListReviews(ctx, c, "o", "r", 5)
	if err != nil {
		t.Fatalf("ListReviews() error = %v", err)
	}
	if len(reviews) != 1 || reviews[0].GetState() != "APPROVED" {
		t.Fatalf("reviews = %v", reviews)
	}
	runs, _, err := ListCheckRuns(ctx, c, "o", "r", "abc")
	if err != nil {
		t.Fatalf("ListCheckRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].GetConclusion() != "success" {
		t.Fatalf("runs = %v", runs)
	}
	if calls := g.Calls(ghfake.RouteCheckRuns); len(calls) != 1 || calls[0].Query.Get("filter") != "latest" {
		t.Fatalf("check run reads = %+v, want one with filter=latest", calls)
	}
	status, _, err := GetCombinedStatus(ctx, c, "o", "r", "abc")
	if err != nil {
		t.Fatalf("GetCombinedStatus() error = %v", err)
	}
	if len(status.Statuses) != 1 || status.Statuses[0].GetState() != "success" {
		t.Fatalf("status = %+v", status)
	}
}

func TestIsNotFound(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/pulls/404", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})
	mux.HandleFunc("/api/v3/repos/o/r/pulls/500", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})
	c := ghfake.Serve(t, mux).Client(t)

	_, _, err := GetPull(context.Background(), c, "o", "r", 404)
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = false, want true", err)
	}
	_, _, err = GetPull(context.Background(), c, "o", "r", 500)
	if err == nil || IsNotFound(err) {
		t.Fatalf("IsNotFound(%v) = true, want false", err)
	}
}

func TestListCommentsAndPullsByHead(t *testing.T) {
	t.Parallel()
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/issues/5/comments", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/api/v3/repos/o/r/issues/5/comments?page=2>; rel="next"`, base))
			fmt.Fprint(w, `[{"id":1,"body":"one"}]`)
		case "2":
			fmt.Fprint(w, `[{"id":2,"body":"two"}]`)
		}
	})
	mux.HandleFunc("/api/v3/repos/o/r/pulls/5/comments", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":3,"path":"a.go","line":4,"pull_request_review_id":9}]`)
	})
	mux.HandleFunc("/api/v3/repos/o/r/pulls", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("head") != "o:fix" || q.Get("state") != "open" {
			http.Error(w, "want head=o:fix and state=open", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `[{"number":5}]`)
	})
	srv := ghfake.Serve(t, mux)
	base = srv.URL
	c := srv.Client(t)
	ctx := context.Background()

	issue, _, err := ListIssueComments(ctx, c, "o", "r", 5)
	if err != nil {
		t.Fatalf("ListIssueComments() error = %v", err)
	}
	if len(issue) != 2 || issue[1].GetID() != 2 {
		t.Fatalf("issue comments = %v", issue)
	}
	review, _, err := ListReviewComments(ctx, c, "o", "r", 5)
	if err != nil {
		t.Fatalf("ListReviewComments() error = %v", err)
	}
	if len(review) != 1 || review[0].GetPullRequestReviewID() != 9 || review[0].GetLine() != 4 {
		t.Fatalf("review comments = %v", review)
	}
	prs, _, err := ListPullsByHead(ctx, c, "o", "r", "o:fix", "open")
	if err != nil {
		t.Fatalf("ListPullsByHead() error = %v", err)
	}
	if len(prs) != 1 || prs[0].GetNumber() != 5 {
		t.Fatalf("prs = %v", prs)
	}
}
