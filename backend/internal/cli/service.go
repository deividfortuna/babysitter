package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/service"
)

type serviceStatusOutput service.Status

func (s serviceStatusOutput) writeText(w io.Writer) error {
	fmt.Fprintf(w, "Installed: %s\n", yesNo(s.Installed))
	fmt.Fprintf(w, "Loaded:    %s\n", yesNo(s.Loaded))
	fmt.Fprintf(w, "Running:   %s\n", yesNo(s.Running))
	if s.PID != 0 {
		fmt.Fprintf(w, "PID:       %d\n", s.PID)
	}
	fmt.Fprintf(w, "Unit:      %s\n", s.Path)
	if s.LogPath != "" {
		fmt.Fprintf(w, "Log:       %s\n", s.LogPath)
	}
	return nil
}

func newServiceCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Run babysitter as a background service of the operating system",
		Long: `Manage babysitter as a background service.

On macOS the service is a launchd user agent in ~/Library/LaunchAgents.
On Linux it is a systemd user unit in ~/.config/systemd/user.
The service runs 'babysitter serve' with the installed binary, so install
the binary first, for example with 'go install ./cmd/babysitter'.`,
	}
	cmd.AddCommand(
		newServiceInstallCmd(opts),
		newServiceSimpleCmd(opts, "uninstall", "Stop the service and remove it", "Uninstalled the service\n", service.Manager.Uninstall),
		newServiceSimpleCmd(opts, "start", "Start the installed service", "Started the service\n", service.Manager.Start),
		newServiceSimpleCmd(opts, "stop", "Stop the service without removing it", "Stopped the service\n", service.Manager.Stop),
		newServiceStatusCmd(opts),
	)
	return cmd
}

func newServiceInstallCmd(opts *options) *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the service and start it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := intervalWithinBound(interval); err != nil {
				return err
			}
			m, err := opts.newManager()
			if err != nil {
				return err
			}
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("find executable: %w", err)
			}
			cfg := service.Config{
				Executable: exe,
				Args:       []string{"serve"},
				LogDir:     service.DefaultLogDir(),
			}
			if cmd.Flags().Changed("interval") {
				cfg.Args = append(cfg.Args, "--interval", interval.String())
			}
			if opts.db != "" {
				cfg.Args = append(cfg.Args, "--db", opts.db)
			}
			if opts.token != "" {
				cfg.Args = append(cfg.Args, "--token", opts.token)
			}
			if err := m.Install(cmd.Context(), cfg); err != nil {
				return err
			}
			st, err := m.Status(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed the service at %s\n", st.Path)
			if st.LogPath != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Log: %s\n", st.LogPath)
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", defaultInterval, "time between polls; unset lets the service follow the settings of the daemon")
	return cmd
}

func newServiceSimpleCmd(opts *options, use, short, done string, action func(service.Manager, context.Context) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := opts.newManager()
			if err != nil {
				return err
			}
			if err := action(m, cmd.Context()); err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), done)
			return nil
		},
	}
}

func newServiceStatusCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the service is installed and running",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := opts.newManager()
			if err != nil {
				return err
			}
			st, err := m.Status(cmd.Context())
			if err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), serviceStatusOutput(st))
		},
	}
}
