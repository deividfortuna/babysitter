package cli

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/google/go-github/v91/github"
	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/notify"
	"github.com/deividfortuna/babysitter/internal/service"
	"github.com/deividfortuna/babysitter/internal/store"
)

type clientFactory func(token string, timeout time.Duration) (*github.Client, error)

type managerFactory func() (service.Manager, error)

type notifierFactory func() notify.Notifier

type releaseClientFactory func() (*github.Client, error)

type options struct {
	token            string
	authDir          string
	authOpts         []ghauth.Option
	authOnce         sync.Once
	auth             *ghauth.Auth
	authErr          error
	timeout          time.Duration
	output           string
	db               string
	version          string
	newClient        clientFactory
	newManager       managerFactory
	newNotifier      notifierFactory
	newReleaseClient releaseClientFactory
	executable       func() (string, error)
	exec             func(dir string, argv []string) error
}

type Option func(*options)

func WithClientFactory(f clientFactory) Option {
	return func(o *options) { o.newClient = f }
}

func WithServiceManager(m service.Manager) Option {
	return func(o *options) {
		o.newManager = func() (service.Manager, error) { return m, nil }
	}
}

func WithNotifier(n notify.Notifier) Option {
	return func(o *options) {
		o.newNotifier = func() notify.Notifier { return n }
	}
}

func WithVersion(v string) Option {
	return func(o *options) { o.version = v }
}

func WithAuthOptions(opts ...ghauth.Option) Option {
	return func(o *options) { o.authOpts = append(o.authOpts, opts...) }
}

func WithReleaseClientFactory(f releaseClientFactory) Option {
	return func(o *options) { o.newReleaseClient = f }
}

func WithExecutable(f func() (string, error)) Option {
	return func(o *options) { o.executable = f }
}

func WithExec(f func(dir string, argv []string) error) Option {
	return func(o *options) { o.exec = f }
}

func (o *options) authIn(dir string) *ghauth.Auth {
	return ghauth.New(dir, append([]ghauth.Option{ghauth.WithFlag(o.token)}, o.authOpts...)...)
}

func (o *options) githubAuth() (*ghauth.Auth, error) {
	o.authOnce.Do(func() {
		dir, err := o.dataDir(o.authDir)
		if err != nil {
			o.authErr = err
			return
		}
		o.auth = o.authIn(dir)
	})
	return o.auth, o.authErr
}

func (o *options) client(ctx context.Context) (*github.Client, error) {
	auth, err := o.githubAuth()
	if err != nil {
		return nil, err
	}
	return o.clientWith(auth)(ctx)
}

func (o *options) clientWith(auth *ghauth.Auth) func(ctx context.Context) (*github.Client, error) {
	return func(ctx context.Context) (*github.Client, error) {
		token, err := auth.Token(ctx)
		if err != nil {
			return nil, err
		}
		return o.newClient(token, o.timeout)
	}
}

func (o *options) whoami(ctx context.Context, token string) (ghauth.Identity, error) {
	client, err := o.newClient(token, o.timeout)
	if err != nil {
		return ghauth.Identity{}, err
	}
	user, err := ghclient.CurrentUser(ctx, client)
	if err != nil {
		return ghauth.Identity{}, err
	}
	return ghauth.Identity{Login: user.GetLogin(), AvatarURL: user.GetAvatarURL()}, nil
}

func (o *options) dbPath() (string, error) {
	if o.db != "" {
		return o.db, nil
	}
	if p := os.Getenv("BABYSITTER_DB"); p != "" {
		return p, nil
	}
	return store.DefaultPath()
}

func (o *options) openStore() (*store.Store, error) {
	path, err := o.dbPath()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(path)
	if err != nil {
		return nil, err
	}
	ghclient.KeepResponses(st)
	return st, nil
}

func NewRootCmd(optFns ...Option) *cobra.Command {
	opts := &options{
		version:          "dev",
		newClient:        ghclient.New,
		newManager:       service.NewManager,
		newNotifier:      notify.New,
		newReleaseClient: ghclient.NewAnonymous,
		executable:       os.Executable,
		exec:             execInWorktree,
	}
	for _, fn := range optFns {
		fn(opts)
	}

	root := &cobra.Command{
		Use:   "babysitter",
		Short: "Watch GitHub pull requests from the terminal",
		Long: `babysitter watches the pull requests of the repositories you register and
keeps their state in a local SQLite database: details, review decision,
CI status and mergeable state.

Register repositories with 'repo add', poll once with 'sync', poll in the
foreground with 'serve', or install a background service with
'service install'. Read the state with 'prs'. Print a snapshot of one
pull request, with the actions to take next, with 'pr'. Send a desktop
notification to the user with 'notify'.

The token comes from, in this order:
  1. the --token flag
  2. the GITHUB_TOKEN environment variable
  3. the babysitter GitHub App, after 'babysitter auth login'
  4. the gh CLI, after 'gh auth login'

The database path comes from, in this order:
  1. the --db flag
  2. the BABYSITTER_DB environment variable
  3. <user config dir>/babysitter/babysitter.db`,
		Version:       opts.version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return validateOutput(opts.output)
		},
	}

	root.PersistentFlags().StringVar(&opts.token, "token", "", "GitHub token (overrides GITHUB_TOKEN and gh)")
	root.PersistentFlags().DurationVar(&opts.timeout, "timeout", 30*time.Second, "request timeout (0 for none)")
	root.PersistentFlags().StringVarP(&opts.output, "output", "o", outputText, "output format: text or json")
	root.PersistentFlags().StringVar(&opts.db, "db", "", "SQLite database path (overrides BABYSITTER_DB)")

	root.AddCommand(newWhoamiCmd(opts))
	root.AddCommand(newAuthCmd(opts))
	root.AddCommand(newReposCmd(opts))
	root.AddCommand(newRepoCmd(opts))
	root.AddCommand(newPRsCmd(opts))
	root.AddCommand(newPRCmd(opts))
	root.AddCommand(newSyncCmd(opts))
	root.AddCommand(newServeCmd(opts))
	root.AddCommand(newServiceCmd(opts))
	root.AddCommand(newDaemonCmd(opts))
	root.AddCommand(newWatchCmd(opts))
	root.AddCommand(newSettingsCmd(opts))
	root.AddCommand(newRateLimitCmd(opts))
	root.AddCommand(newNotifyCmd(opts))
	root.AddCommand(newNotificationsCmd(opts))
	root.AddCommand(newVersionCmd(opts))

	return root
}

type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string { return e.Err.Error() }

func (e *ExitCodeError) Unwrap() error { return e.Err }
