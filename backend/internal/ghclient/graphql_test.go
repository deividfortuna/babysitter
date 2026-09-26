package ghclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestFetchReviewStateCountsThreadsAcrossPages(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	var afters []any
	mux.HandleFunc("/api/v3/graphql", func(w http.ResponseWriter, r *http.Request) {
		var req graphqlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Variables["number"] != float64(5) {
			http.Error(w, "want number=5", http.StatusBadRequest)
			return
		}
		afters = append(afters, req.Variables["after"])
		if req.Variables["after"] == nil {
			fmt.Fprint(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":true},{"isResolved":false}],"pageInfo":{"hasNextPage":true,"endCursor":"c1"}}}}}}`)
			return
		}
		fmt.Fprint(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":false}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	got, resp, err := FetchReviewState(context.Background(), c, "o", "r", 5)
	if err != nil {
		t.Fatalf("FetchReviewState() error = %v", err)
	}
	if n := got.Unresolved(""); n != 2 || resp == nil {
		t.Fatalf("unresolved = %d, resp = %v", n, resp)
	}
	if len(afters) != 2 || afters[1] != "c1" {
		t.Fatalf("cursors = %v", afters)
	}
}

func TestUnresolvedKeepsAThreadAnybodyElseWroteIn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		authors []string
		want    int
	}{
		{"the ignored login alone", []string{"alice"}, 0},
		{"the ignored login, answered by a reviewer", []string{"alice", "bob"}, 1},
		{"a reviewer, answered by the ignored login", []string{"bob", "alice"}, 1},
		{"nobody the payload names", nil, 1},
		{"another login alone", []string{"bob"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			state := ReviewState{Threads: []ReviewThread{{Authors: c.authors}}}

			if n := state.Unresolved("Alice"); n != c.want {
				t.Fatalf("Unresolved() = %d, want %d for %v", n, c.want, c.authors)
			}
		})
	}
}

func TestUnansweredLeavesOutTheThreadsTheLoginWroteLastIn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		ignore string
		thread ReviewThread
		want   int
	}{
		{"a reviewer, answered by the login", "Alice", ReviewThread{Authors: []string{"bob", "alice"}, LastAuthor: "alice"}, 0},
		{"a reviewer, answered, asked again", "Alice", ReviewThread{Authors: []string{"bob", "alice", "bob"}, LastAuthor: "bob"}, 1},
		{"the login, answered by a reviewer", "Alice", ReviewThread{Authors: []string{"alice", "bob"}, LastAuthor: "bob"}, 1},
		{"a reviewer alone", "Alice", ReviewThread{Authors: []string{"bob"}, LastAuthor: "bob"}, 1},
		{"a reviewer after the first page of comments", "Alice", ReviewThread{Authors: []string{"bob", "alice"}, LastAuthor: "bob"}, 1},
		{"a deleted account after the login", "Alice", ReviewThread{Authors: []string{"bob", "alice"}, LastAuthor: ""}, 1},
		{"the login alone, left out", "Alice", ReviewThread{Authors: []string{"alice"}, LastAuthor: "alice"}, 0},
		{"the login alone, counted", "", ReviewThread{Authors: []string{"alice"}, LastAuthor: "alice"}, 1},
		{"resolved after the answer", "Alice", ReviewThread{Resolved: true, Authors: []string{"bob", "alice"}, LastAuthor: "alice"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			state := ReviewState{Threads: []ReviewThread{c.thread}}

			if n := state.Unanswered(c.ignore, "Alice"); n != c.want {
				t.Fatalf("Unanswered() = %d, want %d for %+v", n, c.want, c.thread)
			}
		})
	}
}

func TestAnsweredReviewersNamesWhoWroteInTheAnsweredThreads(t *testing.T) {
	t.Parallel()
	state := ReviewState{Threads: []ReviewThread{
		{Authors: []string{"carol", "alice"}, LastAuthor: "Alice"},
		{Authors: []string{"bob", "alice"}, LastAuthor: "alice"},
		{Authors: []string{"bob", "dave", "alice"}, LastAuthor: "alice"},
		{Authors: []string{"erin", "alice", "erin"}, LastAuthor: "erin"},
		{Resolved: true, Authors: []string{"frank", "alice"}, LastAuthor: "alice"},
		{Authors: []string{"alice"}, LastAuthor: "alice"},
	}}

	got := state.AnsweredReviewers("Alice")
	if strings.Join(got, ",") != "carol,bob,bob,dave" {
		t.Fatalf("AnsweredReviewers() = %v, want carol, bob, bob, dave in thread order", got)
	}
}

