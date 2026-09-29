package cli

import (
	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/prwatch"
)

const (
	branchUpdateFlag   = "branch-update"
	updateOnGitHubFlag = "update-on-github"
)

func behindWord(w httpd.Watch) string {
	switch w.BranchUpdater {
	case prwatch.UpdaterDependabot:
		return "Dependabot rebases it on @dependabot rebase"
	case prwatch.UpdaterSession:
		return string(w.BranchUpdate) + ", by your session"
	default:
		return branchUpdateWord(string(w.BranchUpdate), w.BranchUpdater == prwatch.UpdaterGitHub)
	}
}

func branchUpdateWord(update string, onGitHub bool) string {
	if onGitHub {
		return update + ", on GitHub first"
	}
	return update + ", by the agent"
}
