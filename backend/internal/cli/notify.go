package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/notify"
)

type notifyOutput struct {
	Sent     bool `json:"sent"`
	Recorded bool `json:"recorded"`
	notify.Result
}

func (n notifyOutput) writeText(w io.Writer) error {
	if n.Recorded {
		_, err := fmt.Fprintf(w, "Gave the notification to the daemon: %s\n", n.Title)
		return err
	}
	_, err := fmt.Fprintf(w, "Sent the notification with %s\n", n.Backend)
	return err
}

func daemonShare(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout/2)
}

func remaining(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0
	}
	return time.Until(deadline)
}

func answeredLate(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

func newNotifyCmd(opts *options) *cobra.Command {
	var (
		n           notify.Notification
		dataDirFlag string
		local       bool
	)
	cmd := &cobra.Command{
		Use:   "notify [flags] <message>...",
		Short: "Send a notification to the user",
		Long: `Send a notification to the user.

An agent runs this command when it needs the user to do something it
cannot do alone, for example reply to a review comment, approve a
response, or fix credentials. Put the flags first. The words after the
flags form the message, so a word that starts with a dash is part of the
message and not an option.

The notification goes to the daemon when one runs: the daemon keeps it
in the history the desktop app shows, and it reaches the screen from
there. With no daemon, with a daemon that does not take it, or with
--local, the command talks to the operating system itself. On macOS it
goes through terminal-notifier when it is installed, otherwise through
osascript. On Linux it goes through notify-send. A click opens --url only with terminal-notifier. The other
tools show the URL at the end of the message.

The global --timeout flag limits how long the command can take. The
daemon takes half of it, so a daemon that never answers still leaves
time for the operating system to show the notification. A daemon that
answers late is asked whether it recorded the row before the command
shows a second banner. The first osascript notification waits for the
permission prompt of macOS.`,
		Example: `  babysitter notify "PR #42 has a question from alice, reply needed"
  babysitter notify --title "PR #42" --url https://github.com/octo/hello/pull/42 "Reply to the comment of alice"
  babysitter notify --silent -o json "CI on PR #42 failed three times, help needed"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n.Message = strings.Join(args, " ")
			ctx := cmd.Context()
			if opts.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, opts.timeout)
				defer cancel()
			}
			if !local {
				started := time.Now()
				req := httpd.NewNotificationRequest{
					Title:    n.Title,
					Subtitle: n.Subtitle,
					Body:     n.Message,
					URL:      n.URL,
					Silent:   n.Silent,
				}
				post, done := daemonShare(ctx, opts.timeout)
				out, err := opts.postNotification(post, dataDirFlag, req)
				done()
				if answeredLate(err) {
					lookup, stop := daemonShare(ctx, remaining(ctx))
					out, err = opts.recordedNotification(lookup, dataDirFlag, req, started)
					stop()
				}
				if err == nil {
					return opts.print(cmd.OutOrStdout(), notifyOutput{
						Sent: true, Recorded: true,
						Backend: "daemon", Clickable: out.URL != "",
						Title: out.Title, Message: out.Body, URL: out.URL, Silent: n.Silent,
					})
				}
				if !errors.Is(err, errNoDaemon) {
					fmt.Fprintf(cmd.ErrOrStderr(), "the daemon did not record the notification: %v\n", err)
				}
			}
			res, err := opts.newNotifier().Send(ctx, n)
			if err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), notifyOutput{Sent: true, Result: res})
		},
	}
	cmd.Flags().SetInterspersed(false)
	cmd.Flags().StringVar(&n.Title, "title", notify.DefaultTitle, "title of the notification")
	cmd.Flags().StringVar(&n.Subtitle, "subtitle", "", "second line, for example the repository or the pull request")
	cmd.Flags().StringVar(&n.URL, "url", "", "URL to open on click, or to show in the message")
	cmd.Flags().BoolVar(&n.Silent, "silent", false, "do not play a sound")
	cmd.Flags().BoolVar(&local, "local", false, "show the notification here instead of giving it to the daemon")
	cmd.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory of running.json")
	return cmd
}
