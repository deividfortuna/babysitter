package ghclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestRequiredApprovalsTakesTheLargerRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		rules      string
		protection int
		body       string
		want       int
	}{
		{"ruleset only, protection unreadable", `[{"type":"pull_request","parameters":{"required_approving_review_count":2}}]`, http.StatusForbidden, `{"message":"Resource not accessible by personal access token"}`, 2},
		{"protection only", `[]`, http.StatusOK, `{"required_pull_request_reviews":{"required_approving_review_count":1}}`, 1},
		{"both, the larger wins", `[{"type":"pull_request","parameters":{"required_approving_review_count":1}},{"type":"deletion"}]`, http.StatusOK, `{"required_pull_request_reviews":{"required_approving_review_count":3}}`, 3},
		{"protection without required reviews", `[]`, http.StatusOK, `{"required_status_checks":{"strict":true,"contexts":["build"]}}`, 0},
		{"ruleset over protection without required reviews", `[{"type":"pull_request","parameters":{"required_approving_review_count":2}}]`, http.StatusOK, `{"enforce_admins":{"enabled":true}}`, 2},
		{"no rule at all", `[]`, http.StatusNotFound, `{"message":"Branch not protected"}`, 0},
		{"rules endpoint missing", "", http.StatusNotFound, `{"message":"Not Found"}`, 0},
		{"private repository on a free plan", "forbidden", http.StatusForbidden, `{"message":"Upgrade to GitHub Pro or make this repository public to enable this feature."}`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v3/repos/o/r/rules/branches/main", func(w http.ResponseWriter, r *http.Request) {
				switch tc.rules {
				case "":
					http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
					return
				case "forbidden":
					http.Error(w, `{"message":"Upgrade to GitHub Pro or make this repository public to enable this feature."}`, http.StatusForbidden)
					return
				}
				fmt.Fprint(w, tc.rules)
			})
			mux.HandleFunc("/api/v3/repos/o/r/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.protection)
				fmt.Fprint(w, tc.body)
			})
			c := ghfake.Serve(t, mux).Client(t)
			got, resp, err := RequiredApprovals(context.Background(), c, "o", "r", "main")
			if err != nil || got != tc.want || resp == nil {
				t.Fatalf("RequiredApprovals() = %d, %v, %v; want %d", got, resp, err, tc.want)
			}
		})
	}
}

func TestRequiredApprovalsReportsARefusedRead(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/rules/branches/main", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[]`)
	})
	mux.HandleFunc("/api/v3/repos/o/r/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Resource not accessible by personal access token"}`, http.StatusForbidden)
	})
	c := ghfake.Serve(t, mux).Client(t)
	n, _, err := RequiredApprovals(context.Background(), c, "o", "r", "main")
	if !errors.Is(err, ErrRulesUnreadable) || n != 0 {
		t.Fatalf("RequiredApprovals() = %d, %v; want the refusal, not no approval", n, err)
	}
}

func TestRequiredApprovalsReportsAFailure(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/rules/branches/main", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})
	c := ghfake.Serve(t, mux).Client(t)
	if _, _, err := RequiredApprovals(context.Background(), c, "o", "r", "main"); err == nil {
		t.Fatal("RequiredApprovals() took a server error for no rule")
	}
}
