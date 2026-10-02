package ghauth

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/testutil"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type harness struct {
	g     *ghfake.GitHub
	srv   *ghfake.Server
	dir   string
	clock *clock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "")
	g := ghfake.New()
	return &harness{g: g, srv: g.Serve(t), dir: t.TempDir(), clock: &clock{now: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)}}
}

func noGH(context.Context) (string, error) { return "", errors.New("gh is not installed") }

func (h *harness) auth(opts ...Option) *Auth {
	base := []Option{
		WithApp(App{ClientID: "Iv1.test", Slug: "babysitter"}),
		WithOAuth(OAuth{BaseURL: h.srv.URL, PollUnit: time.Millisecond}),
		WithGH(noGH),
		WithClock(h.clock.Now),
	}
	return New(h.dir, append(base, opts...)...)
}

func (h *harness) whoami(t *testing.T) Whoami {
	return func(ctx context.Context, token string) (Identity, error) {
		u, err := ghclient.CurrentUser(ctx, h.srv.Client(t))
		return Identity{Login: u.GetLogin(), AvatarURL: u.GetAvatarURL()}, err
	}
}

func (h *harness) signIn(t *testing.T, a *Auth) Credentials {
	t.Helper()
	ctx := context.Background()
	code, err := a.RequestCode(ctx)
	if err != nil {
		t.Fatalf("RequestCode: %v", err)
	}
	h.g.ApproveDevice()
	c, err := a.Complete(ctx, code, h.whoami(t))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return c
}

func TestCredentialOrder(t *testing.T) {
	h := newHarness(t)
	gh := func(context.Context) (string, error) { return "gho_from_gh", nil }
	h.signIn(t, h.auth())

	cases := []struct {
		name   string
		env    string
		opts   []Option
		origin Origin
	}{
		{"the flag comes first", "ghp_env", []Option{WithFlag("ghp_flag"), WithGH(gh)}, OriginFlag},
		{"GITHUB_TOKEN comes before the app", "ghp_env", []Option{WithGH(gh)}, OriginEnv},
		{"the app comes before gh", "", []Option{WithGH(gh)}, OriginApp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", tc.env)
			c, err := h.auth(tc.opts...).Credential(context.Background())
			if err != nil {
				t.Fatalf("Credential: %v", err)
			}
			if c.Origin != tc.origin {
				t.Fatalf("origin = %q, want %q", c.Origin, tc.origin)
			}
		})
	}
}

func TestCredentialFallsBackToGHWhenSignedOut(t *testing.T) {
	h := newHarness(t)
	gh := func(context.Context) (string, error) { return "gho_from_gh", nil }

	c, err := h.auth(WithGH(gh)).Credential(context.Background())
	if err != nil {
		t.Fatalf("Credential: %v", err)
	}
	if c.Origin != OriginGH || c.Token != "gho_from_gh" {
		t.Fatalf("credential = %+v, want the token of gh", c)
	}
}

func TestCredentialWithoutAnySource(t *testing.T) {
	h := newHarness(t)

	_, err := h.auth().Credential(context.Background())
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}

func TestCredentialTrimsGITHUB_TOKEN(t *testing.T) {
	h := newHarness(t)
	t.Setenv("GITHUB_TOKEN", "  abc123  ")

	got, err := h.auth().Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "abc123" {
		t.Fatalf("Token = %q, want abc123", got)
	}
}

func TestSignInSavesTheAccountAndTheToken(t *testing.T) {
	h := newHarness(t)
	h.g.Viewer().AvatarURL = new("https://avatars.githubusercontent.com/u/1")
	a := h.auth()

	c := h.signIn(t, a)

	if c.Login != "alice" || c.AvatarURL != "https://avatars.githubusercontent.com/u/1" || !strings.HasPrefix(c.AccessToken, "ghu_") || !strings.HasPrefix(c.RefreshToken, "ghr_") {
		t.Fatalf("credentials = %+v", c)
	}
	if want := h.clock.Now().Add(ghfake.TokenLifetime * time.Second); !c.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", c.ExpiresAt, want)
	}
	cred, err := a.Credential(context.Background())
	if err != nil || cred.Origin != OriginApp || cred.Token != c.AccessToken {
		t.Fatalf("Credential = %+v, %v", cred, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(h.dir, signInFileName))
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("the sign in file has mode %v, want 0600", perm)
		}
	}
}

