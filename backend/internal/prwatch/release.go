package prwatch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

var (
	ErrNothingToRetry     = errors.New("only a failed proposal can be retried")
	ErrProposalMoved      = errors.New("the work branch moved since the proposal was made")
	ErrHeadCommitsMissing = errors.New("the pull request branch has commits the work branch lacks")
	ErrRebaseAsks         = errors.New("the pull request branch moved; the next poll rebases the work and offers it again")
)

type releaseError struct {
	what string
	err  error
}

func (e *releaseError) Error() string { return e.what + ": " + e.err.Error() }
func (e *releaseError) Unwrap() error { return e.err }

type headCommitsMissing struct {
	remote  string
	missing []string
}

func (e *headCommitsMissing) Error() string {
	return fmt.Sprintf("%s: %s", ErrHeadCommitsMissing, shortList(e.missing))
}
func (e *headCommitsMissing) Unwrap() error { return ErrHeadCommitsMissing }

type moveConflict struct {
	remote string
	merges bool
	err    *gitrelease.ConflictError
}

func (e *moveConflict) Error() string {
	return fmt.Sprintf("%s conflicts in %s", e.move(), strings.Join(e.err.Files, ", "))
}
func (e *moveConflict) Unwrap() error { return e.err }

func (e *moveConflict) move() string {
	if e.merges {
		return "the merge of " + textx.ShortSHA(e.remote)
	}
	return "the rebase onto " + textx.ShortSHA(e.remote)
}

func (e *moveConflict) action(proposal int) string {
	if e.merges {
		return fmt.Sprintf("merge %s into proposal %d", textx.ShortSHA(e.remote), proposal)
	}
	return fmt.Sprintf("rebase proposal %d", proposal)
}

func (e *moveConflict) of(proposal int) string {
	if e.merges {
		return fmt.Sprintf("the merge of %s into proposal %d", textx.ShortSHA(e.remote), proposal)
	}
	return fmt.Sprintf("the rebase of proposal %d onto %s", proposal, textx.ShortSHA(e.remote))
}

func (s *Service) release(ctx context.Context, w store.Watch, p store.Proposal, retry bool) (failure, err error) {
	failure = s.releaseWork(ctx, w, &p)
	if failure == nil {
		return nil, s.store.SetProposalOutcome(ctx, p.ID, store.ProposalReleased, "", s.now())
	}
	if err := s.releaseFailed(ctx, w, p, failure); err != nil {
		return failure, err
	}
	if goesBack(failure, retry) {
		s.handWork(ctx, w, p, failure)
	}
	return failure, nil
}

func goesBack(failure error, retry bool) bool {
	var conflict *moveConflict
	var missing *headCommitsMissing
	return errors.As(failure, &conflict) || retry && errors.As(failure, &missing)
}

func (s *Service) releaseWork(ctx context.Context, w store.Watch, p *store.Proposal) error {
	if p.Pushes() {
		if err := s.pushWork(ctx, w, p); err != nil {
			return &releaseError{what: fmt.Sprintf("push proposal %d", p.Number), err: err}
		}
	}
	if err := s.postReplies(ctx, w, *p); err != nil {
		return &releaseError{what: fmt.Sprintf("post the replies of proposal %d", p.Number), err: err}
	}
	return nil
}

func (s *Service) pushWork(ctx context.Context, w store.Watch, p *store.Proposal) error {
	dir := w.WorktreeDir
	work, err := s.rel.Head(ctx, dir)
	if err != nil {
		return err
	}
	if work != p.WorkSHA {
		return fmt.Errorf("%w: it is at %s, the proposal names %s", ErrProposalMoved, textx.ShortSHA(work), textx.ShortSHA(p.WorkSHA))
	}
	remote, err := s.rel.Fetch(ctx, dir, w.HeadRef)
	if err != nil {
		return err
	}
	landed, err := s.rel.Contains(ctx, dir, remote, work)
	if err != nil || landed {
		return err
	}
	fastForward, err := s.rel.Contains(ctx, dir, work, remote)
	if err != nil {
		return err
	}
	if fastForward {
		return s.rel.Push(ctx, dir, gitrelease.Push{SHA: work, Branch: w.HeadRef})
	}
	missing, err := s.rel.Missing(ctx, dir, work, remote, p.BaseSHA)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return s.rel.Push(ctx, dir, gitrelease.Push{SHA: work, Branch: w.HeadRef, Lease: remote})
	}
	movable, err := s.movable(ctx, w, dir, p.HeadSHA, work, remote)
	if err != nil {
		return err
	}
	if !movable {
		return &headCommitsMissing{remote: remote, missing: missing}
	}
	if rebaseAsks(w) {
		return fmt.Errorf("%w: it is at %s", ErrRebaseAsks, textx.ShortSHA(remote))
	}
	return s.rebaseWork(ctx, w, p, remote)
}

