package ghfake

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Repo is a repository of the fake. Change it inside Update once the fake
// serves.
type Repo struct {
	Owner, Name string
	// Push is the push permission of the viewer.
	Push bool
	// Visibility is "public", "private" or "internal", and empty sends none.
	Visibility string
	Stars      int
	UpdatedAt  time.Time
	// MergeMethods are the merge methods the repository allows, out of
	// "merge", "squash" and "rebase".
	MergeMethods []string
	// Approvals is the approving review count the rules of a branch ask for.
	// A branch without an entry has no rules.
	Approvals map[string]int
	// Protected is the approving review count the protection of a branch
	// asks for. A branch without an entry is not protected.
	Protected map[string]int
	// Deleted holds the IDs of comments that no longer exist: GitHub answers
	// 404 when a client reads one or replies to it.
	Deleted []int64
	// Runs are the workflow runs of the repository.
	Runs []*Run

	pulls map[int]*PR
	order []*PR
}

// FullName is owner/name.
func (r *Repo) FullName() string { return r.Owner + "/" + r.Name }

// PR is a pull request of the fake. Change it inside Update once the fake
// serves.
type PR struct {
	Number int
	ID     int64
	Title  string
	// State is "open" or "closed".
	State     string
	Merged    bool
	MergedAt  time.Time
	ClosedAt  time.Time
	Draft     bool
	Mergeable *bool
	// MergeableState is what GitHub computes: "clean", "blocked", "dirty",
	// "unknown" and so on.
	MergeableState string
	Author         string
	// AuthorAvatar is the avatar GitHub returns for the author. Empty gives
	// the one AvatarURL makes from the login.
	AuthorAvatar string
	URL          string
	HeadRef      string
	HeadSHA      string
	// HeadRepo is the full name of the repository of the head branch, which
	// differs from the repository of the pull request for a fork.
	HeadRepo string
	// Assignees are the logins the pull request is assigned to.
	Assignees []string
	Body      string
	BaseRef   string
	BaseSHA   string
	// BehindBy is how many commits of BaseRef the head does not have, as a
	// compare of BaseRef with HeadSHA answers.
	BehindBy  int
	Labels    []string
	Milestone string
	// AutoMerge is the merge method of auto-merge; empty leaves it off.
	AutoMerge    string
	Additions    int
	Deletions    int
	Commits      int
	ChangedFiles int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// Requested are the users asked for a review, as REST lists them.
	Requested []string
	// RequestedTeams are the teams asked for a review, which only GraphQL
	// lists.
	RequestedTeams []string

	Reviews        []Review
	IssueComments  []Comment
	ReviewComments []ReviewComment
	Threads        []Thread

	// CheckRuns and Statuses answer for the head commit of the pull request.
	CheckRuns []CheckRun
	Statuses  []Status
	// StatusState is the combined state of Statuses.
	StatusState string

	// Merges holds the body of every merge the fake accepted or refused.
	Merges []Merge
	// RefuseMerge makes a merge fail with this status and GitHub's message
	// for an unmergeable pull request.
	RefuseMerge int
	// Refuse holds the logins GitHub refuses as reviewers, with a 422.
	Refuse []string
	// ReviewRequests holds the reviewers of every review request, in order.
	ReviewRequests     [][]string
	BranchUpdates      []BranchUpdate
	RefuseBranchUpdate string
}

type BranchUpdate struct {
	Method       string
	ExpectedHead string
}

// Merge is one merge request the fake received.
type Merge struct {
	Method  string
	SHA     string
	Title   string
	Message string
}

// Review is a submitted or pending review.
type Review struct {
	ID    int64
	State string
	// CommitID is the commit the review was on.
	CommitID    string
	Author      string
	Body        string
	URL         string
	Association string
	SubmittedAt time.Time
}

// Comment is a comment on the conversation of a pull request.
type Comment struct {
	ID          int64
	Author      string
	Body        string
	URL         string
	Association string
	CreatedAt   time.Time
}

