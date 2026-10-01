package snapshot

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v91/github"
	"golang.org/x/sync/errgroup"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/gitrepo"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/watcher"
)

type SeenStore interface {
	TouchWatch(ctx context.Context, key store.WatchKey, headSHA string, now time.Time) error
	SeenReviewItems(ctx context.Context, key store.WatchKey) (map[store.SeenItem]bool, error)
	MarkReviewItemsSeen(ctx context.Context, key store.WatchKey, items []store.SeenItem, now time.Time) error
	RetryCount(ctx context.Context, key store.WatchKey, headSHA string) (int, error)
	IncrementRetries(ctx context.Context, key store.WatchKey, headSHA string) (int, error)
}

type Options struct {
	MaxFlakyRetries int
	RecordRetry     bool
	Dir             string
	IgnoreAuthor    string
	TokenLogin      string
	Now             func() time.Time
	AfterCall       func(ctx context.Context, resp *github.Response, err error) error
}

func (o Options) ignores(it ReviewItem) bool {
	return o.IgnoreAuthor != "" && strings.EqualFold(it.Author, o.IgnoreAuthor)
}

func (o Options) after(ctx context.Context, resp *github.Response, err error) error {
	if o.AfterCall == nil {
		return err
	}
	return o.AfterCall(ctx, resp, err)
}

func fetch[T any](ctx context.Context, g *errgroup.Group, o Options, dst *T, call func(context.Context) (T, *github.Response, error)) {
	g.Go(func() error {
		out, resp, err := call(ctx)
		if err := o.after(ctx, resp, err); err != nil {
			return err
		}
		*dst = out
		return nil
	})
}

type Snapshot struct {
	SnapshotAt       time.Time    `json:"snapshot_at"`
	PR               PR           `json:"pr"`
	Checks           Checks       `json:"checks"`
	FailedRuns       []Run        `json:"failed_runs"`
	FailedJobs       []FailedJob  `json:"failed_jobs"`
	AwaitingApproval []Run        `json:"awaiting_approval"`
	NewReviewItems   []ReviewItem `json:"new_review_items"`
	Threads          Threads      `json:"threads"`
	Actions          []string     `json:"actions"`
	RetryState       RetryState   `json:"retry_state"`

	key         store.WatchKey
	now         time.Time
	mark        []store.SeenItem
	recordRetry bool
}

type PR struct {
	Repo                string               `json:"repo"`
	Number              int                  `json:"number"`
	NodeID              string               `json:"node_id"`
	URL                 string               `json:"url"`
	Title               string               `json:"title"`
	Author              string               `json:"author"`
	AuthorAvatarURL     string               `json:"author_avatar_url"`
	State               store.PRState        `json:"state"`
	Draft               bool                 `json:"draft"`
	Merged              bool                 `json:"merged"`
	Closed              bool                 `json:"closed"`
	HeadSHA             string               `json:"head_sha"`
	HeadBranch          string               `json:"head_branch"`
	HeadRepo            string               `json:"head_repo"`
	BaseBranch          string               `json:"base_branch"`
	Mergeable           *bool                `json:"mergeable"`
	MergeableState      store.MergeableState `json:"mergeable_state"`
	BehindBy            int                  `json:"behind_by"`
	BehindErr           string               `json:"behind_err,omitempty"`
	ReviewDecision      store.ReviewDecision `json:"review_decision"`
	ReviewersBehindHead []string             `json:"reviewers_behind_head"`
	RequestedReviewers  []string             `json:"requested_reviewers"`
	Approvals           int                  `json:"approvals"`
	ChangesRequested    int                  `json:"changes_requested"`
	UpdateType          dependabot.Level     `json:"update_type,omitempty"`
	Reviewers           []Reviewer           `json:"reviewers"`
	Labels              []string             `json:"labels"`
	Assignees           []string             `json:"assignees"`
	Milestone           string               `json:"milestone,omitempty"`
	AutoMerge           string               `json:"auto_merge,omitempty"`
	Additions           int                  `json:"additions"`
	Deletions           int                  `json:"deletions"`
	Commits             int                  `json:"commits"`
	ChangedFiles        int                  `json:"changed_files"`
	CreatedAt           time.Time            `json:"created_at"`
	Body                string               `json:"body"`
}

type Reviewer struct {
	Login string              `json:"login"`
	State watcher.ReviewState `json:"state"`
}

