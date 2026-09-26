package ghfake

import (
	"net/http"
	"strings"
)

// graphql answers the review state query of ghclient: the review requests
// and the review threads of one pull request, on one page.
func (c *call) graphql() {
	var req struct {
		Variables struct {
			Owner  string `json:"owner"`
			Name   string `json:"name"`
			Number int    `json:"number"`
		} `json:"variables"`
	}
	if !c.decode(&req) {
		return
	}
	v := req.Variables
	r, ok := c.g.repos[strings.ToLower(v.Owner+"/"+v.Name)]
	var p *PR
	if ok {
		p = r.find(v.Number)
	}
	if p == nil {
		c.json(http.StatusOK, map[string]any{
			"data":   map[string]any{"repository": nil},
			"errors": []map[string]string{{"type": "NOT_FOUND", "message": "Could not resolve to a PullRequest."}},
		})
		return
	}

	requests := []any{}
	for _, login := range p.Requested {
		requests = append(requests, map[string]any{"requestedReviewer": map[string]any{"__typename": "User", "login": login}})
	}
	for _, slug := range p.RequestedTeams {
		requests = append(requests, map[string]any{"requestedReviewer": map[string]any{"__typename": "Team", "slug": slug}})
	}
	threads := []any{}
	for _, t := range p.Threads {
		comments := []any{}
		for _, login := range t.Authors {
			comments = append(comments, map[string]any{"author": author(login)})
		}
		node := map[string]any{"isResolved": t.Resolved, "comments": map[string]any{"nodes": comments}}
		if len(t.Authors) > 0 {
			last := map[string]any{"databaseId": t.LastCommentID, "author": author(t.Authors[len(t.Authors)-1])}
			node["lastComment"] = map[string]any{"nodes": []any{last}}
		}
		threads = append(threads, node)
	}
	c.json(http.StatusOK, map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{
		"reviewRequests": map[string]any{"nodes": requests},
		"reviewThreads": map[string]any{
			"nodes":    threads,
			"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
		},
	}}}})
}

// author is a GraphQL actor. A login that ends in [bot] is a Bot, which
// GraphQL names without the suffix.
func author(login string) map[string]any {
	if bot, ok := strings.CutSuffix(login, "[bot]"); ok {
		return map[string]any{"__typename": "Bot", "login": bot}
	}
	return map[string]any{"__typename": "User", "login": login}
}
