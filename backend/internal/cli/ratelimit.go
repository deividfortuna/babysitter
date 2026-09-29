package cli

import (
	"fmt"
	"io"
	"math"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
)

var rateStateWords = map[string]string{
	"unknown": "unknown, GitHub has not answered a call yet",
	"ok":      "plenty left",
	"low":     "nearly used, polling pauses before it runs out",
	"paused":  "polling paused until the reset",
	"slowed":  "GitHub asked to slow down, polling waits for the retry",
}

type rateLimitOutput httpd.RateLimit

func (r rateLimitOutput) writeText(out io.Writer) error {
	now := time.Now()
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "State\t%s\n", rateStateWord(r.State))
	if r.State != "unknown" {
		fmt.Fprintf(tw, "Left\t%d of %d\n", r.Remaining, r.Limit)
	}
	if r.ResetAt != nil {
		fmt.Fprintf(tw, "Resets\t%s\n", untilWord(*r.ResetAt, now))
	}
	if r.RetryAt != nil {
		fmt.Fprintf(tw, "Retry\t%s\n", untilWord(*r.RetryAt, now))
	}
	return tw.Flush()
}

func rateStateWord(state string) string {
	if word, ok := rateStateWords[state]; ok {
		return word
	}
	return state
}

func untilWord(at, now time.Time) string {
	minutes := max(1, int(math.Ceil(at.Sub(now).Minutes())))
	return fmt.Sprintf("in %dm, at %s", minutes, at.Local().Format("15:04"))
}

func newRateLimitCmd(opts *options) *cobra.Command {
	var dataDirFlag string
	cmd := &cobra.Command{
		Use:   "ratelimit",
		Short: "Print the GitHub API budget of the running daemon",
		Long: `The requests the token of the daemon may still make before the rate
limit of GitHub resets, as the last answer of GitHub reported them. It is
the same budget the desktop app shows in the sidebar.

When the budget runs low, the daemon pauses its polls until the reset and
then goes on by itself. When GitHub refuses a call for its secondary rate
limit, the polls wait for the retry GitHub asked for. A longer poll
interval spends less: see 'babysitter settings set --watch-interval',
'--watch-max-interval', the longest wait of a watch where nothing happens,
and '--check-max-interval', the longest wait between reads of pending
checks.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := opts.daemonClient(dataDirFlag)
			if err != nil {
				return err
			}
			var got httpd.RateLimit
			if err := c.get(cmd.Context(), "/ratelimit", &got); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), rateLimitOutput(got))
		},
	}
	cmd.Flags().StringVar(&dataDirFlag, "data-dir", "", "directory of running.json")
	return cmd
}
