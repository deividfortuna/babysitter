package redact

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"token ghp_abcdefghijklmnopqrstuvwxyz1234 here":               "token [redacted] here",
		"github_pat_11ABCDEFG0123456789abcdefghijklmn":                "[redacted]",
		"Authorization: Bearer abcdefghijklmnopqrstuvwxyz":            "Authorization: [redacted]",
		"api_key=0123456789abcdefghij":                                "api_key=[redacted]",
		"https://x/callback?access_token=0123456789abcdefghij&b=1":    "https://x/callback?access_token=[redacted]&b=1",
		"refresh-token: 0123456789abcdefghij":                         "refresh-token: [redacted]",
		`{"api_key":"0123456789abcdefghij"}`:                          `{"api_key":"[redacted]"}`,
		`{"note":"the token is ghp_abcdefghijklmnopqrstuvwxyz1234"}`:  `{"note":"the token is [redacted]"}`,
		"AKIAIOSFODNN7EXAMPLE":                                        "[redacted]",
		"plain text with go test ./... and nothing secret":            "plain text with go test ./... and nothing secret",
		"-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----": "[redacted]",
	}
	for in, want := range cases {
		if got := Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTextLeavesAPayloadAsJSON(t *testing.T) {
	t.Parallel()
	in := `{"body":"run it with api_key=0123456789abcdefghij","comment_id":31,"reply":{"token":"ghp_abcdefghijklmnopqrstuvwxyz1234"}}`
	out := Text(in)
	for _, secret := range []string{"0123456789abcdefghij", "ghp_"} {
		if strings.Contains(out, secret) {
			t.Fatalf("Text() = %q, which still carries %q", out, secret)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("Text() = %q, which no longer parses as JSON: %v", out, err)
	}
	if payload["comment_id"] != float64(31) {
		t.Fatalf("payload = %v", payload)
	}
}

func TestErrKeepsWhatTheCallerMatchesOn(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("not found")
	dirty := fmt.Errorf("%w: GET https://x:ghp_abcdefghijklmnopqrstuvwxyz1234@api.github.com/repos", sentinel)
	clean := Err(dirty)
	if strings.Contains(clean.Error(), "ghp_") {
		t.Fatalf("Err() = %q, which still carries the token", clean)
	}
	if !errors.Is(clean, sentinel) {
		t.Fatal("Err() dropped the error the caller matches on")
	}
}

func TestErrOfACleanErrorIsTheErrorItself(t *testing.T) {
	t.Parallel()
	plain := errors.New("the pull request is not open")
	if got := Err(plain); !errors.Is(got, plain) {
		t.Fatalf("Err() = %v, want the error it was given", got)
	}
	if Err(nil) != nil {
		t.Fatal("Err(nil) is not nil")
	}
}

func TestRedactTakesTheKeyNamesOfACloudCredential(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"secret_access_key=0123456789abcdef":          "secret_access_key=[redacted]",
		"private_key: 0123456789abcdefghij":           "private_key: [redacted]",
		`{"SecretAccessKey":"0123456789abcdefghij"}`:  `{"SecretAccessKey":"[redacted]"}`,
		"aws-secret-key=0123456789abcdefghij":         "aws-secret-key=[redacted]",
		"the monkey=0123456789abcdefghij in the room": "the monkey=0123456789abcdefghij in the room",
		"sortkey=0123456789abcdefghij":                "sortkey=0123456789abcdefghij",
	}
	for in, want := range cases {
		if got := Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
}
