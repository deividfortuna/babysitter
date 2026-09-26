package cli

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/ghclient"
)

type repoOutput struct {
	FullName   string    `json:"full_name"`
	Visibility string    `json:"visibility"`
	Stars      int       `json:"stars"`
	UpdatedAt  time.Time `json:"updated_at"`
	URL        string    `json:"url"`
}

type repoList []repoOutput

func (l repoList) writeText(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REPOSITORY\tVISIBILITY\tSTARS\tUPDATED")
	for _, r := range l {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n",
			r.FullName, r.Visibility, r.Stars, r.UpdatedAt.Format("2006-01-02"))
	}
	return tw.Flush()
}

func newReposCmd(opts *options) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "repos",
		Short: "List repositories of the authenticated user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client, err := opts.client(ctx)
			if err != nil {
				return err
			}
			repos, err := ghclient.ListRepos(ctx, client, ghclient.ListReposOptions{Limit: limit})
			if err != nil {
				return err
			}

			items := make(repoList, 0, len(repos))
			for _, r := range repos {
				items = append(items, repoOutput{
					FullName:   r.GetFullName(),
					Visibility: r.GetVisibility(),
					Stars:      r.GetStargazersCount(),
					UpdatedAt:  r.GetUpdatedAt().Time,
					URL:        r.GetHTMLURL(),
				})
			}
			return opts.print(cmd.OutOrStdout(), items)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 20, "maximum number of repositories to print (0 for all)")
	return cmd
}