func TestSignInRefusedOnGitHub(t *testing.T) {
	h := newHarness(t)
	a := h.auth()
	ctx := context.Background()
	code, err := a.RequestCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h.g.DenyDevice()

	_, err = a.Complete(ctx, code, h.whoami(t))

	if !errors.Is(err, ErrDenied) {
		t.Fatalf("err = %v, want ErrDenied", err)
	}
	if _, err := newCredentialsFile(h.dir).Load(); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("Load = %v, want ErrSignedOut", err)
	}
}

func TestSignInWaitsWhileTheCodeIsPending(t *testing.T) {
	h := newHarness(t)
	a := h.auth()
	ctx := context.Background()
	code, err := a.RequestCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := a.Complete(ctx, code, h.whoami(t))
		done <- err
	}()

	testutil.Eventually(t, func() bool { return h.g.Count(ghfake.RouteOAuthGrant) >= 2 }, "the daemon polls for the token")
	h.g.ApproveDevice()

	if err := <-done; err != nil {
		t.Fatalf("Complete: %v", err)
	}
}

func TestRequestCodeWithoutApp(t *testing.T) {
	h := newHarness(t)

	_, err := h.auth(WithApp(App{Slug: "babysitter"})).RequestCode(context.Background())

	if !errors.Is(err, ErrNoApp) {
		t.Fatalf("err = %v, want ErrNoApp", err)
	}
}

func TestTokenIsRenewedBeforeItExpires(t *testing.T) {
	h := newHarness(t)
	first := h.signIn(t, h.auth())
	h.clock.Advance(ghfake.TokenLifetime*time.Second - time.Minute)

	daemon, cli := h.auth(), h.auth()
	got, err := daemon.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got == first.AccessToken {
		t.Fatal("the daemon used the token that is about to expire")
	}
	again, err := cli.Token(context.Background())
	if err != nil {
		t.Fatalf("Token of the second process: %v", err)
	}
	if again != got {
		t.Fatalf("the second process has %q, want the renewed %q", again, got)
	}
	if n := refreshes(h.g); n != 1 {
		t.Fatalf("%d renewals, want 1: a refresh token works once", n)
	}
}

func TestConcurrentRenewalsUseTheRefreshTokenOnce(t *testing.T) {
	h := newHarness(t)
	h.signIn(t, h.auth())
	h.clock.Advance(ghfake.TokenLifetime * time.Second)

	var failed atomic.Int32
	var wg sync.WaitGroup
	for range 4 {
		a := h.auth()
		wg.Go(func() {
			if _, err := a.Token(context.Background()); err != nil {
				failed.Add(1)
			}
		})
	}
	wg.Wait()

	if failed.Load() != 0 {
		t.Fatalf("%d of the processes failed to get a token", failed.Load())
	}
	if n := refreshes(h.g); n != 1 {
		t.Fatalf("%d renewals, want 1", n)
	}
}

func TestRevokedRefreshTokenEndsTheSession(t *testing.T) {
	h := newHarness(t)
	h.signIn(t, h.auth())
	h.g.RevokeRefresh()
	h.clock.Advance(ghfake.TokenLifetime * time.Second)
	gh := func(context.Context) (string, error) { return "gho_from_gh", nil }

	_, err := h.auth(WithGH(gh)).Credential(context.Background())

	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want ErrSessionExpired and no quiet switch to gh", err)
	}
	if st := h.auth(WithGH(gh)).Status(context.Background()); st.State != StateExpired || st.Login != "alice" {
		t.Fatalf("Status = %+v, want the expired sign in of alice", st)
	}
}

func TestExpiredRefreshTokenEndsTheSessionWithoutAsking(t *testing.T) {
	h := newHarness(t)
	h.signIn(t, h.auth())
	h.clock.Advance(ghfake.RefreshLifetime * time.Second)

	_, err := h.auth().Credential(context.Background())

	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("err = %v, want ErrSessionExpired", err)
	}
	if n := refreshes(h.g); n != 0 {
		t.Fatalf("%d renewals, want 0", n)
	}
}

