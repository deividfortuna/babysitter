package daemon

import (
	"context"
	"errors"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/gitrepo"
	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/watcher"
)

type authController struct {
	auth      *ghauth.Auth
	signIns   *ghauth.SignIns
	newClient watcher.ClientFunc
}

func newAuthController(ctx context.Context, cfg Config, bus *events.Bus) *authController {
	if cfg.Auth == nil {
		return nil
	}
	changed := func() { bus.Publish(events.AuthChanged, "", 0) }
	cfg.Auth.OnChange(changed)
	return &authController{
		auth:      cfg.Auth,
		signIns:   cfg.Auth.SignIns(ctx, cfg.Whoami, changed),
		newClient: cfg.NewClient,
	}
}

func (c *authController) controller() httpd.AuthController {
	if c == nil {
		return nil
	}
	return c
}

func (c *authController) gitEnv() gitrepo.AuthEnv {
	if c == nil {
		return nil
	}
	return c.auth.GitEnv
}

func (c *authController) checkAccess() prwatch.AccessCheck {
	if c == nil {
		return nil
	}
	return c.auth.CheckRepo
}

func (c *authController) Status(ctx context.Context) httpd.Auth {
	st := c.auth.Status(ctx)
	prompt, waiting := c.signIns.Current()
	out := httpd.Auth{
		State:         authState(st, waiting),
		Origin:        string(st.Origin),
		AppAvailable:  st.Available,
		Login:         st.Login,
		AvatarURL:     st.AvatarURL,
		InstallURL:    st.InstallURL,
		Installations: []httpd.AuthInstallation{},
	}
	if st.SignedIn && !st.ExpiresAt.IsZero() {
		out.ExpiresAt = &st.ExpiresAt
	}
	if waiting {
		out.SignIn = &httpd.SignInPrompt{UserCode: prompt.UserCode, VerificationURI: prompt.VerificationURI, ExpiresAt: prompt.ExpiresAt}
	}
	if err := c.signIns.Err(); err != nil {
		out.SignInFailure, out.SignInError = signInFailure(err), err.Error()
	}
	if reportable(st.Err) {
		out.Error = st.Err.Error()
	}
	if st.Origin == ghauth.OriginApp {
		out.Installations, out.Error = c.installations(ctx, out.Error)
	}
	return out
}

func authState(st ghauth.Status, waiting bool) string {
	switch {
	case waiting:
		return httpd.AuthWaiting
	case !st.SignedIn:
		return httpd.AuthSignedOut
	case st.Expired():
		return httpd.AuthExpired
	case st.Origin == ghauth.OriginApp:
		return httpd.AuthConnected
	}
	return httpd.AuthNotInUse
}

func signInFailure(err error) string {
	switch {
	case errors.Is(err, ghauth.ErrCodeExpired):
		return httpd.SignInCodeExpired
	case errors.Is(err, ghauth.ErrDenied):
		return httpd.SignInDenied
	}
	return httpd.SignInFailed
}

func reportable(err error) bool {
	expected := errors.Is(err, ghauth.ErrNoToken) || errors.Is(err, ghauth.ErrSessionExpired)
	return err != nil && !expected
}

func (c *authController) installations(ctx context.Context, prior string) ([]httpd.AuthInstallation, string) {
	client, err := c.newClient(ctx)
	if err != nil {
		return []httpd.AuthInstallation{}, err.Error()
	}
	installs, err := ghclient.UserInstallations(ctx, client)
	if err != nil {
		return []httpd.AuthInstallation{}, err.Error()
	}
	out := make([]httpd.AuthInstallation, 0, len(installs))
	for _, inst := range installs {
		out = append(out, httpd.AuthInstallation{Login: inst.Account, AvatarURL: inst.AvatarURL, Organization: inst.Organization})
	}
	return out, prior
}

func (c *authController) StartSignIn(ctx context.Context) (httpd.SignInPrompt, error) {
	prompt, err := c.signIns.Start(ctx)
	if err != nil {
		return httpd.SignInPrompt{}, err
	}
	return httpd.SignInPrompt{UserCode: prompt.UserCode, VerificationURI: prompt.VerificationURI, ExpiresAt: prompt.ExpiresAt}, nil
}

func (c *authController) CancelSignIn() {
	c.signIns.Cancel()
}

func (c *authController) SignOut() error {
	c.signIns.Cancel()
	return c.auth.SignOut()
}