func rebaseAsks(w store.Watch) bool {
	return w.Asks() && !w.AutoApproveRebase
}

func (s *Service) movable(ctx context.Context, w store.Watch, dir, head, work, remote string) (bool, error) {
	workOnHead, err := s.rel.Contains(ctx, dir, work, head)
	if err != nil || !workOnHead {
		return false, err
	}
	if w.BranchUpdate != store.BranchMerge {
		merges, err := s.rel.HasMerges(ctx, dir, head, work)
		if err != nil || merges {
			return false, err
		}
	}
	return s.rel.Contains(ctx, dir, remote, head)
}

func (s *Service) rebaseWork(ctx context.Context, w store.Watch, p *store.Proposal, remote string) error {
	if err := s.moveWork(ctx, w, p, remote); err != nil {
		return err
	}
	return s.rel.Push(ctx, w.WorktreeDir, gitrelease.Push{SHA: p.WorkSHA, Branch: w.HeadRef})
}

func (s *Service) moveWork(ctx context.Context, w store.Watch, p *store.Proposal, remote string) error {
	work, err := s.moveOnto(ctx, w, *p, remote)
	if err != nil {
		return err
	}
	if err := s.store.MoveProposal(ctx, p.ID, remote, remote, work); err != nil {
		return err
	}
	p.HeadSHA, p.BaseSHA, p.WorkSHA = remote, remote, work
	return nil
}

func (s *Service) moveOnto(ctx context.Context, w store.Watch, p store.Proposal, remote string) (string, error) {
	merges := w.BranchUpdate == store.BranchMerge
	move := s.rel.Rebase
	if merges {
		move = s.rel.Merge
	}
	if err := move(ctx, w.WorktreeDir, remote); err != nil {
		if conflict, ok := errors.AsType[*gitrelease.ConflictError](err); ok {
			return "", &moveConflict{remote: remote, merges: merges, err: conflict}
		}
		return "", err
	}
	work, err := s.rel.Head(ctx, w.WorktreeDir)
	if err != nil {
		return "", err
	}
	if merges {
		return work, nil
	}
	if err := s.renameReplies(ctx, w, p, remote, work); err != nil {
		return "", err
	}
	return work, nil
}

func (s *Service) renameReplies(ctx context.Context, w store.Watch, p store.Proposal, head, work string) error {
	replies, err := s.store.ProposalReplies(ctx, p.ID)
	if err != nil {
		return err
	}
	waiting := unsent(replies)
	if len(waiting) == 0 {
		return nil
	}
	before, err := s.rel.Log(ctx, w.WorktreeDir, p.HeadSHA, p.WorkSHA)
	if err != nil {
		return err
	}
	after, err := s.rel.Log(ctx, w.WorktreeDir, head, work)
	if err != nil {
		return err
	}
	renames := renamed(before, after)
	for _, r := range waiting {
		body, edited := renameCommits(r.Body, renames), renameCommits(r.Edited, renames)
		if body == r.Body && edited == r.Edited {
			continue
		}
		if err := s.store.RewriteReply(ctx, p.ID, r.ID, body, edited); err != nil {
			return err
		}
	}
	return nil
}

func renamed(before, after []gitrelease.Commit) map[string]string {
	out := make(map[string]string, len(before))
	if len(before) == len(after) {
		for i, c := range before {
			out[c.SHA] = after[i].SHA
		}
		return out
	}
	next := 0
	for _, c := range before {
		for j := next; j < len(after); j++ {
			if after[j].Subject == c.Subject {
				out[c.SHA] = after[j].SHA
				next = j + 1
				break
			}
		}
	}
	return out
}

const minAbbrev = 7

var wordRE = regexp.MustCompile(`[[:alnum:]]+`)

