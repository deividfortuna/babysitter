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

const (
	refreshMargin = 5 * time.Minute
	firstRetry    = 15 * time.Second
	maxDoublings  = 3
)

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

	gitConfigMu sync.Mutex

	refreshMu sync.Mutex
	refused   string
	failures  int
	retryAt   time.Time
	lastErr   error

	mu       sync.Mutex
	unsaved  *renewal
	known    bool
	identity identity
	onChange func()
}

type renewal struct {
	Credentials
	onFile string
}

func (r *renewal) follows(stored Credentials) bool {
	return stored.RefreshToken == r.onFile
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

func (a *Auth) GitEnv(ctx context.Context) ([]string, error) {
	token, err := a.AppToken(ctx)
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
	a.hold(nil)
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

func (a *Auth) AppToken(ctx context.Context) (string, error) {
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
	c, err := a.load()
	if errors.Is(err, ErrSignedOut) {
		a.observe(identity{})
	}
	if err != nil {
		return Credentials{}, err
	}
	if a.settled(c.Token) {
		a.observe(identity{login: c.Login})
		return c, nil
	}
	renewed, err := a.refresh(ctx)
	a.observe(identityAfter(c, renewed, err))
	return renewed, err
}

func identityAfter(before, after Credentials, err error) identity {
	switch {
	case err == nil:
		return identity{login: after.Login}
	case errors.Is(err, ErrSignedOut):
		return identity{}
	}
	return identity{login: before.Login, expired: errors.Is(err, ErrSessionExpired)}
}

func (a *Auth) refresh(ctx context.Context) (Credentials, error) {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	unlock, err := a.file.Lock(ctx)
	if err != nil {
		return Credentials{}, err
	}
	defer unlock()
	c, onFile, err := a.lockedLoad()
	if err != nil {
		return Credentials{}, err
	}
	if a.fresh(c.Token) {
		return c, nil
	}
	if !a.renewable(c.Token) {
		return Credentials{}, ErrSessionExpired
	}
	if a.now().Before(a.retryAt) {
		return a.fallback(c)
	}
	rotation := context.WithoutCancel(ctx)
	tok, err := a.oauth.Refresh(rotation, c.RefreshToken)
	if errors.Is(err, errRefreshRefused) {
		a.refused = c.RefreshToken
		return Credentials{}, fmt.Errorf("%w: %w", ErrSessionExpired, err)
	}
	if err != nil {
		a.backOff(c.Token, err)
		return a.fallback(c)
	}
	a.failures, a.retryAt, a.lastErr = 0, time.Time{}, nil
	c.Token = tok
	a.keep(c, onFile)
	return c, nil
}

func (a *Auth) backOff(t Token, err error) {
	wait := firstRetry << min(a.failures, maxDoublings)
	if a.usable(t) {
		wait = min(wait, t.ExpiresAt.Sub(a.now()))
	}
	a.failures++
	a.retryAt = a.now().Add(wait)
	a.lastErr = err
}

func (a *Auth) fallback(c Credentials) (Credentials, error) {
	if a.usable(c.Token) {
		return c, nil
	}
	return Credentials{}, a.lastErr
}

func (a *Auth) load() (Credentials, error) {
	if r := a.held(); r != nil {
		return r.Credentials, nil
	}
	return a.file.Load()
}

func (a *Auth) lockedLoad() (Credentials, string, error) {
	stored, err := a.file.Load()
	r := a.held()
	if r == nil {
		return stored, stored.RefreshToken, err
	}
	if unreadable(err) {
		return r.Credentials, r.onFile, nil
	}
	if !r.follows(stored) {
		a.hold(nil)
		return stored, stored.RefreshToken, err
	}
	return r.Credentials, a.keep(r.Credentials, r.onFile), nil
}

func unreadable(err error) bool {
	return err != nil && !errors.Is(err, ErrSignedOut)
}

func (a *Auth) keep(c Credentials, onFile string) string {
	if err := a.file.Save(c); err != nil {
		a.hold(&renewal{Credentials: c, onFile: onFile})
		return onFile
	}
	a.hold(nil)
	return c.RefreshToken
}

func (a *Auth) held() *renewal {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.unsaved
}

func (a *Auth) hold(r *renewal) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unsaved = r
}

func (a *Auth) settled(t Token) bool {
	return a.fresh(t) && a.held() == nil
}

func (a *Auth) fresh(t Token) bool {
	return t.ExpiresAt.IsZero() || a.now().Add(refreshMargin).Before(t.ExpiresAt)
}

func (a *Auth) usable(t Token) bool {
	return t.ExpiresAt.IsZero() || a.now().Before(t.ExpiresAt)
}

func (a *Auth) renewable(t Token) bool {
	refreshLives := t.RefreshExpiresAt.IsZero() || a.now().Before(t.RefreshExpiresAt)
	return t.RefreshToken != "" && t.RefreshToken != a.refused && refreshLives
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
