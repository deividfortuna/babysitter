package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
)

const appBinary = "/Applications/Babysitter.app/Contents/Resources/daemon/babysitter"

func releasesServer(t *testing.T, g *ghfake.GitHub) Option {
	t.Helper()
	srv := g.Serve(t)
	return WithReleaseClientFactory(func() (*github.Client, error) {
		return srv.NewClient()
	})
}

func latestRelease(tag string) *ghfake.GitHub {
	g := ghfake.New()
	g.Release("deividfortuna/babysitter", &github.RepositoryRelease{
		TagName: tag, HTMLURL: "https://github.com/deividfortuna/babysitter/releases/tag/" + tag,
	})
	return g
}

// askedNothing is a GitHub that fails the test on any request.
func askedNothing(t *testing.T, who string) *ghfake.GitHub {
	g := ghfake.New()
	g.Observe(func(a ghfake.Action) { t.Errorf("%s asked GitHub for %s", who, a.Path) })
	return g
}

func runVersion(t *testing.T, opts ...Option) string {
	t.Helper()
	root := NewRootCmd(opts...)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"version"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("version error = %v", err)
	}
	return out.String()
}

func executableAt(path string) Option {
	return WithExecutable(func() (string, error) { return path, nil })
}

func TestVersionFromTheAppTellsHowToUpgrade(t *testing.T) {
	t.Parallel()
	out := runVersion(t, WithVersion("0.2.0"), releasesServer(t, latestRelease("v0.3.0")), executableAt(appBinary))

	want := "babysitter version 0.2.0\n" +
		"babysitter 0.3.0 is out. The desktop app updates itself, or run: brew upgrade --cask babysitter\n"
	if out != want {
		t.Fatalf("version = %q, want %q", out, want)
	}
}

func TestVersionOutsideTheAppPointsAtTheReleasePage(t *testing.T) {
	t.Parallel()
	standalone := filepath.Join(t.TempDir(), "bin", "babysitter")
	out := runVersion(t, WithVersion("0.2.0"), releasesServer(t, latestRelease("v0.3.0")), executableAt(standalone))

	want := "babysitter version 0.2.0\n" +
		"babysitter 0.3.0 is out. Download it from https://github.com/deividfortuna/babysitter/releases/tag/v0.3.0\n"
	if out != want {
		t.Fatalf("version = %q, want %q", out, want)
	}
}

func TestVersionFollowsTheLinkThatHomebrewMakes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	daemon := filepath.Join(dir, "Babysitter.app", "Contents", "Resources", "daemon")
	if err := os.MkdirAll(daemon, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(daemon, "babysitter")
	if err := os.WriteFile(binary, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "babysitter")
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}

	out := runVersion(t, WithVersion("0.2.0"), releasesServer(t, latestRelease("v0.3.0")), executableAt(link))

	if !bytes.Contains([]byte(out), []byte("brew upgrade --cask babysitter")) {
		t.Fatalf("version = %q, want the Homebrew command", out)
	}
}

func TestVersionOfTheLatestReleasePrintsOnlyTheVersion(t *testing.T) {
	t.Parallel()
	out := runVersion(t, WithVersion("0.3.0"), releasesServer(t, latestRelease("v0.3.0")), executableAt(appBinary))

	if out != "babysitter version 0.3.0\n" {
		t.Fatalf("version = %q", out)
	}
}

func TestVersionOfAPrereleaseSeesTheStableReleaseOfItsVersion(t *testing.T) {
	t.Parallel()
	out := runVersion(t, WithVersion("0.3.0-beta.1"), releasesServer(t, latestRelease("v0.3.0")), executableAt(appBinary))

	if !bytes.Contains([]byte(out), []byte("babysitter 0.3.0 is out.")) {
		t.Fatalf("version = %q, want the notice", out)
	}
}

func TestVersionIsQuietWhenGitHubHasNoRelease(t *testing.T) {
	t.Parallel()
	out := runVersion(t, WithVersion("0.2.0"), releasesServer(t, ghfake.New()), executableAt(appBinary))

	if out != "babysitter version 0.2.0\n" {
		t.Fatalf("version = %q", out)
	}
}

func TestVersionIsQuietWhenGitHubCannotBeReached(t *testing.T) {
	t.Parallel()
	srv := ghfake.Serve(t, http.NotFoundHandler())
	srv.Close()
	unreachable := WithReleaseClientFactory(func() (*github.Client, error) {
		return srv.NewClient()
	})

	out := runVersion(t, WithVersion("0.2.0"), unreachable, executableAt(appBinary))

	if out != "babysitter version 0.2.0\n" {
		t.Fatalf("version = %q", out)
	}
}

func TestVersionOfADevelopmentBuildAsksNothing(t *testing.T) {
	t.Parallel()
	out := runVersion(t, releasesServer(t, askedNothing(t, "a dev build")), executableAt(appBinary))

	if out != "babysitter version dev\n" {
		t.Fatalf("version = %q", out)
	}
}

func TestVersionAsJSON(t *testing.T) {
	t.Parallel()
	root := NewRootCmd(WithVersion("0.2.0"), releasesServer(t, latestRelease("v0.3.0")), executableAt(appBinary))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"version", "-o", "json"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("version error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("version output %q is not JSON: %v", out.String(), err)
	}
	want := map[string]any{
		"version":          "0.2.0",
		"latest":           "0.3.0",
		"update_available": true,
		"upgrade":          "The desktop app updates itself, or run: brew upgrade --cask babysitter",
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %v, want %v (all: %v)", key, got[key], value, got)
		}
	}
}

func TestVersionFlagKeepsItsOneLine(t *testing.T) {
	t.Parallel()
	root := NewRootCmd(WithVersion("0.2.0"), releasesServer(t, askedNothing(t, "--version")))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("--version error = %v", err)
	}

	if out.String() != "babysitter version 0.2.0\n" {
		t.Fatalf("--version = %q", out.String())
	}
}
