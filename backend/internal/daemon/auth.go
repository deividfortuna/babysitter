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
	out := httpd.Auth{
		Origin:        string(st.Origin),
		AppAvailable:  st.Available,
		SignedIn:      st.SignedIn,
		Login:         st.Login,
		InstallURL:    st.InstallURL,
		Installations: []string{},
	}
	if st.SignedIn && !st.ExpiresAt.IsZero() {
		out.ExpiresAt = &st.ExpiresAt
	}
	if prompt, ok := c.signIns.Current(); ok {
		out.SignIn = &httpd.SignInPrompt{UserCode: prompt.UserCode, VerificationURI: prompt.VerificationURI, ExpiresAt: prompt.ExpiresAt}
	}
	if err := errors.Join(st.Err, c.signIns.Err()); err != nil {
		out.Error = err.Error()
	}
	if st.Origin == ghauth.OriginApp {
		out.Installations, out.Error = c.installations(ctx, out.Error)
	}
	return out
}

func (c *authController) installations(ctx context.Context, prior string) ([]string, string) {
	client, err := c.newClient(ctx)
	if err != nil {
		return []string{}, err.Error()
	}
	installs, err := ghclient.UserInstallations(ctx, client)
	if err != nil {
		return []string{}, err.Error()
	}
	accounts := make([]string, 0, len(installs))
	for _, inst := range installs {
		accounts = append(accounts, inst.Account)
	}
	return accounts, prior
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