func (pr PR) MarshalJSON() ([]byte, error) {
	type fields PR
	return json.Marshal(struct {
		fields
		Behind bool `json:"behind"`
	}{fields(pr), pr.Behind()})
}

func (pr PR) Behind() bool {
	return pr.MergeableState == store.MergeableBehind || pr.blockedBehind()
}

func (pr PR) blockedBehind() bool {
	return pr.MergeableState == store.MergeableBlocked && pr.BehindBy > 0
}

type Threads struct {
	Unresolved int      `json:"unresolved"`
	Unanswered int      `json:"unanswered"`
	Reviewers  []string `json:"reviewers,omitempty"`
	LastAnswer int64    `json:"last_answer,omitempty"`
	Err        string   `json:"err,omitempty"`
}

func (t Threads) waitOnReviewer() bool {
	return t.Err == "" && t.Unanswered == 0
}

type Checks struct {
	Status       checks.CIStatus `json:"status"`
	PassedCount  int             `json:"passed_count"`
	FailedCount  int             `json:"failed_count"`
	PendingCount int             `json:"pending_count"`
	SkippedCount int             `json:"skipped_count"`
	AllTerminal  bool            `json:"all_terminal"`
	Items        []Check         `json:"items"`
}

type Check struct {
	Name         string        `json:"name"`
	Source       checks.Source `json:"source"`
	Status       string        `json:"status"`
	Conclusion   string        `json:"conclusion"`
	URL          string        `json:"url"`
	CheckSuiteID int64         `json:"check_suite_id,omitempty"`
}

type Run struct {
	RunID        int64             `json:"run_id"`
	CheckSuiteID int64             `json:"check_suite_id,omitempty"`
	WorkflowName string            `json:"workflow_name"`
	Status       checks.RunStatus  `json:"status"`
	Conclusion   checks.Conclusion `json:"conclusion"`
	HTMLURL      string            `json:"html_url"`
}

func (s *Snapshot) FailedRunsWithoutCheck() []Run {
	out := make([]Run, 0, len(s.FailedRuns))
	for _, r := range s.FailedRuns {
		if !s.Checks.covers(r) {
			out = append(out, r)
		}
	}
	return out
}

func (c Checks) covers(r Run) bool {
	for _, it := range c.Items {
		sameSuite := r.CheckSuiteID != 0 && it.CheckSuiteID == r.CheckSuiteID
		if sameSuite || it.Name == r.WorkflowName {
			return true
		}
	}
	return false
}

type FailedJob struct {
	RunID         int64             `json:"run_id"`
	WorkflowName  string            `json:"workflow_name"`
	RunStatus     checks.RunStatus  `json:"run_status"`
	RunConclusion checks.Conclusion `json:"run_conclusion"`
	JobID         int64             `json:"job_id"`
	JobName       string            `json:"job_name"`
	Status        checks.RunStatus  `json:"status"`
	Conclusion    checks.Conclusion `json:"conclusion"`
	HTMLURL       string            `json:"html_url"`
	LogsEndpoint  string            `json:"logs_endpoint"`
}

type ReviewItem struct {
	Kind              store.ReviewItemKind `json:"kind"`
	ID                int64                `json:"id"`
	Author            string               `json:"author"`
	AuthorAssociation string               `json:"author_association"`
	CreatedAt         time.Time            `json:"created_at"`
	Body              string               `json:"body"`
	Path              string               `json:"path,omitempty"`
	Line              *int                 `json:"line,omitempty"`
	Side              string               `json:"side,omitempty"`
	CommitID          string               `json:"commit_id,omitempty"`
	State             watcher.ReviewState  `json:"state,omitempty"`
	URL               string               `json:"url"`
}

type RetryState struct {
	CurrentSHARetriesUsed int `json:"current_sha_retries_used"`
	MaxFlakyRetries       int `json:"max_flaky_retries"`
}

