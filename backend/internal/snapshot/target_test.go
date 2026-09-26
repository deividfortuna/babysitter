package snapshot

import (
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		arg, repo string
		want      Target
		wantErr   string
	}{
		{"", "", Target{}, ""},
		{"", "octo/hello", Target{Owner: "octo", Name: "hello"}, ""},
		{"7", "", Target{Number: 7}, ""},
		{"7", "octo/hello", Target{Owner: "octo", Name: "hello", Number: 7}, ""},
		{"octo/hello#7", "", Target{Owner: "octo", Name: "hello", Number: 7}, ""},
		{"https://github.com/octo/hello/pull/7", "", Target{Owner: "octo", Name: "hello", Number: 7}, ""},
		{"https://github.com/octo/hello/pull/7/files", "", Target{Owner: "octo", Name: "hello", Number: 7}, ""},
		{"https://github.com/octo/hello/pull/7", "other/repo", Target{Owner: "octo", Name: "hello", Number: 7}, ""},
		{"0", "", Target{}, "invalid pull request number"},
		{"octo/hello", "", Target{}, "want a number"},
		{"octo/hello#x", "", Target{}, "invalid pull request number"},
		{"https://github.com/octo/hello/issues/7", "", Target{}, "invalid pull request URL"},
		{"7", "nope", Target{}, "want owner/name"},
	}
	for _, c := range cases {
		got, err := ParseTarget(c.arg, c.repo)
		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("ParseTarget(%q, %q) err = %v, want %q", c.arg, c.repo, err, c.wantErr)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("ParseTarget(%q, %q) = %+v, %v, want %+v", c.arg, c.repo, got, err, c.want)
		}
	}
}
