package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestWatchStartSendsMergeWhenReadyOnlyWhenTyped(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	if _, err := runWatch(t, d, "start", "octo/hello#3"); err != nil {
		t.Fatal(err)
	}
	if _, err := runWatch(t, d, "start", "octo/hello#3", "--merge-when-ready"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.starts[0]["mergeWhenReady"]; ok {
		t.Fatalf("a start without the flag sent mergeWhenReady: %v", d.starts[0])
	}
	if d.starts[1]["mergeWhenReady"] != true {
		t.Fatalf("start = %v, want mergeWhenReady true", d.starts[1])
	}
}

func TestWatchMergeApproveAsksTheDaemonToApprove(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	var bodies []map[string]any
	d.mux.HandleFunc("/api/v1/watches/1/merge", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		if body["approve"] == true && len(bodies) == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			fmt.Fprint(w, `{"error":{"code":"approve_refused","message":"the update is outside the Dependabot merge scope of the repository: a major update, the scope is patch"}}`)
			return
		}
		fmt.Fprint(w, strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]"))
	})
	if _, err := runWatch(t, d, "merge", "1", "--approve"); err == nil || !strings.Contains(err.Error(), "outside the Dependabot merge scope") {
		t.Fatalf("merge --approve out of scope = %v, want the refusal", err)
	}
	if _, err := runWatch(t, d, "merge", "1", "--approve"); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[1]["approve"] != true {
		t.Fatalf("bodies = %v, want approve true", bodies)
	}
}

func TestWatchStatusShowsWhyAutoStartBeganTheWatch(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.watches = strings.Replace(d.watches, `"status":"active"`, `"status":"active","autoReason":"dependabot","updateType":"patch","mergeWhenReady":true`, 1)
	out, err := runWatch(t, d, "status", "1")
	if err != nil || !strings.Contains(out, "Auto:      started on its own: Dependabot opened it; patch update; merges when ready") {
		t.Fatalf("status = %q, %v", out, err)
	}
}