func Collect(ctx context.Context, c *github.Client, st SeenStore, t Target, o Options) (*Snapshot, error) {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	t, err := resolve(ctx, c, t, o)
	if err != nil {
		return nil, err
	}

	detail, resp, err := ghclient.GetPull(ctx, c, t.Owner, t.Name, t.Number)
	if err := o.after(ctx, resp, err); err != nil {
		return nil, err
	}
	if base := detail.GetBase().GetRepo(); base.GetOwner().GetLogin() != "" && base.GetName() != "" {
		t.Owner, t.Name = base.GetOwner().GetLogin(), base.GetName()
	}
	at := now()
	s := &Snapshot{
		SnapshotAt: at.UTC().Truncate(time.Second),
		PR:         toPR(t, detail),
		key:        store.WatchKey{Owner: t.Owner, Name: t.Name, Number: t.Number},
		now:        at,
	}

	var (
		reviews        []*github.PullRequestReview
		issueComments  []*github.IssueComment
		reviewComments []*github.PullRequestComment
		checkRuns      []*github.CheckRun
		combined       *github.CombinedStatus
		workflowRuns   []*github.WorkflowRun
		threads        Threads
		reviewState    ghclient.ReviewState
		behind         behindBase
	)
	g, gctx := errgroup.WithContext(ctx)
	fetch(gctx, g, o, &reviews, func(ctx context.Context) ([]*github.PullRequestReview, *github.Response, error) {
		return ghclient.ListReviews(ctx, c, t.Owner, t.Name, t.Number)
	})
	fetch(gctx, g, o, &issueComments, func(ctx context.Context) ([]*github.IssueComment, *github.Response, error) {
		return ghclient.ListIssueComments(ctx, c, t.Owner, t.Name, t.Number)
	})
	fetch(gctx, g, o, &reviewComments, func(ctx context.Context) ([]*github.PullRequestComment, *github.Response, error) {
		return ghclient.ListReviewComments(ctx, c, t.Owner, t.Name, t.Number)
	})
	fetch(gctx, g, o, &checkRuns, func(ctx context.Context) ([]*github.CheckRun, *github.Response, error) {
		return ghclient.ListCheckRuns(ctx, c, t.Owner, t.Name, s.PR.HeadSHA)
	})
	fetch(gctx, g, o, &combined, func(ctx context.Context) (*github.CombinedStatus, *github.Response, error) {
		return ghclient.GetCombinedStatus(ctx, c, t.Owner, t.Name, s.PR.HeadSHA)
	})
	fetch(gctx, g, o, &workflowRuns, func(ctx context.Context) ([]*github.WorkflowRun, *github.Response, error) {
		return ghclient.ListWorkflowRuns(ctx, c, t.Owner, t.Name, s.PR.HeadSHA)
	})
	g.Go(func() error {
		var err error
		threads, reviewState, err = fetchReviewState(gctx, c, t, o)
		return err
	})
	g.Go(func() error {
		var err error
		behind, err = fetchBehindBy(gctx, c, t, s.PR, o)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	s.PR.BehindBy, s.PR.BehindErr = behind.by, behind.failure
	s.PR.RequestedReviewers = withRequests(s.PR.RequestedReviewers, reviewState.Requested)
	s.PR.ReviewDecision, s.PR.Approvals, s.PR.ChangesRequested = watcher.ReviewDecision(reviews, s.PR.Author, len(s.PR.RequestedReviewers))
	s.Threads = threads
	index := indexReviews(reviews)
	s.Threads.Reviewers = index.requestable(reviewState.AnsweredReviewers(o.TokenLogin), s.PR.Author)
	s.PR.ReviewersBehindHead = index.behindHead(s.PR.Author, s.PR.HeadSHA)
	s.PR.Reviewers = latestReviews(reviews)
	items := reviewItems(issueComments, reviewComments, reviews)
	s.Checks = summarizeChecks(checkRuns, combined)
	workflowRuns = latestRuns(workflowRuns, s.PR.HeadSHA)
	s.FailedRuns = selectRuns(workflowRuns, s.PR.HeadSHA, isFailed)
	s.AwaitingApproval = selectRuns(workflowRuns, s.PR.HeadSHA, isActionRequired)
	if s.FailedJobs, err = failedJobs(ctx, c, t, workflowRuns, s.PR.HeadSHA, o); err != nil {
		return nil, err
	}

	if err := st.TouchWatch(ctx, s.key, s.PR.HeadSHA, s.now); err != nil {
		return nil, err
	}
	seen, err := st.SeenReviewItems(ctx, s.key)
	if err != nil {
		return nil, err
	}
	s.NewReviewItems = make([]ReviewItem, 0, len(items))
	for _, it := range items {
		if o.ignores(it) || seen[store.SeenItem{Kind: it.Kind, ID: it.ID}] {
			continue
		}
		s.NewReviewItems = append(s.NewReviewItems, it)
		s.mark = append(s.mark, store.SeenItem{Kind: it.Kind, ID: it.ID})
	}
	used, err := st.RetryCount(ctx, s.key, s.PR.HeadSHA)
	if err != nil {
		return nil, err
	}
	if o.RecordRetry {
		used++
		s.recordRetry = true
	}
	s.RetryState = RetryState{CurrentSHARetriesUsed: used, MaxFlakyRetries: o.MaxFlakyRetries}

	s.Actions = recommend(s)
	return s, nil
}

func (s *Snapshot) Commit(ctx context.Context, st SeenStore) error {
	if s.recordRetry {
		if _, err := st.IncrementRetries(ctx, s.key, s.PR.HeadSHA); err != nil {
			return err
		}
		s.recordRetry = false
	}
	return st.MarkReviewItemsSeen(ctx, s.key, s.mark, s.now)
}

func resolve(ctx context.Context, c *github.Client, t Target, o Options) (Target, error) {
	if t.Complete() {
		return t, nil
	}
	if o.Dir == "" {
		return Target{}, ErrIncompleteTarget
	}
	headOwner, err := resolveRepo(ctx, &t, o.Dir)
	if err != nil {
		return Target{}, err
	}
	if t.Number != 0 {
		return t, nil
	}
	branch, err := gitrepo.CurrentBranch(ctx, o.Dir)
	if err != nil {
		return Target{}, fmt.Errorf("resolve pull request from git: %w", err)
	}
	head := headOwner + ":" + branch
	for _, state := range []string{string(store.StateOpen), string(store.FilterAll)} {
		prs, resp, err := ghclient.ListPullsByHead(ctx, c, t.Owner, t.Name, head, state)
		if err := o.after(ctx, resp, err); err != nil {
			return Target{}, err
		}
		if len(prs) > 0 {
			t.Number = prs[0].GetNumber()
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("no pull request for branch %q on %s", branch, t.Repo())
}

func resolveRepo(ctx context.Context, t *Target, dir string) (headOwner string, err error) {
	origin, err := gitrepo.RemoteURL(ctx, dir, "origin")
	if err != nil {
		if t.Owner != "" {
			return t.Owner, nil
		}
		return "", fmt.Errorf("resolve repository from git: %w", err)
	}
	headOwner, name, err := store.ParseFullName(origin)
	if err != nil {
		if t.Owner != "" {
			return t.Owner, nil
		}
		return "", fmt.Errorf("resolve repository from origin: %w", err)
	}
	if t.Owner != "" {
		return headOwner, nil
	}
	t.Owner, t.Name = headOwner, name
	if upstream, err := gitrepo.RemoteURL(ctx, dir, "upstream"); err == nil {
		if owner, name, err := store.ParseFullName(upstream); err == nil {
			t.Owner, t.Name = owner, name
		}
	}
	return headOwner, nil
}

func toPR(t Target, pr *github.PullRequest) PR {
	out := PR{
		Repo:            t.Repo(),
		Number:          pr.GetNumber(),
		NodeID:          pr.GetNodeID(),
		URL:             pr.GetHTMLURL(),
		Title:           pr.GetTitle(),
		Author:          pr.GetUser().GetLogin(),
		AuthorAvatarURL: pr.GetUser().GetAvatarURL(),
		State:           store.PRState(pr.GetState()),
		Draft:           pr.GetDraft(),
		Merged:          pr.GetMerged(),
		Closed:          store.PRState(pr.GetState()) == store.StateClosed,
		HeadSHA:         pr.GetHead().GetSHA(),
		HeadBranch:      pr.GetHead().GetRef(),
		HeadRepo:        pr.GetHead().GetRepo().GetFullName(),
		BaseBranch:      pr.GetBase().GetRef(),
		Mergeable:       pr.Mergeable,
		MergeableState:  store.MergeableState(pr.GetMergeableState()),
		UpdateType:      watcher.UpdateTypeOf(pr),
		Labels:          labelNames(pr.Labels),
		Assignees:       logins(pr.Assignees),
		Milestone:       pr.GetMilestone().GetTitle(),
		AutoMerge:       autoMergeMethod(pr.GetAutoMerge()),
		Additions:       pr.GetAdditions(),
		Deletions:       pr.GetDeletions(),
		Commits:         pr.GetCommits(),
		ChangedFiles:    pr.GetChangedFiles(),
		CreatedAt:       pr.GetCreatedAt().Time,
		Body:            pr.GetBody(),
	}
	for _, u := range pr.RequestedReviewers {
		if login := u.GetLogin(); login != "" {
			out.RequestedReviewers = append(out.RequestedReviewers, login)
		}
	}
	slices.Sort(out.RequestedReviewers)
	if out.Merged {
		out.State = store.StateMerged
	}
	return out
}

func labelNames(labels []*github.Label) []string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, l.GetName())
	}
	return out
}

func logins(users []*github.User) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, u.GetLogin())
	}
	return out
}