// ReviewComment is an inline comment of a review.
type ReviewComment struct {
	Comment
	ReviewID     int64
	InReplyTo    int64
	Path         string
	Line         int
	OriginalLine int
	// Side is LEFT or RIGHT: the side of the diff the line is on.
	Side string
	// CommitID is the commit Line is on, and OriginalCommitID the one
	// OriginalLine is on.
	CommitID         string
	OriginalCommitID string
}

// Thread is a review thread as GraphQL lists it.
type Thread struct {
	Resolved bool
	// Authors are the logins of the comments of the thread, oldest first. A
	// login that ends in [bot] is a Bot.
	Authors []string
	// LastCommentID is the database ID of the last comment.
	LastCommentID int64
}

// CheckRun is a check run on the head commit.
type CheckRun struct {
	ID           int64
	Name         string
	Status       string
	Conclusion   string
	URL          string
	CheckSuiteID int64
}

// Status is a commit status on the head commit.
type Status struct {
	Context string
	State   string
	URL     string
}

// Run is a workflow run.
type Run struct {
	ID           int64
	Name         string
	WorkflowID   int64
	HeadSHA      string
	Status       string
	Conclusion   string
	URL          string
	CheckSuiteID int64
	Jobs         []*Job
	// RefuseJobs makes the jobs of the run answer 400, for a test that
	// asserts the code never asks for them.
	RefuseJobs bool
}

// Job is a job of a workflow run.
type Job struct {
	ID         int64
	Name       string
	Status     string
	Conclusion string
	URL        string
	// Log is the text of the log the download answers with.
	Log string
	// LogStatus makes the download of the log answer this status.
	LogStatus int
}

// At parses an RFC 3339 time for the tables of a test, and panics on a
// malformed one.
func At(rfc3339 string) time.Time {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		panic(err)
	}
	return t
}

// Repo returns the repository, made with the defaults when the fake does
// not have it: the viewer can push and every merge method is allowed.
func (g *GitHub) Repo(fullName string) *Repo {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.repo(fullName)
}

func (g *GitHub) repo(fullName string) *Repo {
	key := strings.ToLower(fullName)
	if r, ok := g.repos[key]; ok {
		return r
	}
	owner, name, _ := strings.Cut(fullName, "/")
	r := &Repo{
		Owner: owner, Name: name, Push: true,
		MergeMethods: []string{"merge", "squash", "rebase"},
		Approvals:    map[string]int{},
		Protected:    map[string]int{},
		pulls:        map[int]*PR{},
	}
	g.repos[key] = r
	g.order = append(g.order, r)
	return r
}

func (r *Repo) pr(number int) *PR {
	if p, ok := r.pulls[number]; ok {
		return p
	}
	p := &PR{
		Number: number, ID: int64(number) * 11, Title: "Fix the thing",
		State: "open", Mergeable: new(true), MergeableState: "clean",
		Author: "alice", URL: fmt.Sprintf("https://github.com/%s/pull/%d", r.FullName(), number),
		HeadRef: "fix", HeadSHA: "abc", HeadRepo: r.FullName(), BaseRef: "main", BaseSHA: "base",
		StatusState: "success",
	}
	r.pulls[number] = p
	r.order = append(r.order, p)
	return p
}

// PR returns the pull request, made with the defaults when the fake does
// not have it: open, by alice, from fix at abc into main, mergeable, with a
// green combined status and no check.
func (g *GitHub) PR(fullName string, number int) *PR {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.repo(fullName).pr(number)
}

// DeletePR takes the pull request out of the fake, so every read of it
// answers 404, the way GitHub answers for a pull request that was deleted
// with its repository or that the token can no longer see.
func (g *GitHub) DeletePR(fullName string, number int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repo(fullName)
	delete(r.pulls, number)
	r.order = slices.DeleteFunc(r.order, func(p *PR) bool { return p.Number == number })
}

func (r *Repo) find(number int) *PR {
	return r.pulls[number]
}

func (r *Repo) deleted(id int64) bool {
	return slices.Contains(r.Deleted, id)
}
