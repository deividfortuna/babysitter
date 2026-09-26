package ghclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

type etagAPI struct {
	mu       sync.Mutex
	etag     string
	body     string
	status   int
	header   map[string]string
	requests int
	full     int
	notMod   int
	seen     []string
}

func (a *etagAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests++
	a.seen = append(a.seen, r.Header.Get("If-None-Match"))
	for k, v := range a.header {
		w.Header().Set(k, v)
	}
	if a.status != 0 {
		http.Error(w, `{"message":"Not Found"}`, a.status)
		return
	}
	if a.etag != "" {
		w.Header().Set("ETag", a.etag)
	}
	if match := r.Header.Get("If-None-Match"); match != "" && match == a.etag {
		a.notMod++
		w.WriteHeader(http.StatusNotModified)
		return
	}
	a.full++
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, a.body)
}

func (a *etagAPI) set(etag, body string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.etag, a.body = etag, body
}

func (a *etagAPI) counts() (requests, full, notMod int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.requests, a.full, a.notMod
}

func cachedClient(t *testing.T) (*github.Client, *etagAPI) {
	t.Helper()
	api := &etagAPI{etag: `W/"one"`, body: `[{"number":1}]`}
	c := ghfake.Serve(t, api).Client(t, github.WithTransport(&cachingTransport{cache: newResponseCache(maxCacheEntries, maxCacheTotalBytes)}))
	return c, api
}

func TestCacheReplaysUnchangedAnswer(t *testing.T) {
	c, api := cachedClient(t)
	ctx := context.Background()

	first, _, err := ListOpenPulls(ctx, c, "o", "r")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, resp, err := ListOpenPulls(ctx, c, "o", "r")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if len(first) != 1 || len(second) != 1 || second[0].GetNumber() != 1 {
		t.Fatalf("pull requests = %v and %v, want one each", first, second)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	requests, full, notMod := api.counts()
	if requests != 2 || full != 1 || notMod != 1 {
		t.Fatalf("requests = %d, with a body = %d, not modified = %d; want 2, 1, 1", requests, full, notMod)
	}
	if api.seen[0] != "" || api.seen[1] != `W/"one"` {
		t.Fatalf("validators sent = %q, want the second call to carry the etag", api.seen)
	}
}

func TestCacheReadsAgainWhenTheAnswerChanged(t *testing.T) {
	c, api := cachedClient(t)
	ctx := context.Background()

	if _, _, err := ListOpenPulls(ctx, c, "o", "r"); err != nil {
		t.Fatal(err)
	}
	api.set(`W/"two"`, `[{"number":1},{"number":2}]`)
	prs, _, err := ListOpenPulls(ctx, c, "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 {
		t.Fatalf("pull requests = %d, want 2", len(prs))
	}
	if _, full, notMod := api.counts(); full != 2 || notMod != 0 {
		t.Fatalf("answers with a body = %d, not modified = %d; want 2 and 0", full, notMod)
	}

	if _, _, err := ListOpenPulls(ctx, c, "o", "r"); err != nil {
		t.Fatal(err)
	}
	if _, _, notMod := api.counts(); notMod != 1 {
		t.Fatalf("not modified = %d, want 1", notMod)
	}
}

func TestCacheKeepsTheRateLimitOfTheLastAnswer(t *testing.T) {
	c, api := cachedClient(t)
	api.mu.Lock()
	api.header = map[string]string{"X-RateLimit-Limit": "5000", "X-RateLimit-Remaining": "4999"}
	api.mu.Unlock()
	ctx := context.Background()

	if _, _, err := ListOpenPulls(ctx, c, "o", "r"); err != nil {
		t.Fatal(err)
	}
	api.mu.Lock()
	api.header["X-RateLimit-Remaining"] = "4998"
	api.mu.Unlock()
	_, resp, err := ListOpenPulls(ctx, c, "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Rate.Remaining != 4998 {
		t.Fatalf("remaining = %d, want the value of the 304 (4998)", resp.Rate.Remaining)
	}
}

func TestCacheKeepsThePaginationLinks(t *testing.T) {
	api := &etagAPI{etag: `W/"one"`, body: `{"total_count":1,"check_runs":[{"id":1}]}`}
	srv := ghfake.Serve(t, api)
	api.mu.Lock()
	api.header = map[string]string{"Link": `<` + srv.URL + `/next>; rel="next"`}
	api.mu.Unlock()
	tr := &cachingTransport{cache: newResponseCache(maxCacheEntries, maxCacheTotalBytes)}

	get := func() *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	first := get()
	io.Copy(io.Discard, first.Body)
	first.Body.Close()
	second := get()
	defer second.Body.Close()

	if got := second.Header.Get("Link"); got != first.Header.Get("Link") {
		t.Fatalf("Link on the replay = %q, want %q", got, first.Header.Get("Link"))
	}
	if got := second.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type on the replay = %q, want application/json", got)
	}
	body, _ := io.ReadAll(second.Body)
	if !strings.Contains(string(body), `"check_runs"`) {
		t.Fatalf("replayed body = %q", body)
	}
}

