package prwatch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/store"
)

var ErrEmptyReply = errors.New("the reply is empty")

var ErrNoSuchComment = errors.New("no such comment")

type ReplyRequest struct {
	InReplyTo int64
	Body      string
}

type ReplyOutcome struct {
	Posted   *store.Activity
	Proposal int
}

func (s *Service) Reply(ctx context.Context, id int64, req ReplyRequest) (ReplyOutcome, error) {
	body := strings.TrimSpace(redact.Text(req.Body))
	if body == "" {
		return ReplyOutcome{}, ErrEmptyReply
	}
	client, err := s.newClient(ctx)
	if err != nil {
		return ReplyOutcome{}, err
	}
	unlock := s.locks.Lock(id)
	defer unlock()
	w, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return ReplyOutcome{}, err
	}
	if w.Status != store.WatchActive {
		return ReplyOutcome{}, ErrWatchStopped
	}
	to, err := s.replyTarget(ctx, client, w, req.InReplyTo)
	if err != nil {
		return ReplyOutcome{}, err
	}
	if !s.gates(w) {
		row, err := s.postNow(ctx, client, w, to, body)
		return ReplyOutcome{Posted: &row}, err
	}
	p, err := s.openTurn(ctx, w)
	if err != nil {
		return ReplyOutcome{}, err
	}
	if _, err := s.store.AddProposalReplyTo(ctx, p.ID, to, body, s.now()); err != nil {
		return ReplyOutcome{}, err
	}
	return ReplyOutcome{Proposal: p.Number}, nil
}

func (s *Service) postNow(ctx context.Context, client *github.Client, w store.Watch, to store.SeenItem, body string) (store.Activity, error) {
	c, err := s.post(ctx, client, w, to, body)
	if err != nil {
		return store.Activity{}, err
	}
	now := s.now()
	if err := s.store.MarkReviewItemsSeen(ctx, w.Key(), []store.SeenItem{{Kind: c.kind, ID: c.id}}, now); err != nil {
		return store.Activity{}, err
	}
	return s.record(ctx, w, repliedRow(w, to, body, byAgent, c, now))
}

const (
	byAgent  = "the agent"
	byAuthor = "you"
)

func repliedRow(w store.Watch, to store.SeenItem, body, writer string, c posted, at time.Time) store.Activity {
	return store.Activity{
		Kind: store.ActivityReplied, Ref: fmt.Sprint(c.id), At: at, Actor: w.BotLogin,
		Summary: replySummary(to, body, writer), URL: c.url,
		Payload: mustJSON(map[string]any{"comment_id": c.id, "kind": c.kind, "in_reply_to": to.ID, "in_reply_kind": to.Kind, "body": body}),
	}
}

func (s *Service) replyTarget(ctx context.Context, client *github.Client, w store.Watch, id int64) (store.SeenItem, error) {
	if id == 0 {
		return store.SeenItem{}, nil
	}
	review, resp, err := ghclient.GetReviewComment(ctx, client, w.Owner, w.Name, id)
	err = s.guard.After(ctx, resp, err)
	switch {
	case err == nil:
		return targetOn(w, store.SeenItem{Kind: store.KindReviewComment, ID: id}, review.GetPullRequestURL())
	case !ghclient.IsNotFound(err):
		return store.SeenItem{}, err
	}
	conversation, resp, err := ghclient.GetIssueComment(ctx, client, w.Owner, w.Name, id)
	err = s.guard.After(ctx, resp, err)
	switch {
	case ghclient.IsNotFound(err):
		return store.SeenItem{}, noSuchComment(id)
	case err != nil:
		return store.SeenItem{}, err
	}
	return targetOn(w, store.SeenItem{Kind: store.KindIssueComment, ID: id}, conversation.GetIssueURL())
}

func targetOn(w store.Watch, to store.SeenItem, url string) (store.SeenItem, error) {
	if path.Base(url) == strconv.Itoa(w.Number) {
		return to, nil
	}
	return store.SeenItem{}, fmt.Errorf("%w: comment %d is on another pull request", ErrNoSuchComment, to.ID)
}

func noSuchComment(id int64) error {
	return fmt.Errorf("%w: comment %d is deleted, or it was never on the pull request", ErrNoSuchComment, id)
}

func (s *Service) noReplyTarget(ctx context.Context, client *github.Client, w store.Watch, inReplyTo int64, refused error) error {
	if _, err := s.replyTarget(ctx, client, w, inReplyTo); errors.Is(err, ErrNoSuchComment) {
		return err
	}
	return refused
}

type posted struct {
	kind store.ReviewItemKind
	id   int64
	url  string
}

func replySummary(to store.SeenItem, body, writer string) string {
	switch {
	case inThread(to):
		return fmt.Sprintf("%s replied in the thread of comment %d: %s", writer, to.ID, firstLine(body))
	case to.ID != 0:
		return fmt.Sprintf("%s answered comment %d on the conversation: %s", writer, to.ID, firstLine(body))
	}
	return writer + " commented: " + firstLine(body)
}

func inThread(to store.SeenItem) bool {
	return to.ID != 0 && to.Kind != store.KindIssueComment
}

func (s *Service) post(ctx context.Context, client *github.Client, w store.Watch, to store.SeenItem, body string) (posted, error) {
	if inThread(to) {
		reply, resp, err := ghclient.ReplyToReviewComment(ctx, client, w.Owner, w.Name, w.Number, to.ID, body)
		if err := s.afterWrite(ctx, resp, err); err != nil {
			if ghclient.IsNotFound(err) {
				return posted{}, noSuchComment(to.ID)
			}
			if ghclient.IsNoReplyTarget(err) {
				return posted{}, s.noReplyTarget(ctx, client, w, to.ID, err)
			}
			return posted{}, err
		}
		return posted{kind: store.KindReviewComment, id: reply.GetID(), url: reply.GetHTMLURL()}, nil
	}
	comment, resp, err := ghclient.CommentOnPull(ctx, client, w.Owner, w.Name, w.Number, body)
	if err := s.afterWrite(ctx, resp, err); err != nil {
		return posted{}, err
	}
	return posted{kind: store.KindIssueComment, id: comment.GetID(), url: comment.GetHTMLURL()}, nil
}

func (s *Service) replyCommand(w store.Watch) string {
	return s.watchCommand(w, "reply")
}

func (s *Service) watchCommand(w store.Watch, verb string) string {
	return fmt.Sprintf("%s watch %s %d", agent.ShellWord(cmp.Or(s.exe, "babysitter")), verb, w.ID)
}
