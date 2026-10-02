package daemon

import (
	"context"
	"errors"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghauth"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/watcher"
)

type authController struct {
	auth      *ghauth.Auth
	signIns   *ghauth.SignIns
	newClient watcher.ClientFunc
}

func newAuthController(ctx context.Context, cfg Config, bus *events.Bus) *authController {
	cfg.Auth.OnChange(func() { bus.Publish(events.AuthChanged, "", 0) })
	return &authController{
		auth:      cfg.Auth,
		signIns:   cfg.Auth.SignIns(ctx, cfg.Whoami),
		newClient: cfg.NewClient,
	}
}

func (c *authController) Status(ctx context.Context) httpd.Auth {
	st := c.auth.Status(ctx)
	app := c.auth.App()
	out := httpd.Auth{
		State:         string(st.State),
		Origin:        string(st.Origin),
		AppAvailable:  app.Available(),
		Login:         st.Login,
		AvatarURL:     st.AvatarURL,
		InstallURL:    app.InstallURL(),
		Installations: []httpd.AuthInstallation{},
	}
	if prompt, waiting := c.signIns.Current(); waiting {
		out.State, out.SignIn = httpd.AuthWaiting, signInPrompt(prompt)
	}
	if err := c.signIns.Err(); err != nil {
		out.SignInFailure, out.SignInError = signInFailure(err), err.Error()
	}
	if st.Err != nil {
		out.Error = st.Err.Error()
	}
	tokenUnreadable := st.State == ghauth.StateConnected && st.Origin != ghauth.OriginApp
	if tokenUnreadable {
		out.InstallsError, out.Error = out.Error, ""
		return out
	}
	if st.Origin != ghauth.OriginApp {
		return out
	}
	installs, err := c.installations(ctx)
	if err != nil {
		out.InstallsError = err.Error()
		return out
	}
	out.Installations = installs
	return out
}

func signInPrompt(p ghauth.Prompt) *httpd.SignInPrompt {
	return &httpd.SignInPrompt{UserCode: p.UserCode, VerificationURI: p.VerificationURI, ExpiresAt: p.ExpiresAt}
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

func (c *authController) installations(ctx context.Context) ([]httpd.AuthInstallation, error) {
	client, err := c.newClient(ctx)
	if err != nil {
		return nil, err
	}
	installs, err := ghclient.UserInstallations(ctx, client)
	if err != nil {
		return nil, err
	}
	out := make([]httpd.AuthInstallation, 0, len(installs))
	for _, inst := range installs {
		out = append(out, httpd.AuthInstallation{Login: inst.Account, AvatarURL: inst.AvatarURL, Organization: inst.Organization})
	}
	return out, nil
}

func (c *authController) StartSignIn(ctx context.Context) (httpd.SignInPrompt, error) {
	prompt, err := c.signIns.Start(ctx)
	if err != nil {
		return httpd.SignInPrompt{}, err
	}
	return *signInPrompt(prompt), nil
}

func (c *authController) CancelSignIn() {
	c.signIns.Cancel()
}

func (c *authController) SignOut(ctx context.Context) error {
	c.signIns.Cancel()
	return c.auth.SignOut(ctx)
}
