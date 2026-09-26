package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/ghclient"
)

type userOutput struct {
	Login       string `json:"login"`
	Name        string `json:"name"`
	PublicRepos int    `json:"public_repos"`
	Followers   int    `json:"followers"`
	Following   int    `json:"following"`
}

func (u userOutput) writeText(w io.Writer) error {
	fmt.Fprintf(w, "Login:        %s\n", u.Login)
	fmt.Fprintf(w, "Name:         %s\n", u.Name)
	fmt.Fprintf(w, "Public repos: %d\n", u.PublicRepos)
	fmt.Fprintf(w, "Followers:    %d\n", u.Followers)
	_, err := fmt.Fprintf(w, "Following:    %d\n", u.Following)
	return err
}

func newWhoamiCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print the authenticated user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client, err := opts.client(ctx)
			if err != nil {
				return err
			}
			user, err := ghclient.CurrentUser(ctx, client)
			if err != nil {
				return err
			}
			return opts.print(cmd.OutOrStdout(), userOutput{
				Login:       user.GetLogin(),
				Name:        user.GetName(),
				PublicRepos: user.GetPublicRepos(),
				Followers:   user.GetFollowers(),
				Following:   user.GetFollowing(),
			})
		},
	}
}
