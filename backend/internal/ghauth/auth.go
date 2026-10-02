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
}

type Auth struct {
	app   App
	flag  string
	file  credentialsFile
	oauth OAuth
	gh    func(ctx context.Context) (string, error)
	now   func() time.Time

	refreshMu sync.Mutex

	mu       sync.Mutex
	known    bool
	identity identity
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
	a := &Auth{app: DefaultApp(), file: newCredentialsFile(dataDir), gh: ghCLIToken, now: time.Now}
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
	if c, ok := a.override(); ok {
		return c, nil
	}
	c, err := a.appCredentials(ctx)
	if err == nil {
		return Credential{Token: c.AccessToken, Origin: OriginApp}, nil
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

func (a *Auth) GitEnv(ctx context.Context, network bool) ([]string, error) {
	if !network {
		return gitEnvFor(a.storedToken()), nil
	}
	token, err := a.appToken(ctx)
	if err != nil {
		return nil, err
	}
	return gitEnvFor(token), nil
}

func gitEnvFor(token string) []string {
	if token == "" {
		return nil
	}
	return gitEnv(token)
}

func (a *Auth) SignOut(ctx context.Context) error {
	unlock, err := a.file.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if err := a.file.Remove(); err != nil {
		return err
	}
	a.announce(identity{})
	return nil
}

func (a *Auth) override() (Credential, bool) {
	if a.flag != "" {
		return Credential{Token: a.flag, Origin: OriginFlag}, true
	}
	if t := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); t != "" {
		return Credential{Token: t, Origin: OriginEnv}, true
	}
	return Credential{}, false
}

func (a *Auth) appToken(ctx context.Context) (string, error) {
	if _, ok := a.override(); ok {
		return "", nil
	}
	c, err := a.appCredentials(ctx)
	if errors.Is(err, ErrSignedOut) {
		return "", nil
	}
	return c.AccessToken, err
}

func (a *Auth) appCredentials(ctx context.Context) (Credentials, error) {
	c, err := a.file.Load()
	if errors.Is(err, ErrSignedOut) {
		a.observe(identity{})
	}
	if err != nil {
		return Credentials{}, err
	}
	if a.fresh(c.Token) {
		a.observe(identity{login: c.Login})
		return c, nil
	}
	renewed, err := a.refresh(ctx)
	a.observe(identity{login: c.Login, expired: errors.Is(err, ErrSessionExpired)})
	return renewed, err
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
	rotation := context.WithoutCancel(ctx)
	tok, err := a.oauth.Refresh(rotation, c.RefreshToken)
	if errors.Is(err, errRefreshRefused) {
		return Credentials{}, fmt.Errorf("%w: %w", ErrSessionExpired, err)
	}
	if err != nil && a.usable(c.Token) {
		return c, nil
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

func (a *Auth) storedToken() string {
	if _, ok := a.override(); ok {
		return ""
	}
	c, err := a.file.Load()
	if err != nil {
		return ""
	}
	if !a.usable(c.Token) {
		return ""
	}
	return c.AccessToken
}

func (a *Auth) fresh(t Token) bool {
	return t.ExpiresAt.IsZero() || a.now().Add(refreshMargin).Before(t.ExpiresAt)
}

func (a *Auth) usable(t Token) bool {
	return t.ExpiresAt.IsZero() || a.now().Before(t.ExpiresAt)
}

func (a *Auth) renewable(t Token) bool {
	refreshLives := t.RefreshExpiresAt.IsZero() || a.now().Before(t.RefreshExpiresAt)
	return t.RefreshToken != "" && refreshLives
}

type identity struct {
	login   string
	expired bool
}

func (a *Auth) observe(id identity) {
	a.mu.Lock()
	changed := a.known && a.identity != id
	a.known, a.identity = true, id
	fn := a.onChange
	a.mu.Unlock()
	if changed {
		a.notify(fn)
	}
}

func (a *Auth) announce(id identity) {
	a.mu.Lock()
	a.known, a.identity = true, id
	fn := a.onChange
	a.mu.Unlock()
	a.notify(fn)
}

func (a *Auth) changed() {
	a.mu.Lock()
	fn := a.onChange
	a.mu.Unlock()
	a.notify(fn)
}

func (a *Auth) notify(fn func()) {
	if fn != nil {
		go fn()
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
