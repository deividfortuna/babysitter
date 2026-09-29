package ghfake

import (
	"fmt"
	"net/http"
	"strings"
)

type graphqlVariables struct {
	Owner  string `json:"owner"`
	Name   string `json:"name"`
	Number int    `json:"number"`
	ID     string `json:"id"`
	Head   string `json:"head"`
	Method string `json:"method"`
}

func (c *call) graphql() {
	var req struct {
		Query     string           `json:"query"`
		Variables graphqlVariables `json:"variables"`
	}
	if !c.decode(&req) {
		return
	}
	if strings.Contains(req.Query, "updatePullRequestBranch") {
		c.updateBranch(req.Variables)
		return
	}
	c.reviewState(req.Variables)
}

func (c *call) pullOfQuery(v graphqlVariables) *PR {
	r, ok := c.g.repos[strings.ToLower(v.Owner+"/"+v.Name)]
	var p *PR
	if ok {
		p = r.find(v.Number)
	}
	if p == nil {
		c.graphqlError("NOT_FOUND", "Could not resolve to a PullRequest.")
	}
	return p
}

func (c *call) graphqlError(kind, message string) {
	c.json(http.StatusOK, map[string]any{
		"data":   nil,
		"errors": []map[string]string{{"type": kind, "message": message}},
	})
}

func nodeID(r *Repo, p *PR) string {
	return fmt.Sprintf("PR_%s#%d", r.FullName(), p.Number)
}

func (c *call) updateBranch(v graphqlVariables) {
	p := c.g.pullOfNode(v.ID)
	if p == nil {
		c.graphqlError("NOT_FOUND", "Could not resolve to a node with the global id of '"+v.ID+"'")
		return
	}
	p.BranchUpdates = append(p.BranchUpdates, BranchUpdate{Method: v.Method, ExpectedHead: v.Head})
	switch {
	case p.RefuseBranchUpdate != "":
		c.graphqlError("UNPROCESSABLE", p.RefuseBranchUpdate)
		return
	case v.Head != p.HeadSHA:
		c.graphqlError("UNPROCESSABLE", "Expected head oid "+v.Head+" does not match the head of the pull request")
		return
	}
	accepted := p.HeadSHA
	p.HeadSHA = fmt.Sprintf("%s-%d", strings.ToLower(v.Method), c.g.id())
	p.MergeableState = "unknown"
	c.json(http.StatusOK, map[string]any{"data": map[string]any{"updatePullRequestBranch": map[string]any{
		"pullRequest": map[string]any{"headRefOid": accepted},
	}}})
}

func (g *GitHub) pullOfNode(id string) *PR {
	for _, r := range g.order {
		for _, p := range r.order {
			if nodeID(r, p) == id {
				return p
			}
		}
	}
	return nil
}

func (c *call) reviewState(v graphqlVariables) {
	p := c.pullOfQuery(v)
	if p == nil {
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
