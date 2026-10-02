package cli

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/httpd"
)

func tokenDaemon(t *testing.T, token string) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+httpd.Prefix+"/auth/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(httpd.TokenSecretHeader) != "s3cret" {
			http.Error(w, `{"error":{"code":"token_forbidden","message":"no"}}`, http.StatusForbidden)
			return
		}
		if token == "" {
			http.Error(w, `{"error":{"code":"app_not_in_use","message":"no app"}}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"` + token + `"}`))
	})
	return serveDaemon(t, mux, "s3cret")
}

func failingTokenDaemon(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+httpd.Prefix+"/auth/token", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"token_failed","message":"the GitHub App sign in expired"}}`, http.StatusInternalServerError)
	})
	return serveDaemon(t, mux, "s3cret")
}

func runAuthToken(t *testing.T, dataDir string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"auth", "token", "--data-dir", dataDir})
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func runGitCredential(t *testing.T, dataDir, operation, request string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetIn(strings.NewReader(request))
	root.SetArgs([]string{"auth", "git-credential", "--data-dir", dataDir, operation})
	err := root.ExecuteContext(context.Background())
	return out.String(), err
}

func TestGitCredentialGivesTheAppToken(t *testing.T) {
	t.Parallel()
	dataDir := tokenDaemon(t, "ghu_app")

	out, err := runGitCredential(t, dataDir, "get", "protocol=https\nhost=github.com\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "username=x-access-token\npassword=ghu_app\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestGitCredentialGivesNothingSoGitAsksTheNextHelper(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		dataDir   string
		operation string
		request   string
	}{
		{"another host", tokenDaemon(t, "ghu_app"), "get", "protocol=https\nhost=gitlab.com\n\n"},
		{"plain http", tokenDaemon(t, "ghu_app"), "get", "protocol=http\nhost=github.com\n\n"},
		{"a store", tokenDaemon(t, "ghu_app"), "store", "protocol=https\nhost=github.com\nusername=x\npassword=y\n\n"},
		{"the app not in use", tokenDaemon(t, ""), "get", "protocol=https\nhost=github.com\n\n"},
		{"no daemon", t.TempDir(), "get", "protocol=https\nhost=github.com\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runGitCredential(t, tc.dataDir, tc.operation, tc.request)
			if err != nil {
				t.Fatalf("err = %v, want none", err)
			}
			if out != "" {
				t.Fatalf("output = %q, want none", out)
			}
		})
	}
}

func TestGitCredentialStopsGitWhenTheDaemonCannotGetTheAppToken(t *testing.T) {
	t.Parallel()
	dataDir := failingTokenDaemon(t)

	out, err := runGitCredential(t, dataDir, "get", "protocol=https\nhost=github.com\n\n")
	if err == nil {
		t.Fatal("err = nil, want the error of the daemon")
	}
	if out != "quit=1\n" {
		t.Fatalf("output = %q, want quit=1 so git asks no other helper", out)
	}
}

func TestAuthTokenPrintsTheAppToken(t *testing.T) {
	t.Parallel()
	out, err := runAuthToken(t, tokenDaemon(t, "ghu_app"))
	if err != nil {
		t.Fatal(err)
	}
	if out != "ghu_app\n" {
		t.Fatalf("output = %q, want the token", out)
	}
}

func TestAuthTokenPrintsNothingWhenTheAppIsNotInUse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		dataDir string
	}{
		{"the app not in use", tokenDaemon(t, "")},
		{"no daemon", t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runAuthToken(t, tc.dataDir)
			if err != nil {
				t.Fatalf("err = %v, want none", err)
			}
			if out != "" {
				t.Fatalf("output = %q, want none", out)
			}
		})
	}
}

func TestAuthTokenFailsWhenTheDaemonCannotGetTheAppToken(t *testing.T) {
	t.Parallel()
	out, err := runAuthToken(t, failingTokenDaemon(t))
	if err == nil {
		t.Fatal("err = nil, want the error of the daemon")
	}
	if out != "" {
		t.Fatalf("output = %q, want none", out)
	}
}
