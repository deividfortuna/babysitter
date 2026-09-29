package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/autostart"
	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/store"
)

const approvalsDefault = "default"

type overridesOutput struct {
	Provider          string `json:"provider,omitempty"`
	Model             string `json:"model,omitempty"`
	Effort            string `json:"effort,omitempty"`
	ApprovalMode      string `json:"approval_mode,omitempty"`
	MergeMethod       string `json:"merge_method,omitempty"`
	ApprovalsRequired string `json:"approvals_required,omitempty"`
	IncludeExisting   *bool  `json:"include_existing,omitempty"`
	AutoApproveRebase *bool  `json:"auto_approve_rebase,omitempty"`
	IncludeOwn        *bool  `json:"include_own,omitempty"`
	KeepWorktree      *bool  `json:"keep_worktree,omitempty"`
	BranchUpdate      string `json:"branch_update,omitempty"`
	UpdateOnGitHub    *bool  `json:"update_on_github,omitempty"`
}

type repoConfigOutput struct {
	Repository          string          `json:"repository"`
	CheckoutDir         string          `json:"checkout_dir"`
	AutoStartMine       bool            `json:"auto_start_mine"`
	AutoStartMineSince  *time.Time      `json:"auto_start_mine_since,omitempty"`
	IncludeDrafts       bool            `json:"include_drafts"`
	AutoWatchDependabot bool            `json:"auto_watch_dependabot"`
	AutoWatchSince      *time.Time      `json:"auto_watch_dependabot_since,omitempty"`
	Overrides           overridesOutput `json:"overrides"`
	DependabotScope     string          `json:"dependabot_scope"`
	DependabotApproval  string          `json:"dependabot_approval"`
	DependabotLimit     int             `json:"dependabot_limit"`
}

func configOutput(repo store.Repo, c store.RepoConfig) repoConfigOutput {
	o := c.Overrides
	out := repoConfigOutput{
		Repository: repo.FullName(), CheckoutDir: c.CheckoutDir,
		AutoStartMine: c.OwnOn(), AutoStartMineSince: c.OwnSince, IncludeDrafts: c.IncludeDrafts,
		AutoWatchDependabot: c.DependabotOn(), AutoWatchSince: c.DependabotSince,
		Overrides: overridesOutput{
			Provider: o.Provider, Model: o.Model, Effort: o.Effort, ApprovalMode: string(o.ApprovalMode), MergeMethod: o.MergeMethod,
			IncludeExisting: o.IncludeExisting, AutoApproveRebase: o.AutoApproveRebase, IncludeOwn: o.IncludeOwn, KeepWorktree: o.KeepWorktree,
			BranchUpdate: string(o.BranchUpdate), UpdateOnGitHub: o.UpdateOnGitHub,
		},
		DependabotScope: string(c.DependabotScope), DependabotApproval: string(c.DependabotApproval), DependabotLimit: c.DependabotLimit,
	}
	if o.ApprovalsSet {
		out.Overrides.ApprovalsRequired = approvalsBranch
		if o.Approvals != nil {
			out.Overrides.ApprovalsRequired = strconv.Itoa(*o.Approvals)
		}
	}
	return out
}

func (c repoConfigOutput) writeText(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Repository:\t%s\n", c.Repository)
	fmt.Fprintf(tw, "Checkout:\t%s\n", orDash(c.CheckoutDir))
	fmt.Fprintf(tw, "My pull requests:\t%s\n", toggleWord(c.AutoStartMineSince))
	fmt.Fprintf(tw, "Include drafts:\t%s\n", onOff(c.IncludeDrafts))
	fmt.Fprintf(tw, "Dependabot:\t%s\n", toggleWord(c.AutoWatchSince))
	fmt.Fprintf(tw, "Merge scope:\t%s\n", c.DependabotScope)
	fmt.Fprintf(tw, "Approval:\t%s\n", c.DependabotApproval)
	fmt.Fprintf(tw, "At the same time:\t%d\n", c.DependabotLimit)
	fmt.Fprintf(tw, "Watch defaults:\t%s\n", c.Overrides.words())
	return tw.Flush()
}

