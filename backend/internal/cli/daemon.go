package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/daemon"
	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/logbook"
	"github.com/deividfortuna/babysitter/internal/processalive"
	"github.com/deividfortuna/babysitter/internal/runfile"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (o *options) dataDir(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if p := os.Getenv("BABYSITTER_DATA_DIR"); p != "" {
		return p, nil
	}
	p, err := store.DefaultPath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(p), nil
}

type daemonStatusOutput struct {
	Running   bool   `json:"running"`
	Healthy   bool   `json:"healthy"`
	PID       int    `json:"pid,omitempty"`
	Port      int    `json:"port,omitempty"`
	URL       string `json:"url,omitempty"`
	Owner     string `json:"owner,omitempty"`
	Version   string `json:"version,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	RunFile   string `json:"runFile"`
}

func (s daemonStatusOutput) writeText(w io.Writer) error {
	if !s.Running {
		fmt.Fprintln(w, "Running:  no")
		fmt.Fprintf(w, "Run file: %s\n", s.RunFile)
		return nil
	}
	fmt.Fprintln(w, "Running:  yes")
	fmt.Fprintf(w, "Healthy:  %s\n", yesNo(s.Healthy))
	fmt.Fprintf(w, "PID:      %d\n", s.PID)
	fmt.Fprintf(w, "URL:      %s\n", s.URL)
	fmt.Fprintf(w, "Owner:    %s\n", s.Owner)
	fmt.Fprintf(w, "Version:  %s\n", s.Version)
	fmt.Fprintf(w, "Started:  %s\n", s.StartedAt)
	fmt.Fprintf(w, "Run file: %s\n", s.RunFile)
	return nil
}

func newDaemonCmd(opts *options) *cobra.Command {
	var dataDirFlag string
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the daemon behind the desktop app",
		Long: `The daemon polls the watched repositories like 'serve' and also serves
the API the desktop app uses on the loopback interface. It writes
running.json in the data directory so the app and 'daemon stop' can find
it.

The data directory comes from, in this order:
  1. the --data-dir flag
  2. the BABYSITTER_DATA_DIR environment variable
  3. the directory of the default database path, which --db and
     BABYSITTER_DB do not move`,
	}
	cmd.PersistentFlags().StringVar(&dataDirFlag, "data-dir", "", "directory of running.json and the supervisor socket")
	cmd.AddCommand(
		newDaemonStartCmd(opts, &dataDirFlag),
		newDaemonStopCmd(opts, &dataDirFlag),
		newDaemonStatusCmd(opts, &dataDirFlag),
		newDaemonLogsCmd(opts, &dataDirFlag),
		newDaemonLogLevelCmd(opts, &dataDirFlag),
		newDaemonPairCmd(opts, &dataDirFlag),
	)
	return cmd
}

func newDaemonStartCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var (
		port          int
		interval      time.Duration
		watchInterval time.Duration
		watchMax      time.Duration
		checkMax      time.Duration
		agentBin      string
		agentModel    string
		copilotBin    string
		copilotModel  string
		owner         string
		logLevel      string
		remoteAddr    string
		remoteName    string
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Run the daemon in the foreground until interrupted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if owner != runfile.OwnerApp && owner != runfile.OwnerCLI {
				return fmt.Errorf("owner must be %q or %q, got %q", runfile.OwnerApp, runfile.OwnerCLI, owner)
			}
			dataDir, err := opts.dataDir(*dataDirFlag)
			if err != nil {
				return err
			}
			dbPath, err := opts.dbPath()
			if err != nil {
				return err
			}
			auth := opts.authIn(dataDir)
			level, err := logbook.ParseLevel(logLevel)
			if err != nil {
				return err
			}
			book, err := logbook.Open(logbook.Options{Path: daemonLogPath(dataDir), Level: level})
			if err != nil {
				return err
			}
			defer book.Close()
			logger := slog.New(slog.NewMultiHandler(
				slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: book.Leveler()}),
				book.Handler(),
			))
			err = daemon.Run(cmd.Context(), daemon.Config{
				DataDir:          dataDir,
				DBPath:           dbPath,
				Port:             port,
				Interval:         interval,
				WatchInterval:    watchInterval,
				WatchMaxInterval: watchMax,
				CheckMaxInterval: checkMax,
				AgentBin:         agentBin,
				AgentModel:       agentModel,
				CopilotBin:       copilotBin,
				CopilotModel:     copilotModel,
				Owner:            owner,
				Version:          opts.version,
				RemoteAddr:       remoteAddr,
				RemoteName:       remoteName,
				NewClient:        opts.clientWith(auth),
				Auth:             auth,
				Whoami:           opts.whoami,
				Notifier:         opts.newNotifier(),
				Log:              logger,
				Logs:             book,
			})
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "loopback port to bind, 0 picks a free one")
	cmd.Flags().DurationVar(&interval, "interval", 0, "time between polls of the watched repositories, for this run only; unset takes the setting of the daemon")
	cmd.Flags().DurationVar(&watchInterval, "watch-interval", 0, "time between polls of a watched pull request, for this run only; unset takes the setting of the daemon")
	cmd.Flags().DurationVar(&watchMax, "watch-max-interval", 0, "longest time between polls of a watched pull request where nothing happens, for this run only; unset takes the setting of the daemon")
	cmd.Flags().DurationVar(&checkMax, "check-max-interval", 0, "longest time between reads of the pending checks of an open pull request, for this run only; unset takes the setting of the daemon")
	cmd.Flags().StringVar(&agentBin, "agent-bin", "claude", "Claude Code command that babysits watched pull requests, or none")
	cmd.Flags().StringVar(&agentModel, "agent-model", "", "model of the agent, empty for its default")
	cmd.Flags().StringVar(&copilotBin, "copilot-bin", "copilot", "Copilot CLI command that babysits watched pull requests")
	cmd.Flags().StringVar(&copilotModel, "copilot-model", "", "model of Copilot CLI, empty for its default")
	cmd.Flags().StringVar(&owner, "owner", runfile.OwnerCLI, "who started the daemon: cli or app")
	cmd.Flags().StringVar(&remoteAddr, "remote", "", "also serve the API to other machines on this address, for example :7420; a client needs the token that 'daemon pair' prints")
	cmd.Flags().StringVar(&remoteName, "remote-name", "", "name the daemon shows to other machines, the host name if empty")
	cmd.Flags().StringVar(&logLevel, "log-level", "info", "lowest level the daemon logs: "+strings.Join(logbook.LevelNames, ", ")+"; 'daemon log-level' changes it while the daemon runs")
	return cmd
}

func newDaemonStopCmd(opts *options, dataDirFlag *string) *cobra.Command {
	var wait time.Duration
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running daemon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir, err := opts.dataDir(*dataDirFlag)
			if err != nil {
				return err
			}
			info, err := runfile.Live(runfile.Path(dataDir))
			if err != nil {
				return err
			}
			if info == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "No daemon is running")
				return nil
			}
			url := fmt.Sprintf("http://127.0.0.1:%d%s/control/shutdown", info.Port, httpd.Prefix)
			req, err := http.NewRequestWithContext(cmd.Context(), http.MethodPost, url, strings.NewReader(""))
			if err != nil {
				return err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("ask the daemon to stop: %w", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusAccepted {
				return fmt.Errorf("ask the daemon to stop: unexpected status %s", resp.Status)
			}
			deadline := time.Now().Add(wait)
			for processalive.Alive(info.PID) {
				if time.Now().After(deadline) {
					return fmt.Errorf("the daemon (pid %d) did not stop within %s", info.PID, wait)
				}
				time.Sleep(100 * time.Millisecond)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Stopped the daemon (pid %d)\n", info.PID)
			return nil
		},
	}
	cmd.Flags().DurationVar(&wait, "wait", 10*time.Second, "how long to wait for the process to exit")
	return cmd
}

func newDaemonStatusCmd(opts *options, dataDirFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the daemon runs and answers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataDir, err := opts.dataDir(*dataDirFlag)
			if err != nil {
				return err
			}
			runPath := runfile.Path(dataDir)
			out := daemonStatusOutput{RunFile: runPath}
			info, err := runfile.Live(runPath)
			if err != nil {
				return err
			}
			if info != nil {
				out.Running = true
				out.PID = info.PID
				out.Port = info.Port
				out.URL = fmt.Sprintf("http://127.0.0.1:%d%s", info.Port, httpd.Prefix)
				out.Owner = info.Owner
				out.Version = info.Version
				out.StartedAt = info.StartedAt.Format(time.RFC3339)
				out.Healthy = healthy(cmd.Context(), out.URL)
			}
			return opts.print(cmd.OutOrStdout(), out)
		},
	}
}

func healthy(ctx context.Context, baseURL string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