func autoMergeMethod(a *github.PullRequestAutoMerge) string {
	if a == nil {
		return ""
	}
	return cmp.Or(a.GetMergeMethod(), "merge")
}

func latestReviews(reviews []*github.PullRequestReview) []Reviewer {
	sorted := slices.Clone(reviews)
	slices.SortStableFunc(sorted, func(a, b *github.PullRequestReview) int {
		return a.GetSubmittedAt().Compare(b.GetSubmittedAt().Time)
	})
	latest := map[string]watcher.ReviewState{}
	var order []string
	for _, r := range sorted {
		login, state := r.GetUser().GetLogin(), watcher.StateOf(r)
		if login == "" || state == watcher.ReviewStatePending {
			continue
		}
		held, known := latest[login]
		if !known {
			order = append(order, login)
		}
		if !known || state.Counts() || !held.Counts() {
			latest[login] = state
		}
	}
	out := make([]Reviewer, 0, len(order))
	for _, login := range order {
		out = append(out, Reviewer{Login: login, State: latest[login]})
	}
	return out
}

func reviewItems(issueComments []*github.IssueComment, reviewComments []*github.PullRequestComment, reviews []*github.PullRequestReview) []ReviewItem {
	pending := make(map[int64]bool)
	for _, r := range reviews {
		if watcher.StateOf(r) == watcher.ReviewStatePending {
			pending[r.GetID()] = true
		}
	}
	var items []ReviewItem
	for _, c := range issueComments {
		items = append(items, ReviewItem{
			Kind:              store.KindIssueComment,
			ID:                c.GetID(),
			Author:            c.GetUser().GetLogin(),
			AuthorAssociation: c.GetAuthorAssociation(),
			CreatedAt:         c.GetCreatedAt().Time,
			Body:              c.GetBody(),
			URL:               c.GetHTMLURL(),
		})
	}
	for _, c := range reviewComments {
		if pending[c.GetPullRequestReviewID()] {
			continue
		}
		line, commit := c.Line, c.GetCommitID()
		if line == nil {
			line, commit = c.OriginalLine, c.GetOriginalCommitID()
		}
		items = append(items, ReviewItem{
			Kind:              store.KindReviewComment,
			ID:                c.GetID(),
			Author:            c.GetUser().GetLogin(),
			AuthorAssociation: c.GetAuthorAssociation(),
			CreatedAt:         c.GetCreatedAt().Time,
			Body:              c.GetBody(),
			Path:              c.GetPath(),
			Line:              line,
			Side:              c.GetSide(),
			CommitID:          commit,
			URL:               c.GetHTMLURL(),
		})
	}
	for _, r := range reviews {
		if pending[r.GetID()] {
			continue
		}
		if watcher.StateOf(r) == watcher.ReviewStateCommented && strings.TrimSpace(r.GetBody()) == "" {
			continue
		}
		items = append(items, ReviewItem{
			Kind:              store.KindReview,
			ID:                r.GetID(),
			Author:            r.GetUser().GetLogin(),
			AuthorAssociation: r.GetAuthorAssociation(),
			CreatedAt:         r.GetSubmittedAt().Time,
			Body:              r.GetBody(),
			State:             watcher.StateOf(r),
			URL:               r.GetHTMLURL(),
		})
	}
	slices.SortStableFunc(items, func(a, b ReviewItem) int {
		return cmp.Or(
			a.CreatedAt.Compare(b.CreatedAt),
			cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.ID, b.ID),
		)
	})
	return items
}