func TestLastAnswerIDIsTheNewestAnswerOfTheLogin(t *testing.T) {
	t.Parallel()
	state := ReviewState{Threads: []ReviewThread{
		{Authors: []string{"bob", "alice"}, LastAuthor: "alice", LastCommentID: 10},
		{Authors: []string{"carol", "alice"}, LastAuthor: "Alice", LastCommentID: 30},
		{Authors: []string{"dave"}, LastAuthor: "dave", LastCommentID: 50},
		{Resolved: true, Authors: []string{"erin", "alice"}, LastAuthor: "alice", LastCommentID: 70},
	}}

	if id := state.LastAnswerID("alice"); id != 30 {
		t.Fatalf("LastAnswerID() = %d, want 30", id)
	}
	if id := (ReviewState{}).LastAnswerID("alice"); id != 0 {
		t.Fatalf("LastAnswerID() of no thread = %d, want 0", id)
	}
}

func TestFetchReviewStateReadsTheLastCommentOfAThread(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	var query string
	mux.HandleFunc("/api/v3/graphql", func(w http.ResponseWriter, r *http.Request) {
		var req graphqlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		query = req.Query
		fmt.Fprint(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[`+
			`{"isResolved":false,"comments":{"nodes":[{"author":{"__typename":"Bot","login":"copilot-pull-request-reviewer"}},{"author":{"__typename":"User","login":"alice"}}]},"lastComment":{"nodes":[{"databaseId":41,"author":{"__typename":"User","login":"alice"}}]}},`+
			`{"isResolved":false,"comments":{"nodes":[{"author":{"__typename":"User","login":"bob"}},{"author":{"__typename":"User","login":"alice"}}]},"lastComment":{"nodes":[{"author":null}]}}`+
			`],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	got, _, err := FetchReviewState(context.Background(), c, "o", "r", 5)
	if err != nil {
		t.Fatalf("FetchReviewState() error = %v", err)
	}
	if !strings.Contains(query, "lastComment: comments(last: 1)") {
		t.Fatalf("query does not ask for the last comment on its own:\n%s", query)
	}
	if n := got.Unanswered("alice", "alice"); n != 1 {
		t.Fatalf("unanswered = %d, want the thread a deleted account wrote last in", n)
	}
	if who := got.AnsweredReviewers("alice"); strings.Join(who, ",") != "copilot-pull-request-reviewer[bot]" {
		t.Fatalf("answered reviewers = %v, want the bot in its REST form", who)
	}
	if !strings.Contains(query, "lastComment: comments(last: 1) { nodes { databaseId") {
		t.Fatalf("query does not ask for the id of the last comment:\n%s", query)
	}
	if id := got.LastAnswerID("alice"); id != 41 {
		t.Fatalf("last answer = %d, want 41", id)
	}
}

func TestFetchReviewStateReadsEveryAuthorOfAThread(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/graphql", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[{"isResolved":false,"comments":{"nodes":[{"author":{"login":"alice"}},{"author":{"login":"bob"}}]}}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	got, _, err := FetchReviewState(context.Background(), c, "o", "r", 5)
	if err != nil {
		t.Fatalf("FetchReviewState() error = %v", err)
	}
	if n := got.Unresolved("alice"); n != 1 {
		t.Fatalf("unresolved = %d, want the thread bob answered", n)
	}
	if strings.Join(got.Threads[0].Authors, ",") != "alice,bob" {
		t.Fatalf("authors = %v", got.Threads[0].Authors)
	}
}

func TestFetchReviewStateReportsGraphQLErrors(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/graphql", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":null,"errors":[{"message":"Resource not accessible by integration"}]}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	_, _, err := FetchReviewState(context.Background(), c, "o", "r", 5)
	if err == nil || err.Error() != "review threads o/r#5: Resource not accessible by integration" {
		t.Fatalf("err = %v", err)
	}
}

func TestMergePullSendsMethodAndSHA(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/pulls/5/merge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "want PUT", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body["merge_method"] != "squash" || body["sha"] != "abc" {
			http.Error(w, fmt.Sprintf("body = %v", body), http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"sha":"def","merged":true,"message":"Pull Request successfully merged"}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	res, _, err := MergePull(context.Background(), c, "o", "r", 5, "squash", "abc")
	if err != nil {
		t.Fatalf("MergePull() error = %v", err)
	}
	if !res.GetMerged() || res.GetSHA() != "def" {
		t.Fatalf("result = %+v", res)
	}
}

func TestMergePullTakesAnAnswerThatMergedNothingForARefusal(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/pulls/5/merge", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"sha":"","merged":false,"message":"Base branch was modified. Review and try the merge again."}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	_, _, err := MergePull(context.Background(), c, "o", "r", 5, "squash", "abc")
	if !errors.Is(err, ErrNotMerged) || !IsRefused(err) {
		t.Fatalf("MergePull() error = %v, want a refusal", err)
	}
	if !strings.Contains(err.Error(), "Base branch was modified") {
		t.Fatalf("the error drops the reason of GitHub: %v", err)
	}
}

func TestMergePullRefused(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/o/r/pulls/5/merge", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprint(w, `{"message":"Pull Request is not mergeable"}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	_, _, err := MergePull(context.Background(), c, "o", "r", 5, "squash", "abc")
	if !IsRefused(err) {
		t.Fatalf("IsRefused(%v) = false", err)
	}
	if IsRefused(fmt.Errorf("network down")) {
		t.Fatal("a plain error is not a refusal")
	}
}

func TestMergeMethodsFollowsTheRepository(t *testing.T) {
	t.Parallel()
	all := &github.Repository{AllowSquashMerge: new(true), AllowMergeCommit: new(true), AllowRebaseMerge: new(true)}
	if got := fmt.Sprint(MergeMethods(all)); got != "[squash merge rebase]" {
		t.Fatalf("all = %s", got)
	}
	rebaseOnly := &github.Repository{AllowRebaseMerge: new(true)}
	if got := fmt.Sprint(MergeMethods(rebaseOnly)); got != "[rebase]" {
		t.Fatalf("rebase only = %s", got)
	}
	if got := MergeMethods(&github.Repository{}); len(got) != 0 {
		t.Fatalf("none = %v", got)
	}
}

func TestReviewStateCarriesTeamAndBotRequests(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/graphql", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"repository":{"pullRequest":{
			"reviewRequests":{"nodes":[
				{"requestedReviewer":{"__typename":"Team","slug":"platform-reviewers"}},
				{"requestedReviewer":{"__typename":"Bot","login":"copilot-pull-request-reviewer"}},
				{"requestedReviewer":{"__typename":"User","login":"bob"}}]},
			"reviewThreads":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	got, _, err := FetchReviewState(context.Background(), c, "o", "r", 5)
	if err != nil {
		t.Fatalf("FetchReviewState() error = %v", err)
	}
	want := "platform-reviewers,copilot-pull-request-reviewer,bob"
	if strings.Join(got.Requested, ",") != want {
		t.Fatalf("requested = %v, want %s", got.Requested, want)
	}
}

func TestReviewStateCarriesTheAuthorOfEachThread(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/graphql", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"repository":{"pullRequest":{
			"reviewRequests":{"nodes":[]},
			"reviewThreads":{"nodes":[
				{"isResolved":false,"comments":{"nodes":[{"author":{"login":"carol"}}]}},
				{"isResolved":true,"comments":{"nodes":[{"author":{"login":"bob"}}]}}],
			"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	got, _, err := FetchReviewState(context.Background(), c, "o", "r", 5)
	if err != nil {
		t.Fatalf("FetchReviewState() error = %v", err)
	}
	if len(got.Threads) != 2 || strings.Join(got.Threads[0].Authors, ",") != "carol" || got.Threads[0].Resolved ||
		strings.Join(got.Threads[1].Authors, ",") != "bob" || !got.Threads[1].Resolved {
		t.Fatalf("threads = %+v", got.Threads)
	}
}