func renameCommits(text string, renames map[string]string) string {
	return wordRE.ReplaceAllStringFunc(text, func(word string) string {
		for old, neu := range renames {
			if word == old {
				return neu
			}
			if abbreviates(word, old) {
				return neu[:min(len(word), len(neu))]
			}
		}
		return word
	})
}

func abbreviates(word, sha string) bool {
	return len(word) >= minAbbrev && strings.HasPrefix(sha, strings.ToLower(word))
}

func (s *Service) postReplies(ctx context.Context, w store.Watch, p store.Proposal) error {
	replies, err := s.store.ProposalReplies(ctx, p.ID)
	if err != nil {
		return err
	}
	pending := unsent(replies)
	if len(pending) == 0 {
		return nil
	}
	client, err := s.newClient(ctx)
	if err != nil {
		return err
	}
	for _, r := range pending {
		c, err := s.post(ctx, client, w, r.Target(), r.Text())
		if s.neverPosts(ctx, client, w, r.InReplyTo, err) {
			if err := s.dropReply(ctx, w, r, err); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			if serr := s.store.SetReplyError(ctx, r.ID, redact.Text(err.Error())); serr != nil {
				return errors.Join(err, serr)
			}
			return err
		}
		if err := s.recordPosted(ctx, w, r, c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recordPosted(ctx context.Context, w store.Watch, r store.ProposalReply, c posted) error {
	now := s.now()
	if err := s.store.MarkReplyPosted(ctx, r.ID, c.kind, c.id, c.url, now); err != nil {
		return err
	}
	if err := s.store.MarkReviewItemsSeen(ctx, w.Key(), []store.SeenItem{{Kind: c.kind, ID: c.id}}, now); err != nil {
		return err
	}
	_, err := s.record(ctx, w, repliedRow(w, r.Target(), r.Text(), writerOf(r), c, now))
	return err
}

func writerOf(r store.ProposalReply) string {
	if r.Edited != "" {
		return byAuthor
	}
	return byAgent
}

func unsent(replies []store.ProposalReply) []store.ProposalReply {
	var out []store.ProposalReply
	for _, r := range replies {
		if r.Waiting() {
			out = append(out, r)
		}
	}
	return out
}

func (s *Service) neverPosts(ctx context.Context, client *github.Client, w store.Watch, inReplyTo int64, err error) bool {
	return errors.Is(err, ErrNoSuchComment) && s.commentGone(ctx, client, w, inReplyTo)
}

func (s *Service) commentGone(ctx context.Context, client *github.Client, w store.Watch, id int64) bool {
	_, resp, err := ghclient.GetPull(ctx, client, w.Owner, w.Name, w.Number)
	if err := s.guard.After(ctx, resp, err); err != nil {
		return false
	}
	_, resp, err = ghclient.GetReviewComment(ctx, client, w.Owner, w.Name, id)
	return ghclient.IsNotFound(s.guard.After(ctx, resp, err))
}

func (s *Service) dropReply(ctx context.Context, w store.Watch, r store.ProposalReply, failure error) error {
	msg := redact.Text(failure.Error())
	now := s.now()
	if err := s.store.DropReply(ctx, r.ID, msg, now); err != nil {
		return err
	}
	what := fmt.Sprintf("post the reply to comment %d", r.InReplyTo)
	s.log.Warn("reply dropped", "watch", w.ID, "in_reply_to", r.InReplyTo, "err", msg)
	_, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityAgentFailed, Ref: stampRef(fmt.Sprintf("reply %d", r.ID), now),
		Summary: fmt.Sprintf("could not %s: %s; the reply is dropped", what, firstLine(msg)),
		Payload: mustJSON(map[string]any{"what": what, "error": msg, "body": r.Text(), "by": "daemon"}),
	})
	return err
}

func (s *Service) releaseFailed(ctx context.Context, w store.Watch, p store.Proposal, failure error) error {
	ctx = context.WithoutCancel(ctx)
	msg := redact.Text(failure.Error())
	now := s.now()
	if err := s.store.SetProposalOutcome(ctx, p.ID, store.ProposalFailed, msg, now); err != nil {
		return err
	}
	what := fmt.Sprintf("release proposal %d", p.Number)
	if step, ok := errors.AsType[*releaseError](failure); ok {
		what, msg = step.what, redact.Text(step.err.Error())
	}
	payload := map[string]any{"what": what, "error": msg, "proposal": p.Number, "by": "daemon"}
	next := "the agent resolves it in its next turn"
	if !conflicts(failure) {
		retry := s.retryCommand(w, p.Number)
		payload["retry"] = retry
		next = fmt.Sprintf("retry with `%s`", retry)
	}
	s.log.Error("release failed", "watch", w.ID, "proposal", p.Number, "err", msg)
	_, err := s.record(ctx, w, store.Activity{
		Kind: store.ActivityAgentFailed, Ref: stampRef(fmt.Sprintf("proposal %d", p.Number), now),
		Summary: fmt.Sprintf("could not %s: %s; %s", what, firstLine(msg), next),
		Payload: mustJSON(payload),
	})
	return err
}

func conflicts(failure error) bool {
	var conflict *moveConflict
	return errors.As(failure, &conflict)
}

func (s *Service) retryCommand(w store.Watch, n int) string {
	return s.proposalCommand("retry", w, n)
}

func (s *Service) proposalCommand(sub string, w store.Watch, n int) string {
	return fmt.Sprintf("%s watch %s %d %d", agent.ShellWord(cmp.Or(s.exe, "babysitter")), sub, w.ID, n)
}

func (s *Service) Retry(ctx context.Context, id int64, number int) (store.Proposal, error) {
	unlock := s.locks.Lock(id)
	defer unlock()
	w, err := s.activeGatedWatch(ctx, id)
	if err != nil {
		return store.Proposal{}, err
	}
	p, err := s.store.GetProposal(ctx, w.ID, number)
	if err != nil {
		return store.Proposal{}, err
	}
	if p.Status != store.ProposalFailed {
		return store.Proposal{}, notRetryable(p)
	}
	if neverApproved(w, p) {
		return store.Proposal{}, fmt.Errorf("%w: you never approved proposal %d, and the watch asks before work goes out", ErrNothingToRetry, p.Number)
	}
	if _, err := s.release(ctx, w, p, true); err != nil {
		return store.Proposal{}, err
	}
	return s.store.GetProposal(ctx, w.ID, number)
}

func neverApproved(w store.Watch, p store.Proposal) bool {
	return w.Asks() && p.ApprovedAt == nil
}

func notRetryable(p store.Proposal) error {
	if p.Status == store.ProposalSuperseded {
		return fmt.Errorf("%w: proposal %d is superseded: the next turn of the agent took its work over", ErrNothingToRetry, p.Number)
	}
	return fmt.Errorf("%w: proposal %d is %s", ErrNothingToRetry, p.Number, p.Status)
}

func (s *Service) handWork(ctx context.Context, w store.Watch, p store.Proposal, failure error) {
	c, summary, remote := handWorkOf(w, p, failure)
	if err := s.store.MoveProposal(ctx, p.ID, remote, p.BaseSHA, p.WorkSHA); err != nil {
		s.agentFailed(ctx, w, "record the head the agent has to build on", err)
		return
	}
	text, err := agent.ConflictMessage(c)
	if err != nil {
		s.agentFailed(ctx, w, "tell the agent about work the daemon cannot rebase", err)
		return
	}
	if _, err := s.deliver(ctx, w, text, summary, deliverRoutine, nil); err != nil {
		s.agentFailed(ctx, w, "tell the agent about work the daemon cannot rebase", err)
	}
}

func handWorkOf(w store.Watch, p store.Proposal, failure error) (c agent.Conflict, summary, remote string) {
	c = agent.Conflict{PR: pullRequestOf(w), Proposal: p.Number}
	if conflict, ok := errors.AsType[*moveConflict](failure); ok {
		c.Remote, c.Files = textx.ShortSHA(conflict.remote), strings.Join(conflict.err.Files, ", ")
		return c, fmt.Sprintf("told the agent %s conflicts", conflict.of(p.Number)), conflict.remote
	}
	var missing *headCommitsMissing
	errors.As(failure, &missing)
	c.Remote, c.Missing = textx.ShortSHA(missing.remote), shortList(missing.missing)
	return c, fmt.Sprintf("told the agent proposal %d lacks %s of %s", p.Number, c.Missing, c.Remote), missing.remote
}

func shortList(shas []string) string {
	out := make([]string, 0, len(shas))
	for _, sha := range shas {
		out = append(out, textx.ShortSHA(sha))
	}
	return strings.Join(out, ", ")
}
