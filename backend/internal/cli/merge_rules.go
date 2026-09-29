package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/textx"
)

var errNoMergeRule = errors.New("nothing to change: pass --approvals, --merge-method, --merge-when-ready, --branch-update or --update-on-github")

type mergeRulesOutput httpd.Watch

func (w mergeRulesOutput) writeText(out io.Writer) error {
	_, err := fmt.Fprintf(out, "Watch %d needs %s before it is ready to merge, and merges with %s. %s A branch behind its base: %s.\n",
		w.ID, approvalsCount(w.ApprovalsRequired), mergesWith(w.MergeMethod), mergeWhenReadyWord(w.MergeWhenReady),
		branchUpdateWord(string(w.BranchUpdate), w.UpdateOnGitHub))
	return err
}

func mergeWhenReadyWord(on bool) string {
	if on {
		return "The daemon merges it as soon as it is ready."
	}
	return "You merge it."
}

func mergesWith(method string) string {
	if method == "" {
		return "the " + mergeMethodWord(method)
	}
	return method
}

func approvalsCount(n int) string {
	if n == 0 {
		return "no approval"
	}
	return textx.Plural(n, "approval")
}

func newWatchMergeRulesCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		approvals, mergeMethod, branchUpdate string
		mergeWhenReady, updateOnGitHub       bool
	)
	cmd := &cobra.Command{
		Use:   "merge-rules <watch>",
		Short: "Change the approvals, the merge method, merge when ready and the branch update of a running watch",
		Long: `A watch copies the approvals and the merge method from the settings of
the daemon when it starts. This command changes them for this watch
only. --approvals takes a number, 0 for none, or 'branch' to read the
rule of the base branch again. --merge-method takes squash, merge,
rebase, or empty for the first method the repository allows.
--merge-when-ready lets the daemon merge as soon as the watch is ready;
--merge-when-ready=false turns it off. --branch-update takes rebase or
merge: how a branch behind its base is updated, and how the agent solves
a conflict. --update-on-github asks GitHub to update the branch before
the agent does it. A flag you leave out keeps what
the watch has. The next poll judges the pull
request against the new approvals.`,
		Args: cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			wanted, err := typedApprovals(cmd, approvals)
			if err != nil {
				return err
			}
			req := httpd.UpdateWatchRequest{
				ApprovalsRequired: wanted,
				MergeMethod:       typed(cmd, "merge-method", &mergeMethod),
				MergeWhenReady:    typed(cmd, "merge-when-ready", &mergeWhenReady),
				BranchUpdate:      typedBranchUpdate(cmd, branchUpdate),
				UpdateOnGitHub:    typed(cmd, updateOnGitHubFlag, &updateOnGitHub),
			}
			if !anyChanged(cmd, "approvals", "merge-method", "merge-when-ready", branchUpdateFlag, updateOnGitHubFlag) {
				return errNoMergeRule
			}
			var out httpd.Watch
			if err := c.patch(cmd.Context(), fmt.Sprintf("/watches/%d", w.ID), req, &out); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), mergeRulesOutput(out))
		}),
	}
	cmd.Flags().StringVar(&approvals, "approvals", "", "approvals the pull request needs before it is ready to merge: a number, 0 for none, or 'branch' for the rule of the base branch")
	cmd.Flags().StringVar(&mergeMethod, "merge-method", "", "merge method of the watch: squash, merge, rebase, or empty for the first method the repository allows")
	cmd.Flags().BoolVar(&mergeWhenReady, "merge-when-ready", false, "the daemon merges with the method of the watch as soon as the watch is ready to merge")
	cmd.Flags().StringVar(&branchUpdate, branchUpdateFlag, "", "how the branch is updated when it falls behind its base: rebase or merge. The agent solves a conflict the same way")
	cmd.Flags().BoolVar(&updateOnGitHub, updateOnGitHubFlag, false, "ask GitHub to update a branch that fell behind its base before the agent does it")
	return cmd
}
