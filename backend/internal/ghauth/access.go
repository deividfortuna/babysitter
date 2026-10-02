package ghauth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient"
)

var ErrNotInstalled = errors.New("the GitHub App is not installed on the repository")

func (a *Auth) CheckRepo(ctx context.Context, client *github.Client, owner, name string) error {
	token, err := a.appToken(ctx)
	if err != nil {
		return err
	}
	if token == "" {
		return nil
	}
	installed, err := ghclient.AppInstalled(ctx, client, owner, name)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("%w: %s/%s, install it at %s", ErrNotInstalled, owner, name, a.app.InstallURL())
	}
	return nil
}
