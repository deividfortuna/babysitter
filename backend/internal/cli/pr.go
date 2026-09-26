package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

type prSnapshot snapshot.Snapshot

func (s prSnapshot) writeText(w io.Writer) error {
	pr := s.PR
	state := string(pr.State)
	if pr.Draft && pr.State == store.StateOpen {
		state = "open (draft)"
	}
	mergeable := "unknown"
	if pr.Mergeable != nil {
		mergeable = fmt.Sprintf("%t", *pr.Mergeable)
	}
	fmt.Fprintf(w, "%s#%d  %s  by %s\n", pr.Repo, pr.Number, pr.Title, pr.Author)
	fmt.Fprintf(w, "URL:        %s\n", pr.URL)
	fmt.Fprintf(w, "State:      %s   head %s@%s   base %s\n", state, pr.HeadBranch, textx.ShortSHA(pr.HeadSHA), pr.BaseBranch)
	fmt.Fprintf(w, "Mergeable:  %s / %s   Review: %s\n", mergeable, pr.MergeableState, pr.ReviewDecision)
	c := s.Checks
	fmt.Fprintf(w, "Checks:     %s   %d passed, %d failed, %d pending, %d skipped\n",
		c.Status, c.PassedCount, c.FailedCount, c.PendingCount, c.SkippedCount)

	if len(s.AwaitingApproval) > 0 {
		fmt.Fprintln(w, "\nRuns that wait for approval:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, r := range s.AwaitingApproval {
			why := string(r.Conclusion)
			if why == "" {
				why = string(r.Status)
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", r.WorkflowName, why, r.HTMLURL)
		}
		tw.Flush()
	}
	if len(s.FailedJobs) > 0 {
		fmt.Fprintln(w, "\nFailed jobs:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, j := range s.FailedJobs {
			fmt.Fprintf(tw, "  %s / %s\t%s\t%s\tlogs: %s\n", j.WorkflowName, j.JobName, j.Conclusion, j.HTMLURL, j.LogsEndpoint)
		}
		tw.Flush()
	} else if len(s.FailedRuns) > 0 {
		fmt.Fprintln(w, "\nFailed runs:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, r := range s.FailedRuns {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", r.WorkflowName, r.Conclusion, r.HTMLURL)
		}
		tw.Flush()
	}

	if len(s.NewReviewItems) > 0 {
		fmt.Fprintln(w, "\nNew review items:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, it := range s.NewReviewItems {
			where := ""
			if it.Path != "" {
				where = it.Path
				if it.Line != nil {
					where = fmt.Sprintf("%s:%d", it.Path, *it.Line)
				}
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%q\t%s\n", it.Kind, it.Author, where, textx.FirstLine(it.Body, 60), it.URL)
		}
		tw.Flush()
	}

	fmt.Fprintf(w, "\nActions:    %s\n", strings.Join(s.Actions, ", "))
	_, err := fmt.Fprintf(w, "Retries:    %d of %d used for %s\n",
		s.RetryState.CurrentSHARetriesUsed, s.RetryState.MaxFlakyRetries, textx.ShortSHA(pr.HeadSHA))
	return err
}

func newPRCmd(opts *options) *cobra.Command {
	var (
		repo        string
		maxRetries  int
		recordRetry bool
	)
	cmd := &cobra.Command{
		Use:   "pr [target]",
		Short: "Print a snapshot of one pull request",
		Long: `pr fetches one pull request from GitHub and prints its state: details,
checks, failed workflow jobs, review items that no earlier snapshot showed,
and the actions a babysitter should take next.

The target is a pull request URL, owner/name#number, or a number with
--repo. Without a target, pr reads the current branch and the remotes of
the git repository in the working directory: the branch was pushed to
origin, and the pull request lives in the upstream remote when there is
one, in origin otherwise.

The actions are:
  ready_to_merge          CI is green, no new review items, nothing blocks the merge
  process_review_comment  new review items need an answer or a change
  diagnose_ci_failure     a check, a run or a job failed, read its logs
  retry_failed_checks     every check ended, a run failed, and the retry budget has room
  stop_exhausted_retries  every check ended, a run failed, and the retry budget is used
  stop_action_required    a workflow run waits for approval, or a check waits for a person
  stop_pr_closed          the pull request is merged or closed
  idle                    nothing to do now, checks are still running

Review items that a snapshot showed once are kept in the database and do
not show again for the same pull request. After 'gh run rerun', pass
--record-retry once so the snapshot counts the retry against the budget
of the head commit.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if maxRetries < 0 {
				return fmt.Errorf("max flaky retries must be 0 or more, got %d", maxRetries)
			}
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			target, err := snapshot.ParseTarget(arg, repo)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			client, err := opts.client(ctx)
			if err != nil {
				return err
			}
			st, err := opts.openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			s, err := snapshot.Collect(ctx, client, st, target, snapshot.Options{
				MaxFlakyRetries: maxRetries,
				RecordRetry:     recordRetry,
			})
			if err != nil {
				return err
			}
			if err := opts.print(cmd.OutOrStdout(), prSnapshot(*s)); err != nil {
				return err
			}
			return s.Commit(ctx, st)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repository of a bare pull request number (owner/name)")
	cmd.Flags().IntVar(&maxRetries, "max-flaky-retries", 3, "retry budget per head commit")
	cmd.Flags().BoolVar(&recordRetry, "record-retry", false, "count one flaky retry cycle for the head commit")
	return cmd
}
