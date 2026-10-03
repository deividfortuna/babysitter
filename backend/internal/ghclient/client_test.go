package ghclient

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestCurrentUser(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/user", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"login":"octocat","name":"The Octocat"}`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	u, err := CurrentUser(context.Background(), c)
	if err != nil {
		t.Fatalf("CurrentUser() error = %v", err)
	}
	if u.GetLogin() != "octocat" {
		t.Fatalf("login = %q, want %q", u.GetLogin(), "octocat")
	}
}

func twoPageRepos(t *testing.T) (http.Handler, *int, *string) {
	t.Helper()
	var (
		calls int
		base  string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/user/repos", func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Query().Get("page") {
		case "", "1":
			w.Header().Set("Link", fmt.Sprintf(`<%s/api/v3/user/repos?page=2>; rel="next"`, base))
			fmt.Fprint(w, `[{"full_name":"a/one"},{"full_name":"a/two"}]`)
		case "2":
			fmt.Fprint(w, `[{"full_name":"a/three"}]`)
		default:
			http.Error(w, "unexpected page", http.StatusBadRequest)
		}
	})
	return mux, &calls, &base
}

func TestListReposFollowsPagination(t *testing.T) {
	t.Parallel()
	h, calls, base := twoPageRepos(t)
	srv := ghfake.Serve(t, h)
	*base = srv.URL
	c := srv.Client(t)

	repos, err := ListRepos(context.Background(), c, ListReposOptions{})
	if err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("len(repos) = %d, want 3", len(repos))
	}
	if repos[2].GetFullName() != "a/three" {
		t.Fatalf("last repo = %q, want %q", repos[2].GetFullName(), "a/three")
	}
	if *calls != 2 {
		t.Fatalf("requests = %d, want 2", *calls)
	}
}

func TestListReposStopsAtLimit(t *testing.T) {
	t.Parallel()
	h, calls, base := twoPageRepos(t)
	srv := ghfake.Serve(t, h)
	*base = srv.URL
	c := srv.Client(t)

	repos, err := ListRepos(context.Background(), c, ListReposOptions{Limit: 1})
	if err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if len(repos) != 1 || repos[0].GetFullName() != "a/one" {
		t.Fatalf("repos = %v, want only a/one", repos)
	}
	if *calls != 1 {
		t.Fatalf("requests = %d, want 1", *calls)
	}
}

func TestListReposSendsSmallPageForSmallLimit(t *testing.T) {
	t.Parallel()
	var perPage string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/user/repos", func(w http.ResponseWriter, r *http.Request) {
		perPage = r.URL.Query().Get("per_page")
		fmt.Fprint(w, `[]`)
	})
	c := ghfake.Serve(t, mux).Client(t)

	if _, err := ListRepos(context.Background(), c, ListReposOptions{Limit: 5}); err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}
	if perPage != "5" {
		t.Fatalf("per_page = %q, want 5", perPage)
	}
}
