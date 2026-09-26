package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestListWorkflowRunsAndJobs(t *testing.T) {
	t.Parallel()
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("head_sha") != "abc" {
			http.Error(w, "want head_sha=abc", http.StatusBadRequest)
			return
		}
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/api/v3/repos/o/r/actions/runs?head_sha=abc&page=2>; rel="next"`, base))
			fmt.Fprint(w, `{"total_count":2,"workflow_runs":[{"id":1,"head_sha":"abc"}]}`)
		case "2":
			fmt.Fprint(w, `{"total_count":2,"workflow_runs":[{"id":2,"head_sha":"abc"}]}`)
		}
	})
	mux.HandleFunc("/api/v3/repos/o/r/actions/runs/1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter") != "latest" {
			http.Error(w, "want filter=latest", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"total_count":1,"jobs":[{"id":10,"run_id":1,"name":"test","conclusion":"failure"}]}`)
	})
	srv := ghfake.Serve(t, mux)
	base = srv.URL
	c := srv.Client(t)
	ctx := context.Background()

	runs, _, err := ListWorkflowRuns(ctx, c, "o", "r", "abc")
	if err != nil {
		t.Fatalf("ListWorkflowRuns() error = %v", err)
	}
	if len(runs) != 2 || runs[1].GetID() != 2 {
		t.Fatalf("runs = %v", runs)
	}
	jobs, _, err := ListWorkflowJobs(ctx, c, "o", "r", 1)
	if err != nil {
		t.Fatalf("ListWorkflowJobs() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].GetID() != 10 || jobs[0].GetConclusion() != "failure" {
		t.Fatalf("jobs = %v", jobs)
	}
}
