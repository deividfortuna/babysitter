package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

func providerLabel(provider, model, effort string) string {
	var details []string
	if model != "" {
		details = append(details, model)
	}
	if effort != "" {
		details = append(details, effort+" effort")
	}
	return provider + suffix(" (", strings.Join(details, ", "), ")")
}

type watchOutput httpd.Watch

func (w watchOutput) writeText(out io.Writer) error {
	fmt.Fprintf(out, "Watch:     %d\n", w.ID)
	fmt.Fprintf(out, "PR:        %s#%d %s\n", w.Repo, w.Number, w.Title)
	fmt.Fprintf(out, "URL:       %s\n", w.URL)
	fmt.Fprintf(out, "Branch:    %s -> %s\n", w.HeadRef, w.BaseRef)
	fmt.Fprintf(out, "Provider:  %s\n", providerLabel(w.Provider, w.Model, w.Effort))
	fmt.Fprintf(out, "Status:    %s%s\n", w.Status, suffix(" (", string(w.StopReason), ")"))
	fmt.Fprintf(out, "Head:      %s\n", orDash(textx.ShortSHA(w.HeadSHA)))
	fmt.Fprintf(out, "Checks:    %s\n", checks.Summarize(w.CheckStates, w.HeadSHA, w.GreenSHA))
	fmt.Fprintf(out, "Mergeable: %s\n", orDash(string(w.MergeableState)))
	fmt.Fprintf(out, "Merge:     %s\n", mergeLine(httpd.Watch(w)))
	fmt.Fprintf(out, "Behind:    %s\n", behindWord(httpd.Watch(w)))
	if line := autoLine(httpd.Watch(w)); line != "" {
		fmt.Fprintf(out, "Auto:      %s\n", line)
	}
	if w.Status == store.WatchActive {
		fmt.Fprintf(out, "Approval:  %s\n", approvalLine(httpd.Watch(w)))
	}
	fmt.Fprintf(out, "Started:   %s\n", w.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(out, "Last poll: %s\n", timeOrDash(w.LastPollAt))
	fmt.Fprintf(out, "Source:    %s\n", w.SourceDir)
	if w.WorktreeDir != "" {
		fmt.Fprintf(out, "Worktree:  %s\n", w.WorktreeDir)
	}
	fmt.Fprintf(out, "Agent:     %s\n", sessionLine(httpd.Watch(w)))
	if w.LastError != "" {
		fmt.Fprintf(out, "Error:     %s\n", w.LastError)
	}
	if w.Summary != nil {
		fmt.Fprintf(out, "Summary:   %s at %s, checks %s\n", w.Summary.PRState, textx.ShortSHA(w.Summary.HeadSHA), w.Summary.Checks)
		fmt.Fprintf(out, "           %d messages to the agent\n", w.Summary.Messages)
		fmt.Fprintf(out, "           %s\n", worktreeLine(httpd.Watch(w)))
	}
	return nil
}

func autoLine(w httpd.Watch) string {
	var parts []string
	if w.AutoReason != store.AutoNone {
		parts = append(parts, "started on its own: "+w.AutoReason.Word())
	}
	if w.UpdateType != "" {
		parts = append(parts, string(w.UpdateType)+" update")
	}
	if w.MergeWhenReady {
		parts = append(parts, "merges when ready")
	}
	return strings.Join(parts, "; ")
}

func mergeLine(w httpd.Watch) string {
	method := mergesWith(w.MergeMethod)
	switch {
	case w.Status != "active":
		return "-"
	case w.ReadySince != nil:
		return fmt.Sprintf("ready since %s, %s; run `babysitter watch merge %d`", w.ReadySince.Format(time.RFC3339), method, w.ID)
	case len(w.ReadyBlockers) == 0:
		return "not assessed yet"
	default:
		return "not ready: " + strings.Join(w.ReadyBlockers, "; ")
	}
}

func ignoredOnSelf(cmd *cobra.Command, approvalMode string) []string {
	var ignored []string
	gateAsked := cmd.Flags().Changed("approval-mode") && approvalMode != string(store.ApprovalAuto)
	if gateAsked {
		ignored = append(ignored, "--approval-mode "+approvalMode)
	}
	if cmd.Flags().Changed("auto-approve-rebase") {
		ignored = append(ignored, "--auto-approve-rebase")
	}
	return ignored
}

func nextHint(w httpd.Watch) string {
	return fmt.Sprintf("take each message with `babysitter watch next %d --wait 9m`", w.ID)
}

func sessionLine(w httpd.Watch) string {
	s := w.Session
	switch {
	case w.Provider == prwatch.ProviderSelf && w.Status == store.WatchActive:
		return "your own session; " + nextHint(w)
	case withYou(w):
		return "with you since " + w.TakenOverAt.Format(time.RFC3339)
	case s.State == "" || s.State == agent.StateNone:
		return "no session"
	case s.PID > 0:
		return fmt.Sprintf("%s (pid %d)", s.State, s.PID)
	default:
		return string(s.State)
	}
}

func withYou(w httpd.Watch) bool {
	return w.Status == store.WatchActive && w.TakenOverAt != nil
}

func agentWord(w httpd.Watch) string {
	if withYou(w) {
		return "with you"
	}
	return string(w.Session.State)
}

type watchListOutput httpd.WatchList

func (l watchListOutput) writeText(out io.Writer) error {
	if len(l.Watches) == 0 {
		fmt.Fprintln(out, "No pull request is watched")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tPULL REQUEST\tBRANCH\tPROVIDER\tSTATUS\tAGENT\tHEAD\tCHECKS\tLAST POLL")
	for _, w := range l.Watches {
		fmt.Fprintf(tw, "%d\t%s#%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", w.ID, w.Repo, w.Number, w.HeadRef, providerLabel(w.Provider, w.Model, w.Effort), w.Status, agentWord(w), textx.ShortSHA(w.HeadSHA), checks.Summarize(w.CheckStates, w.HeadSHA, w.GreenSHA), timeOrDash(w.LastPollAt))
	}
	return tw.Flush()
}

type activityListOutput httpd.ActivityList

func (l activityListOutput) writeText(out io.Writer) error {
	if len(l.Activity) == 0 {
		fmt.Fprintln(out, "No activity")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTIME\tKIND\tSUMMARY")
	for _, a := range l.Activity {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", a.ID, a.At.Local().Format("2006-01-02 15:04"), a.Kind, a.Summary)
	}
	return tw.Flush()
}

func worktreeLine(w httpd.Watch) string {
	switch {
	case w.WorktreeDir == "":
		return "no worktree"
	case !worktreeGone(w):
		return "worktree left at " + w.WorktreeDir
	case w.Summary.WorkBranchLeft != "":
		return "worktree removed from " + w.WorktreeDir + ", its branch " + w.Summary.WorkBranchLeft + " stays in " + w.SourceDir
	default:
		return "worktree removed from " + w.WorktreeDir
	}
}

func worktreeGone(w httpd.Watch) bool {
	return w.Summary != nil && w.Summary.WorktreeRemoved
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func suffix(before, s, after string) string {
	if s == "" {
		return ""
	}
	return before + s + after
}

func timeOrDash(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func newWatchCmd(opts *options) *cobra.Command {
	var dataDirFlag string
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch a pull request from the daemon",
		Long: `The daemon watches a pull request: it reports new comments, reviews,
check results, commits and mergeability once, and tells a coding agent
that runs in a worktree of your checkout. The agent fixes, commits and
replies; when its turn ends, the daemon pushes the commits and posts
the replies. You can read what it printed and send it a message.

Every subcommand talks to the running daemon. Start it with
'babysitter daemon start' or open the desktop app.`,
	}
	cmd.PersistentFlags().StringVar(&dataDirFlag, "data-dir", "", "directory of running.json")
	cmd.AddCommand(
		newWatchStartCmd(opts, &dataDirFlag),
		newWatchListCmd(opts, &dataDirFlag),
		newWatchStatusCmd(opts, &dataDirFlag),
		newWatchStopCmd(opts, &dataDirFlag),
		newWatchMergeCmd(opts, &dataDirFlag),
		newWatchActivityCmd(opts, &dataDirFlag),
		newWatchPollCmd(opts, &dataDirFlag),
		newWatchNextCmd(opts, &dataDirFlag),
		newWatchSendCmd(opts, &dataDirFlag),
		newWatchTakeoverCmd(opts, &dataDirFlag),
		newWatchHandbackCmd(opts, &dataDirFlag),
		newWatchReplyCmd(opts, &dataDirFlag),
		newWatchRetryCmd(opts, &dataDirFlag),
		newWatchProposalsCmd(opts, &dataDirFlag),
		newWatchApproveCmd(opts, &dataDirFlag),
		newWatchRejectCmd(opts, &dataDirFlag),
		newWatchModeCmd(opts, &dataDirFlag),
		newWatchMergeRulesCmd(opts, &dataDirFlag),
		newWatchOutputCmd(opts, &dataDirFlag),
		newWatchHookCmd(opts, &dataDirFlag),
	)
	return cmd
}

func newWatchStartCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		repo            string
		provider        string
		model           string
		effort          string
		includeExisting bool
		includeOwn      bool
		approvals       string
		mergeMethod     string
		approvalMode    string
		autoRebase      bool
		mergeWhenReady  bool
		keepWorktree    bool
		branchUpdate    store.BranchUpdate
		updateOnGitHub  bool
		noCheckout      bool
	)
	cmd := &cobra.Command{
		Use:   "start [target]",
		Short: "Start to watch a pull request from the current checkout",
		Long: `The target is a pull request URL, owner/name#number, or a number with
--repo. Without a target, the pull request of the current branch is
watched. Run the command in the checkout of the branch: the daemon makes
a worktree from it and reads your git identity from it.

With --no-checkout the current folder is not used. The daemon clones the
head repository once into <data dir>/checkouts/<owner>/<name> and makes
the worktree from that clone. The target must then name the repository
and the number, and the provider must be claude or copilot.

Only activity after the start goes to the agent, except that a branch
already in conflict with its base, or a check that already failed, is
told at once. A branch already behind its base goes to GitHub first, as
the next paragraph says, and to the agent when GitHub does not update
it.

When the branch falls behind its base, the daemon first asks GitHub to
update it with --branch-update (rebase or merge), and the agent does it
only when GitHub refuses. --update-on-github=false leaves it to the
agent. The agent solves a conflict with the same method.
--include-existing also reports the comments and reviews that already
exist on the pull request.

The daemon says when the pull request is ready to merge: checks green,
the approvals --approvals asks for, or the override of the repository,
or the setting of the daemon, or the rule of the base branch, nobody requesting changes, every review thread
resolved, nothing
pending from the agent. You merge it with
'babysitter watch merge' when it suits you. With --merge-when-ready the
daemon merges it with the method of the watch as soon as it is ready.

A flag you do not type takes the override of the repository
('babysitter repo config'), and without one the setting of the daemon
('babysitter settings'). The watch keeps the values it starts with.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			if _, err := snapshot.ParseTarget(target, repo); err != nil {
				return err
			}
			dir, err := sourceDirOf(noCheckout)
			if err != nil {
				return err
			}
			c, err := opts.daemonClient(*dataDirFlag)
			if err != nil {
				return err
			}
			var w httpd.Watch
			req := httpd.StartWatchRequest{
				Target: target, Repo: repo, Provider: provider, Model: model, Effort: effort, SourceDir: dir,
				MergeMethod:       typed(cmd, "merge-method", &mergeMethod),
				IncludeExisting:   typed(cmd, "include-existing", &includeExisting),
				IncludeOwn:        typed(cmd, "include-own", &includeOwn),
				ApprovalMode:      typed(cmd, "approval-mode", &approvalMode),
				AutoApproveRebase: typed(cmd, "auto-approve-rebase", &autoRebase),
				MergeWhenReady:    typed(cmd, "merge-when-ready", &mergeWhenReady),
				KeepWorktree:      typed(cmd, "keep-worktree", &keepWorktree),
				BranchUpdate:      typed(cmd, branchUpdateFlag, &branchUpdate),
				UpdateOnGitHub:    typed(cmd, updateOnGitHubFlag, &updateOnGitHub),
			}
			if req.ApprovalsRequired, err = typedApprovals(cmd, approvals); err != nil {
				return err
			}
			if err := c.post(cmd.Context(), "/watches", req, &w); err != nil {
				return err
			}
			if opts.output == outputJSON {
				return opts.print(cmd.OutOrStdout(), watchOutput(w))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Watching %s#%d (%s) as watch %d\n", w.Repo, w.Number, w.HeadRef, w.ID)
			if w.Provider == prwatch.ProviderSelf {
				fmt.Fprintf(cmd.OutOrStdout(), "You are the agent, in %s; %s\n", w.SourceDir, nextHint(w))
				if ignored := ignoredOnSelf(cmd, approvalMode); len(ignored) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "A self watch has no approval gate: your session pushes and replies itself, so the watch runs in auto and ignores %s.\n",
						textx.JoinAnd(ignored))
				}
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Worktree: %s\n", w.WorktreeDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repository of a bare number, as owner/name")
	cmd.Flags().StringVar(&provider, "provider", "", "AI provider that babysits the watch: claude, copilot, or self when your own session is the agent and takes each message with 'watch next'; without the flag the repository, then the daemon, decides")
	cmd.Flags().StringVar(&model, "model", "", "model of that provider, empty for the model of the layer that gives the provider; none for self")
	cmd.Flags().StringVar(&effort, "effort", "", "effort level of that model, for example low, medium or high; empty for the effort of the layer that gives the model; none for self")
	cmd.Flags().BoolVar(&includeExisting, "include-existing", false, "also report the review items that already exist; without the flag the repository, then the daemon, decides")
	cmd.Flags().BoolVar(&includeOwn, "include-own", false, "also report your own comments to the agent, for a repository you review yourself; without the flag the repository, then the daemon, decides")
	cmd.Flags().StringVar(&approvals, "approvals", "", "approvals the pull request needs before it is ready to merge: a number, 0 for none, or 'branch' for the rule of the base branch; without the flag the repository, then the daemon, decides")
	cmd.Flags().StringVar(&mergeMethod, "merge-method", "", "merge method of the watch: squash, merge, rebase, or empty for the first method the repository allows; without the flag the repository, then the daemon, decides")
	cmd.Flags().StringVar(&approvalMode, "approval-mode", "", "manual holds the work of each turn of the agent until you approve it, auto pushes and posts when the turn ends; without the flag the repository, then the daemon, decides")
	cmd.Flags().BoolVar(&autoRebase, "auto-approve-rebase", false, "let approved work go out after a clean rebase or merge without asking again; without the flag the repository, then the daemon, decides")
	cmd.Flags().BoolVar(&mergeWhenReady, "merge-when-ready", false, "the daemon merges with the method of the watch as soon as the watch is ready to merge; off without the flag")
	cmd.Flags().BoolVar(&keepWorktree, "keep-worktree", false, "a stop leaves the worktree of the watch on disk; without the flag the repository, then the daemon, decides")
	cmd.Flags().StringVar((*string)(&branchUpdate), branchUpdateFlag, "", "how the branch is updated when it falls behind its base: rebase or merge. The agent solves a conflict the same way; without the flag the repository, then the daemon, decides")
	cmd.Flags().BoolVar(&updateOnGitHub, updateOnGitHubFlag, false, "ask GitHub to update a branch that fell behind its base before the agent does it; without the flag the repository, then the daemon, decides")
	cmd.Flags().BoolVar(&noCheckout, "no-checkout", false, "do not use the current folder: the daemon clones the head repository into its data directory and makes the worktree from that clone")
	return cmd
}

func sourceDirOf(noCheckout bool) (string, error) {
	if noCheckout {
		return "", nil
	}
	return os.Getwd()
}

func newWatchListCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the watched pull requests",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := opts.daemonClient(*dataDirFlag)
			if err != nil {
				return err
			}
			var l httpd.WatchList
			if err := c.get(cmd.Context(), "/watches?status="+watchStatus(all), &l); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), watchListOutput(l))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include stopped watches")
	return cmd
}

func onWatch(opts *options, dataDirFlag *string, run func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		c, err := opts.daemonClient(*dataDirFlag)
		if err != nil {
			return err
		}
		w, err := findWatch(cmd.Context(), c, args[0])
		if err != nil {
			return err
		}
		return run(cmd, c, w, args[1:])
	}
}

func newWatchStatusCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status <watch>",
		Short: "Show one watch",
		Long:  "The watch is its id, a pull request URL or owner/name#number.",
		Args:  cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, _ *daemonClient, w httpd.Watch, _ []string) error {
			return opts.print(cmd.OutOrStdout(), watchOutput(w))
		}),
	}
}

func newWatchStopCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var keepWorktree bool
	cmd := &cobra.Command{
		Use:   "stop <watch>",
		Short: "Stop watching a pull request and print the summary",
		Long:  "The stop deletes the worktree of the watch, unless the rule the watch started with says to keep it. Use --keep-worktree to leave it on disk, or --keep-worktree=false to delete it.",
		Args:  cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			body := httpd.StopWatchRequest{KeepWorktree: typed(cmd, "keep-worktree", &keepWorktree)}
			var stopped httpd.Watch
			if err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/stop", w.ID), body, &stopped); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), watchOutput(stopped))
		}),
	}
	cmd.Flags().BoolVar(&keepWorktree, "keep-worktree", false, "leave the worktree of the watch on disk; without the flag the rule the watch started with decides")
	return cmd
}

func newWatchMergeCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		method  string
		approve bool
	)
	cmd := &cobra.Command{
		Use:   "merge <watch>",
		Short: "Merge the pull request of a watch and stop the watch",
		Long: `The daemon takes a fresh look at the pull request first and refuses
while something blocks the merge; the reasons are printed. The merge
uses --method, else the method of the watch, else the first method the
repository allows. A merge stops the watch and prints its summary.

--approve first submits an approving review in your name. The daemon
does this only for a pull request of Dependabot whose update is within
the merge scope of the repository (see 'babysitter repo config').`,
		Args: cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			body := httpd.MergeWatchRequest{Method: method, Approve: approve}
			var merged httpd.Watch
			if err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/merge", w.ID), body, &merged); err != nil {
				return mergeError(err)
			}
			return opts.print(cmd.OutOrStdout(), watchOutput(merged))
		}),
	}
	cmd.Flags().StringVar(&method, "method", "", "merge method for this merge: squash, merge or rebase")
	cmd.Flags().BoolVar(&approve, "approve", false, "approve the Dependabot update in your name before the merge")
	return cmd
}

func mergeError(err error) error {
	const prefix = "the pull request is not ready to merge: "
	text := err.Error()
	if !strings.HasPrefix(text, prefix) {
		return err
	}
	blockers := strings.Split(strings.TrimPrefix(text, prefix), "; ")
	return errors.New("the pull request is not ready to merge:\n  " + strings.Join(blockers, "\n  "))
}

func newWatchActivityCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		since int64
		limit int
	)
	cmd := &cobra.Command{
		Use:   "activity <watch>",
		Short: "Show what happened on a watched pull request",
		Args:  cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			var l httpd.ActivityList
			path := fmt.Sprintf("/watches/%d/activity?since=%d&limit=%d", w.ID, since, limit)
			if err := c.get(cmd.Context(), path, &l); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), activityListOutput(l))
		}),
	}
	cmd.Flags().Int64Var(&since, "since", 0, "only rows with an id above this")
	cmd.Flags().IntVar(&limit, "limit", 0, "at most this many rows, default 200")
	return cmd
}

func watchStatus(all bool) string {
	if all {
		return "all"
	}
	return "active"
}

func findWatch(ctx context.Context, c *daemonClient, arg string) (httpd.Watch, error) {
	if id, err := strconv.ParseInt(arg, 10, 64); err == nil && id > 0 {
		var w httpd.Watch
		if err := c.get(ctx, fmt.Sprintf("/watches/%d", id), &w); err != nil {
			return httpd.Watch{}, err
		}
		return w, nil
	}
	target, err := snapshot.ParseTarget(arg, "")
	if err != nil {
		return httpd.Watch{}, err
	}
	var l httpd.WatchList
	if err := c.get(ctx, "/watches?status=all", &l); err != nil {
		return httpd.Watch{}, err
	}
	var found *httpd.Watch
	for i := range l.Watches {
		w := &l.Watches[i]
		if !strings.EqualFold(w.Repo, target.Repo()) || w.Number != target.Number {
			continue
		}
		if found == nil || (w.Status == store.WatchActive && found.Status != store.WatchActive) || (w.Status == found.Status && w.ID > found.ID) {
			found = w
		}
	}
	if found == nil {
		return httpd.Watch{}, fmt.Errorf("no watch for %s#%d", target.Repo(), target.Number)
	}
	return *found, nil
}

func newWatchPollCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "poll <watch>",
		Short: "Ask the daemon to look at a watched pull request now",
		Long: `The daemon polls the pull request outside its schedule, for example
right after a push. The poll runs in the background; read what it found
with 'babysitter watch activity' or 'babysitter watch status'.`,
		Args: cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			var accepted httpd.SyncAccepted
			if err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/poll", w.ID), nil, &accepted); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), pollOutput{WatchID: w.ID, SyncAccepted: accepted})
		}),
	}
}

type pollOutput struct {
	WatchID int64 `json:"watchId"`
	httpd.SyncAccepted
}

func (p pollOutput) writeText(out io.Writer) error {
	_, err := fmt.Fprintf(out, "Polling watch %d now\n", p.WatchID)
	return err
}

type nextOutput httpd.NextMessage

func (n nextOutput) writeText(out io.Writer) error {
	if n.Message != nil {
		text, _ := n.Message.Payload["message"].(string)
		_, err := fmt.Fprintf(out, "Message %d for watch %d:\n\n%s", n.Message.ID, n.Watch.ID, text)
		return err
	}
	w := n.Watch
	if w.Status != store.WatchActive {
		return watchOutput(w).writeText(out)
	}
	stand := fmt.Sprintf("head %s, checks %s, %s", orDash(textx.ShortSHA(w.HeadSHA)), checks.Summarize(w.CheckStates, w.HeadSHA, w.GreenSHA), mergeLine(w))
	if w.ReadySince != nil {
		stand = "the pull request is ready to merge, " + mergeLine(w)
	}
	_, err := fmt.Fprintf(out, "Nothing to do on watch %d: %s\n", w.ID, stand)
	return err
}

const nextWaitGrace = 5 * time.Minute

func newWatchNextCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var wait time.Duration
	cmd := &cobra.Command{
		Use:   "next <watch>",
		Short: "Take the next message of a watch whose agent is your own session",
		Long: `A watch started with --provider self has no agent session in the
daemon: the session that started it is the agent, and this command hands
it the next message. The daemon takes a fresh look at the pull request,
then answers with the actionable activity the agent was not told about
yet, as one message: the review comments with their ids, the reviews,
the failed checks with the log of each failed job, the branch behind its
base or in conflict with it. The message is recorded as a nudged row, so
nothing is handed out twice.

With --wait, the command waits up to that long for something to happen
when there is nothing to do. It returns early when the watch stops or the
pull request is ready to merge, which the agent has to know as well.
Without a message the text form says where the watch stands; the JSON
form carries the watch alone, with no message key.

One caller at a time: the daemon refuses this command while another
call already waits for the message of the same watch.`,
		Args: cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			if wait < 0 {
				return errors.New("--wait must not be negative")
			}
			wait = min(wait, httpd.MaxNextWait)
			c.http.Timeout = wait + nextWaitGrace
			var out httpd.NextMessage
			path := fmt.Sprintf("/watches/%d/next?wait=%s", w.ID, wait)
			if err := c.post(cmd.Context(), path, nil, &out); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), nextOutput(out))
		}),
	}
	cmd.Flags().DurationVar(&wait, "wait", 0, "how long to wait for a message when there is none, for example 5m; at most 10m")
	return cmd
}

type messageOutput httpd.Activity

func (m messageOutput) writeText(out io.Writer) error {
	_, err := fmt.Fprintf(out, "Sent to the agent of watch %d: %s\n", m.WatchID, m.Summary)
	return err
}

func newWatchSendCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "send <watch> <message>...",
		Short: "Send a message to the agent of a watched pull request",
		Long: `The message is typed into the session of the agent, as if you had. The
words after the watch are joined with spaces. The agent takes it while
it works or waits; it is refused while the agent waits on a permission
decision, where the message would decide in your place.`,
		Args: cobra.MinimumNArgs(2),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, args []string) error {
			body := httpd.SendMessageRequest{Message: strings.Join(args, " ")}
			var row httpd.Activity
			if err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/send", w.ID), body, &row); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), messageOutput(row))
		}),
	}
}

type replyOutput struct {
	watch int64
	httpd.ReplyResult
}

func (m replyOutput) writeText(out io.Writer) error {
	if m.Posted != nil {
		_, err := fmt.Fprintf(out, "Posted on the pull request of watch %d: %s\n", m.watch, m.Posted.URL)
		return err
	}
	_, err := fmt.Fprintf(out, "Recorded the reply of watch %d in proposal %d. The daemon posts it when your turn ends.\n", m.watch, m.Proposal)
	return err
}

func newWatchReplyCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var inReplyTo int64
	cmd := &cobra.Command{
		Use:   "reply <watch> <text>...",
		Short: "Reply on a watched pull request through the daemon",
		Long: `The agent of a watch replies with this command. The daemon records the
text, and posts it on the pull request as the token user when the turn
of the agent ends, after it pushes the commits of that turn. The reply
of a watch whose agent is your own session goes out at once. The daemon
remembers the comment it made, so the next poll never reports it back
to the agent as feedback. With --to, the text answers a comment: a
review comment in its thread, or a comment on the conversation on the
conversation. A second reply to the same comment takes the place of the
first one before it goes out. Without --to, the text is a comment on
the pull request, which answers a review or no comment. The words after
the watch are joined with spaces.`,
		Args: cobra.MinimumNArgs(2),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, args []string) error {
			body := httpd.ReplyRequest{InReplyTo: inReplyTo, Body: strings.Join(args, " ")}
			var res httpd.ReplyResult
			if err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/reply", w.ID), body, &res); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), replyOutput{watch: w.ID, ReplyResult: res})
		}),
	}
	cmd.Flags().Int64Var(&inReplyTo, "to", 0, "the comment the text answers: a review comment, in its thread, or a comment on the conversation")
	return cmd
}

func newWatchRetryCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "retry <watch> [proposal]",
		Short: "Push and post a proposal whose release failed",
		Long: `The daemon pushes the commits of each turn of the agent and posts its
replies. When that fails, the activity of the watch says why and names
this command. A retry pushes again, and when the pull request branch
moved since, it rebases a turn that only added commits onto it first, or merges the branch into the work when the watch merges.
Work the daemon cannot rebase or merge, a rebase or merge that conflicts or a rewrite
that lacks commits of the branch, goes to the agent, which brings it up
to the branch in its next turn. Without a number, the newest proposal
that failed is taken.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, args []string) error {
			number, err := proposalNumber(cmd.Context(), c, w, args, "failed", isFailed)
			if err != nil {
				return err
			}
			var p httpd.Proposal
			if err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/proposals/%d/retry", w.ID, number), nil, &p); err != nil {
				return err
			}
			if p.Status == store.ProposalFailed {
				return fmt.Errorf("proposal %d of watch %d failed again: %s", p.Number, w.ID, p.Error)
			}
			return opts.print(cmd.OutOrStdout(), releasedOutput{watch: w, Proposal: p})
		}),
	}
}

