package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/snapshot"
)

// snapshotAPI is helloPR with a comment by carol and a failed workflow run
// on the head.
func snapshotAPI() *ghfake.GitHub {
	g, pr := helloPR()
	pr.IssueComments = []ghfake.Comment{{
		ID: 10, Author: "carol", CreatedAt: ghfake.At("2026-09-01T00:00:00Z"), Body: "please rename this\nand that",
		URL: "https://github.com/octo/hello/pull/3#issuecomment-10",
	}}
	g.Repo("octo/hello").Runs = []*ghfake.Run{{
		ID: 100, Name: "ci", HeadSHA: "abc", Status: "completed", Conclusion: "failure",
		URL: "https://github.com/octo/hello/actions/runs/100",
		Jobs: []*ghfake.Job{{
			ID: 1000, Name: "test", Status: "completed", Conclusion: "failure",
			URL: "https://github.com/octo/hello/actions/runs/100/job/1000",
		}},
	}}
	return g
}

func TestPRCommand(t *testing.T) {
	t.Parallel()
	mux := snapshotAPI()
	db := filepath.Join(t.TempDir(), "babysitter.db")

	out, err := runCLI(t, mux, db, "pr", "https://github.com/octo/hello/pull/3", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var s snapshot.Snapshot
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("json %q: %v", out, err)
	}
	if s.PR.Repo != "octo/hello" || s.PR.Number != 3 || !s.PR.Draft || s.PR.ReviewDecision != "changes_requested" ||
		s.Checks.Status != "failure" || len(s.FailedJobs) != 1 || len(s.NewReviewItems) != 2 {
		t.Fatalf("snapshot = %+v", s)
	}
	if want := "process_review_comment,diagnose_ci_failure,retry_failed_checks"; strings.Join(s.Actions, ",") != want {
		t.Fatalf("actions = %v", s.Actions)
	}

	out, err = runCLI(t, mux, db, "pr", "3", "--repo", "octo/hello", "--record-retry")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"octo/hello#3  Fix the thing  by alice",
		"State:      open (draft)   head fix@abc   base main",
		"Mergeable:  unknown / blocked   Review: changes_requested",
		"Checks:     failure   0 passed, 1 failed, 0 pending, 0 skipped",
		"ci / test", "logs: repos/octo/hello/actions/jobs/1000/logs",
		"Actions:    diagnose_ci_failure, retry_failed_checks",
		"Retries:    1 of 3 used for abc",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text output is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "New review items") {
		t.Errorf("second run still shows review items:\n%s", out)
	}

	out, err = runCLI(t, mux, filepath.Join(t.TempDir(), "fresh.db"), "pr", "octo/hello#3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "New review items:") || !regexp.MustCompile(`issue_comment +carol +"please rename this"`).MatchString(out) {
		t.Errorf("fresh run text:\n%s", out)
	}

	for range 2 {
		if _, err := runCLI(t, mux, db, "pr", "octo/hello#3", "--record-retry"); err != nil {
			t.Fatal(err)
		}
	}
	out, err = runCLI(t, mux, db, "pr", "octo/hello#3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Actions:    diagnose_ci_failure, stop_exhausted_retries") || !strings.Contains(out, "Retries:    3 of 3 used for abc") {
		t.Errorf("exhausted text:\n%s", out)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("broken pipe") }

func TestPRCommandKeepsItemsWhenOutputFails(t *testing.T) {
	t.Parallel()
	mux := snapshotAPI()
	db := filepath.Join(t.TempDir(), "babysitter.db")

	if err := runCLIWriter(t, mux, db, failingWriter{}, "pr", "octo/hello#3"); err == nil || !strings.Contains(err.Error(), "broken pipe") {
		t.Fatalf("pr with a broken output err = %v", err)
	}

	out, err := runCLI(t, mux, db, "pr", "octo/hello#3")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "New review items:") {
		t.Errorf("items were hidden by a snapshot that nobody read:\n%s", out)
	}
}

func TestPRCommandRejectsBadInput(t *testing.T) {
	t.Parallel()
	db := filepath.Join(t.TempDir(), "x.db")
	if _, err := runCLI(t, ghfake.New(), db, "pr", "nope"); err == nil || !strings.Contains(err.Error(), "invalid pull request") {
		t.Fatalf("err = %v", err)
	}
	if _, err := runCLI(t, ghfake.New(), db, "pr", "octo/hello#3", "--max-flaky-retries", "-1"); err == nil || !strings.Contains(err.Error(), "max flaky retries") {
		t.Fatalf("err = %v", err)
	}
}
