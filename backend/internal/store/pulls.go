package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/events"
)

type PRState string

const (
	StateOpen   PRState = "open"
	StateClosed PRState = "closed"
	StateMerged PRState = "merged"
)

var PRStates = []PRState{StateOpen, StateClosed, StateMerged}

func (s PRState) Valid() bool { return slices.Contains(PRStates, s) }

type ReviewDecision string

const (
	ReviewApproved         ReviewDecision = "approved"
	ReviewChangesRequested ReviewDecision = "changes_requested"
	ReviewRequired         ReviewDecision = "review_required"
	ReviewNone             ReviewDecision = "none"
)

var ReviewDecisions = []ReviewDecision{ReviewApproved, ReviewChangesRequested, ReviewRequired, ReviewNone}

func (d ReviewDecision) Valid() bool { return slices.Contains(ReviewDecisions, d) }

type MergeableState string

const (
	MergeableUnknown  MergeableState = "unknown"
	MergeableClean    MergeableState = "clean"
	MergeableDirty    MergeableState = "dirty"
	MergeableBehind   MergeableState = "behind"
	MergeableBlocked  MergeableState = "blocked"
	MergeableDraft    MergeableState = "draft"
	MergeableUnstable MergeableState = "unstable"
	MergeableHasHooks MergeableState = "has_hooks"
)

func (m MergeableState) Known() bool { return m != "" && m != MergeableUnknown }

type PullRequest struct {
	RepoID             int64
	RepoFullName       string
	Number             int
	GitHubID           int64
	Title              string
	Author             string
	AuthorAvatarURL    string
	State              PRState
	Draft              bool
	BaseRef            string
	HeadRef            string
	HeadSHA            string
	HTMLURL            string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	MergedAt           *time.Time
	ClosedAt           *time.Time
	MergeableState     MergeableState
	ReviewDecision     ReviewDecision
	Approvals          int
	ChangesRequested   int
	RequestedReviewers []string
	Labels             []string
	Additions          int
	Deletions          int
	CIStatus           checks.CIStatus
	SyncedAt           time.Time
	Assignees          []string
	Fork               bool
	UpdateType         dependabot.Level
}

func (pr PullRequest) AssignedTo(login string) bool {
	return slices.ContainsFunc(pr.Assignees, func(a string) bool { return strings.EqualFold(a, login) })
}

type PRStateFilter string

const (
	FilterOpen PRStateFilter = "open"
	FilterAll  PRStateFilter = "all"
)

type ListPRsOptions struct {
	Owner string
	Name  string
	State PRStateFilter
}

const prColumns = `p.repo_id, r.owner || '/' || r.name, p.number, p.github_id, p.title, p.author,
	p.state, p.draft, p.base_ref, p.head_ref, p.head_sha, p.html_url,
	p.created_at, p.updated_at, p.merged_at, p.closed_at,
	p.mergeable_state, p.review_decision, p.approvals, p.changes_requested,
	p.requested_reviewers, p.labels, p.additions, p.deletions, p.ci_status, p.synced_at,
	p.assignees, p.fork, p.update_type, p.author_avatar_url`

