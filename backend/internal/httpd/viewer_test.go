package httpd

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

func TestViewerAnswersTheAccountAndAsksGitHubOnce(t *testing.T) {
	calls := 0
	h := NewRouter(Deps{Log: testutil.Logger(t), Bus: events.NewBus(), Viewer: func(context.Context) (Viewer, error) {
		calls++
		return Viewer{Login: "octocat", Name: "Mona Lisa", AvatarURL: "https://example.test/mona.png"}, nil
	}})

	var got Viewer
	if rec := call(t, h, http.MethodGet, "/viewer", "", &got); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := Viewer{Login: "octocat", Name: "Mona Lisa", AvatarURL: "https://example.test/mona.png"}
	if got != want {
		t.Fatalf("viewer = %+v, want %+v", got, want)
	}

	call(t, h, http.MethodGet, "/viewer", "", &got)
	if calls != 1 {
		t.Fatalf("GitHub asked %d times, want 1", calls)
	}
}

func TestViewerIsUnavailableWithoutAClient(t *testing.T) {
	h := NewRouter(Deps{Log: testutil.Logger(t), Bus: events.NewBus()})

	rec := call(t, h, http.MethodGet, "/viewer", "", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if code := errorCode(t, rec); code != "viewer_unavailable" {
		t.Fatalf("code = %q, want viewer_unavailable", code)
	}
}

func TestViewerIsUnavailableWhenGitHubRefuses(t *testing.T) {
	h := NewRouter(Deps{Log: testutil.Logger(t), Bus: events.NewBus(), Viewer: func(context.Context) (Viewer, error) {
		return Viewer{}, errors.New("get current user: GET https://api.github.com/user?access_token=ghp_0123456789abcdefghijABCDEFGH: 401 Bad credentials")
	}})

	rec := call(t, h, http.MethodGet, "/viewer", "", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "ghp_0123456789abcdefghijABCDEFGH") {
		t.Fatalf("body leaks the token: %s", body)
	}
}

func TestViewerAnswersWhenTheLookupTellsAnAuthChange(t *testing.T) {
	bus := events.NewBus()
	h := NewRouter(Deps{Log: testutil.Logger(t), Bus: bus, Viewer: func(context.Context) (Viewer, error) {
		bus.Publish(events.AuthChanged, "", 0)
		return Viewer{Login: "octocat"}, nil
	}})

	done := make(chan int, 1)
	go func() { done <- call(t, h, http.MethodGet, "/viewer", "", nil).Code }()

	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GET /viewer hangs when the token lookup tells an auth change")
	}
}