func summarizeChecks(runs []*github.CheckRun, combined *github.CombinedStatus) Checks {
	c := Checks{Status: checks.Overall(runs, combined), Items: make([]Check, 0, len(runs))}
	count := func(state checks.State) {
		switch state {
		case checks.Pending:
			c.PendingCount++
		case checks.Failed:
			c.FailedCount++
		case checks.Passed:
			c.PassedCount++
		default:
			c.SkippedCount++
		}
	}
	for _, r := range runs {
		c.Items = append(c.Items, Check{Name: r.GetName(), Source: checks.SourceCheckRun, Status: r.GetStatus(), Conclusion: r.GetConclusion(), URL: r.GetHTMLURL(), CheckSuiteID: r.GetCheckSuite().GetID()})
		count(checks.ClassifyCheckRun(r))
	}
	if combined != nil {
		for _, s := range combined.Statuses {
			c.Items = append(c.Items, Check{Name: s.GetContext(), Source: checks.SourceStatus, Status: s.GetState(), URL: s.GetTargetURL()})
			count(checks.ClassifyStatus(s))
		}
	}
	c.AllTerminal = c.PendingCount == 0
	return c
}

func isFailed(r *github.WorkflowRun) bool {
	return checks.Conclusion(r.GetConclusion()).Failed()
}

