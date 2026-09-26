package prwatch

import (
	"maps"
	"testing"

	"github.com/deividfortuna/babysitter/internal/gitrelease"
)

func TestRenameCommitsWritesTheRebasedSHA(t *testing.T) {
	t.Parallel()
	const (
		old = "b50aed2f0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f"
		neu = "07bdda71a2b3c4d5e6f708192a3b4c5d6e7f8091"
	)
	renames := map[string]string{old: neu}
	for _, tc := range []struct{ in, want string }{
		{"Added in b50aed2. Thanks.", "Added in 07bdda7. Thanks."},
		{"See `b50aed2f0c`.", "See `07bdda71a2`."},
		{"https://github.com/o/n/commit/" + old, "https://github.com/o/n/commit/" + neu},
		{"(B50AED2)", "(07bdda7)"},
		{"b50aed is too short, xb50aed2 is another word", "b50aed is too short, xb50aed2 is another word"},
		{"", ""},
	} {
		if got := renameCommits(tc.in, renames); got != tc.want {
			t.Errorf("renameCommits(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRenamedPairsTheCommitsOfARebase(t *testing.T) {
	t.Parallel()
	before := []gitrelease.Commit{{SHA: "a1", Subject: "Add median"}, {SHA: "a2", Subject: "Test median"}, {SHA: "a3", Subject: "Fix the doc"}}
	for _, tc := range []struct {
		name  string
		after []gitrelease.Commit
		want  map[string]string
	}{
		{
			"one by one",
			[]gitrelease.Commit{{SHA: "b1", Subject: "x"}, {SHA: "b2", Subject: "y"}, {SHA: "b3", Subject: "z"}},
			map[string]string{"a1": "b1", "a2": "b2", "a3": "b3"},
		},
		{
			"a commit the base already had",
			[]gitrelease.Commit{{SHA: "b1", Subject: "Add median"}, {SHA: "b3", Subject: "Fix the doc"}},
			map[string]string{"a1": "b1", "a3": "b3"},
		},
	} {
		if got := renamed(before, tc.after); !maps.Equal(got, tc.want) {
			t.Errorf("%s: renamed() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
