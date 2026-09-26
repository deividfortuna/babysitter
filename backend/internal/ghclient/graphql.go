package ghclient

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-github/v91/github"
)

const reviewStateQuery = `query($owner: String!, $name: String!, $number: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewRequests(first: 100) {
        nodes {
          requestedReviewer {
            __typename
            ... on User { login }
            ... on Team { slug }
            ... on Bot { login }
            ... on Mannequin { login }
          }
        }
      }
      reviewThreads(first: 100, after: $after) {
        nodes {
          isResolved
          comments(first: 100) { nodes { author { __typename login } } }
          lastComment: comments(last: 1) { nodes { databaseId author { __typename login } } }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type reviewStateResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewRequests struct {
					Nodes []struct {
						RequestedReviewer struct {
							Login string `json:"login"`
							Slug  string `json:"slug"`
						} `json:"requestedReviewer"`
					} `json:"nodes"`
				} `json:"reviewRequests"`
				ReviewThreads struct {
					Nodes []struct {
						IsResolved  bool           `json:"isResolved"`
						Comments    threadComments `json:"comments"`
						LastComment threadComments `json:"lastComment"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type threadComment struct {
	DatabaseID int64         `json:"databaseId"`
	Author     graphqlAuthor `json:"author"`
}

type threadComments struct {
	Nodes []threadComment `json:"nodes"`
}

func (c threadComments) authors() []string {
	var out []string
	for _, n := range c.Nodes {
		if login := n.Author.restLogin(); login != "" {
			out = append(out, login)
		}
	}
	return out
}

func (c threadComments) last() threadComment {
	if len(c.Nodes) == 0 {
		return threadComment{}
	}
	return c.Nodes[len(c.Nodes)-1]
}

type graphqlAuthor struct {
	Typename string `json:"__typename"`
	Login    string `json:"login"`
}

func (a graphqlAuthor) restLogin() string {
	if a.Typename == "Bot" {
		return a.Login + "[bot]"
	}
	return a.Login
}

type ReviewThread struct {
	Resolved      bool
	Authors       []string
	LastAuthor    string
	LastCommentID int64
}

type ReviewState struct {
	Threads   []ReviewThread
	Requested []string
}

func (s ReviewState) Unresolved(ignore string) int {
	n := 0
	for _, t := range s.Threads {
		if t.open(ignore) {
			n++
		}
	}
	return n
}

func (s ReviewState) Unanswered(ignore, self string) int {
	n := 0
	for _, t := range s.Threads {
		if t.awaitsAnswer(ignore, self) {
			n++
		}
	}
	return n
}

func (s ReviewState) AnsweredReviewers(self string) []string {
	var out []string
	for _, t := range s.Threads {
		if !t.awaitsReviewer(self) {
			continue
		}
		for _, login := range t.Authors {
			if !strings.EqualFold(login, self) {
				out = append(out, login)
			}
		}
	}
	return out
}

func (s ReviewState) LastAnswerID(self string) int64 {
	var newest int64
	for _, t := range s.Threads {
		if t.awaitsReviewer(self) {
			newest = max(newest, t.LastCommentID)
		}
	}
	return newest
}

func (t ReviewThread) open(ignore string) bool {
	return !t.Resolved && !t.writtenOnlyBy(ignore)
}

func (t ReviewThread) awaitsAnswer(ignore, self string) bool {
	return t.open(ignore) && !t.answeredBy(self)
}

func (t ReviewThread) awaitsReviewer(self string) bool {
	return !t.Resolved && t.answeredBy(self)
}

func (t ReviewThread) answeredBy(login string) bool {
	wroteLast := login != "" && strings.EqualFold(t.LastAuthor, login)
	return wroteLast && !t.writtenOnlyBy(login)
}

func (t ReviewThread) writtenOnlyBy(login string) bool {
	if login == "" || len(t.Authors) == 0 {
		return false
	}
	for _, author := range t.Authors {
		if !strings.EqualFold(author, login) {
			return false
		}
	}
	return true
}

func FetchReviewState(ctx context.Context, c *github.Client, owner, repo string, number int) (ReviewState, *github.Response, error) {
	var (
		out   ReviewState
		after *string
		last  *github.Response
	)
	for {
		body := graphqlRequest{Query: reviewStateQuery, Variables: map[string]any{
			"owner": owner, "name": repo, "number": number, "after": after,
		}}
		req, err := c.NewRequest(ctx, "POST", "graphql", body)
		if err != nil {
			return ReviewState{}, nil, fmt.Errorf("review threads %s/%s#%d: %w", owner, repo, number, err)
		}
		var page reviewStateResponse
		resp, err := c.Do(req, &page)
		last = resp
		if err != nil {
			return ReviewState{}, resp, fmt.Errorf("review threads %s/%s#%d: %w", owner, repo, number, err)
		}
		if len(page.Errors) > 0 {
			return ReviewState{}, resp, fmt.Errorf("review threads %s/%s#%d: %w", owner, repo, number, graphqlError(page))
		}
		pr := page.Data.Repository.PullRequest
		if after == nil {
			for _, n := range pr.ReviewRequests.Nodes {
				if who := cmp.Or(n.RequestedReviewer.Slug, n.RequestedReviewer.Login); who != "" {
					out.Requested = append(out.Requested, who)
				}
			}
		}
		for _, n := range pr.ReviewThreads.Nodes {
			lastComment := n.LastComment.last()
			out.Threads = append(out.Threads, ReviewThread{
				Resolved:      n.IsResolved,
				Authors:       n.Comments.authors(),
				LastAuthor:    lastComment.Author.restLogin(),
				LastCommentID: lastComment.DatabaseID,
			})
		}
		if !pr.ReviewThreads.PageInfo.HasNextPage {
			return out, last, nil
		}
		cursor := pr.ReviewThreads.PageInfo.EndCursor
		after = &cursor
	}
}

func graphqlError(out reviewStateResponse) error {
	msgs := make([]string, 0, len(out.Errors))
	for _, e := range out.Errors {
		msgs = append(msgs, e.Message)
	}
	return errors.New(strings.Join(msgs, "; "))
}