func TestCacheSkipsAnswersWithoutAValidator(t *testing.T) {
	c, api := cachedClient(t)
	api.set("", `[{"number":1}]`)
	ctx := context.Background()

	for range 2 {
		if _, _, err := ListOpenPulls(ctx, c, "o", "r"); err != nil {
			t.Fatal(err)
		}
	}
	if _, full, notMod := api.counts(); full != 2 || notMod != 0 {
		t.Fatalf("answers with a body = %d, not modified = %d; want 2 and 0", full, notMod)
	}
}

func TestCacheSeparatesTokens(t *testing.T) {
	cache := newResponseCache(maxCacheEntries, maxCacheTotalBytes)
	api := &etagAPI{etag: `W/"one"`, body: `[]`}
	srv := ghfake.Serve(t, api)

	get := func(token string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := (&cachingTransport{cache: cache}).RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	get("one")
	get("two")
	if _, full, notMod := api.counts(); full != 2 || notMod != 0 {
		t.Fatalf("answers with a body = %d, not modified = %d; want 2 and 0", full, notMod)
	}
	get("one")
	if _, _, notMod := api.counts(); notMod != 1 {
		t.Fatalf("not modified = %d, want the second read of the first token to hit", notMod)
	}
}

func TestCacheDropsTheLeastRecentlyUsedAnswer(t *testing.T) {
	c := newResponseCache(2, maxCacheTotalBytes)
	c.put("a", entry{etag: "a"})
	c.put("b", entry{etag: "b"})
	if _, ok := c.get("a"); !ok {
		t.Fatal("a is gone, want it held")
	}
	c.put("c", entry{etag: "c"})
	if _, ok := c.get("b"); ok {
		t.Fatal("b is held, want the least recently used one dropped")
	}
	for _, key := range []string{"a", "c"} {
		if _, ok := c.get(key); !ok {
			t.Fatalf("%s is gone, want it held", key)
		}
	}
}

func TestCacheDropsAnswersOverTheByteBudget(t *testing.T) {
	c := newResponseCache(maxCacheEntries, 20)
	c.put("a", entry{body: make([]byte, 12)})
	c.put("b", entry{body: make([]byte, 12)})
	if _, ok := c.get("a"); ok {
		t.Fatal("a is held, want it dropped for the budget")
	}
	if c.bytes != 12 {
		t.Fatalf("cache holds %d bytes, want 12", c.bytes)
	}

	c.remove("b")
	if c.bytes != 0 {
		t.Fatalf("cache holds %d bytes after the last answer went, want 0", c.bytes)
	}
}

func TestCacheCountsAReplacedAnswerOnce(t *testing.T) {
	c := newResponseCache(maxCacheEntries, maxCacheTotalBytes)
	c.put("a", entry{body: make([]byte, 30)})
	c.put("a", entry{body: make([]byte, 10)})
	if c.bytes != 10 {
		t.Fatalf("cache holds %d bytes, want the size of the answer it kept (10)", c.bytes)
	}
}

func TestCacheForgetsAnAnswerThatIsGone(t *testing.T) {
	c, api := cachedClient(t)
	ctx := context.Background()
	if _, _, err := ListOpenPulls(ctx, c, "o", "r"); err != nil {
		t.Fatal(err)
	}

	api.mu.Lock()
	api.status = http.StatusNotFound
	api.mu.Unlock()
	if _, _, err := ListOpenPulls(ctx, c, "o", "r"); !IsNotFound(err) {
		t.Fatalf("second call error = %v, want not found", err)
	}

	api.mu.Lock()
	api.status = 0
	api.mu.Unlock()
	if _, _, err := ListOpenPulls(ctx, c, "o", "r"); err != nil {
		t.Fatalf("third call: %v", err)
	}
	if _, full, notMod := api.counts(); full != 2 || notMod != 0 {
		t.Fatalf("answers with a body = %d, not modified = %d; want 2 and 0", full, notMod)
	}
	if api.seen[2] != "" {
		t.Fatalf("the third call sent %q, want no validator after the answer was gone", api.seen[2])
	}
}

type diskStore struct {
	mu   sync.Mutex
	rows map[string]CachedResponse
	err  error
}

func newDiskStore() *diskStore {
	return &diskStore{rows: map[string]CachedResponse{}}
}

func (d *diskStore) CachedResponse(_ context.Context, key string) (CachedResponse, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return CachedResponse{}, false, d.err
	}
	r, ok := d.rows[key]
	return r, ok, nil
}

