package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func updaterOf(onGitHub any) string {
	if onGitHub == true {
		return "github"
	}
	return "agent"
}

func (d *fakeDaemon) mergeRulesRoute() {
	d.mux.HandleFunc("PATCH /api/v1/watches/1", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)
		d.decisions = append(d.decisions, "merge-rules "+string(raw))
		approvals, method, whenReady, update, onGitHub := any(1), any("squash"), any(false), any("rebase"), any(true)
		if v, ok := body["branchUpdate"]; ok {
			update = v
		}
		if v, ok := body["updateOnGitHub"]; ok {
			onGitHub = v
		}
		if v, ok := body["mergeWhenReady"]; ok {
			whenReady = v
		}
		if v, ok := body["approvalsRequired"]; ok {
			approvals = v
		}
		if approvals == nil {
			approvals = 2
		}
		if v, ok := body["mergeMethod"]; ok {
			method = v
		}
		one := strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]")
		fmt.Fprint(w, strings.Replace(one, `"status":"active"`, fmt.Sprintf(`"status":"active","approvalsRequired":%v,"mergeMethod":%q,"mergeWhenReady":%v,"branchUpdate":%q,"updateOnGitHub":%v,"branchUpdater":%q`, approvals, method, whenReady, update, onGitHub, updaterOf(onGitHub)), 1))
	})
}

func TestWatchMergeRulesChangesARunningWatch(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.mergeRulesRoute()

	out, err := runWatch(t, d, "merge-rules", "1", "--approvals", "0", "--merge-method", "rebase")
	if err != nil || out != "Watch 1 needs no approval before it is ready to merge, and merges with rebase. You merge it. A branch behind its base: rebase, on GitHub first.\n" {
		t.Fatalf("merge-rules = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "merge-rules", "1", "--approvals", "branch")
	if err != nil || out != "Watch 1 needs 2 approvals before it is ready to merge, and merges with squash. You merge it. A branch behind its base: rebase, on GitHub first.\n" {
		t.Fatalf("merge-rules --approvals branch = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "merge-rules", "1", "--merge-method", "")
	if err != nil || out != "Watch 1 needs 1 approval before it is ready to merge, and merges with the first method the repository allows. You merge it. A branch behind its base: rebase, on GitHub first.\n" {
		t.Fatalf("merge-rules --merge-method '' = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "merge-rules", "1", "--merge-when-ready")
	if err != nil || !strings.Contains(out, "The daemon merges it as soon as it is ready.") {
		t.Fatalf("merge-rules --merge-when-ready = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "merge-rules", "1", "--branch-update", "merge", "--update-on-github=false")
	if err != nil || !strings.HasSuffix(out, "A branch behind its base: merge, by the agent.\n") {
		t.Fatalf("merge-rules --branch-update merge = %q, %v", out, err)
	}
	want := []string{
		`merge-rules {"approvalsRequired":0,"mergeMethod":"rebase"}`,
		`merge-rules {"approvalsRequired":null}`,
		`merge-rules {"mergeMethod":""}`,
		`merge-rules {"mergeWhenReady":true}`,
		`merge-rules {"branchUpdate":"merge","updateOnGitHub":false}`,
	}
	if strings.Join(d.decisions, "\n") != strings.Join(want, "\n") {
		t.Fatalf("bodies = %q, want %q", d.decisions, want)
	}
}

func TestWatchMergeRulesNeedsSomethingToChange(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.mergeRulesRoute()

	if _, err := runWatch(t, d, "merge-rules", "1"); err == nil || !strings.Contains(err.Error(), "--approvals") {
		t.Fatalf("merge-rules without a flag = %v, want the flags named", err)
	}
	if _, err := runWatch(t, d, "merge-rules", "1", "--approvals", "two"); err == nil {
		t.Fatal("merge-rules --approvals two went through")
	}
	if len(d.decisions) != 0 {
		t.Fatalf("bodies = %q, want none", d.decisions)
	}
}
