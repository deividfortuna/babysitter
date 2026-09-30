package cli

import (
	"cmp"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/textx"
)

type pullRequestView httpd.PullRequestView

func (v pullRequestView) writeText(w io.Writer) error {
	state := string(v.State)
	if v.Draft {
		state += " (draft)"
	}
	mergeable := "unknown"
	if v.Mergeable != nil {
		mergeable = fmt.Sprintf("%t", *v.Mergeable)
	}
	c := v.Checks
	fmt.Fprintf(w, "%s#%d  %s  by %s\n", v.Repo, v.Number, v.Title, v.Author)
	fmt.Fprintf(w, "URL:        %s\n", v.URL)
	fmt.Fprintf(w, "State:      %s   head %s@%s   base %s\n", state, v.HeadRef, textx.ShortSHA(v.HeadSHA), v.BaseRef)
	fmt.Fprintf(w, "Mergeable:  %s / %s   Review: %s\n", mergeable, v.MergeableState, v.ReviewDecision)
	fmt.Fprintf(w, "Reviewers:  %s\n", reviewersLine(v.Reviewers))
	fmt.Fprintf(w, "Requested:  %s\n", listOrDash(v.RequestedReviewers))
	fmt.Fprintf(w, "Labels:     %s\n", listOrDash(v.Labels))
	fmt.Fprintf(w, "Assignees:  %s\n", listOrDash(v.Assignees))
	fmt.Fprintf(w, "Milestone:  %s   Auto-merge: %s\n", orDash(v.Milestone), autoMergeWord(v.AutoMerge))
	fmt.Fprintf(w, "Size:       +%d -%d in %s, %s\n", v.Additions, v.Deletions, textx.Plural(v.ChangedFiles, "file"), textx.Plural(v.Commits, "commit"))
	fmt.Fprintf(w, "Checks:     %s   %d passed, %d failed, %d pending, %d skipped\n", c.Status, c.Passed, c.Failed, c.Pending, c.Skipped)
	fmt.Fprintf(w, "Created:    %s   Read from GitHub: %s\n", v.CreatedAt.Format(time.RFC3339), v.SnapshotAt.Format(time.RFC3339))
	body := strings.TrimSpace(v.Body)
	if body == "" {
		body = "No description."
	}
	_, err := fmt.Fprintf(w, "\n%s\n", body)
	return err
}

func reviewersLine(reviewers []httpd.PullRequestReview) string {
	parts := make([]string, 0, len(reviewers))
	for _, r := range reviewers {
		parts = append(parts, fmt.Sprintf("%s (%s)", r.Login, strings.ToLower(strings.ReplaceAll(r.State, "_", " "))))
	}
	return listOrDash(parts)
}

func listOrDash(items []string) string {
	return orDash(strings.Join(items, ", "))
}

func autoMergeWord(method string) string {
	if method == "" {
		return "off"
	}
	return "on, " + method
}

type pullRequestDiff httpd.PullRequestDiff

func (d pullRequestDiff) writeText(w io.Writer) error {
	_, err := io.WriteString(w, d.Diff)
	return err
}

func newWatchViewCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "view <watch>",
		Short: "Show the pull request of a watch as the daemon last read it",
		Long: `view prints the pull request of an active watch from the last snapshot of
the daemon: state, branches, mergeability, reviewers, labels, assignees,
milestone, auto-merge, size, checks and the description. It makes no call
to GitHub, so it costs no API budget and marks no review item as seen.`,
		Args: cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			var out httpd.PullRequestView
			if err := c.get(cmd.Context(), fmt.Sprintf("/watches/%d/view", w.ID), &out); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), pullRequestView(out))
		}),
	}
}

func newWatchDiffCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "diff <watch>",
		Short: "Print the diff of the pull request of a watch",
		Long: `diff prints the diff of the pull request of an active watch as it is on
GitHub. The daemon fetches the base and the head branch into the worktree
of the watch, or into the checkout of a self watch, and diffs the head
against their merge base. Commits that are not pushed yet are not in it.
It makes no call to the GitHub API.`,
		Args: cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			var out httpd.PullRequestDiff
			if err := c.get(cmd.Context(), fmt.Sprintf("/watches/%d/diff", w.ID), &out); err != nil {
				return err
			}
			if err := opts.print(cmd.OutOrStdout(), pullRequestDiff(out)); err != nil {
				return err
			}
			if out.Truncated {
				fmt.Fprintf(cmd.ErrOrStderr(), "The diff stops at 1 MB. Read the rest with `git diff %s %s` in %s.\n", out.Base, out.Head, cmp.Or(w.WorktreeDir, w.SourceDir))
			}
			return nil
		}),
	}
}
