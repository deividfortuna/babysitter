package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

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
