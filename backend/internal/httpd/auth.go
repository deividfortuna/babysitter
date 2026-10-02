package httpd

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/deividfortuna/babysitter/internal/events"
	"github.com/deividfortuna/babysitter/internal/ghauth"
)

type Auth struct {
	Origin        string        `json:"origin" enum:",flag,env,app,gh" description:"Where the token of the daemon comes from: the --token flag, GITHUB_TOKEN, the babysitter GitHub App or the gh CLI; empty when there is no token"`
	AppAvailable  bool          `json:"appAvailable" description:"This build of babysitter knows the babysitter GitHub App"`
	SignedIn      bool          `json:"signedIn" description:"A GitHub account signed in with the babysitter GitHub App"`
	Login         string        `json:"login,omitempty" description:"The account signed in with the app"`
	ExpiresAt     *time.Time    `json:"expiresAt,omitempty" description:"When the token of the app expires; the daemon renews it before"`
	InstallURL    string        `json:"installUrl" description:"Where the user installs the app on more repositories"`
	Installations []string      `json:"installations" description:"The accounts the app is installed on, read only while the app gives the token"`
	SignIn        *SignInPrompt `json:"signIn,omitempty" description:"The sign in that waits for the user to enter the code on GitHub"`
	Error         string        `json:"error,omitempty" description:"Why the last sign in failed, or why the sign in of the app no longer works"`
}

type SignInPrompt struct {
	UserCode        string    `json:"userCode"`
	VerificationURI string    `json:"verificationUri"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

type AuthController interface {
	Status(ctx context.Context) Auth
	StartSignIn(ctx context.Context) (SignInPrompt, error)
	CancelSignIn()
	SignOut() error
}

var (
	errNoAuth = errors.New("the daemon has no GitHub sign in")

	signInErrors = newErrorMap("signin_failed",
		unavailable("app_unavailable", ghauth.ErrNoApp, errNoAuth),
	)
	signOutErrors = newErrorMap("signout_failed",
		unavailable("auth_unavailable", errNoAuth),
	)
)

func (a *api) handleGetAuth(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "auth_unavailable", errNoAuth.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.auth.Status(r.Context()))
}

func (a *api) handleStartSignIn(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		signInErrors.write(w, errNoAuth)
		return
	}
	prompt, err := a.auth.StartSignIn(r.Context())
	if signInErrors.write(w, err) {
		return
	}
	a.publishAuth()
	writeJSON(w, http.StatusAccepted, prompt)
}

func (a *api) handleCancelSignIn(w http.ResponseWriter, r *http.Request) {
	if a.auth != nil {
		a.auth.CancelSignIn()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if a.auth == nil {
		signOutErrors.write(w, errNoAuth)
		return
	}
	if signOutErrors.write(w, a.auth.SignOut()) {
		return
	}
	a.publishAuth()
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) publishAuth() {
	if a.bus != nil {
		a.bus.Publish(events.AuthChanged, "", 0)
	}
}

func (a *api) forgetViewerOnAuthChange() {
	if a.bus == nil {
		return
	}
	a.bus.Subscribe(func(e events.Event) {
		if e.Type == events.AuthChanged {
			a.viewer.reset()
		}
	})
}