type sessionOutput httpd.SessionOutput

func (s sessionOutput) writeText(out io.Writer) error {
	_, err := io.WriteString(out, s.Output)
	return err
}

func newWatchOutputCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var lines int
	cmd := &cobra.Command{
		Use:   "output <watch>",
		Short: "Print the last lines the agent of a watched pull request printed",
		Long: `The output is the terminal of the agent as it was drawn, escape
sequences included, so it reads best in a terminal. Without a running
session, the log of the last one answers.`,
		Args: cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			var out httpd.SessionOutput
			if err := c.get(cmd.Context(), fmt.Sprintf("/watches/%d/output?lines=%d", w.ID, lines), &out); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), sessionOutput(out))
		}),
	}
	cmd.Flags().IntVar(&lines, "lines", 200, "at most this many lines from the end, 0 for everything kept")
	return cmd
}

const hookTimeout = 5 * time.Second

func newWatchHookCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var watch int64
	cmd := &cobra.Command{
		Use:    "hook <event>",
		Short:  "Report an event of an agent session to the daemon and apply its verdict on a tool",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			event, payload := args[0], hookPayload(cmd.InOrStdin())
			verdict, err := reportHook(cmd, opts, *dataDirFlag, watch, event, payload)
			if event != agent.EventPreToolUse {
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "babysitter hook:", err)
				}
				return nil
			}
			if err != nil {
				return refuseTool(cmd.OutOrStdout(), "babysitter: the daemon did not answer, try again ("+err.Error()+")")
			}
			if verdict.Decision == httpd.HookAllow {
				return nil
			}
			return refuseTool(cmd.OutOrStdout(), verdict.Reason)
		},
	}
	cmd.Flags().Int64Var(&watch, "watch", 0, "the watch the session belongs to")
	return cmd
}

