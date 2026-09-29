package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

type proposalListOutput httpd.ProposalList

func (l proposalListOutput) writeText(out io.Writer) error {
	if len(l.Proposals) == 0 {
		_, err := fmt.Fprintln(out, "No proposals yet.")
		return err
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROPOSAL\tSTATUS\tWORK\tREPLIES\tNOTE")
	for _, p := range l.Proposals {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", p.Number, p.Status, orDash(textx.ShortSHA(p.WorkSHA)), repliesCount(p.Replies), proposalNote(p))
	}
	return tw.Flush()
}

func repliesCount(replies []httpd.ProposalReply) string {
	dropped := 0
	for _, r := range replies {
		if r.Dropped || r.DroppedAt != nil {
			dropped++
		}
	}
	out := textx.Count(len(replies)-dropped, "reply", "replies")
	if dropped > 0 {
		out += fmt.Sprintf(", %d dropped", dropped)
	}
	return out
}

func proposalNote(p httpd.Proposal) string {
	switch {
	case p.Error != "":
		return p.Error
	case p.Reason != "":
		return "rejected: " + p.Reason
	case p.PushRejected:
		return "push rejected"
	case p.MovedBy == store.BranchMerge:
		return fmt.Sprintf("merged %s into %s", textx.ShortSHA(p.HeadSHA), textx.ShortSHA(p.RebasedFrom))
	case p.RebasedFrom != "":
		return "rebased from " + textx.ShortSHA(p.RebasedFrom)
	}
	return ""
}

type proposalOutput struct {
	watch int64
	httpd.ProposalDetail
	diff bool
	file string
}

func (p proposalOutput) writeText(out io.Writer) error {
	fmt.Fprintf(out, "Proposal:  %d of watch %d\n", p.Number, p.watch)
	fmt.Fprintf(out, "Status:    %s\n", p.Status)
	fmt.Fprintf(out, "Head:      %s\n", orDash(textx.ShortSHA(p.HeadSHA)))
	fmt.Fprintf(out, "Work:      %s\n", orDash(textx.ShortSHA(p.WorkSHA)))
	if note := proposalNote(p.Proposal); note != "" {
		fmt.Fprintf(out, "Note:      %s\n", note)
	}
	if p.CodeError != "" {
		fmt.Fprintf(out, "Code:      not read from the worktree: %s\n", p.CodeError)
	}
	for i, c := range p.Commits {
		fmt.Fprintf(out, "%s%s %s%s\n", label(i, "Commits:   "), textx.ShortSHA(c.SHA), c.Subject, heldBackWord(c.HeldBack))
	}
	if p.Commit != "" {
		fmt.Fprintf(out, "Commit:    %s only, for the files and the diff\n", textx.ShortSHA(p.Commit))
	}
	for i, f := range p.Files {
		fmt.Fprintf(out, "%s%s %s %s\n", label(i, "Files:     "), f.Status, f.Path, lineCounts(f))
	}
	for _, r := range p.Replies {
		fmt.Fprintf(out, "\n%s:\n%s\n", replyHeading(r), indent(replyText(r)))
	}
	if p.Status == store.ProposalPending {
		fmt.Fprintf(out, "\nDecide with `babysitter watch approve %d %d` or `babysitter watch reject %d %d`.\n", p.watch, p.Number, p.watch, p.Number)
	}
	if !p.diff {
		if p.Diff != "" {
			fmt.Fprintln(out, "Pass --diff to read the diff.")
		}
		return nil
	}
	if _, err := fmt.Fprintf(out, "\n%s", p.Diff); err != nil {
		return err
	}
	if p.Truncated {
		fmt.Fprintln(out, "\n"+p.cutNote())
	}
	return nil
}

func (p proposalOutput) cutNote() string {
	if p.file != "" {
		return "The diff of " + p.file + " is longer than one megabyte, so it is not shown."
	}
	return "The diff stops at the last whole file under one megabyte. Pass --file <path> to read a file after it."
}

func lineCounts(f httpd.ProposalFile) string {
	if f.Binary {
		return "binary"
	}
	return fmt.Sprintf("+%d -%d", f.Added, f.Deleted)
}

func heldBackWord(heldBack bool) string {
	if heldBack {
		return " (kept off by an earlier decision)"
	}
	return ""
}

func label(i int, heading string) string {
	if i == 0 {
		return heading
	}
	return strings.Repeat(" ", len(heading))
}

func replyHeading(r httpd.ProposalReply) string {
	var where string
	switch {
	case r.InReplyTo == 0:
		where = "on the pull request"
	case r.Answers != nil:
		where = fmt.Sprintf("to comment %d of %s", r.InReplyTo, r.Answers.Actor)
	default:
		where = fmt.Sprintf("to comment %d", r.InReplyTo)
	}
	state := ""
	switch {
	case r.Dropped:
		state = ", dropped"
	case r.DroppedAt != nil:
		state = ", dropped: " + r.Error
	case r.Edited != "":
		state = ", edited"
	case r.PostedURL != "":
		state = ", posted " + r.PostedURL
	}
	return fmt.Sprintf("reply %d %s%s", r.ID, where, state)
}

func replyText(r httpd.ProposalReply) string {
	if r.Edited != "" {
		return r.Edited
	}
	return r.Body
}

func indent(text string) string {
	return "  " + strings.ReplaceAll(strings.TrimRight(text, "\n"), "\n", "\n  ")
}

type releasedOutput struct {
	watch httpd.Watch
	httpd.Proposal
}

func (p releasedOutput) writeText(out io.Writer) error {
	var what string
	switch {
	case p.PushRejected:
		what = "its replies are posted; the commits stay on the work branch"
	case !p.HasPush:
		what = "its replies are posted"
	default:
		what = fmt.Sprintf("%s is on %s", textx.ShortSHA(p.WorkSHA), p.watch.HeadRef)
	}
	_, err := fmt.Fprintf(out, "Proposal %d of watch %d went out: %s.\n", p.Number, p.watch.ID, what)
	return err
}

func released(opts *options, cmd *cobra.Command, w httpd.Watch, p httpd.Proposal) error {
	if p.Status == store.ProposalFailed {
		return fmt.Errorf("proposal %d of watch %d did not go out: %s", p.Number, w.ID, p.Error)
	}
	return opts.print(cmd.OutOrStdout(), releasedOutput{watch: w, Proposal: p})
}

func proposalNumber(ctx context.Context, c *daemonClient, w httpd.Watch, args []string, what string, fits func(httpd.Proposal) bool) (int, error) {
	if len(args) > 0 {
		return parseProposalNumber(args[0])
	}
	var l httpd.ProposalList
	if err := c.get(ctx, fmt.Sprintf("/watches/%d/proposals", w.ID), &l); err != nil {
		return 0, err
	}
	for _, p := range l.Proposals {
		if fits(p) {
			return p.Number, nil
		}
	}
	return 0, fmt.Errorf("watch %d has no proposal that %s", w.ID, what)
}

func parseProposalNumber(arg string) (int, error) {
	n, err := strconv.Atoi(arg)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("the proposal is a number, got %q", arg)
	}
	return n, nil
}

