package cli

import (
	"testing"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
)

func TestBehindWordNamesWhoUpdatesTheBranch(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		watch httpd.Watch
		want  string
	}{
		"GitHub first": {httpd.Watch{BranchUpdate: store.BranchRebase, BranchUpdater: prwatch.UpdaterGitHub}, "rebase, on GitHub first"},
		"agent only":   {httpd.Watch{BranchUpdate: store.BranchMerge, BranchUpdater: prwatch.UpdaterAgent}, "merge, by the agent"},
		"Dependabot":   {httpd.Watch{BranchUpdate: store.BranchRebase, BranchUpdater: prwatch.UpdaterDependabot}, "Dependabot rebases it on @dependabot rebase"},
		"self":         {httpd.Watch{BranchUpdate: store.BranchRebase, BranchUpdater: prwatch.UpdaterSession}, "rebase, by your session"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := behindWord(c.watch); got != c.want {
				t.Fatalf("behindWord() = %q, want %q", got, c.want)
			}
		})
	}
}
