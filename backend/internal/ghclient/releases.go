package ghclient

import (
	"context"
	"fmt"

	"github.com/google/go-github/v91/github"
)

type Release struct {
	Tag string
	URL string
}

func NewAnonymous() (*github.Client, error) {
	c, err := github.NewClient()
	if err != nil {
		return nil, fmt.Errorf("new github client: %w", err)
	}
	return c, nil
}

func LatestRelease(ctx context.Context, c *github.Client, owner, repo string) (Release, error) {
	r, _, err := c.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return Release{}, fmt.Errorf("get latest release of %s/%s: %w", owner, repo, err)
	}
	return Release{Tag: r.GetTagName(), URL: r.GetHTMLURL()}, nil
}
