package daemon

import (
	"errors"
	"fmt"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/httpd"
)

func TestAuthState(t *testing.T) {
	t.Parallel()
	expired := fmt.Errorf("%w: bad_refresh_token", ghauth.ErrSessionExpired)
	cases := []struct {
		name    string
		st      ghauth.Status
		waiting bool
		want    string
	}{
		{"no sign in", ghauth.Status{Origin: ghauth.OriginGH}, false, httpd.AuthSignedOut},
		{"a code waits on GitHub", ghauth.Status{Origin: ghauth.OriginGH}, true, httpd.AuthWaiting},
		{"a new code for an expired sign in", ghauth.Status{SignedIn: true, Err: expired}, true, httpd.AuthWaiting},
		{"the app gives the token", ghauth.Status{SignedIn: true, Origin: ghauth.OriginApp}, false, httpd.AuthConnected},
		{"GITHUB_TOKEN comes first", ghauth.Status{SignedIn: true, Origin: ghauth.OriginEnv}, false, httpd.AuthNotInUse},
		{"the flag comes first", ghauth.Status{SignedIn: true, Origin: ghauth.OriginFlag}, false, httpd.AuthNotInUse},
		{"the refresh token was refused", ghauth.Status{SignedIn: true, Err: expired}, false, httpd.AuthExpired},
	}
	for _, tc := range cases {
		if got := authState(tc.st, tc.waiting); got != tc.want {
			t.Errorf("%s: authState = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSignInFailure(t *testing.T) {
	t.Parallel()
	cases := map[error]string{
		ghauth.ErrCodeExpired:             httpd.SignInCodeExpired,
		ghauth.ErrDenied:                  httpd.SignInDenied,
		errors.New("GitHub answered 502"): httpd.SignInFailed,
	}
	for err, want := range cases {
		if got := signInFailure(err); got != want {
			t.Errorf("signInFailure(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestReportableLeavesTheStatesThePageShows(t *testing.T) {
	t.Parallel()
	if reportable(ghauth.ErrNoToken) || reportable(fmt.Errorf("%w: x", ghauth.ErrSessionExpired)) || reportable(nil) {
		t.Fatal("an expected state is reported as an error")
	}
	if !reportable(errors.New("read the GitHub App sign in: permission denied")) {
		t.Fatal("a failure to read the sign in is not reported")
	}
}