const refusedToolExitCode = 2

func hookPayload(r io.Reader) map[string]any {
	payload := map[string]any{}
	data, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		_ = json.Unmarshal(data, &payload)
	}
	return payload
}

func refuseTool(w io.Writer, reason string) error {
	_ = json.NewEncoder(w).Encode(map[string]string{
		"permissionDecision":       "deny",
		"permissionDecisionReason": reason,
	})
	return &ExitCodeError{Code: refusedToolExitCode, Err: errors.New(reason)}
}

func reportHook(cmd *cobra.Command, opts *options, dataDir string, watch int64, event string, payload map[string]any) (httpd.HookResponse, error) {
	var verdict httpd.HookResponse
	if watch <= 0 {
		return verdict, fmt.Errorf("a watch is required")
	}
	c, err := opts.daemonClient(dataDir)
	if err != nil {
		return verdict, err
	}
	timeout := hookTimeoutWithin(opts.timeout)
	c.http.Timeout = timeout
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	if err := c.post(ctx, fmt.Sprintf("/watches/%d/hook", watch), httpd.HookRequest{Event: event, Payload: payload}, &verdict); err != nil {
		return verdict, err
	}
	if !isVerdict(verdict) {
		return verdict, errors.New("the daemon answered with no verdict")
	}
	return verdict, nil
}

func hookTimeoutWithin(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return hookTimeout
	}
	return min(timeout, hookTimeout)
}

func isVerdict(v httpd.HookResponse) bool {
	refusesWithReason := v.Decision == httpd.HookDeny && v.Reason != ""
	return v.Decision == httpd.HookAllow || refusesWithReason
}

func typed[T any](cmd *cobra.Command, flag string, v *T) *T {
	if !cmd.Flags().Changed(flag) {
		return nil
	}
	return v
}
