package ghclient

import (
	"context"
	"fmt"

	"github.com/google/go-github/v91/github"
)

const maxPerPage = 100

type ListReposOptions struct {
	Limit int
}

func ListRepos(ctx context.Context, c *github.Client, o ListReposOptions) ([]*github.Repository, error) {
	perPage := maxPerPage
	if o.Limit > 0 && o.Limit < perPage {
		perPage = o.Limit
	}
	opts := &github.RepositoryListByAuthenticatedUserOptions{
		Sort:    "updated",
		PerPage: perPage,
	}
	var all []*github.Repository
	for {
		repos, resp, err := c.Repositories.ListByAuthenticatedUser(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list repositories: %w", err)
		}
		all = append(all, repos...)
		if o.Limit > 0 && len(all) >= o.Limit {
			return all[:o.Limit], nil
		}
		if resp.NextPage == 0 {
			return all, nil
		}
		opts.Page = resp.NextPage
	}
}
