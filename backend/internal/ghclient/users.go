package ghclient

import (
	"context"
	"fmt"

	"github.com/google/go-github/v91/github"
)

func CurrentUser(ctx context.Context, c *github.Client) (*github.User, error) {
	u, _, err := c.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}
	return u, nil
}
