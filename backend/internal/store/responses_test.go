package store

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient"
)

var _ ghclient.ResponseStore = (*Store)(nil)

func sampleResponse(etag, body string) ghclient.CachedResponse {
	return ghclient.CachedResponse{
		ETag:         etag,
		LastModified: "Thu, 24 Sep 2026 04:00:00 GMT",
		Status:       http.StatusOK,
		Header:       http.Header{"Content-Type": {"application/json"}, "Link": {`<https://api.github.com/x?page=2>; rel="next"`}},
		Body:         []byte(body),
	}
}

func TestCachedResponseOutlivesTheStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, path := openTemp(t)

	if _, ok, err := s.CachedResponse(ctx, "k"); err != nil || ok {
		t.Fatalf("CachedResponse() before a put = %v, %v; want a miss", ok, err)
	}
	want := sampleResponse(`W/"one"`, `[{"number":1}]`)
	if err := s.PutCachedResponse(ctx, "k", want); err != nil {
		t.Fatal(err)
	}
	s.Close()

	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	got, ok, err := again.CachedResponse(ctx, "k")
	if err != nil || !ok {
		t.Fatalf("CachedResponse() after a reopen = %v, %v; want a hit", ok, err)
	}
	if got.ETag != want.ETag || got.LastModified != want.LastModified || got.Status != want.Status || string(got.Body) != string(want.Body) {
		t.Fatalf("CachedResponse() = %+v, want %+v", got, want)
	}
	if got.Header.Get("Content-Type") != "application/json" || got.Header.Get("Link") != want.Header.Get("Link") {
		t.Fatalf("header = %v, want %v", got.Header, want.Header)
	}
}

func TestPutCachedResponseReplacesTheAnswer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := openTemp(t)

	if err := s.PutCachedResponse(ctx, "k", sampleResponse(`W/"one"`, "old")); err != nil {
		t.Fatal(err)
	}
	if err := s.PutCachedResponse(ctx, "k", sampleResponse(`W/"two"`, "new")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.CachedResponse(ctx, "k")
	if err != nil || !ok || got.ETag != `W/"two"` || string(got.Body) != "new" {
		t.Fatalf("CachedResponse() = %+v, %v, %v; want the second answer", got, ok, err)
	}
}

func TestDeleteCachedResponse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := openTemp(t)

	if err := s.PutCachedResponse(ctx, "k", sampleResponse(`W/"one"`, "x")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCachedResponse(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCachedResponse(ctx, "absent"); err != nil {
		t.Fatalf("DeleteCachedResponse() of a missing key = %v, want nil", err)
	}
	if _, ok, err := s.CachedResponse(ctx, "k"); err != nil || ok {
		t.Fatalf("CachedResponse() after a delete = %v, %v; want a miss", ok, err)
	}
}

func cachedKeys(t *testing.T, s *Store) []string {
	t.Helper()
	var out []string
	for _, k := range []string{"a", "b", "c", "d"} {
		if _, ok, err := s.CachedResponse(context.Background(), k); err != nil {
			t.Fatal(err)
		} else if ok {
			out = append(out, k)
		}
	}
	return out
}

func TestCachedResponsesDropTheOldestPastTheCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := openTemp(t)
	s.responseCap = 2

	for _, k := range []string{"a", "b", "a", "c"} {
		if err := s.PutCachedResponse(ctx, k, sampleResponse(`W/"`+k+`"`, k)); err != nil {
			t.Fatal(err)
		}
	}
	if got := cachedKeys(t, s); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("kept = %v, want [a c]: b is the one written longest ago", got)
	}
}

func TestCachedResponsesDropTheOldestPastTheBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := openTemp(t)
	s.responseBytesCap = 10

	for _, k := range []string{"a", "b", "c"} {
		if err := s.PutCachedResponse(ctx, k, sampleResponse(`W/"`+k+`"`, "12345")); err != nil {
			t.Fatal(err)
		}
	}
	if got := cachedKeys(t, s); !slices.Equal(got, []string{"b", "c"}) {
		t.Fatalf("kept = %v, want [b c]: two bodies of 5 bytes fill 10", got)
	}
}

func TestCachedResponsesKeepTheNewestAnswerOverTheBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := openTemp(t)
	s.responseBytesCap = 4

	if err := s.PutCachedResponse(ctx, "a", sampleResponse(`W/"a"`, "12345")); err != nil {
		t.Fatal(err)
	}
	if got := cachedKeys(t, s); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("kept = %v, want [a]: the answer just written stays", got)
	}
}
