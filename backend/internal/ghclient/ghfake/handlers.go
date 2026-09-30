package ghfake

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v91/github"
)

// call is one request the state answers. The lock of the fake is held.
type call struct {
	g *GitHub
	w http.ResponseWriter
	r *http.Request
	a Action
}

func (c *call) json(status int, v any) { writeJSON(c.w, status, v) }

func (c *call) fail(status int, message string) { writeError(c.w, status, message) }

func (c *call) decode(v any) bool {
	if err := json.Unmarshal(c.a.Body, v); err != nil {
		c.fail(http.StatusBadRequest, "Problems parsing JSON")
		return false
	}
	return true
}

func (c *call) repoOf() *Repo {
	r, ok := c.g.repos[strings.ToLower(c.a.Vars["owner"]+"/"+c.a.Vars["repo"])]
	if !ok {
		c.fail(http.StatusNotFound, defaultNotFoundError)
		return nil
	}
	return r
}

func (c *call) prOf() (*Repo, *PR) {
	r := c.repoOf()
	if r == nil {
		return nil, nil
	}
	n, err := strconv.Atoi(c.a.Vars["number"])
	p := r.find(n)
	if err != nil || p == nil {
		c.fail(http.StatusNotFound, defaultNotFoundError)
		return nil, nil
	}
	return r, p
}

func (c *call) int64Var(name string) int64 {
	n, _ := strconv.ParseInt(c.a.Vars[name], 10, 64)
	return n
}

func (c *call) user() { c.json(http.StatusOK, c.g.viewer) }

func (c *call) userRepos() {
	out := make([]*github.Repository, 0, len(c.g.order))
	for _, r := range c.g.order {
		out = append(out, repository(r))
	}
	c.json(http.StatusOK, out)
}

func (c *call) repo() {
	if r := c.repoOf(); r != nil {
		c.json(http.StatusOK, repository(r))
	}
}

func (c *call) rules() {
	r := c.repoOf()
	if r == nil {
		return
	}
	out := []map[string]any{}
	if n, ok := r.Approvals[c.a.Vars["branch"]]; ok {
		out = append(out, map[string]any{"type": "pull_request", "parameters": map[string]any{"required_approving_review_count": n}})
	}
	c.json(http.StatusOK, out)
}

func (c *call) protection() {
	r := c.repoOf()
	if r == nil {
		return
	}
	n, ok := r.Protected[c.a.Vars["branch"]]
	if !ok {
		c.fail(http.StatusNotFound, "Branch not protected")
		return
	}
	c.json(http.StatusOK, map[string]any{"required_pull_request_reviews": map[string]any{"required_approving_review_count": n}})
}

func (c *call) latestRelease() {
	rel, ok := c.g.releases[strings.ToLower(c.a.Vars["owner"]+"/"+c.a.Vars["repo"])]
	if !ok {
		c.fail(http.StatusNotFound, defaultNotFoundError)
		return
	}
	c.json(http.StatusOK, rel)
}

func (c *call) pulls() {
	r := c.repoOf()
	if r == nil {
		return
	}
	state := cmpOr(c.a.Query.Get("state"), "open")
	head := c.a.Query.Get("head")
	out := []*github.PullRequest{}
	for _, p := range r.order {
		if state != "all" && p.State != state {
			continue
		}
		if head != "" && head != headLabel(p) {
			continue
		}
		out = append(out, pullRequest(r, p))
	}
	c.json(http.StatusOK, out)
}

func headLabel(p *PR) string {
	owner, _, _ := strings.Cut(p.HeadRepo, "/")
	return owner + ":" + p.HeadRef
}

func (c *call) pull() {
	if r, p := c.prOf(); p != nil {
		c.json(http.StatusOK, pullRequest(r, p))
	}
}

