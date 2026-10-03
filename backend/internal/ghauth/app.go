package ghauth

import (
	"cmp"
	"os"
)

const (
	appClientID = "Iv23liQlp9iwjkgklxb3"
	appSlug     = "babysitter-orchestrator"
)

type App struct {
	ClientID string
	Slug     string
}

func DefaultApp() App {
	return App{
		ClientID: cmp.Or(os.Getenv("BABYSITTER_GITHUB_APP_CLIENT_ID"), appClientID),
		Slug:     cmp.Or(os.Getenv("BABYSITTER_GITHUB_APP_SLUG"), appSlug),
	}
}

func (a App) Available() bool {
	return a.ClientID != ""
}

func (a App) InstallURL() string {
	return "https://github.com/apps/" + a.Slug + "/installations/new"
}