func TestOnChangeFollowsTheSignInOfAnotherProcess(t *testing.T) {
	h := newHarness(t)
	daemon := h.auth()
	var changes atomic.Int32
	daemon.OnChange(func() { changes.Add(1) })
	if _, err := daemon.Credential(context.Background()); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}

	h.signIn(t, h.auth())
	if _, err := daemon.Credential(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := daemon.Credential(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.Eventually(t, func() bool { return changes.Load() == 1 }, "one change after the sign in")

	if err := daemon.SignOut(context.Background()); err != nil {
		t.Fatal(err)
	}
	testutil.Eventually(t, func() bool { return changes.Load() == 2 }, "one more change after the sign out")
}

func TestChangesAreToldOutsideTheTokenLookup(t *testing.T) {
	h := newHarness(t)
	daemon := h.auth()
	var mu sync.Mutex
	told := make(chan struct{}, 1)
	daemon.OnChange(func() {
		mu.Lock()
		defer mu.Unlock()
		told <- struct{}{}
	})
	if _, err := daemon.Credential(context.Background()); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
	h.signIn(t, h.auth())

	mu.Lock()
	_, err := daemon.Credential(context.Background())
	mu.Unlock()

	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-told:
	case <-time.After(5 * time.Second):
		t.Fatal("the change was not told")
	}
}

func TestSignOutWaitsForARenewalThatHoldsTheLock(t *testing.T) {
	h := newHarness(t)
	h.signIn(t, h.auth())
	file := newCredentialsFile(h.dir)
	unlock, err := file.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- h.auth().SignOut(context.Background()) }()

	select {
	case err := <-done:
		t.Fatalf("SignOut returned %v while a renewal held the lock", err)
	case <-time.After(200 * time.Millisecond):
	}
	c, _ := file.Load()
	if err := file.Save(c); err != nil {
		t.Fatal(err)
	}
	unlock()

	if err := <-done; err != nil {
		t.Fatalf("SignOut: %v", err)
	}
	if _, err := file.Load(); !errors.Is(err, ErrSignedOut) {
		t.Fatalf("Load after the sign out = %v, want ErrSignedOut: the renewal wrote the sign in back", err)
	}
}

func TestFailedRenewalKeepsATokenThatStillWorks(t *testing.T) {
	h := newHarness(t)
	first := h.signIn(t, h.auth())
	h.clock.Advance(ghfake.TokenLifetime*time.Second - time.Minute)
	h.g.React(ghfake.RouteOAuthGrant, func(ghfake.Action) (ghfake.Response, bool) {
		return ghfake.Response{Status: http.StatusBadGateway, Message: "bad gateway"}, true
	})

	got, err := h.auth().Token(context.Background())

	if err != nil || got != first.AccessToken {
		t.Fatalf("Token = %q, %v; want the stored token while it still works", got, err)
	}
	h.clock.Advance(2 * time.Minute)
	if _, err := h.auth().Token(context.Background()); err == nil {
		t.Fatal("Token after the expiry worked with a renewal that failed")
	}
}