func toggleWord(since *time.Time) string {
	if since == nil {
		return "off"
	}
	return "on since " + since.Local().Format("2006-01-02 15:04")
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func (o overridesOutput) words() string {
	var parts []string
	add := func(label, value string) {
		if value != "" {
			parts = append(parts, label+" "+value)
		}
	}
	add("provider", o.Provider)
	add("model", o.Model)
	add("effort", o.Effort)
	add("approval mode", o.ApprovalMode)
	add("merge method", o.MergeMethod)
	add("approvals", o.ApprovalsRequired)
	add("branch update", o.BranchUpdate)
	addBool := func(label string, value *bool) {
		if value != nil {
			add(label, strconv.FormatBool(*value))
		}
	}
	addBool("include existing", o.IncludeExisting)
	addBool("auto approve rebase", o.AutoApproveRebase)
	addBool("include own", o.IncludeOwn)
	addBool("keep worktree", o.KeepWorktree)
	addBool("update on GitHub", o.UpdateOnGitHub)
	if len(parts) == 0 {
		return "the settings of the daemon"
	}
	return strings.Join(parts, ", ")
}

type repoConfigFlags struct {
	checkout        string
	mine            bool
	drafts          bool
	dependabot      bool
	provider        string
	model           string
	effort          string
	approvalMode    string
	mergeMethod     string
	approvals       string
	includeExisting bool
	autoRebase      bool
	includeOwn      bool
	keepWorktree    bool
	branchUpdate    string
	updateOnGitHub  bool
	resetOverrides  bool
	scope           string
	approval        string
	limit           int
}

func newRepoConfigCmd(opts *options) *cobra.Command {
	var f repoConfigFlags
	cmd := &cobra.Command{
		Use:   "config <owner/name>",
		Short: "Show or change what babysitter does with new pull requests of a repository",
		Long: `Without flags the command shows the configuration. Each flag changes one
field and keeps the others.

--checkout names the git checkout that auto start makes each worktree
from. Its origin must be the repository. Without it, the daemon clones
the repository once into <data dir>/checkouts/<owner>/<name> and makes
each worktree from that clone.

--auto-start-mine starts a watch on each new pull request that you opened
or that is assigned to you. --auto-watch-dependabot starts a watch on
each new pull request of Dependabot, up to --dependabot-limit at the same
time; the rest wait in the queue ('babysitter repo queue'). A toggle
takes only the pull requests created after it went on. Turn one off with
--auto-start-mine=false. The watches that run go on.

The override flags (--provider, --model, --effort, --approval-mode, --merge-method,
--approvals, --include-existing, --auto-approve-rebase, --include-own,
--keep-worktree, --branch-update, --update-on-github) set the overrides of each watch that starts on the
repository, by hand or by auto start. A field that 'watch start' does
not name takes the override, and a field without an override takes the
setting of the daemon. --approvals default and --reset-overrides give
the field back to the settings of the daemon.

--dependabot-scope is the highest update that merges on its own: patch,
minor or major. --dependabot-approval is never, ask (a notification asks
you to approve), or green (the daemon approves in your name when the
build is green and the update is in scope).

The daemon starts nothing with 'babysitter serve'; auto start runs in
'babysitter daemon start' and in the app.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			owner, name, err := store.ParseFullName(args[0])
			if err != nil {
				return err
			}
			st, err := opts.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			repo, err := st.GetRepo(ctx, owner, name)
			if err != nil {
				return err
			}
			cfg, err := st.GetRepoConfig(ctx, repo.ID)
			if err != nil {
				return err
			}
			change, err := f.change(cmd, cfg.Overrides)
			if err != nil {
				return err
			}
			if change != (autostart.Change{}) {
				if cfg, err = autostart.Configure(ctx, st, repo, change, time.Now()); err != nil {
					return err
				}
			}
			return opts.print(cmd.OutOrStdout(), configOutput(repo, cfg))
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&f.checkout, "checkout", "", "the git checkout of the repository that auto start makes each worktree from; empty clears it")
	fl.BoolVar(&f.mine, "auto-start-mine", false, "start a watch on each new pull request that you opened or that is assigned to you")
	fl.BoolVar(&f.drafts, "include-drafts", false, "auto start also takes your drafts")
	fl.BoolVar(&f.dependabot, "auto-watch-dependabot", false, "start a watch on each new pull request of Dependabot")
	fl.StringVar(&f.provider, "provider", "", "AI provider of the watches on the repository: claude or copilot; empty takes the provider of the daemon")
	fl.StringVar(&f.model, "model", "", "model of that provider, empty for its default; a new model takes its default effort unless --effort names one")
	fl.StringVar(&f.effort, "effort", "", "effort level of that model, empty for its default, for example low, medium or high")
	fl.StringVar(&f.approvalMode, "approval-mode", "", "manual or auto for the watches on the repository; empty takes the setting of the daemon")
	fl.StringVar(&f.mergeMethod, "merge-method", "", "merge method of the watches on the repository: squash, merge or rebase; empty takes the setting of the daemon")
	fl.StringVar(&f.approvals, "approvals", "", "approvals the watches on the repository need: a number, 0 for none, 'branch' for the rule of the base branch, or 'default' for the setting of the daemon")
	fl.BoolVar(&f.includeExisting, "include-existing", false, "the watches on the repository also report the review items that already exist")
	fl.BoolVar(&f.autoRebase, "auto-approve-rebase", false, "the watches on the repository let approved work go out after a clean rebase without asking again")
	fl.BoolVar(&f.includeOwn, "include-own", false, "the watches on the repository also report your own comments")
	fl.BoolVar(&f.keepWorktree, "keep-worktree", false, "a stop leaves the worktree of a watch on the repository on disk")
	fl.StringVar(&f.branchUpdate, branchUpdateFlag, "", "how the watches on the repository update a branch that fell behind its base: rebase or merge; empty takes the setting of the daemon")
	fl.BoolVar(&f.updateOnGitHub, updateOnGitHubFlag, false, "the watches on the repository ask GitHub to update a branch that fell behind its base before the agent does it")
	fl.BoolVar(&f.resetOverrides, "reset-overrides", false, "give every override back to the settings of the daemon")
	fl.StringVar(&f.scope, "dependabot-scope", "", "the highest Dependabot update that merges on its own: patch, minor or major")
	fl.StringVar(&f.approval, "dependabot-approval", "", "never, ask or green")
	fl.IntVar(&f.limit, "dependabot-limit", 1, "how many Dependabot watches run at the same time, 1 or more")
	return cmd
}

func (f repoConfigFlags) change(cmd *cobra.Command, current store.WatchOverrides) (autostart.Change, error) {
	c := autostart.Change{
		IncludeDrafts:       typed(cmd, "include-drafts", &f.drafts),
		AutoStartMine:       typed(cmd, "auto-start-mine", &f.mine),
		AutoWatchDependabot: typed(cmd, "auto-watch-dependabot", &f.dependabot),
		DependabotLimit:     typed(cmd, "dependabot-limit", &f.limit),
	}
	if dir := typed(cmd, "checkout", &f.checkout); dir != nil {
		abs, err := absoluteOrEmpty(*dir)
		if err != nil {
			return autostart.Change{}, err
		}
		c.CheckoutDir = &abs
	}
	if scope := typed(cmd, "dependabot-scope", &f.scope); scope != nil {
		level := dependabot.Level(*scope)
		c.DependabotScope = &level
	}
	if approval := typed(cmd, "dependabot-approval", &f.approval); approval != nil {
		a := store.DependabotApproval(*approval)
		c.DependabotApproval = &a
	}
	overrides, changed, err := f.overrides(cmd, current)
	if err != nil {
		return autostart.Change{}, err
	}
	if changed {
		c.Overrides = &overrides
	}
	return c, nil
}

func absoluteOrEmpty(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	return filepath.Abs(dir)
}

func (f repoConfigFlags) overrides(cmd *cobra.Command, o store.WatchOverrides) (store.WatchOverrides, bool, error) {
	flags := cmd.Flags()
	if f.resetOverrides {
		o = store.WatchOverrides{}
	}
	if flags.Changed("provider") {
		o.Provider, o.Model, o.Effort = f.provider, "", ""
	}
	if flags.Changed("model") {
		o.Model, o.Effort = f.model, ""
	}
	if flags.Changed("effort") {
		o.Effort = f.effort
	}
	if flags.Changed("approval-mode") {
		o.ApprovalMode = store.ApprovalMode(f.approvalMode)
	}
	if flags.Changed("merge-method") {
		o.MergeMethod = f.mergeMethod
	}
	if flags.Changed("include-existing") {
		o.IncludeExisting = &f.includeExisting
	}
	if flags.Changed("auto-approve-rebase") {
		o.AutoApproveRebase = &f.autoRebase
	}
	if flags.Changed("include-own") {
		o.IncludeOwn = &f.includeOwn
	}
	if flags.Changed("keep-worktree") {
		o.KeepWorktree = &f.keepWorktree
	}
	if flags.Changed(branchUpdateFlag) {
		o.BranchUpdate = store.BranchUpdate(f.branchUpdate)
	}
	if flags.Changed(updateOnGitHubFlag) {
		o.UpdateOnGitHub = &f.updateOnGitHub
	}
	if flags.Changed("approvals") {
		var err error
		if o.ApprovalsSet, o.Approvals, err = overrideApprovals(f.approvals); err != nil {
			return store.WatchOverrides{}, false, err
		}
	}
	changed := f.resetOverrides || anyChanged(cmd, overrideFlags...)
	return o, changed, nil
}

var overrideFlags = []string{
	"provider", "model", "effort", "approval-mode", "merge-method", "include-existing", "approvals",
	"auto-approve-rebase", "include-own", "keep-worktree", branchUpdateFlag, updateOnGitHubFlag,
}

func overrideApprovals(value string) (bool, *int, error) {
	if strings.EqualFold(strings.TrimSpace(value), approvalsDefault) {
		return false, nil, nil
	}
	n, err := parseApprovals(value)
	return err == nil, n, err
}

func anyChanged(cmd *cobra.Command, names ...string) bool {
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

type queueItemOutput struct {
	Position   int       `json:"position"`
	Number     int       `json:"number"`
	UpdateType string    `json:"update_type"`
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"created_at"`
}

type queueOutput []queueItemOutput

func (q queueOutput) writeText(w io.Writer) error {
	if len(q) == 0 {
		_, err := fmt.Fprintln(w, "No Dependabot pull request waits.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tPR\tUPDATE\tOPENED\tTITLE")
	for _, item := range q {
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\n", item.Position, item.Number, item.UpdateType, item.CreatedAt.Local().Format("2006-01-02 15:04"), item.Title)
	}
	return tw.Flush()
}

func newRepoQueueCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "queue <owner/name>",
		Short: "List the Dependabot pull requests that wait for a place, oldest first",
		Long: `Auto watch Dependabot starts at most --dependabot-limit watches at the same
time. The other new pull requests of Dependabot wait here, oldest first.
The first one starts on the next pass after a place is free. A pull
request leaves the queue when it closes, merges, or when you start a
watch on it by hand.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			owner, name, err := store.ParseFullName(args[0])
			if err != nil {
				return err
			}
			st, err := opts.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			repo, err := st.GetRepo(ctx, owner, name)
			if err != nil {
				return err
			}
			prs, err := autostart.Queue(ctx, st, repo)
			if err != nil {
				return err
			}
			out := make(queueOutput, 0, len(prs))
			for i, pr := range prs {
				out = append(out, queueItemOutput{
					Position: i + 1, Number: pr.Number, UpdateType: string(pr.UpdateType), Title: pr.Title, URL: pr.HTMLURL, CreatedAt: pr.CreatedAt,
				})
			}
			return opts.print(cmd.OutOrStdout(), out)
		},
	}
}
