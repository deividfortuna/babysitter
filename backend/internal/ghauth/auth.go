package ghauth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Origin string

const (
	OriginFlag Origin = "flag"
	OriginEnv  Origin = "env"
	OriginApp  Origin = "app"
	OriginGH   Origin = "gh"
)

const refreshMargin = 5 * time.Minute

var (
	ErrNoToken        = errors.New("no GitHub token found: run 'babysitter auth login', set GITHUB_TOKEN or run 'gh auth login'")
	ErrSessionExpired = errors.New("the GitHub App sign in expired: run 'babysitter auth login' or sign in again in the app")
	ErrNoApp          = errors.New("this build of babysitter has no GitHub App")
)

type Credential struct {
	Token  string
	Origin Origin
	Login  string
}

type Auth struct {
	app   App
	flag  string
	file  File
	oauth OAuth
	gh    func(ctx context.Context) (string, error)
	now   func() time.Time

	refreshMu sync.Mutex

	mu       sync.Mutex
	known    bool
	identity string
	onChange func()
}

type Option func(*Auth)

func WithFlag(token string) Option {
	return func(a *Auth) { a.flag = token }
}

func WithApp(app App) Option {
	return func(a *Auth) { a.app = app }
}

func WithOAuth(o OAuth) Option {
	return func(a *Auth) { a.oauth = o }
}

func WithGH(fn func(ctx context.Context) (string, error)) Option {
	return func(a *Auth) { a.gh = fn }
}

func WithClock(now func() time.Time) Option {
	return func(a *Auth) { a.now = now }
}

func New(dataDir string, opts ...Option) *Auth {
	a := &Auth{app: DefaultApp(), file: NewFile(dataDir), gh: ghCLIToken, now: time.Now}
	for _, opt := range opts {
		opt(a)
	}
	if a.oauth.ClientID == "" {
		a.oauth.ClientID = a.app.ClientID
	}
	if a.oauth.Now == nil {
		a.oauth.Now = a.now
	}
	return a
}

func (a *Auth) App() App {
	return a.app
}

func (a *Auth) OnChange(fn func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onChange = fn
}

func (a *Auth) Credential(ctx context.Context) (Credential, error) {
	if a.flag != "" {
		return Credential{Token: a.flag, Origin: OriginFlag}, nil
	}
	if t := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); t != "" {
		return Credential{Token: t, Origin: OriginEnv}, nil
	}
	c, err := a.AppCredentials(ctx)
	if err == nil {
		return Credential{Token: c.AccessToken, Origin: OriginApp, Login: c.Login}, nil
	}
	if !errors.Is(err, ErrSignedOut) {
		return Credential{}, err
	}
	t, err := a.gh(ctx)
	if err != nil {
		return Credential{}, ErrNoToken
	}
	return Credential{Token: t, Origin: OriginGH}, nil
}

func (a *Auth) Token(ctx context.Context) (string, error) {
	c, err := a.Credential(ctx)
	return c.Token, err
}

func (a *Auth) GitEnv(ctx context.Context) ([]string, error) {
	c, err := a.Credential(ctx)
	if errors.Is(err, ErrNoToken) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if c.Origin != OriginApp {
		return nil, nil
	}
	return GitEnv(c.Token), nil
}

func (a *Auth) SignOut() error {
	if err := a.file.Remove(); err != nil {
		return err
	}
	a.observe("")
	return nil
}

func (a *Auth) AppCredentials(ctx context.Context) (Credentials, error) {
	c, err := a.file.Load()
	if errors.Is(err, ErrSignedOut) {
		a.observe("")
	}
	if err != nil {
		return Credentials{}, err
	}
	a.observe(c.Login)
	if a.fresh(c.Token) {
		return c, nil
	}
	return a.refresh(ctx)
}

func (a *Auth) refresh(ctx context.Context) (Credentials, error) {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	unlock, err := a.file.Lock(ctx)
	if err != nil {
		return Credentials{}, err
	}
	defer unlock()
	c, err := a.file.Load()
	if err != nil {
		return Credentials{}, err
	}
	if a.fresh(c.Token) {
		return c, nil
	}
	if !a.renewable(c.Token) {
		return Credentials{}, ErrSessionExpired
	}
	tok, err := a.oauth.Refresh(ctx, c.RefreshToken)
	if errors.Is(err, ErrRefreshRefused) {
		return Credentials{}, fmt.Errorf("%w: %w", ErrSessionExpired, err)
	}
	if err != nil {
		return Credentials{}, err
	}
	c.Token = tok
	if err := a.file.Save(c); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

func (a *Auth) fresh(t Token) bool {
	return t.ExpiresAt.IsZero() || a.now().Add(refreshMargin).Before(t.ExpiresAt)
}

func (a *Auth) renewable(t Token) bool {
	refreshLives := t.RefreshExpiresAt.IsZero() || a.now().Before(t.RefreshExpiresAt)
	return t.RefreshToken != "" && refreshLives
}

func (a *Auth) observe(login string) {
	a.mu.Lock()
	changed := a.known && a.identity != login
	a.known, a.identity = true, login
	fn := a.onChange
	a.mu.Unlock()
	if changed && fn != nil {
		fn()
	}
}

func ghCLIToken(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(string(out))
	if t == "" {
		return "", ErrNoToken
	}
	return t, nil
}

type Status struct {
	Origin     Origin
	Available  bool
	SignedIn   bool
	Login      string
	AvatarURL  string
	ExpiresAt  time.Time
	InstallURL string
	Err        error
}

func (s Status) Expired() bool {
	return s.SignedIn && errors.Is(s.Err, ErrSessionExpired)
}

func (a *Auth) Status(ctx context.Context) Status {
	st := Status{Available: a.app.Available(), InstallURL: a.app.InstallURL()}
	cred, err := a.Credential(ctx)
	st.Origin, st.Err = cred.Origin, err
	if c, err := a.file.Load(); err == nil {
		st.SignedIn, st.Login, st.AvatarURL, st.ExpiresAt = true, c.Login, c.AvatarURL, c.ExpiresAt
	}
	return st
}