func (d *diskStore) PutCachedResponse(_ context.Context, key string, r CachedResponse) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.rows[key] = r
	return nil
}

func (d *diskStore) DeleteCachedResponse(_ context.Context, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	delete(d.rows, key)
	return nil
}

func (d *diskStore) len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.rows)
}

func storedClient(t *testing.T, srv *ghfake.Server, disk ResponseStore) *github.Client {
	t.Helper()
	cache := newResponseCache(maxCacheEntries, maxCacheTotalBytes)
	cache.disk = disk
	return srv.Client(t, github.WithTransport(&cachingTransport{cache: cache}))
}

func TestCacheReadsTheAnswerAnEarlierProcessKept(t *testing.T) {
	api := &etagAPI{etag: `W/"one"`, body: `[{"number":1}]`}
	srv := ghfake.Serve(t, api)
	disk := newDiskStore()
	ctx := context.Background()

	if _, _, err := ListOpenPulls(ctx, storedClient(t, srv, disk), "o", "r"); err != nil {
		t.Fatal(err)
	}
	if disk.len() != 1 {
		t.Fatalf("the store holds %d answers, want the first one", disk.len())
	}
	prs, _, err := ListOpenPulls(ctx, storedClient(t, srv, disk), "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 1 || prs[0].GetNumber() != 1 {
		t.Fatalf("pull requests = %v, want the one the store kept", prs)
	}
	if _, full, notMod := api.counts(); full != 1 || notMod != 1 {
		t.Fatalf("answers with a body = %d, not modified = %d; want 1 and 1", full, notMod)
	}
	if api.seen[1] != `W/"one"` {
		t.Fatalf("the new process sent %q, want the etag the store kept", api.seen[1])
	}
}