func needsApproval(r *github.WorkflowRun) bool {
	return checks.Conclusion(r.GetConclusion()) == checks.ConclusionActionRequired ||
		checks.RunStatus(r.GetStatus()) == checks.StatusActionRequired
}

func isActionRequired(r *github.WorkflowRun) bool {
	return needsApproval(r) || checks.RunStatus(r.GetStatus()) == checks.StatusWaiting
}

func latestRuns(runs []*github.WorkflowRun, headSHA string) []*github.WorkflowRun {
	latest := make(map[string]*github.WorkflowRun)
	for _, r := range runs {
		if r.GetHeadSHA() != headSHA {
			continue
		}
		key := fmt.Sprintf("%d/%s", r.GetWorkflowID(), r.GetName())
		if cur, ok := latest[key]; !ok || r.GetID() > cur.GetID() {
			latest[key] = r
		}
	}
	out := make([]*github.WorkflowRun, 0, len(latest))
	for _, r := range latest {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b *github.WorkflowRun) int { return cmp.Compare(a.GetID(), b.GetID()) })
	return out
}

func selectRuns(runs []*github.WorkflowRun, headSHA string, keep func(*github.WorkflowRun) bool) []Run {
	out := make([]Run, 0)
	for _, r := range runs {
		if r.GetHeadSHA() != headSHA || !keep(r) {
			continue
		}
		out = append(out, Run{
			RunID:        r.GetID(),
			CheckSuiteID: r.GetCheckSuiteID(),
			WorkflowName: r.GetName(),
			Status:       checks.RunStatus(r.GetStatus()),
			Conclusion:   checks.Conclusion(r.GetConclusion()),
			HTMLURL:      r.GetHTMLURL(),
		})
	}
	slices.SortFunc(out, func(a, b Run) int {
		return cmp.Or(cmp.Compare(a.WorkflowName, b.WorkflowName), cmp.Compare(a.RunID, b.RunID))
	})
	return out
}