func TestGitEnvOnlyForTheApp(t *testing.T) {
	h := newHarness(t)
	t.Setenv("GITHUB_TOKEN", "ghp_env")
	env, err := h.auth().GitEnv(context.Background())
	if err != nil || env != nil {
		t.Fatalf("GitEnv with GITHUB_TOKEN = %v, %v; want nothing, so git keeps the credentials of the user", env, err)
	}

	t.Setenv("GITHUB_TOKEN", "")
	env, err = h.auth().GitEnv(context.Background())
	if err != nil || env != nil {
		t.Fatalf("GitEnv without a token = %v, %v; want nothing", env, err)
	}

	c := h.signIn(t, h.auth())
	env, err = h.auth().GitEnv(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(env, gitEnv(c.AccessToken)) {
		t.Fatalf("GitEnv = %v, want the env of the app token", env)
	}
}

func TestGitEnvSendsTheTokenAsAHeaderAndRewritesSSH(t *testing.T) {
	env := gitEnv("ghu_abc")
	want := []string{
		"GIT_CONFIG_COUNT=3",
		"GIT_CONFIG_KEY_0=url.https://github.com/.insteadOf",
		"GIT_CONFIG_VALUE_0=git@github.com:",
		"GIT_CONFIG_KEY_1=url.https://github.com/.insteadOf",
		"GIT_CONFIG_VALUE_1=ssh://git@github.com/",
		"GIT_CONFIG_KEY_2=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_2=AUTHORIZATION: basic eC1hY2Nlc3MtdG9rZW46Z2h1X2FiYw==",
	}
	if !slices.Equal(env, want) {
		t.Fatalf("GitEnv = %v, want %v", env, want)
	}
}

func TestCheckRepo(t *testing.T) {
	h := newHarness(t)
	h.g.Install("acme", "api")
	h.g.Install("alice")
	h.signIn(t, h.auth())
	client := h.srv.Client(t)

	cases := []struct {
		owner, name string
		want        error
	}{
		{"acme", "api", nil},
		{"ACME", "API", nil},
		{"acme", "web", ErrNotInstalled},
		{"alice", "anything", nil},
		{"globex", "api", ErrNotInstalled},
	}
	for _, tc := range cases {
		err := h.auth().CheckRepos(context.Background(), client, tc.owner+"/"+tc.name)
		if !errors.Is(err, tc.want) {
			t.Errorf("CheckRepos(%s/%s) = %v, want %v", tc.owner, tc.name, err, tc.want)
		}
	}
}

func TestCheckReposListsTheInstallationsOnce(t *testing.T) {
	h := newHarness(t)
	h.g.Install("acme")
	h.g.Install("alice", "fork")
	h.signIn(t, h.auth())

	err := h.auth().CheckRepos(context.Background(), h.srv.Client(t), "acme/api", "alice/fork", "alice/other")

	if !errors.Is(err, ErrNotInstalled) || !strings.Contains(err.Error(), "alice/other") || strings.Contains(err.Error(), "acme/api") {
		t.Fatalf("CheckRepos = %v, want only alice/other named", err)
	}
	if n := h.g.Count(ghfake.RouteInstallations); n != 1 {
		t.Fatalf("%d listings of the installations, want 1", n)
	}
}

func TestCheckRepoSkipsOtherSources(t *testing.T) {
	h := newHarness(t)
	t.Setenv("GITHUB_TOKEN", "ghp_env")

	if err := h.auth().CheckRepos(context.Background(), h.srv.Client(t), "acme/api"); err != nil {
		t.Fatalf("CheckRepo = %v, want nil for a personal token", err)
	}
	if n := h.g.Count(ghfake.RouteInstallations); n != 0 {
		t.Fatalf("%d calls to the installations, want 0", n)
	}
}

func TestGitEnvAndCheckRepoNeverAskGH(t *testing.T) {
	h := newHarness(t)
	var asked atomic.Int32
	gh := func(context.Context) (string, error) {
		asked.Add(1)
		return "gho_from_gh", nil
	}
	a := h.auth(WithGH(gh))

	if env, err := a.GitEnv(context.Background()); err != nil || env != nil {
		t.Fatalf("GitEnv = %v, %v; want nothing for the gh CLI", env, err)
	}
	if err := a.CheckRepos(context.Background(), h.srv.Client(t), "acme/api"); err != nil {
		t.Fatalf("CheckRepo = %v, want nil for the gh CLI", err)
	}
	if n := asked.Load(); n != 0 {
		t.Fatalf("gh asked %d times, want 0: only the app token matters there", n)
	}
}

func TestStatusStates(t *testing.T) {
	h := newHarness(t)
	gh := func(context.Context) (string, error) { return "gho_from_gh", nil }
	signedOut := h.auth(WithGH(gh)).Status(context.Background())
	if signedOut.State != StateSignedOut || signedOut.Origin != OriginGH || signedOut.Err != nil {
		t.Fatalf("signed out Status = %+v", signedOut)
	}
	h.signIn(t, h.auth())
	if st := h.auth().Status(context.Background()); st.State != StateConnected || st.Origin != OriginApp {
		t.Fatalf("signed in Status = %+v", st)
	}
	t.Setenv("GITHUB_TOKEN", "ghp_env")
	if st := h.auth().Status(context.Background()); st.State != StateNotInUse || st.Origin != OriginEnv {
		t.Fatalf("Status with GITHUB_TOKEN = %+v", st)
	}
}

func refreshes(g *ghfake.GitHub) int {
	n := 0
	for _, a := range g.Calls(ghfake.RouteOAuthGrant) {
		if strings.Contains(string(a.Body), "grant_type=refresh_token") {
			n++
		}
	}
	return n
}