func TestCacheWritesTheNewAnswerToTheStore(t *testing.T) {
	api := &etagAPI{etag: `W/"one"`, body: `[{"number":1}]`}
	srv := ghfake.Serve(t, api)
	disk := newDiskStore()
	ctx := context.Background()

	if _, _, err := ListOpenPulls(ctx, storedClient(t, srv, disk), "o", "r"); err != nil {
		t.Fatal(err)
	}
	api.set(`W/"two"`, `[{"number":1},{"number":2}]`)
	if _, _, err := ListOpenPulls(ctx, storedClient(t, srv, disk), "o", "r"); err != nil {
		t.Fatal(err)
	}
	prs, _, err := ListOpenPulls(ctx, storedClient(t, srv, disk), "o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 {
		t.Fatalf("pull requests = %d, want the 2 of the new answer", len(prs))
	}
	if api.seen[2] != `W/"two"` {
		t.Fatalf("the third process sent %q, want the etag of the new answer", api.seen[2])
	}
}

func TestCacheDropsAnAnswerThatIsGoneFromTheStore(t *testing.T) {
	api := &etagAPI{etag: `W/"one"`, body: `[{"number":1}]`}
	srv := ghfake.Serve(t, api)
	disk := newDiskStore()
	ctx := context.Background()

	if _, _, err := ListOpenPulls(ctx, storedClient(t, srv, disk), "o", "r"); err != nil {
		t.Fatal(err)
	}
	api.mu.Lock()
	api.status = http.StatusNotFound
	api.mu.Unlock()
	if _, _, err := ListOpenPulls(ctx, storedClient(t, srv, disk), "o", "r"); !IsNotFound(err) {
		t.Fatalf("second call error = %v, want not found", err)
	}
	if disk.len() != 0 {
		t.Fatalf("the store holds %d answers, want none after the answer was gone", disk.len())
	}
}

func TestCacheReadsGitHubWhenTheStoreFails(t *testing.T) {
	api := &etagAPI{etag: `W/"one"`, body: `[{"number":1}]`}
	srv := ghfake.Serve(t, api)
	disk := newDiskStore()
	disk.err = errors.New("database is locked")
	c := storedClient(t, srv, disk)
	ctx := context.Background()

	for range 2 {
		prs, _, err := ListOpenPulls(ctx, c, "o", "r")
		if err != nil || len(prs) != 1 {
			t.Fatalf("pull requests = %v, %v; want the answer of GitHub", prs, err)
		}
	}
	if _, full, notMod := api.counts(); full != 1 || notMod != 1 {
		t.Fatalf("answers with a body = %d, not modified = %d; want the memory to hold the answer", full, notMod)
	}
}

func TestKeepResponsesFeedsTheClientsOfNew(t *testing.T) {
	api := &etagAPI{etag: `W/"one"`, body: `[{"number":1}]`}
	srv := ghfake.Serve(t, api)
	t.Cleanup(func() { KeepResponses(nil) })
	disk := newDiskStore()
	ctx := context.Background()
	call := func() {
		t.Helper()
		c := srv.Client(t, github.WithTransport(sharedTransport()))
		if _, _, err := ListOpenPulls(ctx, c, "o", "r"); err != nil {
			t.Fatal(err)
		}
	}

	KeepResponses(disk)
	call()
	KeepResponses(disk)
	call()

	if disk.len() != 1 {
		t.Fatalf("the store holds %d answers, want 1", disk.len())
	}
	if api.seen[1] != `W/"one"` {
		t.Fatalf("the second process sent %q, want the etag the store kept", api.seen[1])
	}
}

func TestAClientBuiltBeforeKeepResponsesKeepsItsAnswers(t *testing.T) {
	api := &etagAPI{etag: `W/"one"`, body: `[{"number":1}]`}
	srv := ghfake.Serve(t, api)
	t.Cleanup(func() { KeepResponses(nil) })
	disk := newDiskStore()
	c := srv.Client(t, github.WithTransport(sharedTransport()))

	KeepResponses(disk)
	if _, _, err := ListOpenPulls(context.Background(), c, "o", "r"); err != nil {
		t.Fatal(err)
	}

	if disk.len() != 1 {
		t.Fatalf("the store holds %d answers, want the answer of the client built before", disk.len())
	}
}