func (c *call) merge() {
	_, p := c.prOf()
	if p == nil {
		return
	}
	var body struct {
		Method  string `json:"merge_method"`
		SHA     string `json:"sha"`
		Title   string `json:"commit_title"`
		Message string `json:"commit_message"`
	}
	if !c.decode(&body) {
		return
	}
	p.Merges = append(p.Merges, Merge{Method: body.Method, SHA: body.SHA, Title: body.Title, Message: body.Message})
	if p.RefuseMerge != 0 {
		c.fail(p.RefuseMerge, "Pull Request is not mergeable")
		return
	}
	if p.State != "open" {
		c.fail(http.StatusMethodNotAllowed, "Pull Request is not mergeable")
		return
	}
	if body.SHA != "" && body.SHA != p.HeadSHA {
		c.fail(http.StatusConflict, "Head branch was modified. Review and try the merge again.")
		return
	}
	p.Merged, p.State, p.MergedAt = true, "closed", time.Now().UTC()
	p.ClosedAt = p.MergedAt
	c.json(http.StatusOK, &github.PullRequestMergeResult{
		SHA: new(fmt.Sprintf("merge-%d", p.Number)), Merged: new(true), Message: new("Pull Request successfully merged"),
	})
}

func (c *call) requestReviews() {
	r, p := c.prOf()
	if p == nil {
		return
	}
	var body struct {
		Reviewers []string `json:"reviewers"`
		Teams     []string `json:"team_reviewers"`
	}
	if !c.decode(&body) {
		return
	}
	p.ReviewRequests = append(p.ReviewRequests, append(slices.Clone(body.Reviewers), body.Teams...))
	for _, login := range body.Reviewers {
		if slices.Contains(p.Refuse, login) {
			c.fail(http.StatusUnprocessableEntity, "Reviews may only be requested from collaborators. One or more of the users or teams you specified is not a collaborator of the "+r.FullName()+" repository.")
			return
		}
	}
	for _, login := range body.Reviewers {
		if !slices.Contains(p.Requested, login) {
			p.Requested = append(p.Requested, login)
		}
	}
	for _, slug := range body.Teams {
		if !slices.Contains(p.RequestedTeams, slug) {
			p.RequestedTeams = append(p.RequestedTeams, slug)
		}
	}
	c.json(http.StatusCreated, pullRequest(r, p))
}

func (c *call) reviews() {
	_, p := c.prOf()
	if p == nil {
		return
	}
	out := make([]wireReview, 0, len(p.Reviews))
	for _, rv := range p.Reviews {
		out = append(out, wireReview{&github.PullRequestReview{
			ID: new(rv.ID), State: new(rv.State), CommitID: str(rv.CommitID), User: user(rv.Author), Body: new(rv.Body),
			HTMLURL: str(rv.URL), SubmittedAt: stamp(rv.SubmittedAt),
		}, rv.Association})
	}
	c.json(http.StatusOK, out)
}

var reviewStates = map[string]string{"APPROVE": "APPROVED", "REQUEST_CHANGES": "CHANGES_REQUESTED", "COMMENT": "COMMENTED"}

func (c *call) submitReview() {
	_, p := c.prOf()
	if p == nil {
		return
	}
	var body struct {
		Event    string `json:"event"`
		Body     string `json:"body"`
		CommitID string `json:"commit_id"`
	}
	if !c.decode(&body) {
		return
	}
	state, ok := reviewStates[body.Event]
	if !ok {
		c.fail(http.StatusUnprocessableEntity, "Unprocessable Entity")
		return
	}
	commit := body.CommitID
	if commit == "" {
		commit = p.HeadSHA
	}
	id := c.g.id()
	rv := Review{
		ID: id, State: state, CommitID: commit, Author: c.g.viewer.GetLogin(), Body: body.Body,
		URL: fmt.Sprintf("%s#pullrequestreview-%d", p.URL, id), SubmittedAt: time.Now().UTC(),
	}
	p.Reviews = append(p.Reviews, rv)
	c.json(http.StatusOK, &github.PullRequestReview{
		ID: new(rv.ID), State: new(rv.State), CommitID: str(rv.CommitID), User: user(rv.Author), Body: new(rv.Body),
		HTMLURL: str(rv.URL), SubmittedAt: stamp(rv.SubmittedAt),
	})
}

func (c *call) reviewComments() {
	r, p := c.prOf()
	if p == nil {
		return
	}
	out := make([]wireReviewComment, 0, len(p.ReviewComments))
	for _, rc := range p.ReviewComments {
		out = append(out, reviewComment(r, p, rc))
	}
	c.json(http.StatusOK, out)
}

