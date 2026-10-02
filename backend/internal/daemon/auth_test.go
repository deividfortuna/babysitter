package daemon

import (
	"errors"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/httpd"
)

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
