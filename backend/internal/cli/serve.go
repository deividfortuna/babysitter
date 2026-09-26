package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/watcher"
)

var defaultInterval = store.DefaultSettings().PollInterval

func (o *options) newWatcher(cmd *cobra.Command, extra ...watcher.Option) (*watcher.Watcher, func(), error) {
	st, err := o.openStore()
	if err != nil {
		return nil, nil, err
	}
	return o.watcherOn(cmd, st, extra...), func() { st.Close() }, nil
}

func (o *options) watcherOn(cmd *cobra.Command, st *store.Store, extra ...watcher.Option) *watcher.Watcher {
	logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))
	opts := append([]watcher.Option{watcher.WithLogger(logger)}, extra...)
	return watcher.New(st, o.client, opts...)
}

func newSyncCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Poll every watched repository once and update the store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w, closeStore, err := opts.newWatcher(cmd)
			if err != nil {
				return err
			}
			defer closeStore()
			return w.SyncAll(cmd.Context())
		},
	}
}

func newServeCmd(opts *options) *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Poll the watched repositories in a loop until interrupted",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := opts.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			typed := cmd.Flags().Changed("interval")
			every, err := serveInterval(cmd.Context(), st, typed, interval)
			if err != nil {
				return err
			}
			if typed && every != interval {
				fmt.Fprintf(cmd.ErrOrStderr(), "the interval %s is outside %s..%s; polling every %s\n",
					interval, store.MinInterval, store.MaxInterval, every)
			}
			w := opts.watcherOn(cmd, st, watcher.WithInterval(every))
			ctx, stop := context.WithCancel(cmd.Context())
			defer stop()
			if !typed {
				defer startFollowInterval(ctx, st, w, store.MinInterval, slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil)))()
			}
			err = w.Run(ctx)
			if endedByContext(err) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", defaultInterval, "time between polls; unset takes the setting of the daemon")
	return cmd
}

func endedByContext(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func startFollowInterval(ctx context.Context, st *store.Store, w *watcher.Watcher, every time.Duration, log *slog.Logger) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		followInterval(ctx, st, w, every, log)
	}()
	return func() {
		cancel()
		<-done
	}
}

func followInterval(ctx context.Context, st *store.Store, w *watcher.Watcher, every time.Duration, log *slog.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(min(w.Interval(), every)):
		}
		settings, err := st.Settings(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Warn("read the poll interval of the settings", "err", err)
			continue
		}
		if settings.PollInterval != w.Interval() {
			log.Info("poll interval changed", "was", w.Interval(), "now", settings.PollInterval)
			w.SetInterval(settings.PollInterval)
		}
	}
}

func serveInterval(ctx context.Context, st *store.Store, typed bool, flag time.Duration) (time.Duration, error) {
	settings, err := st.Settings(ctx)
	if err != nil {
		return 0, err
	}
	if !typed {
		return settings.PollInterval, nil
	}
	if flag <= 0 {
		return 0, fmt.Errorf("interval must be greater than zero, got %s", flag)
	}
	return clampInterval(flag), nil
}

func clampInterval(d time.Duration) time.Duration {
	return min(max(d, store.MinInterval), store.MaxInterval)
}

func intervalWithinBound(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("interval must be greater than zero, got %s", d)
	}
	settings := store.DefaultSettings()
	settings.PollInterval = d
	return settings.Validate()
}
