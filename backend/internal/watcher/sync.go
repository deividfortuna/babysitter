package watcher

import (
	"context"
	"strings"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (w *Watcher) syncRepo(ctx context.Context, c *github.Client, repo store.Repo) error {
	stored, err := w.store.OpenPRs(ctx, repo.ID)
	if err != nil {
		return err
	}
	prev := make(map[int]store.PullRequest, len(stored))
	for _, pr := range stored {
		prev[pr.Number] = pr
	}

	remote, resp, err := ghclient.ListOpenPulls(ctx, c, repo.Owner, repo.Name)
	if err := w.afterCall(ctx, resp, err); err != nil {
		return err
	}

	seen := make(map[int]bool, len(remote))
	live := make(map[checkKey]bool, len(remote))
	for _, pr := range remote {
		seen[pr.GetNumber()] = true
		live[checkKey{repo.ID, pr.GetNumber(), pr.GetHead().GetSHA()}] = true
		if err := w.syncOpenPR(ctx, c, repo, pr, prev); err != nil {
			return err
		}
	}
	for number, old := range prev {
		if seen[number] {
			continue
		}
		if err := w.syncGonePR(ctx, c, repo, old); err != nil {
			return err
		}
	}
	w.forgetChecks(repo.ID, live)
	return nil
}

func (w *Watcher) syncOpenPR(ctx context.Context, c *github.Client, repo store.Repo, pr *github.PullRequest, prev map[int]store.PullRequest) error {
	now := w.now()
	old, known := prev[pr.GetNumber()]
	cur := toStorePR(repo, pr, old, now)

	changed := !known ||
		!old.UpdatedAt.Equal(pr.GetUpdatedAt().Time) ||
		old.HeadSHA != pr.GetHead().GetSHA() ||
		!old.MergeableState.Known()

	if changed {
		detail, resp, err := ghclient.GetPull(ctx, c, repo.Owner, repo.Name, pr.GetNumber())
		if err := w.afterCall(ctx, resp, err); err != nil {
			return err
		}
		cur.MergeableState = store.MergeableState(detail.GetMergeableState())
		cur.Additions = detail.GetAdditions()
		cur.Deletions = detail.GetDeletions()

		reviews, resp, err := ghclient.ListReviews(ctx, c, repo.Owner, repo.Name, pr.GetNumber())
		if err := w.afterCall(ctx, resp, err); err != nil {
			return err
		}
		cur.ReviewDecision, cur.Approvals, cur.ChangesRequested = ReviewDecision(reviews, cur.Author, len(cur.RequestedReviewers))
	}

	status, err := w.syncChecks(ctx, c, repo, cur, now)
	if err != nil {
		return err
	}
	cur.CIStatus = status

	return w.store.UpsertPR(ctx, cur)
}

func (w *Watcher) syncChecks(ctx context.Context, c *github.Client, repo store.Repo, pr store.PullRequest, now time.Time) (checks.CIStatus, error) {
	if !w.checksDue(pr, now) {
		return pr.CIStatus, nil
	}
	runs, resp, err := ghclient.ListCheckRuns(ctx, c, repo.Owner, repo.Name, pr.HeadSHA)
	if err := w.afterCall(ctx, resp, err); err != nil {
		return "", err
	}
	combined, resp, err := ghclient.GetCombinedStatus(ctx, c, repo.Owner, repo.Name, pr.HeadSHA)
	if err := w.afterCall(ctx, resp, err); err != nil {
		return "", err
	}
	status := checks.Overall(runs, combined)
	w.markChecked(pr, status, w.now())
	return status, nil
}

func (w *Watcher) checksDue(pr store.PullRequest, now time.Time) bool {
	w.checksMu.Lock()
	defer w.checksMu.Unlock()
	last, ok := w.checked[checkKeyOf(pr)]
	return !ok || now.Add(w.Interval()/2).Sub(last.at) >= last.wait
}

func (w *Watcher) markChecked(pr store.PullRequest, status checks.CIStatus, now time.Time) {
	key := checkKeyOf(pr)
	w.checksMu.Lock()
	defer w.checksMu.Unlock()
	w.checked[key] = checkRead{at: now, wait: w.nextCheckWait(w.checked[key], status), status: status}
}

func (w *Watcher) nextCheckWait(last checkRead, status checks.CIStatus) time.Duration {
	switch {
	case status != checks.CIPending:
		return w.checkTTL
	case last.status != checks.CIPending:
		return w.checkWaitFloor()
	default:
		return min(2*last.wait, w.longestCheckWait)
	}
}

func (w *Watcher) checkWaitFloor() time.Duration {
	return min(firstCheckWait, w.Interval(), w.longestCheckWait)
}

func (w *Watcher) forgetPendingChecks() {
	w.checksMu.Lock()
	defer w.checksMu.Unlock()
	for key, last := range w.checked {
		if last.status == checks.CIPending {
			delete(w.checked, key)
		}
	}
}

func checkKeyOf(pr store.PullRequest) checkKey {
	return checkKey{pr.RepoID, pr.Number, pr.HeadSHA}
}

func (w *Watcher) forgetChecks(repoID int64, live map[checkKey]bool) {
	w.checksMu.Lock()
	defer w.checksMu.Unlock()
	for key := range w.checked {
		if key.repoID == repoID && !live[key] {
			delete(w.checked, key)
		}
	}
}

func (w *Watcher) syncGonePR(ctx context.Context, c *github.Client, repo store.Repo, old store.PullRequest) error {
	cur := old
	cur.SyncedAt = w.now()

	detail, resp, err := ghclient.GetPull(ctx, c, repo.Owner, repo.Name, old.Number)
	switch {
	case ghclient.IsNotFound(err):
		now := w.now()
		cur.State = store.StateClosed
		cur.ClosedAt = &now
	case err != nil:
		return w.afterCall(ctx, resp, err)
	default:
		cur = toStorePR(repo, detail, old, w.now())
		cur.MergeableState = store.MergeableState(detail.GetMergeableState())
		cur.Additions = detail.GetAdditions()
		cur.Deletions = detail.GetDeletions()
		switch {
		case detail.GetMerged():
			cur.State = store.StateMerged
		case store.PRState(detail.GetState()) == store.StateOpen:
			cur.State = store.StateOpen
		default:
			cur.State = store.StateClosed
		}
	}
	if err := w.afterCall(ctx, resp, nil); err != nil {
		return err
	}
	return w.store.UpsertPR(ctx, cur)
}

func toStorePR(repo store.Repo, pr *github.PullRequest, old store.PullRequest, now time.Time) store.PullRequest {
	cur := store.PullRequest{
		RepoID:             repo.ID,
		RepoFullName:       repo.FullName(),
		Number:             pr.GetNumber(),
		GitHubID:           pr.GetID(),
		Title:              pr.GetTitle(),
		Author:             pr.GetUser().GetLogin(),
		State:              store.StateOpen,
		Draft:              pr.GetDraft(),
		BaseRef:            pr.GetBase().GetRef(),
		HeadRef:            pr.GetHead().GetRef(),
		HeadSHA:            pr.GetHead().GetSHA(),
		HTMLURL:            pr.GetHTMLURL(),
		CreatedAt:          pr.GetCreatedAt().Time,
		UpdatedAt:          pr.GetUpdatedAt().Time,
		MergeableState:     old.MergeableState,
		ReviewDecision:     old.ReviewDecision,
		Approvals:          old.Approvals,
		ChangesRequested:   old.ChangesRequested,
		RequestedReviewers: make([]string, 0, len(pr.RequestedReviewers)),
		Labels:             make([]string, 0, len(pr.Labels)),
		Additions:          old.Additions,
		Deletions:          old.Deletions,
		CIStatus:           old.CIStatus,
		SyncedAt:           now,
		Assignees:          make([]string, 0, len(pr.Assignees)),
		Fork:               isFork(pr),
		UpdateType:         UpdateTypeOf(pr),
	}
	if t := pr.GetMergedAt(); !t.IsZero() {
		cur.MergedAt = &t.Time
	}
	if t := pr.GetClosedAt(); !t.IsZero() {
		cur.ClosedAt = &t.Time
	}
	for _, u := range pr.RequestedReviewers {
		if login := u.GetLogin(); login != "" {
			cur.RequestedReviewers = append(cur.RequestedReviewers, login)
		}
	}
	for _, l := range pr.Labels {
		if name := l.GetName(); name != "" {
			cur.Labels = append(cur.Labels, name)
		}
	}
	for _, u := range pr.Assignees {
		if login := u.GetLogin(); login != "" {
			cur.Assignees = append(cur.Assignees, login)
		}
	}
	return cur
}

func isFork(pr *github.PullRequest) bool {
	head := pr.GetHead().GetRepo().GetFullName()
	return head == "" || !strings.EqualFold(head, pr.GetBase().GetRepo().GetFullName())
}

func UpdateTypeOf(pr *github.PullRequest) dependabot.Level {
	if !agent.IsDependabot(pr.GetUser().GetLogin()) {
		return ""
	}
	return dependabot.UpdateType(pr.GetTitle(), pr.GetBody())
}

func (w *Watcher) afterCall(ctx context.Context, resp *github.Response, err error) error {
	return w.guard.After(ctx, resp, err)
}
