package ghfake

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-github/v91/github"
)

// The routes of the GitHub App sign in. The OAuth ones are on github.com,
// not on the API, and answer an error as status 200 with an error field.
const (
	RouteDeviceCode        = "POST /login/device/code"
	RouteOAuthGrant        = "POST /login/oauth/access_token"
	RouteInstallations     = "GET /user/installations"
	RouteInstallationRepos = "GET /user/installations/{id}/repositories"
)

// TokenLifetime and RefreshLifetime are the expires_in and
// refresh_token_expires_in the fake answers with, as GitHub does.
const (
	TokenLifetime   = 28800
	RefreshLifetime = 15897600
)

type device struct {
	code     string
	userCode string
	answer   string
}

type installation struct {
	id      int64
	account string
	repos   []string
}

type apps struct {
	devices  []*device
	refresh  map[string]bool
	issued   int
	installs []*installation
}

// ApproveDevice enters every pending code on GitHub and accepts it.
func (g *GitHub) ApproveDevice() {
	g.answerDevices("approved")
}

// DenyDevice enters every pending code on GitHub and cancels it.
func (g *GitHub) DenyDevice() {
	g.answerDevices("denied")
}

func (g *GitHub) answerDevices(answer string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, d := range g.apps.devices {
		d.answer = answer
	}
}

// RevokeRefresh makes every refresh token issued so far invalid.
func (g *GitHub) RevokeRefresh() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.apps.refresh = nil
}

// Install installs the app on account, on every repository of the account
// when repos is empty, and on those names only when it is not.
func (g *GitHub) Install(account string, repos ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.apps.installs = append(g.apps.installs, &installation{id: g.id(), account: account, repos: repos})
}

func (c *call) form() url.Values {
	v, _ := url.ParseQuery(string(c.a.Body))
	return v
}

func (c *call) deviceCode() {
	a := &c.g.apps
	n := len(a.devices) + 1
	d := &device{code: fmt.Sprintf("device-%d", n), userCode: fmt.Sprintf("ABCD-%04d", n)}
	a.devices = append(a.devices, d)
	c.json(http.StatusOK, map[string]any{
		"device_code":      d.code,
		"user_code":        d.userCode,
		"verification_uri": "https://github.com/login/device",
		"expires_in":       900,
		"interval":         5,
	})
}

func (c *call) accessToken() {
	form := c.form()
	switch form.Get("grant_type") {
	case "urn:ietf:params:oauth:grant-type:device_code":
		c.exchangeDevice(form.Get("device_code"))
	case "refresh_token":
		c.refreshToken(form.Get("refresh_token"))
	default:
		c.json(http.StatusOK, map[string]string{"error": "unsupported_grant_type"})
	}
}

func (c *call) exchangeDevice(code string) {
	a := &c.g.apps
	for i, d := range a.devices {
		if d.code != code {
			continue
		}
		switch d.answer {
		case "approved":
			a.devices = append(a.devices[:i], a.devices[i+1:]...)
			c.issueToken()
		case "denied":
			c.json(http.StatusOK, map[string]string{"error": "access_denied"})
		default:
			c.json(http.StatusOK, map[string]string{"error": "authorization_pending"})
		}
		return
	}
	c.json(http.StatusOK, map[string]string{"error": "incorrect_device_code"})
}

func (c *call) refreshToken(token string) {
	a := &c.g.apps
	if !a.refresh[token] {
		c.json(http.StatusOK, map[string]string{"error": "bad_refresh_token", "error_description": "The refresh token passed is incorrect or expired."})
		return
	}
	delete(a.refresh, token)
	c.issueToken()
}

func (c *call) issueToken() {
	a := &c.g.apps
	a.issued++
	refresh := fmt.Sprintf("ghr_fake%022d", a.issued)
	if a.refresh == nil {
		a.refresh = map[string]bool{}
	}
	a.refresh[refresh] = true
	c.json(http.StatusOK, map[string]any{
		"access_token":             fmt.Sprintf("ghu_fake%022d", a.issued),
		"expires_in":               TokenLifetime,
		"refresh_token":            refresh,
		"refresh_token_expires_in": RefreshLifetime,
		"token_type":               "bearer",
		"scope":                    "",
	})
}

func (c *call) installations() {
	out := make([]*github.Installation, 0, len(c.g.apps.installs))
	for _, inst := range c.g.apps.installs {
		selection := "all"
		if len(inst.repos) > 0 {
			selection = "selected"
		}
		out = append(out, &github.Installation{
			ID:                  new(inst.id),
			Account:             c.g.installationAccount(inst.account),
			RepositorySelection: new(selection),
		})
	}
	c.json(http.StatusOK, map[string]any{"total_count": len(out), "installations": out})
}

// installationAccount is the viewer for its own login and an organization
// for any other account.
func (g *GitHub) installationAccount(login string) *github.User {
	kind := "Organization"
	if strings.EqualFold(login, g.viewer.GetLogin()) {
		kind = "User"
	}
	return &github.User{
		Login:     new(login),
		Type:      new(kind),
		AvatarURL: new("https://avatars.githubusercontent.com/" + login),
	}
}

func (c *call) installationRepos() {
	id := c.int64Var("id")
	for _, inst := range c.g.apps.installs {
		if inst.id != id {
			continue
		}
		repos := make([]*github.Repository, 0, len(inst.repos))
		for _, name := range inst.repos {
			repos = append(repos, &github.Repository{Name: new(name), FullName: new(inst.account + "/" + name)})
		}
		c.json(http.StatusOK, map[string]any{"total_count": len(repos), "repositories": repos})
		return
	}
	c.fail(http.StatusNotFound, defaultNotFoundError)
}
