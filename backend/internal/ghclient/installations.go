package ghclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v91/github"
)

type Installation struct {
	ID           int64
	Account      string
	AvatarURL    string
	Organization bool
	AllRepo      bool
}

func UserInstallations(ctx context.Context, c *github.Client) ([]Installation, error) {
	var out []Installation
	for inst, err := range c.Apps.ListUserInstallationsIter(ctx, &github.ListOptions{PerPage: maxPerPage}) {
		if err != nil {
			return nil, fmt.Errorf("list the installations of the GitHub App: %w", err)
		}
		out = append(out, Installation{
			ID:           inst.GetID(),
			Account:      inst.GetAccount().GetLogin(),
			AvatarURL:    inst.GetAccount().GetAvatarURL(),
			Organization: inst.GetAccount().GetType() == "Organization",
			AllRepo:      inst.GetRepositorySelection() == "all",
		})
	}
	return out, nil
}

func AppInstalled(ctx context.Context, c *github.Client, owner, name string) (bool, error) {
	installs, err := UserInstallations(ctx, c)
	if err != nil {
		return false, err
	}
	for _, inst := range installs {
		if !strings.EqualFold(inst.Account, owner) {
			continue
		}
		if inst.AllRepo {
			return true, nil
		}
		return installationHas(ctx, c, inst.ID, name)
	}
	return false, nil
}

func installationHas(ctx context.Context, c *github.Client, id int64, name string) (bool, error) {
	for repo, err := range c.Apps.ListUserReposIter(ctx, id, &github.ListOptions{PerPage: maxPerPage}) {
		if err != nil {
			return false, fmt.Errorf("list the repositories of the GitHub App: %w", err)
		}
		if strings.EqualFold(repo.GetName(), name) {
			return true, nil
		}
	}
	return false, nil
}
