package cli

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

type validatorsAPI struct {
	mu   sync.Mutex
	seen []string
}

func (a *validatorsAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seen = append(a.seen, r.Header.Get("If-None-Match"))
	w.Header().Set("ETag", `W/"one"`)
	if r.Header.Get("If-None-Match") == `W/"one"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `[{"number":1}]`)
}

func TestACommandFindsTheAnswersOfTheCommandBefore(t *testing.T) {
	t.Cleanup(func() { ghclient.KeepResponses(nil) })
	api := &validatorsAPI{}
	srv := ghfake.Serve(t, api)
	opts := &options{db: filepath.Join(t.TempDir(), "babysitter.db")}
	command := func() {
		t.Helper()
		st, err := opts.openStore()
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		base, err := ghclient.New("token", 0)
		if err != nil {
			t.Fatal(err)
		}
		c := srv.Client(t, github.WithHTTPClient(base.Client()))
		if _, _, err := ghclient.ListOpenPulls(context.Background(), c, "o", "r"); err != nil {
			t.Fatal(err)
		}
	}

	command()
	ghclient.KeepResponses(nil)
	command()

	if len(api.seen) != 2 || api.seen[1] != `W/"one"` {
		t.Fatalf("validators sent = %q, want the second command to send the etag of the first", api.seen)
	}
}