func (c *call) reviewComment() {
	r := c.repoOf()
	if r == nil {
		return
	}
	id := c.int64Var("id")
	if !r.deleted(id) {
		for _, p := range r.order {
			for _, rc := range p.ReviewComments {
				if rc.ID == id {
					c.json(http.StatusOK, reviewComment(r, p, rc))
					return
				}
			}
		}
	}
	c.fail(http.StatusNotFound, defaultNotFoundError)
}

func (c *call) reply() {
	r, p := c.prOf()
	if p == nil {
		return
	}
	var body struct {
		Body      string `json:"body"`
		InReplyTo int64  `json:"in_reply_to"`
	}
	if !c.decode(&body) {
		return
	}
	if r.deleted(body.InReplyTo) {
		c.fail(http.StatusNotFound, defaultNotFoundError)
		return
	}
	id := c.g.id()
	rc := ReviewComment{
		ID: id, Author: c.g.viewer.GetLogin(), Body: body.Body, CreatedAt: time.Now().UTC(),
		URL:       fmt.Sprintf("%s#discussion_r%d", p.URL, id),
		InReplyTo: body.InReplyTo,
	}
	p.ReviewComments = append(p.ReviewComments, rc)
	c.json(http.StatusCreated, reviewComment(r, p, rc))
}

func (c *call) issueComments() {
	r, p := c.prOf()
	if p == nil {
		return
	}
	out := make([]wireIssueComment, 0, len(p.IssueComments))
	for _, ic := range p.IssueComments {
		out = append(out, issueComment(r, p, ic))
	}
	c.json(http.StatusOK, out)
}

func (c *call) issueComment() {
	r := c.repoOf()
	if r == nil {
		return
	}
	id := c.int64Var("id")
	if !r.deleted(id) {
		for _, p := range r.order {
			for _, ic := range p.IssueComments {
				if ic.ID == id {
					c.json(http.StatusOK, issueComment(r, p, ic))
					return
				}
			}
		}
	}
	c.fail(http.StatusNotFound, defaultNotFoundError)
}

