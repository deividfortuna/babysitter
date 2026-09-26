package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestJobLogsFollowsTheRedirectToStorage(t *testing.T) {
	t.Parallel()
	var base string
	var authOnStorage string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/actions/jobs/42/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", base+"/storage/42")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/storage/42", func(w http.ResponseWriter, r *http.Request) {
		authOnStorage = r.Header.Get("Authorization")
		fmt.Fprint(w, "2026-09-18T10:23:45.1234567Z ##[error]it broke\n")
	})
	srv := ghfake.Serve(t, mux)
	base = srv.URL
	c := srv.Client(t)

	body, resp, err := JobLogs(context.Background(), c, "o", "r", 42)
	if err != nil {
		t.Fatalf("JobLogs() error = %v", err)
	}
	if !strings.Contains(body, "##[error]it broke") {
		t.Errorf("body = %q", body)
	}
	if authOnStorage != "" {
		t.Errorf("the download carried the token: %q", authOnStorage)
	}
	if resp == nil || resp.Rate.Remaining != 4999 {
		t.Errorf("resp does not carry the rate limit of the endpoint: %+v", resp)
	}
}

func TestJobLogsReportsAFailedDownload(t *testing.T) {
	t.Parallel()
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/actions/jobs/42/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", base+"/storage/42")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/storage/42", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	})
	srv := ghfake.Serve(t, mux)
	base = srv.URL
	c := srv.Client(t)

	if _, _, err := JobLogs(context.Background(), c, "o", "r", 42); err == nil {
		t.Fatal("JobLogs() error = nil, want the status of the download")
	}
}

func TestJobLogsReportsAFailedEndpoint(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/actions/jobs/42/logs", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	})
	c := ghfake.Serve(t, mux).Client(t)

	if _, _, err := JobLogs(context.Background(), c, "o", "r", 42); err == nil {
		t.Fatal("JobLogs() error = nil, want the error of the endpoint")
	}
}
