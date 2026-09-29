package cli

import (
	"testing"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/store"
)

func TestBehindWordNamesWhoUpdatesTheBranch(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		watch httpd.Watch
		want  string
	}{
		"GitHub first": {httpd.Watch{Provider: "claude", BranchUpdate: store.BranchRebase, UpdateOnGitHub: true}, "rebase, on GitHub first"},
		"agent only":   {httpd.Watch{Provider: "claude", BranchUpdate: store.BranchMerge}, "merge, by the agent"},
		"Dependabot":   {httpd.Watch{Provider: "claude", Dependabot: true, BranchUpdate: store.BranchRebase, UpdateOnGitHub: true}, "Dependabot rebases it on @dependabot rebase"},
		"self":         {httpd.Watch{Provider: "self", BranchUpdate: store.BranchRebase, UpdateOnGitHub: true}, "rebase, by your session"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := behindWord(c.watch); got != c.want {
				t.Fatalf("behindWord() = %q, want %q", got, c.want)
			}
		})
	}
}