func (s *Store) UpsertPR(ctx context.Context, pr PullRequest) error {
	reviewers, err := jsonList(pr.RequestedReviewers)
	if err != nil {
		return fmt.Errorf("upsert pull request: %w", err)
	}
	labels, err := jsonList(pr.Labels)
	if err != nil {
		return fmt.Errorf("upsert pull request: %w", err)
	}
	assignees, err := jsonList(pr.Assignees)
	if err != nil {
		return fmt.Errorf("upsert pull request: %w", err)
	}
	if pr.ReviewDecision == "" {
		pr.ReviewDecision = ReviewNone
	}
	if pr.CIStatus == "" {
		pr.CIStatus = checks.CINone
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO pull_requests (
	repo_id, number, github_id, title, author, state, draft,
	base_ref, head_ref, head_sha, html_url,
	created_at, updated_at, merged_at, closed_at,
	mergeable_state, review_decision, approvals, changes_requested,
	requested_reviewers, labels, additions, deletions, ci_status, synced_at,
	assignees, fork, update_type, author_avatar_url
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (repo_id, number) DO UPDATE SET
	github_id = excluded.github_id,
	title = excluded.title,
	author = excluded.author,
	author_avatar_url = excluded.author_avatar_url,
	state = excluded.state,
	draft = excluded.draft,
	base_ref = excluded.base_ref,
	head_ref = excluded.head_ref,
	head_sha = excluded.head_sha,
	html_url = excluded.html_url,
	created_at = excluded.created_at,
	updated_at = excluded.updated_at,
	merged_at = excluded.merged_at,
	closed_at = excluded.closed_at,
	mergeable_state = excluded.mergeable_state,
	review_decision = excluded.review_decision,
	approvals = excluded.approvals,
	changes_requested = excluded.changes_requested,
	requested_reviewers = excluded.requested_reviewers,
	labels = excluded.labels,
	additions = excluded.additions,
	deletions = excluded.deletions,
	ci_status = excluded.ci_status,
	synced_at = excluded.synced_at,
	assignees = excluded.assignees,
	fork = excluded.fork,
	update_type = excluded.update_type`,
		pr.RepoID, pr.Number, pr.GitHubID, pr.Title, pr.Author, pr.State, pr.Draft,
		pr.BaseRef, pr.HeadRef, pr.HeadSHA, pr.HTMLURL,
		timeToDB(pr.CreatedAt), timeToDB(pr.UpdatedAt), timePtrToDB(pr.MergedAt), timePtrToDB(pr.ClosedAt),
		pr.MergeableState, pr.ReviewDecision, pr.Approvals, pr.ChangesRequested,
		reviewers, labels, pr.Additions, pr.Deletions, pr.CIStatus, timeToDB(pr.SyncedAt),
		assignees, pr.Fork, pr.UpdateType, pr.AuthorAvatarURL)
	if err != nil {
		return fmt.Errorf("upsert pull request: %w", err)
	}
	if s.pub != nil {
		s.publish(events.PullUpdated, s.repoFullName(ctx, pr.RepoID), pr.Number)
	}
	return nil
}

func (s *Store) OpenPRs(ctx context.Context, repoID int64) ([]PullRequest, error) {
	return s.queryPRs(ctx,
		"SELECT "+prColumns+" FROM pull_requests p JOIN repos r ON r.id = p.repo_id WHERE p.repo_id = ? AND p.state = ? ORDER BY p.number",
		repoID, StateOpen)
}

func (s *Store) ListPRs(ctx context.Context, o ListPRsOptions) ([]PullRequest, error) {
	q := "SELECT " + prColumns + " FROM pull_requests p JOIN repos r ON r.id = p.repo_id WHERE 1 = 1"
	var args []any
	if o.Owner != "" || o.Name != "" {
		q += " AND r.owner = ? AND r.name = ?"
		args = append(args, o.Owner, o.Name)
	}
	if o.State != FilterAll {
		q += " AND p.state = ?"
		args = append(args, StateOpen)
	}
	q += " ORDER BY r.owner, r.name, p.updated_at DESC, p.number"
	return s.queryPRs(ctx, q, args...)
}

func (s *Store) queryPRs(ctx context.Context, q string, args ...any) ([]PullRequest, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	defer rows.Close()

	var prs []PullRequest
	for rows.Next() {
		pr, err := scanPR(rows)
		if err != nil {
			return nil, fmt.Errorf("list pull requests: %w", err)
		}
		prs = append(prs, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	return prs, nil
}

func scanPR(sc scanner) (PullRequest, error) {
	var (
		pr                             PullRequest
		createdAt, updatedAt, syncedAt string
		mergedAt, closedAt             sql.NullString
		reviewers, labels, assignees   string
	)
	err := sc.Scan(&pr.RepoID, &pr.RepoFullName, &pr.Number, &pr.GitHubID, &pr.Title, &pr.Author,
		&pr.State, &pr.Draft, &pr.BaseRef, &pr.HeadRef, &pr.HeadSHA, &pr.HTMLURL,
		&createdAt, &updatedAt, &mergedAt, &closedAt,
		&pr.MergeableState, &pr.ReviewDecision, &pr.Approvals, &pr.ChangesRequested,
		&reviewers, &labels, &pr.Additions, &pr.Deletions, &pr.CIStatus, &syncedAt,
		&assignees, &pr.Fork, &pr.UpdateType, &pr.AuthorAvatarURL)
	if err != nil {
		return PullRequest{}, err
	}
	if pr.CreatedAt, err = timeFromDB(createdAt); err != nil {
		return PullRequest{}, err
	}
	if pr.UpdatedAt, err = timeFromDB(updatedAt); err != nil {
		return PullRequest{}, err
	}
	if pr.SyncedAt, err = timeFromDB(syncedAt); err != nil {
		return PullRequest{}, err
	}
	if pr.MergedAt, err = timePtrFromDB(mergedAt); err != nil {
		return PullRequest{}, err
	}
	if pr.ClosedAt, err = timePtrFromDB(closedAt); err != nil {
		return PullRequest{}, err
	}
	if err := json.Unmarshal([]byte(reviewers), &pr.RequestedReviewers); err != nil {
		return PullRequest{}, fmt.Errorf("decode requested reviewers: %w", err)
	}
	if err := json.Unmarshal([]byte(labels), &pr.Labels); err != nil {
		return PullRequest{}, fmt.Errorf("decode labels: %w", err)
	}
	if err := json.Unmarshal([]byte(assignees), &pr.Assignees); err != nil {
		return PullRequest{}, fmt.Errorf("decode assignees: %w", err)
	}
	return pr, nil
}

func jsonList(v []string) (string, error) {
	if v == nil {
		v = []string{}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
