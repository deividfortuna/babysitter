package httpd

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/deividfortuna/babysitter/internal/autostart"
	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/httpd/apispec"
	"github.com/deividfortuna/babysitter/internal/logbook"
	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/store"
)

const Prefix = "/api/v1"

const timeLayout = time.RFC3339

type Store interface {
	ListRepos(ctx context.Context) ([]store.Repo, error)
	AddRepo(ctx context.Context, owner, name string) (store.Repo, error)
	RemoveRepoByID(ctx context.Context, id int64) error
	ListPRs(ctx context.Context, o store.ListPRsOptions) ([]store.PullRequest, error)
	ListWatches(ctx context.Context, o store.ListWatchesOptions) ([]store.Watch, error)
	GetWatch(ctx context.Context, id int64) (store.Watch, error)
	ListActivity(ctx context.Context, watchID, sinceID int64, limit int) ([]store.Activity, error)
	ListNotifications(ctx context.Context, o store.ListNotificationsOptions) ([]store.Notification, error)
	UnreadNotifications(ctx context.Context) (int, error)
	MarkNotificationsRead(ctx context.Context, ids []int64, now time.Time) error
	Settings(ctx context.Context) (store.Settings, error)
	SaveSettings(ctx context.Context, next store.Settings) (store.Settings, error)
	PendingProposals(ctx context.Context) (map[int64]int, error)
	GetRepoByID(ctx context.Context, id int64) (store.Repo, error)
	SaveRepoConfig(ctx context.Context, c store.RepoConfig) (store.RepoConfig, error)
	autostart.QueueStore
}

type Syncer interface {
	Kick()
	Sync()
}

type Deps struct {
	Context       context.Context
	Store         Store
	Bus           *events.Bus
	Syncer        Syncer
	Watches       WatchController
	Notifications *notify.Center
	ApplySettings func(store.Settings)
	Log           *slog.Logger
	Version       string
	Shutdown      func()
	Viewer        ViewerFunc
	RateLimit     RateLimitFunc
	Logs          *logbook.Book
	Auth          AuthController
	TokenSecret   string
}

type api struct {
	ctx           context.Context
	store         Store
	bus           *events.Bus
	syncer        Syncer
	watches       WatchController
	notifications *notify.Center
	applySettings func(store.Settings)
	log           *slog.Logger
	version       string
	pid           int
	startedAt     time.Time
	polls         *pollFolder
	shutdown      func()
	viewer        *viewerCache
	rateLimit     RateLimitFunc
	logs          *logbook.Book
	auth          AuthController
	tokenSecret   string
}

func NewRouter(d Deps) http.Handler {
	log := d.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	a := &api{
		ctx:           d.Context,
		store:         d.Store,
		bus:           d.Bus,
		syncer:        d.Syncer,
		watches:       d.Watches,
		notifications: d.Notifications,
		applySettings: d.ApplySettings,
		log:           log,
		version:       d.Version,
		pid:           os.Getpid(),
		startedAt:     time.Now().UTC(),
		polls:         newPollFolder(),
		shutdown:      d.Shutdown,
		viewer:        &viewerCache{fn: d.Viewer},
		rateLimit:     d.RateLimit,
		logs:          d.Logs,
		auth:          d.Auth,
		tokenSecret:   d.TokenSecret,
	}
	if a.ctx == nil {
		a.ctx = context.Background()
	}
	if a.syncer == nil {
		a.syncer = noopSyncer{}
	}
	if a.watches == nil {
		a.watches = noopWatches{}
	}
	if a.shutdown == nil {
		a.shutdown = func() {}
	}
	a.forgetViewerOnAuthChange()

	r := chi.NewRouter()
	r.Use(requestLogger(log))
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Route(Prefix, func(r chi.Router) {
		r.Get("/healthz", a.handleHealth)
		r.Get("/readyz", a.handleHealth)
		r.Get("/openapi.yaml", apispec.ServeYAML)
		r.Get("/events", a.handleEvents)
		r.Get("/settings", a.handleGetSettings)
		r.Put("/settings", a.handlePutSettings)
		r.Get("/repos", a.handleListRepos)
		r.Post("/repos", a.handleAddRepo)
		r.Delete("/repos/{id}", a.handleRemoveRepo)
		r.Get("/repos/{id}/config", a.handleGetRepoConfig)
		r.Patch("/repos/{id}/config", a.handleUpdateRepoConfig)
		r.Get("/repos/{id}/queue", a.handleRepoQueue)
		r.Get("/notifications", a.handleListNotifications)
		r.Post("/notifications", a.handleAddNotification)
		r.Post("/notifications/read", a.handleReadNotifications)
		r.Get("/prs", a.handleListPulls)
		r.Get("/providers", a.handleListProviders)
		r.Get("/viewer", a.handleViewer)
		r.Get("/auth", a.handleGetAuth)
		r.Post("/auth/signin", a.handleStartSignIn)
		r.Delete("/auth/signin", a.handleCancelSignIn)
		r.Post("/auth/signout", a.handleSignOut)
		r.Get("/auth/token", a.handleAppToken)
		r.Get("/ratelimit", a.handleRateLimit)
		r.Get("/logs", a.handleListLogs)
		r.Get("/logs/stream", a.handleStreamLogs)
		r.Get("/logs/level", a.handleGetLogLevel)
		r.Put("/logs/level", a.handlePutLogLevel)
		r.Post("/sync", a.handleSync)
		r.Get("/watches", a.handleListWatches)
		r.Post("/watches", a.handleStartWatch)
		r.Get("/watches/{id}", a.handleGetWatch)
		r.Patch("/watches/{id}", a.handleUpdateWatch)
		r.Post("/watches/{id}/stop", a.handleStopWatch)
		r.Post("/watches/{id}/merge", a.handleMergeWatch)
		r.Post("/watches/{id}/poll", a.handlePollWatch)
		r.Get("/watches/{id}/activity", a.handleListActivity)
		r.Post("/watches/{id}/send", a.handleSendWatch)
		r.Post("/watches/{id}/takeover", a.handleTakeoverWatch)
		r.Post("/watches/{id}/handback", a.handleHandbackWatch)
		r.Post("/watches/{id}/next", a.handleNextWatch)
		r.Post("/watches/{id}/reply", a.handleReplyWatch)
		r.Post("/watches/{id}/proposals/{number}/retry", a.handleRetryProposal)
		r.Get("/watches/{id}/proposals", a.handleListProposals)
		r.Get("/watches/{id}/proposals/{number}", a.handleGetProposal)
		r.Post("/watches/{id}/proposals/{number}/approve", a.handleApproveProposal)
		r.Post("/watches/{id}/proposals/{number}/reject", a.handleRejectProposal)
		r.Post("/watches/{id}/approval", a.handleSetApproval)
		r.Get("/watches/{id}/output", a.handleWatchOutput)
		r.Get("/watches/{id}/view", a.handleViewWatch)
		r.Get("/watches/{id}/diff", a.handleDiffWatch)
		r.Post("/watches/{id}/resize", a.handleResizeWatch)
		r.Post("/watches/{id}/hook", a.handleWatchHook)
		r.Post("/control/shutdown", a.handleShutdown)
	})
	return r
}

type noopSyncer struct{}

func (noopSyncer) Kick() {}

func (noopSyncer) Sync() {}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "took", time.Since(start).Round(time.Millisecond))
		})
	}
}
