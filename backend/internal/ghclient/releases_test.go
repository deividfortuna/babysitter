package ghclient

import (
	"context"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

func TestLatestReleaseGivesTheTagAndThePage(t *testing.T) {
	t.Parallel()
	g := ghfake.New()
	g.Release("o/r", &github.RepositoryRelease{TagName: "v0.3.0", HTMLURL: "https://github.com/o/r/releases/tag/v0.3.0"})
	c := g.Client(t)

	got, err := LatestRelease(context.Background(), c, "o", "r")
	if err != nil {
		t.Fatalf("LatestRelease() error = %v", err)
	}
	want := Release{Tag: "v0.3.0", URL: "https://github.com/o/r/releases/tag/v0.3.0"}
	if got != want {
		t.Fatalf("LatestRelease() = %+v, want %+v", got, want)
	}
}

func TestLatestReleaseFailsWhenTheRepositoryHasNoRelease(t *testing.T) {
	t.Parallel()
	c := ghfake.New().Client(t)

	if _, err := LatestRelease(context.Background(), c, "o", "r"); err == nil {
		t.Fatal("LatestRelease() error = nil, want the 404")
	}
}
