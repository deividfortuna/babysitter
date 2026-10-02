package daemon

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
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

func TestStatusSaysWhenTheInstallationsCannotBeRead(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	dir := t.TempDir()
	signIn := `{"login":"alice","accessToken":"ghu_signedin0000000000000000"}`
	if err := os.WriteFile(filepath.Join(dir, "github-app.json"), []byte(signIn), 0o600); err != nil {
		t.Fatal(err)
	}
	g := ghfake.New()
	g.Fail(ghfake.RouteInstallations, http.StatusBadGateway, "bad gateway")
	srv := g.Serve(t)
	auth := ghauth.New(dir, ghauth.WithGH(func(context.Context) (string, error) { return "", errors.New("no gh") }))
	c := &authController{
		auth:      auth,
		signIns:   auth.SignIns(t.Context(), nil),
		newClient: func(context.Context) (*github.Client, error) { return srv.NewClient() },
	}

	st := c.Status(context.Background())

	if st.State != string(ghauth.StateConnected) {
		t.Fatalf("State = %q, want connected", st.State)
	}
	if st.InstallsError == "" {
		t.Fatal("InstallsError is empty, want the reason the installations are unknown")
	}
	if len(st.Installations) != 0 {
		t.Fatalf("Installations = %v, want none", st.Installations)
	}
	if st.Error != "" {
		t.Fatalf("Error = %q, want the failure only in installationsError", st.Error)
	}
}
