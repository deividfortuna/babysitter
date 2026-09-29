package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/store"
)

const approvalsBranch = "branch"

type settingsOutput httpd.Settings

func (s settingsOutput) writeText(out io.Writer) error {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Repository poll interval\t%s\n", time.Duration(s.PollIntervalSeconds)*time.Second)
	fmt.Fprintf(tw, "Watch poll interval\t%s\n", time.Duration(s.WatchIntervalSeconds)*time.Second)
	fmt.Fprintf(tw, "Longest watch poll interval\t%s\n", time.Duration(s.WatchMaxIntervalSeconds)*time.Second)
	fmt.Fprintf(tw, "Longest check read interval\t%s\n", time.Duration(s.CheckMaxIntervalSeconds)*time.Second)
	fmt.Fprintf(tw, "Approvals\t%s\n", approvalsWord(s.ApprovalsRequired))
	fmt.Fprintf(tw, "Merge method\t%s\n", mergeMethodWord(s.MergeMethod))
	fmt.Fprintf(tw, "Report items that already exist\t%s\n", yesNo(s.IncludeExisting))
	fmt.Fprintf(tw, "Report my own comments\t%s\n", yesNo(s.IncludeOwn))
	fmt.Fprintf(tw, "Keep the worktree on stop\t%s\n", yesNo(s.KeepWorktree))
	fmt.Fprintf(tw, "Show notifications\t%s\n", yesNo(s.NotificationsEnabled))
	fmt.Fprintf(tw, "Notification sound\t%s\n", yesNo(s.NotificationSound))
	fmt.Fprintf(tw, "Muted notification kinds\t%s\n", mutedKindsWord(s.MutedNotificationKinds))
	fmt.Fprintf(tw, "Approval mode\t%s\n", s.ApprovalMode)
	fmt.Fprintf(tw, "Approve a clean rebase on its own\t%s\n", yesNo(s.AutoApproveRebase))
	fmt.Fprintf(tw, "Branch behind its base\t%s\n", branchUpdateWord(s.BranchUpdate, s.UpdateOnGitHub))
	fmt.Fprintf(tw, "Agent\t%s\n", providerLabel(s.Provider, s.Model, s.Effort))
	return tw.Flush()
}

func mutedKindsWord(kinds []string) string {
	if len(kinds) == 0 {
		return "none"
	}
	return strings.Join(kinds, ", ")
}

func approvalsWord(n *int) string {
	if n == nil {
		return "rule of the base branch"
	}
	return strconv.Itoa(*n)
}

func mergeMethodWord(method string) string {
	if method == "" {
		return "first method the repository allows"
	}
	return method
}

func yesNo(on bool) string {
	if on {
		return "yes"
	}
	return "no"
}

func newSettingsCmd(opts *options) *cobra.Command {
	var dataDirFlag string
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Read and write the settings of the running daemon",
		Long: `The settings of the daemon: how often it polls, and what a new watch
takes when you do not say. They are the same settings the Watching pane
of the desktop app shows, and a change takes effect at once.

'settings set' writes only the flags you type; the rest keep the value
they have.`,
	}
	cmd.PersistentFlags().StringVar(&dataDirFlag, "data-dir", "", "directory of running.json")
	cmd.AddCommand(newSettingsGetCmd(opts, &dataDirFlag), newSettingsSetCmd(opts, &dataDirFlag))
	return cmd
}

func newSettingsGetCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print the settings of the daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := opts.daemonClient(*dataDirFlag)
			if err != nil {
				return err
			}
			current, err := readSettings(cmd.Context(), c)
			if err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), settingsOutput(current))
		},
	}
}

func newSettingsSetCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		pollInterval    time.Duration
		watchInterval   time.Duration
		watchMax        time.Duration
		checkMax        time.Duration
		approvals       string
		mergeMethod     string
		includeExisting bool
		includeOwn      bool
		keepWorktree    bool
		notifications   bool
		sound           bool
		mutedKinds      string
		approvalMode    string
		autoRebase      bool
		provider        string
		model           string
		effort          string
		branchUpdate    string
		updateOnGitHub  bool
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Change one or more settings of the daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var wanted *int
			if cmd.Flags().Changed("approvals") {
				var err error
				if wanted, err = parseApprovals(approvals); err != nil {
					return err
				}
			}
			muted := notificationKinds(mutedKinds)
			settings := []struct {
				flag  string
				write func(*httpd.Settings)
			}{
				{"poll-interval", func(s *httpd.Settings) { s.PollIntervalSeconds = int(pollInterval.Seconds()) }},
				{"watch-interval", func(s *httpd.Settings) { s.WatchIntervalSeconds = int(watchInterval.Seconds()) }},
				{"watch-max-interval", func(s *httpd.Settings) { s.WatchMaxIntervalSeconds = int(watchMax.Seconds()) }},
				{"check-max-interval", func(s *httpd.Settings) { s.CheckMaxIntervalSeconds = int(checkMax.Seconds()) }},
				{"approvals", func(s *httpd.Settings) { s.ApprovalsRequired = wanted }},
				{"merge-method", func(s *httpd.Settings) { s.MergeMethod = mergeMethod }},
				{"include-existing", func(s *httpd.Settings) { s.IncludeExisting = includeExisting }},
				{"include-own", func(s *httpd.Settings) { s.IncludeOwn = includeOwn }},
				{"keep-worktree", func(s *httpd.Settings) { s.KeepWorktree = keepWorktree }},
				{"notifications", func(s *httpd.Settings) { s.NotificationsEnabled = notifications }},
				{"notification-sound", func(s *httpd.Settings) { s.NotificationSound = sound }},
				{"mute-notifications", func(s *httpd.Settings) { s.MutedNotificationKinds = muted }},
				{"approval-mode", func(s *httpd.Settings) { s.ApprovalMode = approvalMode }},
				{"auto-approve-rebase", func(s *httpd.Settings) { s.AutoApproveRebase = autoRebase }},
				{"provider", func(s *httpd.Settings) { s.Provider, s.Model, s.Effort = provider, "", "" }},
				{"model", func(s *httpd.Settings) { s.Model, s.Effort = model, "" }},
				{"effort", func(s *httpd.Settings) { s.Effort = effort }},
				{branchUpdateFlag, func(s *httpd.Settings) { s.BranchUpdate = branchUpdate }},
				{updateOnGitHubFlag, func(s *httpd.Settings) { s.UpdateOnGitHub = updateOnGitHub }},
			}

			var asked []func(*httpd.Settings)
			for _, s := range settings {
				if cmd.Flags().Changed(s.flag) {
					asked = append(asked, s.write)
				}
			}
			if len(asked) == 0 {
				return fmt.Errorf("nothing to change: name at least one setting, for example --watch-interval 45s")
			}

			c, err := opts.daemonClient(*dataDirFlag)
			if err != nil {
				return err
			}
			next, err := readSettings(cmd.Context(), c)
			if err != nil {
				return err
			}
			for _, write := range asked {
				write(&next)
			}
			var saved httpd.Settings
			if err := c.put(cmd.Context(), "/settings", next, &saved); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), settingsOutput(saved))
		},
	}
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 0, "time between passes over the repositories you watch, 10s to 24h")
	cmd.Flags().DurationVar(&watchInterval, "watch-interval", 0, "time between polls of a watched pull request, 10s to 24h")
	cmd.Flags().DurationVar(&watchMax, "watch-max-interval", 0, "longest time between polls of a watched pull request where nothing happens, from the watch interval to 24h; the watch interval itself keeps one fixed interval")
	cmd.Flags().DurationVar(&checkMax, "check-max-interval", 0, "longest time between reads of the pending checks of an open pull request by the repository watcher, 10s to 24h")
	cmd.Flags().StringVar(&approvals, "approvals", "", "approvals a new watch wants before the pull request is ready to merge: a number, 0 for none, or 'branch' for the rule of the base branch")
	cmd.Flags().StringVar(&mergeMethod, "merge-method", "", "merge method of a new watch: squash, merge, rebase, or empty for the first one the repository allows")
	cmd.Flags().BoolVar(&includeExisting, "include-existing", false, "a new watch reports the review items that already exist")
	cmd.Flags().BoolVar(&includeOwn, "include-own", false, "a new watch reports the comments of your own user")
	cmd.Flags().BoolVar(&keepWorktree, "keep-worktree", false, "a watch that stops leaves its worktree on disk")
	cmd.Flags().BoolVar(&notifications, "notifications", true, "show what happens on a watched pull request as a notification of the operating system")
	cmd.Flags().BoolVar(&sound, "notification-sound", true, "let a notification make a sound")
	cmd.Flags().StringVar(&approvalMode, "approval-mode", "", "who releases the work of a turn of the agent of a new watch: manual waits for you, auto pushes and posts when the turn ends")
	cmd.Flags().BoolVar(&autoRebase, "auto-approve-rebase", false, "a new watch lets approved work go out after a clean rebase without asking again")
	cmd.Flags().StringVar(&provider, "provider", "", "AI provider of a new watch: claude or copilot; a new provider takes its default model and effort unless --model and --effort name them")
	cmd.Flags().StringVar(&model, "model", "", "model of the provider of a new watch, empty for its default; a new model takes its default effort unless --effort names one")
	cmd.Flags().StringVar(&effort, "effort", "", "effort level of that model, empty for its default, for example low, medium or high")
	cmd.Flags().StringVar(&branchUpdate, branchUpdateFlag, "", "how a new watch updates a branch that fell behind its base: rebase or merge. The agent solves a conflict the same way")
	cmd.Flags().BoolVar(&updateOnGitHub, updateOnGitHubFlag, true, "a new watch asks GitHub to update a branch that fell behind its base, and the agent does it only when GitHub refuses")
	cmd.Flags().StringVar(&mutedKinds, "mute-notifications", "", "notification kinds that reach nobody, separated by commas: "+store.JoinKinds()+". An empty list shows them all again")
	return cmd
}

func notificationKinds(value string) []string {
	out := []string{}
	for part := range strings.SplitSeq(value, ",") {
		if name := strings.TrimSpace(part); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func readSettings(ctx context.Context, c *daemonClient) (httpd.Settings, error) {
	var current httpd.Settings
	if err := c.get(ctx, "/settings", &current); err != nil {
		return httpd.Settings{}, err
	}
	return current, nil
}

func parseApprovals(value string) (*int, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, approvalsBranch) {
		return nil, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("approvals takes a whole number, or %q, got %q", approvalsBranch, value)
	}
	return &n, nil
}

func typedApprovals(cmd *cobra.Command, value string) (httpd.Optional[int], error) {
	if !cmd.Flags().Changed("approvals") {
		return httpd.Optional[int]{}, nil
	}
	wanted, err := parseApprovals(value)
	if err != nil {
		return httpd.Optional[int]{}, err
	}
	return httpd.Optional[int]{Set: true, Value: wanted}, nil
}
