package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/textx"
)

func watchName(w httpd.Watch) string {
	return w.Repo + "#" + strconv.Itoa(w.Number)
}

func newWatchTakeoverCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var shell bool
	cmd := &cobra.Command{
		Use:   "takeover <watch>",
		Short: "Continue the agent session of a watch in this terminal",
		Long: `The takeover ends the session that the daemon runs, declines every
proposal that waits, and runs the agent of the watch in its worktree on
the same conversation, in this terminal. The session has none of the
rules of the daemon: it can push and it runs any tool.

While the session is with you, the daemon keeps polling, but it types,
pushes and posts nothing. Give the session back with
'babysitter watch handback'. Use --shell for a shell in the worktree
instead of the agent.`,
		Args: cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			var tk httpd.TakeoverResponse
			if err := c.post(cmd.Context(), fmt.Sprintf("/watches/%d/takeover", w.ID), httpd.TakeoverRequest{PID: os.Getpid(), Shell: shell}, &tk); err != nil {
				return err
			}
			writeTakeover(cmd.ErrOrStderr(), w, tk)
			argv := tk.Argv
			if shell {
				argv = []string{userShell()}
			}
			if err := opts.exec(tk.WorktreeDir, argv); err != nil {
				return fmt.Errorf("run %s in %s: %w; give the session back with 'babysitter watch handback %s'", argv[0], tk.WorktreeDir, err, watchName(w))
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&shell, "shell", false, "run your shell in the worktree instead of the agent")
	return cmd
}

func writeTakeover(out io.Writer, w httpd.Watch, tk httpd.TakeoverResponse) {
	fmt.Fprintf(out, "The session of %s is now yours.\n", watchName(w))
	fmt.Fprintf(out, "  worktree  %s\n", tk.WorktreeDir)
	fmt.Fprintf(out, "  branch    %s\n", tk.WorkBranch)
	fmt.Fprintf(out, "  push      git push origin HEAD:%s\n", tk.HeadRef)
	if len(tk.Declined) > 0 {
		fmt.Fprintf(out, "%s; nothing of it went out.\n", declinedSentence(tk.Declined))
	}
	if tk.NewConversation {
		fmt.Fprintln(out, "The watch had no conversation yet, so the agent starts a new one.")
	}
	if tk.BranchUpdating {
		fmt.Fprintln(out, "GitHub is updating the branch; your worktree is still on the old head, so fetch before you push.")
	}
	fmt.Fprintf(out, "Give it back with: babysitter watch handback %s\n", watchName(w))
}

func declinedSentence(numbers []int) string {
	words := make([]string, 0, len(numbers))
	for _, n := range numbers {
		words = append(words, strconv.Itoa(n))
	}
	if len(words) == 1 {
		return "Proposal " + words[0] + " was declined"
	}
	return "Proposals " + textx.JoinAnd(words) + " were declined"
}

type handbackOutput httpd.Watch

func (w handbackOutput) writeText(out io.Writer) error {
	_, err := fmt.Fprintf(out, "The session of %s is back with the agent; it continues the same conversation.\n", watchName(httpd.Watch(w)))
	return err
}

func newWatchHandbackCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var yes, force bool
	cmd := &cobra.Command{
		Use:   "handback <watch>",
		Short: "Give the agent session of a watch back to the daemon",
		Long: `The hand-back gives the session you took over back to the daemon. The
agent continues the same conversation and gets the messages the daemon
held while the session was with you.

When the work branch has commits that the pull request does not have, or
the worktree has changes that are not committed, the command lists them
and asks first; --yes skips the question. The hand-back is refused while
the agent you started with takeover still runs, because two processes
on one conversation corrupt it; --force skips that check.`,
		Args: cobra.ExactArgs(1),
		RunE: onWatch(opts, dataDirFlag, func(cmd *cobra.Command, c *daemonClient, w httpd.Watch, _ []string) error {
			path := fmt.Sprintf("/watches/%d/handback", w.ID)
			body := httpd.HandbackRequest{Confirm: yes, Force: force}
			var back httpd.Watch
			err := c.post(cmd.Context(), path, body, &back)
			if refusal, ok := workRefusal(err); ok {
				writeWork(cmd.ErrOrStderr(), refusal)
				if !confirmed(cmd.InOrStdin(), cmd.ErrOrStderr(), "Hand back with this work? [y/N] ") {
					return errors.New("the session stays with you; pass --yes to hand back with this work")
				}
				body.Confirm = true
				err = c.post(cmd.Context(), path, body, &back)
			}
			if errorCode(err) == "author_running" {
				return fmt.Errorf("%w; quit it first, or pass --force", err)
			}
			if err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), handbackOutput(back))
		}),
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "hand back with commits or changes that are not on the pull request, without a question")
	cmd.Flags().BoolVar(&force, "force", false, "hand back although the agent you started with takeover still runs")
	return cmd
}

func workRefusal(err error) (httpd.HandbackRefusal, bool) {
	de, ok := errors.AsType[*daemonError](err)
	if !ok || de.Code != "unconfirmed_work" {
		return httpd.HandbackRefusal{}, false
	}
	var refusal httpd.HandbackRefusal
	if json.Unmarshal(de.Body, &refusal) != nil {
		return httpd.HandbackRefusal{}, false
	}
	return refusal, true
}

func writeWork(out io.Writer, r httpd.HandbackRefusal) {
	if len(r.Commits) > 0 {
		fmt.Fprintf(out, "%s the pull request does not have:\n", textx.Plural(len(r.Commits), "commit"))
		for _, c := range r.Commits {
			fmt.Fprintf(out, "  %s %s\n", textx.ShortSHA(c.SHA), c.Subject)
		}
	}
	if len(r.Files) > 0 {
		fmt.Fprintf(out, "%s changed and not committed:\n", textx.Plural(len(r.Files), "file"))
		for _, f := range r.Files {
			fmt.Fprintf(out, "  %s\n", f)
		}
	}
}

func confirmed(in io.Reader, out io.Writer, question string) bool {
	fmt.Fprint(out, question)
	answer, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}