func isPending(p httpd.Proposal) bool { return p.Status == store.ProposalPending }

func isFailed(p httpd.Proposal) bool { return p.Status == store.ProposalFailed }

func isRejectable(p httpd.Proposal) bool { return p.Status.Rejectable() }

func proposalPathOf(watch int64, number int, commit, file string) string {
	path := fmt.Sprintf("/watches/%d/proposals/%d", watch, number)
	query := url.Values{}
	if commit != "" {
		query.Set("commit", commit)
	}
	if file != "" {
		query.Set("path", file)
	}
	if len(query) == 0 {
		return path
	}
	return path + "?" + query.Encode()
}

func newWatchProposalsCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		diff   bool
		commit string
		file   string
	)
	cmd := &cobra.Command{
		Use:   "proposals <watch> [proposal]",
		Short: "List the proposals of a watch, or read one",
		Long: `The work of each turn of the agent is one proposal: the commits of its
work branch and the replies it recorded. In manual mode a proposal waits
for you. Without a number, the command lists the proposals, newest
first. With one, it prints the commits, the files, and each reply with
the comment it answers, which is what you read before you approve.
--diff adds the plain unified diff. --commit shows the files and the
diff of one commit of the proposal only. --file shows one file only,
which reads a file that a diff longer than one megabyte leaves out.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, args []string) error {
			if len(args) == 0 {
				var l httpd.ProposalList
				if err := c.get(cmd.Context(), fmt.Sprintf("/watches/%d/proposals", w.ID), &l); err != nil {
					return err
				}
				return opts.print(cmd.OutOrStdout(), proposalListOutput(l))
			}
			n, err := parseProposalNumber(args[0])
			if err != nil {
				return err
			}
			var d httpd.ProposalDetail
			if err := c.get(cmd.Context(), proposalPathOf(w.ID, n, commit, file), &d); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), proposalOutput{watch: w.ID, ProposalDetail: d, diff: diff, file: file})
		}),
	}
	cmd.Flags().BoolVar(&diff, "diff", false, "also print the plain unified diff of the work")
	cmd.Flags().StringVar(&commit, "commit", "", "only the files and the diff of this commit, as a SHA of 7 characters or more")
	cmd.Flags().StringVar(&file, "file", "", "only the diff of this file, as the files list names it")
	return cmd
}

func newWatchApproveCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		edits      []string
		drops      []int64
		rejectPush bool
		stopAsking bool
	)
	cmd := &cobra.Command{
		Use:   "approve <watch> [proposal]",
		Short: "Push and post a proposal that waits for you",
		Long: `The daemon pushes the commits of the proposal and posts its replies,
under your account. Without a number, the proposal that waits is taken.
--edit rewrites a reply before it goes out, --drop takes one out, and
--reject-push posts the replies without the commits. The agent hears
what you changed. A dropped reply to a review comment brings that
comment back to the agent. --stop-asking releases the proposal and runs
the watch in auto from then on.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, args []string) error {
			req := httpd.ApproveRequest{Drop: drops, RejectPush: rejectPush, StopAsking: stopAsking}
			for _, e := range edits {
				edit, err := parseEdit(e)
				if err != nil {
					return err
				}
				req.Edits = append(req.Edits, edit)
			}
			n, err := proposalNumber(cmd.Context(), c, w, args, "waits for you", isPending)
			if err != nil {
				return err
			}
			var p httpd.Proposal
			if err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/proposals/%d/approve", w.ID, n), req, &p); err != nil {
				return err
			}
			return released(opts, cmd, w, p)
		}),
	}
	cmd.Flags().StringArrayVar(&edits, "edit", nil, "rewrite a reply before it goes out, as <reply id>=<text>; repeat for more")
	cmd.Flags().Int64SliceVar(&drops, "drop", nil, "take a reply out by its id; repeat for more")
	cmd.Flags().BoolVar(&rejectPush, "reject-push", false, "post the replies without pushing the commits")
	cmd.Flags().BoolVar(&stopAsking, "stop-asking", false, "release the proposal and run the watch in auto from now on")
	return cmd
}

