package cli

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/store"
)

type watchedRepoOutput struct {
	FullName     string     `json:"full_name"`
	AddedAt      time.Time  `json:"added_at"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	LastError    string     `json:"last_error,omitempty"`
}

type watchedRepoList []watchedRepoOutput

func (l watchedRepoList) writeText(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REPOSITORY\tADDED\tLAST SYNC\tERROR")
	for _, r := range l {
		synced := "never"
		if r.LastSyncedAt != nil {
			synced = r.LastSyncedAt.Local().Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.FullName, r.AddedAt.Local().Format("2006-01-02"), synced, r.LastError)
	}
	return tw.Flush()
}

func newRepoCmd(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage the watched repositories",
	}
	cmd.AddCommand(newRepoAddCmd(opts), newRepoRemoveCmd(opts), newRepoListCmd(opts))
	return cmd
}

func newRepoAddCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "add <owner/name>",
		Short: "Start watching a repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			owner, name, err := store.ParseFullName(args[0])
			if err != nil {
				return err
			}
			client, err := opts.client(ctx)
			if err != nil {
				return err
			}
			gr, err := ghclient.GetRepo(ctx, client, owner, name)
			if err != nil {
				return err
			}
			owner, name = gr.GetOwner().GetLogin(), gr.GetName()

			st, err := opts.openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			r, err := st.AddRepo(ctx, owner, name)
			if errors.Is(err, store.ErrRepoExists) {
				fmt.Fprintf(cmd.OutOrStdout(), "Already watching %s/%s\n", owner, name)
				return nil
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Watching %s\n", r.FullName())
			return nil
		},
	}
}

func newRepoRemoveCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <owner/name>",
		Aliases: []string{"rm"},
		Short:   "Stop watching a repository and forget its pull requests",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			owner, name, err := store.ParseFullName(args[0])
			if err != nil {
				return err
			}
			st, err := opts.openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			if err := st.RemoveRepo(cmd.Context(), owner, name); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Stopped watching %s/%s\n", owner, name)
			return nil
		},
	}
}

func newRepoListCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the watched repositories",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := opts.openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			repos, err := st.ListRepos(cmd.Context())
			if err != nil {
				return err
			}
			items := make(watchedRepoList, 0, len(repos))
			for _, r := range repos {
				items = append(items, watchedRepoOutput{
					FullName:     r.FullName(),
					AddedAt:      r.AddedAt,
					LastSyncedAt: r.LastSyncedAt,
					LastError:    r.LastError,
				})
			}
			return opts.print(cmd.OutOrStdout(), items)
		},
	}
}
