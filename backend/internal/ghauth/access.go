package ghauth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient"
)

var ErrNotInstalled = errors.New("the GitHub App is not installed on the repository")

func (a *Auth) CheckRepos(ctx context.Context, client *github.Client, repos ...string) error {
	token, err := a.AppToken(ctx)
	if err != nil {
		return err
	}
	if token == "" {
		return nil
	}
	missing, err := ghclient.MissingInstallations(ctx, client, repos...)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s, install it at %s", ErrNotInstalled, strings.Join(missing, ", "), a.app.InstallURL())
}
