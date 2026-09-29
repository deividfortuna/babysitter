package cli

import (
	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/store"
)

const (
	branchUpdateFlag   = "branch-update"
	updateOnGitHubFlag = "update-on-github"
)

func typedBranchUpdate(cmd *cobra.Command, value string) *store.BranchUpdate {
	if !cmd.Flags().Changed(branchUpdateFlag) {
		return nil
	}
	update := store.BranchUpdate(value)
	return &update
}

func branchUpdateWord(update string, onGitHub bool) string {
	if onGitHub {
		return update + ", on GitHub first"
	}
	return update + ", by the agent"
}