func (c *call) comment() {
	r, p := c.prOf()
	if p == nil {
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if !c.decode(&body) {
		return
	}
	id := c.g.id()
	ic := Comment{
		ID: id, Author: c.g.viewer.GetLogin(), Body: body.Body, CreatedAt: time.Now().UTC(),
		URL: fmt.Sprintf("%s#issuecomment-%d", p.URL, id),
	}
	p.IssueComments = append(p.IssueComments, ic)
	c.json(http.StatusCreated, issueComment(r, p, ic))
}

// headOf returns the pull request whose head is sha, or nil.
func headOf(r *Repo, sha string) *PR {
	for _, p := range r.order {
		if p.HeadSHA == sha {
			return p
		}
	}
	return nil
}

func (c *call) checkRuns() {
	r := c.repoOf()
	if r == nil {
		return
	}
	runs := []*github.CheckRun{}
	if p := headOf(r, c.a.Vars["sha"]); p != nil {
		for _, cr := range p.CheckRuns {
			run := &github.CheckRun{
				ID: new(cr.ID), Name: new(cr.Name), Status: new(cr.Status),
				Conclusion: str(cr.Conclusion), HTMLURL: str(cr.URL), HeadSHA: new(p.HeadSHA),
			}
			if cr.CheckSuiteID != 0 {
				run.CheckSuite = &github.CheckSuite{ID: new(cr.CheckSuiteID)}
			}
			runs = append(runs, run)
		}
	}
	c.json(http.StatusOK, &github.ListCheckRunsResults{Total: new(len(runs)), CheckRuns: runs})
}

func (c *call) status() {
	r := c.repoOf()
	if r == nil {
		return
	}
	out := &github.CombinedStatus{State: new("pending"), SHA: new(c.a.Vars["sha"]), Statuses: []*github.RepoStatus{}}
	if p := headOf(r, c.a.Vars["sha"]); p != nil {
		out.State = new(p.StatusState)
		for _, s := range p.Statuses {
			out.Statuses = append(out.Statuses, &github.RepoStatus{Context: new(s.Context), State: new(s.State), TargetURL: str(s.URL)})
		}
	}
	out.TotalCount = new(len(out.Statuses))
	c.json(http.StatusOK, out)
}

func (c *call) workflowRuns() {
	r := c.repoOf()
	if r == nil {
		return
	}
	sha := c.a.Query.Get("head_sha")
	out := []*github.WorkflowRun{}
	for _, run := range r.Runs {
		head := cmpOr(run.HeadSHA, sha)
		if sha != "" && head != sha {
			continue
		}
		out = append(out, &github.WorkflowRun{
			ID: new(run.ID), Name: str(run.Name), WorkflowID: nonZero(run.WorkflowID), HeadSHA: str(head),
			Status: str(run.Status), Conclusion: str(run.Conclusion), HTMLURL: str(run.URL), CheckSuiteID: nonZero(run.CheckSuiteID),
		})
	}
	c.json(http.StatusOK, &github.WorkflowRuns{TotalCount: new(len(out)), WorkflowRuns: out})
}

func (r *Repo) run(id int64) *Run {
	for _, run := range r.Runs {
		if run.ID == id {
			return run
		}
	}
	return nil
}

func (r *Repo) job(id int64) *Job {
	for _, run := range r.Runs {
		for _, j := range run.Jobs {
			if j.ID == id {
				return j
			}
		}
	}
	return nil
}

func (c *call) jobs() {
	r := c.repoOf()
	if r == nil {
		return
	}
	run := r.run(c.int64Var("run"))
	if run == nil {
		c.fail(http.StatusNotFound, defaultNotFoundError)
		return
	}
	if run.RefuseJobs {
		c.fail(http.StatusBadRequest, fmt.Sprintf("the test does not expect a read of the jobs of run %d", run.ID))
		return
	}
	out := make([]*github.WorkflowJob, 0, len(run.Jobs))
	for _, j := range run.Jobs {
		out = append(out, &github.WorkflowJob{
			ID: new(j.ID), RunID: new(run.ID), Name: new(j.Name),
			Status: str(j.Status), Conclusion: str(j.Conclusion), HTMLURL: str(j.URL),
		})
	}
	c.json(http.StatusOK, &github.Jobs{TotalCount: new(len(out)), Jobs: out})
}

func (c *call) jobLogs() {
	r := c.repoOf()
	if r == nil {
		return
	}
	if r.job(c.int64Var("job")) == nil {
		c.fail(http.StatusNotFound, defaultNotFoundError)
		return
	}
	c.w.Header().Set("Location", fmt.Sprintf("%s/_logs/%s/%s", c.g.base, r.FullName(), c.a.Vars["job"]))
	c.w.WriteHeader(http.StatusFound)
}

func (c *call) jobLogDownload() {
	r, ok := c.g.repos[strings.ToLower(c.a.Vars["owner"]+"/"+c.a.Vars["repo"])]
	var j *Job
	if ok {
		j = r.job(c.int64Var("job"))
	}
	if j == nil {
		http.Error(c.w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	if j.LogStatus != 0 {
		http.Error(c.w, http.StatusText(j.LogStatus), j.LogStatus)
		return
	}
	c.w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(c.w, j.Log)
}

func repository(r *Repo) *github.Repository {
	return &github.Repository{
		Name: new(r.Name), FullName: new(r.FullName()), Owner: user(r.Owner),
		Permissions:      &github.RepositoryPermissions{Push: new(r.Push), Pull: new(true)},
		AllowSquashMerge: new(slices.Contains(r.MergeMethods, "squash")),
		AllowMergeCommit: new(slices.Contains(r.MergeMethods, "merge")),
		AllowRebaseMerge: new(slices.Contains(r.MergeMethods, "rebase")),
		HTMLURL:          new("https://github.com/" + r.FullName()),
		Visibility:       str(r.Visibility),
		StargazersCount:  nonZeroInt(r.Stars),
		UpdatedAt:        stamp(r.UpdatedAt),
	}
}

func pullRequest(r *Repo, p *PR) *github.PullRequest {
	headOwner, headName, _ := strings.Cut(p.HeadRepo, "/")
	out := &github.PullRequest{
		ID: new(p.ID), NodeID: new(nodeID(r, p)), Number: new(p.Number), Title: new(p.Title), State: new(p.State),
		Merged: new(p.Merged), MergedAt: stamp(p.MergedAt), ClosedAt: stamp(p.ClosedAt), Draft: new(p.Draft),
		Mergeable: p.Mergeable, MergeableState: str(p.MergeableState), User: prAuthor(p), HTMLURL: new(p.URL),
		Head: &github.PullRequestBranch{
			Ref: new(p.HeadRef), SHA: new(p.HeadSHA), Label: new(headLabel(p)),
			Repo: &github.Repository{FullName: new(p.HeadRepo), Name: new(headName), Owner: user(headOwner)},
		},
		Base: &github.PullRequestBranch{
			Ref: new(p.BaseRef), SHA: new(p.BaseSHA),
			Repo: &github.Repository{FullName: new(r.FullName()), Name: new(r.Name), Owner: user(r.Owner)},
		},
		Additions: nonZeroInt(p.Additions), Deletions: nonZeroInt(p.Deletions),
		CreatedAt: stamp(p.CreatedAt), UpdatedAt: stamp(p.UpdatedAt),
		RequestedReviewers: []*github.User{},
	}
	for _, login := range p.Requested {
		out.RequestedReviewers = append(out.RequestedReviewers, user(login))
	}
	for _, name := range p.Labels {
		out.Labels = append(out.Labels, &github.Label{Name: name})
	}
	for _, login := range p.Assignees {
		out.Assignees = append(out.Assignees, user(login))
	}
	out.Body = str(p.Body)
	return out
}

// The wire types carry author_association, which go-github marks
// deprecated on its structs although the REST API still sends it.
type wireReview struct {
	*github.PullRequestReview
	AuthorAssociation string `json:"author_association,omitempty"`
}

type wireReviewComment struct {
	*github.PullRequestComment
	AuthorAssociation string `json:"author_association,omitempty"`
}

type wireIssueComment struct {
	*github.IssueComment
	AuthorAssociation string `json:"author_association,omitempty"`
}

func reviewComment(r *Repo, p *PR, rc ReviewComment) wireReviewComment {
	return wireReviewComment{&github.PullRequestComment{
		ID: new(rc.ID), PullRequestReviewID: nonZero(rc.ReviewID), InReplyTo: nonZero(rc.InReplyTo),
		User: user(rc.Author), Body: new(rc.Body), CreatedAt: stamp(rc.CreatedAt),
		Path: str(rc.Path), Line: nonZeroInt(rc.Line), OriginalLine: nonZeroInt(rc.OriginalLine),
		Side: str(rc.Side), CommitID: str(rc.CommitID), OriginalCommitID: str(rc.OriginalCommitID),
		HTMLURL:        str(rc.URL),
		PullRequestURL: new(fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d", r.FullName(), p.Number)),
	}, rc.Association}
}

func issueComment(r *Repo, p *PR, ic Comment) wireIssueComment {
	return wireIssueComment{&github.IssueComment{
		ID: new(ic.ID), User: user(ic.Author), Body: new(ic.Body), CreatedAt: stamp(ic.CreatedAt),
		HTMLURL:  str(ic.URL),
		IssueURL: new(fmt.Sprintf("https://api.github.com/repos/%s/issues/%d", r.FullName(), p.Number)),
	}, ic.Association}
}

func user(login string) *github.User {
	if login == "" {
		return nil
	}
	return &github.User{Login: new(login), AvatarURL: new(AvatarURL(login))}
}

func prAuthor(p *PR) *github.User {
	u := user(p.Author)
	if u != nil && p.AuthorAvatar != "" {
		u.AvatarURL = new(p.AuthorAvatar)
	}
	return u
}

func AvatarURL(login string) string {
	return "https://avatars.githubusercontent.com/" + url.PathEscape(login)
}

func str(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nonZero(n int64) *int64 {
	if n == 0 {
		return nil
	}
	return &n
}

func nonZeroInt(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func stamp(t time.Time) *github.Timestamp {
	if t.IsZero() {
		return nil
	}
	return &github.Timestamp{Time: t}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