func parseEdit(s string) (httpd.ReplyEdit, error) {
	id, text, ok := strings.Cut(s, "=")
	n, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64)
	if !ok || err != nil || n <= 0 {
		return httpd.ReplyEdit{}, fmt.Errorf("an edit is <reply id>=<text>, got %q", s)
	}
	return httpd.ReplyEdit{ReplyID: n, Body: text}, nil
}

type rejectedOutput struct {
	watch int64
	httpd.Proposal
}

func (p rejectedOutput) writeText(out io.Writer) error {
	next := "The agent heard that nothing went out."
	if p.Reason != "" {
		next = "The agent works on your reason."
	}
	_, err := fmt.Fprintf(out, "Rejected proposal %d of watch %d. Nothing was pushed or posted. %s\n", p.Number, p.watch, next)
	return err
}

func newWatchRejectCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		reason  string
		discard bool
	)
	cmd := &cobra.Command{
		Use:   "reject <watch> [proposal]",
		Short: "Reject a proposal: nothing of it goes out",
		Long: `Nothing of the proposal is pushed or posted, and the comments its
replies answered come back to the agent. --reason goes to the agent as a
message from you, and it works on it. --discard resets the work branch
to the head the turn started on, whatever the setting of the worktree
says; without it, the commits stay on the work branch and the agent can
build on them. Without a number, the proposal that waits, or else the
one whose release failed, is taken.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, args []string) error {
			n, err := proposalNumber(cmd.Context(), c, w, args, "waits for you or failed", isRejectable)
			if err != nil {
				return err
			}
			var p httpd.Proposal
			body := httpd.RejectRequest{Reason: reason, Discard: discard}
			if err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/proposals/%d/reject", w.ID, n), body, &p); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), rejectedOutput{watch: w.ID, Proposal: p})
		}),
	}
	cmd.Flags().StringVar(&reason, "reason", "", "what the agent should do instead")
	cmd.Flags().BoolVar(&discard, "discard", false, "reset the work branch and drop the commits of the proposal")
	return cmd
}

type modeOutput httpd.Watch

func (w modeOutput) writeText(out io.Writer) error {
	if w.ApprovalMode == store.ApprovalAuto {
		_, err := fmt.Fprintf(out, "Watch %d runs in auto: the daemon pushes and posts when each turn ends.\n", w.ID)
		return err
	}
	_, err := fmt.Fprintf(out, "Watch %d runs in manual: each turn waits for you%s.\n", w.ID, rebaseWord(w.AutoApproveRebase))
	return err
}

func rebaseWord(auto bool) string {
	if auto {
		return ", and approved work goes out after a clean rebase or merge"
	}
	return ""
}

func newWatchModeCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		autoRebase bool
		release    bool
	)
	cmd := &cobra.Command{
		Use:   "mode <watch> auto|manual",
		Short: "Change who releases the work of the agent of a watch",
		Long: `In manual, the daemon holds the work of each turn of the agent until
you approve it. In auto, it pushes and posts as soon as the turn ends.
A switch to auto releases the proposal that waits for you, so it needs
--release: read the proposal first. --auto-approve-rebase lets work you
approved go out after a clean rebase or merge without asking again. A watch whose
agent is your own session has no gate.`,
		Args: cobra.ExactArgs(2),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, args []string) error {
			mode := store.ApprovalMode(args[0])
			if !mode.Valid() {
				return fmt.Errorf("%w: %q", prwatch.ErrBadApprovalMode, args[0])
			}
			req := httpd.ApprovalRequest{Mode: new(string(mode)), AutoApproveRebase: typed(cmd, "auto-approve-rebase", &autoRebase), Release: release}
			var out httpd.Watch
			err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/approval", w.ID), req, &out)
			releaseNeeded := err != nil && mode == store.ApprovalAuto && !release
			if releaseNeeded {
				return fmt.Errorf("%w; pass --release to switch and release it", err)
			}
			if err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), modeOutput(out))
		}),
	}
	cmd.Flags().BoolVar(&autoRebase, "auto-approve-rebase", false, "let approved work go out after a clean rebase or merge without asking again")
	cmd.Flags().BoolVar(&release, "release", false, "confirm that a switch to auto releases the proposal that waits for you")
	return cmd
}

func approvalLine(w httpd.Watch) string {
	if w.Provider == prwatch.ProviderSelf {
		return "none: your own session pushes and posts"
	}
	mode := string(w.ApprovalMode) + rebaseWord(w.AutoApproveRebase && w.ApprovalMode == store.ApprovalManual)
	if w.PendingProposal > 0 {
		return fmt.Sprintf("%s; proposal %d waits on you: babysitter watch proposals %d %d", mode, w.PendingProposal, w.ID, w.PendingProposal)
	}
	return mode
}
