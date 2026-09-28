package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/deividfortuna/babysitter/internal/autostart"
	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/logbook"
	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/runfile"
	"github.com/deividfortuna/babysitter/internal/session"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/supervisor"
	"github.com/deividfortuna/babysitter/internal/watcher"
	"github.com/deividfortuna/babysitter/internal/worktree"
)

const supervisorGrace = 5 * time.Second

var bannerGrace = supervisorGrace

type Config struct {
	DataDir       string
	DBPath        string
	Port          int
	Interval      time.Duration
	WatchInterval time.Duration
	AgentBin      string
	AgentModel    string
	CopilotBin    string
	CopilotModel  string
	Notifier      notify.Notifier
	Owner         string
	Version       string
	NewClient     watcher.ClientFunc
	Log           *slog.Logger
	Logs          *logbook.Book
}

var ErrAlreadyRunning = errors.New("a daemon is already running")

func Run(ctx context.Context, cfg Config) error {
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	runPath := runfile.Path(cfg.DataDir)
	if live, err := runfile.Live(runPath); err != nil {
		return err
	} else if live != nil {
		return fmt.Errorf("%w: pid %d on port %d", ErrAlreadyRunning, live.PID, live.Port)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	st.SetLogger(log.With("component", "store"))
	ghclient.KeepResponses(st)

	bus := events.NewBus()
	st.SetPublisher(bus)

	stored, err := st.Settings(ctx)
	if err != nil {
		return err
	}
	settings, err := runningSettings(stored, cfg)
	if err != nil {
		return err
	}
	intervalsOverridden := settings.PollInterval != stored.PollInterval || settings.WatchInterval != stored.WatchInterval
	if intervalsOverridden {
		log.Info("the command line asked for other intervals than the settings hold; they hold for this run only",
			"interval", settings.PollInterval, "watchInterval", settings.WatchInterval)
	}

	notifications := notify.NewCenter(notify.CenterDeps{
		Store:   st,
		Desktop: cfg.Notifier,
		Log:     log.With("component", "notify"),
	})
	var autoStart func(ctx context.Context)
	w := watcher.New(st, cfg.NewClient, watcher.WithInterval(settings.PollInterval), watcher.WithLogger(log),
		watcher.WithAfterPass(func(ctx context.Context) { autoStart(ctx) }))
	watchOpts := []prwatch.Option{prwatch.WithInterval(settings.WatchInterval)}
	exe, err := os.Executable()
	if err != nil {
		log.Warn("the agent sessions report nothing: the babysitter command is not known", "err", err)
		exe = ""
	}
	watches := prwatch.New(prwatch.Deps{
		Store:         st,
		NewClient:     cfg.NewClient,
		Git:           worktree.New(),
		Release:       gitrelease.New(),
		Agents:        buildAgents(ctx, cfg, log),
		Host:          session.New(),
		Exe:           exe,
		Notifications: notifications,
		Log:           log.With("component", "prwatch"),
		DataDir:       cfg.DataDir,
		Guard:         w.Guard(),
		Bus:           bus,
	}, watchOpts...)
	viewer := func(ctx context.Context) (httpd.Viewer, error) {
		c, err := cfg.NewClient(ctx)
		if err != nil {
			return httpd.Viewer{}, err
		}
		u, err := ghclient.CurrentUser(ctx, c)
		if err != nil {
			return httpd.Viewer{}, err
		}
		return httpd.Viewer{Login: u.GetLogin(), Name: u.GetName(), AvatarURL: u.GetAvatarURL()}, nil
	}
	autoStart = autostart.New(autostart.Deps{
		Store: st,
		Start: watches.Start,
		Login: func(ctx context.Context) (string, error) {
			v, err := viewer(ctx)
			return v.Login, err
		},
		Log: log.With("component", "autostart"),
	}).Run

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var srv *httpd.Server
	handler := httpd.NewRouter(httpd.Deps{
		Context:       ctx,
		Store:         st,
		Bus:           bus,
		Syncer:        w,
		Watches:       watches,
		Notifications: notifications,
		ApplySettings: func(s store.Settings) {
			w.SetInterval(s.PollInterval)
			watches.SetInterval(s.WatchInterval)
		},
		Log:      log,
		Version:  cfg.Version,
		Shutdown: func() { srv.RequestShutdown() },
		Viewer:   viewer,
		RateLimit: func() httpd.RateLimit {
			return rateLimit(ghclient.SharedRates().Status(w.Guard().Floor))
		},
		Logs: cfg.Logs,
	})
	srv, err = httpd.Listen(ctx, cfg.Port, handler)
	if err != nil {
		return err
	}

	info := runfile.Info{
		PID:       os.Getpid(),
		Port:      srv.Port(),
		StartedAt: time.Now().UTC(),
		Owner:     cfg.Owner,
		Version:   cfg.Version,
	}

	g, gctx := errgroup.WithContext(ctx)

	var supLn interface{ Close() error }
	if ln, addr, err := supervisor.Listen(ctx, cfg.DataDir); err != nil {
		log.Warn("supervisor: socket unavailable, the daemon will not stop when the app quits", "err", err)
	} else {
		supLn = ln
		info.Supervisor = addr
		sup := supervisor.New(supervisorGrace, srv.RequestShutdown, log)
		g.Go(func() error { return sup.Serve(gctx, ln) })
	}

	if err := runfile.Write(runPath, info); err != nil {
		if supLn != nil {
			supLn.Close()
		}
		return err
	}
	defer func() {
		if err := runfile.RemoveIfOwned(runPath, info.PID); err != nil {
			log.Warn("remove run file", "err", err)
		}
	}()

	log.Info("daemon listening", "addr", srv.Addr(), "pid", info.PID, "dataDir", cfg.DataDir)

	g.Go(func() error {
		defer cancel()
		return srv.Serve(gctx)
	})
	g.Go(func() error {
		err := w.Run(gctx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	})
	g.Go(func() error {
		err := watches.Run(gctx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	})
	err = g.Wait()
	drain, stop := context.WithTimeout(context.Background(), bannerGrace)
	defer stop()
	if !notifications.WaitUntil(drain) {
		log.Warn("a notification is still with the operating system; leaving it there", "grace", bannerGrace)
	}
	return err
}