func failedJobs(ctx context.Context, c *github.Client, t Target, runs []*github.WorkflowRun, headSHA string, o Options) ([]FailedJob, error) {
	var (
		mu  sync.Mutex
		out = make([]FailedJob, 0)
	)
	g, gctx := errgroup.WithContext(ctx)
	for _, r := range runs {
		if r.GetHeadSHA() != headSHA || needsApproval(r) {
			continue
		}
		if checks.RunStatus(r.GetStatus()) == checks.StatusCompleted && !isFailed(r) {
			continue
		}
		g.Go(func() error {
			jobs, resp, err := ghclient.ListWorkflowJobs(gctx, c, t.Owner, t.Name, r.GetID())
			if err := o.after(gctx, resp, err); err != nil {
				return err
			}
			var failed []FailedJob
			for _, j := range jobs {
				if !checks.Conclusion(j.GetConclusion()).Failed() {
					continue
				}
				failed = append(failed, FailedJob{
					RunID:         r.GetID(),
					WorkflowName:  r.GetName(),
					RunStatus:     checks.RunStatus(r.GetStatus()),
					RunConclusion: checks.Conclusion(r.GetConclusion()),
					JobID:         j.GetID(),
					JobName:       j.GetName(),
					Status:        checks.RunStatus(j.GetStatus()),
					Conclusion:    checks.Conclusion(j.GetConclusion()),
					HTMLURL:       j.GetHTMLURL(),
					LogsEndpoint:  fmt.Sprintf("repos/%s/actions/jobs/%d/logs", t.Repo(), j.GetID()),
				})
			}
			mu.Lock()
			out = append(out, failed...)
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b FailedJob) int {
		return cmp.Or(
			cmp.Compare(a.WorkflowName, b.WorkflowName),
			cmp.Compare(a.JobName, b.JobName),
			cmp.Compare(a.JobID, b.JobID),
		)
	})
	return out, nil
}

type reviewIndex struct {
	logins   map[string]string
	drafting map[string]bool
	commit   map[string]string
	blocking map[string]bool
}

func indexReviews(reviews []*github.PullRequestReview) reviewIndex {
	x := reviewIndex{
		logins:   map[string]string{},
		drafting: map[string]bool{},
		commit:   map[string]string{},
		blocking: map[string]bool{},
	}
	for _, r := range reviews {
		login := r.GetUser().GetLogin()
		if login == "" {
			continue
		}
		key := strings.ToLower(login)
		if _, known := x.logins[key]; !known {
			x.logins[key] = login
		}
		state := watcher.StateOf(r)
		if state == watcher.ReviewStatePending {
			x.drafting[key] = true
			continue
		}
		x.commit[key] = r.GetCommitID()
		if state.Counts() {
			x.blocking[key] = state == watcher.ReviewStateChangesRequested
		}
	}
	return x
}

func (x reviewIndex) askable(login, author string) bool {
	key := strings.ToLower(login)
	_, submitted := x.commit[key]
	return submitted && !x.drafting[key] && !strings.EqualFold(login, author)
}

func (x reviewIndex) settledOn(login, headSHA string) bool {
	key := strings.ToLower(login)
	sha := x.commit[key]
	return sha != "" && sha == headSHA && !x.blocking[key]
}

func (x reviewIndex) behind(login, author, headSHA string) bool {
	return x.askable(login, author) && !x.settledOn(login, headSHA)
}

func (x reviewIndex) behindHead(author, headSHA string) []string {
	var out []string
	for _, login := range x.logins {
		if x.behind(login, author, headSHA) {
			out = append(out, login)
		}
	}
	slices.Sort(out)
	return out
}

func (x reviewIndex) requestable(logins []string, author string) []string {
	var out []string
	for _, login := range uniqueLogins(logins) {
		if x.askable(login, author) {
			out = append(out, login)
		}
	}
	return out
}

func uniqueLogins(logins []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, login := range logins {
		key := strings.ToLower(login)
		repeated := login == "" || seen[key]
		if repeated {
			continue
		}
		seen[key] = true
		out = append(out, login)
	}
	slices.Sort(out)
	return out
}

func (o Options) afterOptional(ctx context.Context, resp *github.Response, callErr error) (failure string, err error) {
	err = o.after(ctx, resp, callErr)
	if callErr == nil {
		return "", err
	}
	if err != nil && !errors.Is(err, callErr) {
		return "", err
	}
	return callErr.Error(), nil
}

func fetchReviewState(ctx context.Context, c *github.Client, t Target, o Options) (Threads, ghclient.ReviewState, error) {
	state, resp, callErr := ghclient.FetchReviewState(ctx, c, t.Owner, t.Name, t.Number)
	failure, err := o.afterOptional(ctx, resp, callErr)
	if failure != "" || err != nil {
		return Threads{Err: failure}, ghclient.ReviewState{}, err
	}
	return Threads{
		Unresolved: state.Unresolved(o.IgnoreAuthor),
		Unanswered: state.Unanswered(o.IgnoreAuthor, o.TokenLogin),
		LastAnswer: state.LastAnswerID(o.TokenLogin),
	}, state, nil
}

type behindBase struct {
	by      int
	failure string
}

func fetchBehindBy(ctx context.Context, c *github.Client, t Target, pr PR, o Options) (behindBase, error) {
	if pr.MergeableState != store.MergeableBlocked {
		return behindBase{}, nil
	}
	n, resp, callErr := ghclient.BehindBy(ctx, c, t.Owner, t.Name, pr.BaseBranch, pr.HeadSHA)
	failure, err := o.afterOptional(ctx, resp, callErr)
	if failure != "" || err != nil {
		return behindBase{failure: failure}, err
	}
	return behindBase{by: n}, nil
}

func withRequests(rest, graph []string) []string {
	return uniqueLogins(slices.Concat(rest, graph))
}
