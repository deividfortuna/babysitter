package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/textx"
)

var errNoMergeRule = errors.New("nothing to change: pass --approvals, --merge-method or --merge-when-ready")

type mergeRulesOutput httpd.Watch

func (w mergeRulesOutput) writeText(out io.Writer) error {
	_, err := fmt.Fprintf(out, "Watch %d needs %s before it is ready to merge, and merges with %s. %s\n",
		w.ID, approvalsCount(w.ApprovalsRequired), mergesWith(w.MergeMethod), mergeWhenReadyWord(w.MergeWhenReady))
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
		approvals, mergeMethod string
		mergeWhenReady         bool
	)
	cmd := &cobra.Command{
		Use:   "merge-rules <watch>",
		Short: "Change the approvals, the merge method and merge when ready of a running watch",
		Long: `A watch copies the approvals and the merge method from the settings of
the daemon when it starts. This command changes them for this watch
only. --approvals takes a number, 0 for none, or 'branch' to read the
rule of the base branch again. --merge-method takes squash, merge,
rebase, or empty for the first method the repository allows.
--merge-when-ready lets the daemon merge as soon as the watch is ready;
--merge-when-ready=false turns it off. A flag you leave out keeps what
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
			}
			nothingToChange := !req.ApprovalsRequired.Set && req.MergeMethod == nil && req.MergeWhenReady == nil
			if nothingToChange {
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
	return cmd
}
