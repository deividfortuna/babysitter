package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func (d *fakeDaemon) mergeRulesRoute() {
	d.mux.HandleFunc("PATCH /api/v1/watches/1", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)
		d.decisions = append(d.decisions, "merge-rules "+string(raw))
		approvals, method := any(1), any("squash")
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
		fmt.Fprint(w, strings.Replace(one, `"status":"active"`, fmt.Sprintf(`"status":"active","approvalsRequired":%v,"mergeMethod":%q`, approvals, method), 1))
	})
}

func TestWatchMergeRulesChangesARunningWatch(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.mergeRulesRoute()

	out, err := runWatch(t, d, "merge-rules", "1", "--approvals", "0", "--merge-method", "rebase")
	if err != nil || out != "Watch 1 needs no approval before it is ready to merge, and merges with rebase.\n" {
		t.Fatalf("merge-rules = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "merge-rules", "1", "--approvals", "branch")
	if err != nil || out != "Watch 1 needs 2 approvals before it is ready to merge, and merges with squash.\n" {
		t.Fatalf("merge-rules --approvals branch = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "merge-rules", "1", "--merge-method", "")
	if err != nil || out != "Watch 1 needs 1 approval before it is ready to merge, and merges with the first method the repository allows.\n" {
		t.Fatalf("merge-rules --merge-method '' = %q, %v", out, err)
	}
	want := []string{
		`merge-rules {"approvalsRequired":0,"mergeMethod":"rebase"}`,
		`merge-rules {"approvalsRequired":null}`,
		`merge-rules {"mergeMethod":""}`,
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
