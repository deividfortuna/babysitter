package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/logbook"
)

const (
	logsDir        = "logs"
	daemonLogFile  = "daemon.log"
	appLogFile     = "app.log"
	logTimeLayout  = "2006-01-02 15:04:05.000"
	logLevelColumn = 5
)

func daemonLogPath(dataDir string) string {
	return filepath.Join(dataDir, logsDir, daemonLogFile)
}

func appLogPath(dataDir string) string {
	return filepath.Join(dataDir, logsDir, appLogFile)
}

type logList struct {
	Records []logbook.Record `json:"records"`
}

func (l logList) writeText(w io.Writer) error {
	for _, r := range l.Records {
		if err := writeLogLine(w, r); err != nil {
			return err
		}
	}
	return nil
}

func writeLogLine(w io.Writer, r logbook.Record) error {
	var b strings.Builder
	b.WriteString(r.Time.Local().Format(logTimeLayout))
	b.WriteByte(' ')
	fmt.Fprintf(&b, "%-*s ", logLevelColumn, strings.ToUpper(r.Level))
	b.WriteString(r.Msg)
	for _, a := range r.Attrs {
		fmt.Fprintf(&b, " %s=%s", a.Key, quoteIfNeeded(a.Value))
	}
	b.WriteByte('\n')
	_, err := io.WriteString(w, b.String())
	return err
}

func quoteIfNeeded(v string) string {
	if v == "" || strings.ContainsAny(v, " \t\n\"=") {
		return strconv.Quote(v)
	}
	return v
}

type logFilter struct {
	min   slog.Level
	lines int
}

func (f logFilter) keeps(r logbook.Record) bool {
	l, err := logbook.ParseLevel(r.Level)
	return err != nil || l >= f.min
}

func (f logFilter) apply(records []logbook.Record) []logbook.Record {
	kept := make([]logbook.Record, 0, len(records))
	for _, r := range records {
		if f.keeps(r) {
			kept = append(kept, r)
		}
	}
	if f.lines > 0 && len(kept) > f.lines {
		kept = kept[len(kept)-f.lines:]
	}
	return kept
}

func newDaemonLogsCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		lines  int
		follow bool
		level  string
		app    bool
	)
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Print the log of the daemon, or of the desktop app",
		Long: `Print the last records the daemon logged. While the daemon runs, they
come from the daemon itself; when no daemon runs, they come from the log
file it left in the data directory, logs/daemon.log. That file holds up
to 5 MiB and the one before it, daemon.log.1.

--follow keeps printing each new record until you interrupt it, and needs
a running daemon. --app prints the log of the desktop app,
logs/app.log, which holds what the app did to start, attach to and stop
the daemon, and the updates it checked.

The daemon logs at info level. 'babysitter daemon log-level debug' adds
the debug records, such as each HTTP request, until the daemon stops.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := logFilter{min: slog.LevelDebug, lines: lines}
			if level != "" {
				l, err := logbook.ParseLevel(level)
				if err != nil {
					return err
				}
				filter.min = l
			}
			dataDir, err := opts.dataDir(*dataDirFlag)
			if err != nil {
				return err
			}
			if app {
				if follow {
					return errors.New("--follow reads the daemon only; leave out --app")
				}
				return printLogFile(opts, cmd.OutOrStdout(), appLogPath(dataDir), filter)
			}
			c, err := opts.daemonClient(*dataDirFlag)
			if errors.Is(err, errNoDaemon) && !follow {
				return printLogFile(opts, cmd.OutOrStdout(), daemonLogPath(dataDir), filter)
			}
			if err != nil {
				return err
			}
			var got logList
			if err := c.get(cmd.Context(), "/logs?limit=0", &got); err != nil {
				return err
			}
			if err := opts.print(cmd.OutOrStdout(), logList{Records: filter.apply(got.Records)}); err != nil {
				return err
			}
			if !follow {
				return nil
			}
			var after int64
			if n := len(got.Records); n > 0 {
				after = got.Records[n-1].Seq
			}
			return followLogs(cmd.Context(), c, after, filter, logPrinter(opts, cmd.OutOrStdout()))
		},
	}
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "print at most this many records from the end, 0 for all")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing new records until interrupted")
	cmd.Flags().StringVar(&level, "level", "", "print only records at this level or above: "+strings.Join(logbook.LevelNames, ", "))
	cmd.Flags().BoolVar(&app, "app", false, "print the log of the desktop app")
	return cmd
}

func printLogFile(opts *options, w io.Writer, path string, filter logFilter) error {
	records, err := logbook.ReadTail(path, 0)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return opts.print(w, logList{Records: filter.apply(records)})
}

func logPrinter(opts *options, w io.Writer) func(logbook.Record) error {
	if opts.output == outputJSON {
		enc := json.NewEncoder(w)
		return func(r logbook.Record) error { return enc.Encode(r) }
	}
	return func(r logbook.Record) error { return writeLogLine(w, r) }
}

func followLogs(ctx context.Context, c *daemonClient, after int64, filter logFilter, emit func(logbook.Record) error) error {
	url := fmt.Sprintf("%s/logs/stream?after=%d", c.base, after)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return streamEnd(ctx, fmt.Errorf("follow the daemon log: %w", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("follow the daemon log: the daemon answered %s", resp.Status)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	event := ""
	for scanner.Scan() {
		line := scanner.Text()
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			event = name
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || event != "log" {
			continue
		}
		var r logbook.Record
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			return fmt.Errorf("decode a log record: %w", err)
		}
		if !filter.keeps(r) {
			continue
		}
		if err := emit(r); err != nil {
			return err
		}
	}
	return streamEnd(ctx, errors.New("the daemon closed the log stream"))
}

func streamEnd(ctx context.Context, err error) error {
	select {
	case <-ctx.Done():
		return nil
	default:
		return err
	}
}

type logLevelOutput httpd.LogLevel

func (l logLevelOutput) writeText(w io.Writer) error {
	_, err := fmt.Fprintf(w, "Log level: %s\n", l.Level)
	return err
}

func newDaemonLogLevelCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "log-level [" + strings.Join(logbook.LevelNames, "|") + "]",
		Short: "Print or change the lowest level the running daemon logs",
		Long: `With no argument, print the lowest level the running daemon logs. With a
level, the daemon logs from that level on until it stops; the next start
takes --log-level of 'daemon start', info unless you give it. debug adds
the records that only a developer needs, such as each HTTP request.
The desktop app changes the same level with the debug switch in
Settings, Developer.`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: logbook.LevelNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := opts.daemonClient(*dataDirFlag)
			if err != nil {
				return err
			}
			var got httpd.LogLevel
			if len(args) == 0 {
				if err := c.get(cmd.Context(), "/logs/level", &got); err != nil {
					return err
				}
				return opts.print(cmd.OutOrStdout(), logLevelOutput(got))
			}
			if _, err := logbook.ParseLevel(args[0]); err != nil {
				return err
			}
			if err := c.put(cmd.Context(), "/logs/level", httpd.LogLevel{Level: strings.ToLower(args[0])}, &got); err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), logLevelOutput(got))
		},
	}
}
