package cli

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/store"
)

type prOutput struct {
	Repository         string               `json:"repository"`
	Number             int                  `json:"number"`
	Title              string               `json:"title"`
	Author             string               `json:"author"`
	State              store.PRState        `json:"state"`
	Draft              bool                 `json:"draft"`
	BaseRef            string               `json:"base_ref"`
	HeadRef            string               `json:"head_ref"`
	HeadSHA            string               `json:"head_sha"`
	URL                string               `json:"url"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
	MergedAt           *time.Time           `json:"merged_at"`
	ClosedAt           *time.Time           `json:"closed_at"`
	MergeableState     store.MergeableState `json:"mergeable_state"`
	ReviewDecision     store.ReviewDecision `json:"review_decision"`
	Approvals          int                  `json:"approvals"`
	ChangesRequested   int                  `json:"changes_requested"`
	RequestedReviewers []string             `json:"requested_reviewers"`
	Labels             []string             `json:"labels"`
	Additions          int                  `json:"additions"`
	Deletions          int                  `json:"deletions"`
	CIStatus           checks.CIStatus      `json:"ci_status"`
	SyncedAt           time.Time            `json:"synced_at"`
}

type prList []prOutput

func (l prList) writeText(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REPOSITORY\t#\tTITLE\tAUTHOR\tSTATE\tREVIEW\tCI\tMERGEABLE\t+/-\tUPDATED")
	for _, p := range l {
		state := string(p.State)
		if p.Draft && p.State == store.StateOpen {
			state = "draft"
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t+%d/-%d\t%s\n",
			p.Repository, p.Number, truncate(p.Title, 50), p.Author, state,
			p.ReviewDecision, p.CIStatus, p.MergeableState,
			p.Additions, p.Deletions, p.UpdatedAt.Local().Format("2006-01-02 15:04"))
	}
	return tw.Flush()
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n-1]) + "…"
}

func toPROutput(pr store.PullRequest) prOutput {
	return prOutput{
		Repository:         pr.RepoFullName,
		Number:             pr.Number,
		Title:              pr.Title,
		Author:             pr.Author,
		State:              pr.State,
		Draft:              pr.Draft,
		BaseRef:            pr.BaseRef,
		HeadRef:            pr.HeadRef,
		HeadSHA:            pr.HeadSHA,
		URL:                pr.HTMLURL,
		CreatedAt:          pr.CreatedAt,
		UpdatedAt:          pr.UpdatedAt,
		MergedAt:           pr.MergedAt,
		ClosedAt:           pr.ClosedAt,
		MergeableState:     pr.MergeableState,
		ReviewDecision:     pr.ReviewDecision,
		Approvals:          pr.Approvals,
		ChangesRequested:   pr.ChangesRequested,
		RequestedReviewers: pr.RequestedReviewers,
		Labels:             pr.Labels,
		Additions:          pr.Additions,
		Deletions:          pr.Deletions,
		CIStatus:           pr.CIStatus,
		SyncedAt:           pr.SyncedAt,
	}
}

func newPRsCmd(opts *options) *cobra.Command {
	var (
		repo  string
		state string
	)
	cmd := &cobra.Command{
		Use:   "prs",
		Short: "List the pull requests in the local store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if state != string(store.FilterOpen) && state != string(store.FilterAll) {
				return fmt.Errorf("unknown state %q, want open or all", state)
			}
			var q store.ListPRsOptions
			q.State = store.PRStateFilter(state)
			if repo != "" {
				var err error
				if q.Owner, q.Name, err = store.ParseFullName(repo); err != nil {
					return err
				}
			}
			st, err := opts.openStore()
			if err != nil {
				return err
			}
			defer st.Close()

			prs, err := st.ListPRs(cmd.Context(), q)
			if err != nil {
				return err
			}
			items := make(prList, 0, len(prs))
			for _, pr := range prs {
				items = append(items, toPROutput(pr))
			}
			return opts.print(cmd.OutOrStdout(), items)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "only pull requests of this repository (owner/name)")
	cmd.Flags().StringVar(&state, "state", string(store.FilterOpen), "pull request state: open or all")
	return cmd
}
