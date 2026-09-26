package prwatch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/ghclient"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
	"github.com/deividfortuna/babysitter/internal/textx"
)

const maxRereviewTries = 3

func (s *Service) rereview(ctx context.Context, client *github.Client, w store.Watch, snap *snapshot.Snapshot, state agentStatus) error {
	if !state.done() {
		return nil
	}
	if len(snap.PR.RequestedReviewers) > 0 {
		return nil
	}
	if !snapshot.WaitsOnlyForReview(snap, w.ApprovalsRequired) {
		return nil
	}
	logins := rereviewers(snapshot.Rereviewers(snap, w.ApprovalsRequired), w.BotLogin)
	if len(logins) == 0 {
		return nil
	}
	round := rereviewRound{headSHA: snap.PR.HeadSHA, lastAnswer: snap.Threads.LastAnswer}
	asked, err := s.store.HasActivity(ctx, w.ID, store.ActivityReviewRequested, round.ref())
	if err != nil {
		return err
	}
	if asked {
		return nil
	}

	out, err := s.requestReviewers(ctx, client, w, logins)
	snap.PR.RequestedReviewers = out.asked
	if err != nil {
		return s.rereviewFailed(ctx, w, round, out, logins, err)
	}

	_, err = s.record(ctx, w, rereviewRow(w, round, out))
	return err
}

type rereviewRound struct {
	headSHA    string
	lastAnswer int64
}

func (r rereviewRound) ref() string {
	if r.lastAnswer == 0 {
		return "rereview@" + r.headSHA
	}
	return fmt.Sprintf("rereview@%s#%d", r.headSHA, r.lastAnswer)
}

type reviewRequest struct {
	asked     []string
	refused   []string
	cause     string
	unreached []string
	failure   string
}

func (r reviewRequest) answered() bool { return len(r.asked)+len(r.refused) > 0 }

func (r reviewRequest) summary(headSHA string) string {
	var parts []string
	if len(r.asked) > 0 {
		parts = append(parts, fmt.Sprintf("asked %s for a new review of %s", strings.Join(r.asked, ", "), textx.ShortSHA(headSHA)))
	}
	if len(r.refused) > 0 {
		parts = append(parts, fmt.Sprintf("could not ask %s: %s", strings.Join(r.refused, ", "), firstLine(r.cause)))
	}
	if len(r.unreached) > 0 {
		parts = append(parts, fmt.Sprintf("did not reach %s: %s", strings.Join(r.unreached, ", "), firstLine(r.failure)))
	}
	return strings.Join(parts, "; ")
}

func (r reviewRequest) withFailure(logins []string, cause string) reviewRequest {
	out := r
	out.failure = cause
	for _, login := range logins {
		called := slices.Contains(out.asked, login) || slices.Contains(out.refused, login)
		if !called {
			out.unreached = append(out.unreached, login)
		}
	}
	return out
}

func (s *Service) requestReviewers(ctx context.Context, client *github.Client, w store.Watch, logins []string) (reviewRequest, error) {
	resp, err := ghclient.RequestReviewers(ctx, client, w.Owner, w.Name, w.Number, logins)
	err = s.afterWrite(ctx, resp, err)
	switch {
	case err == nil:
		return reviewRequest{asked: logins}, nil
	case !ghclient.IsRefused(err):
		return reviewRequest{}, err
	case len(logins) == 1:
		return reviewRequest{refused: logins, cause: redact.Text(err.Error())}, nil
	}

	out := reviewRequest{}
	for _, login := range logins {
		resp, err := ghclient.RequestReviewers(ctx, client, w.Owner, w.Name, w.Number, []string{login})
		switch err = s.afterWrite(ctx, resp, err); {
		case err == nil:
			out.asked = append(out.asked, login)
		case !ghclient.IsRefused(err):
			return out, err
		default:
			out.refused = append(out.refused, login)
			out.cause = redact.Text(err.Error())
		}
	}
	return out, nil
}

func (s *Service) rereviewFailed(ctx context.Context, w store.Watch, round rereviewRound, out reviewRequest, logins []string, cause error) error {
	msg := redact.Text(cause.Error())
	if out.answered() {
		out = out.withFailure(logins, msg)
		s.log.Warn("ask for a new review", "watch", w.ID, "asked", out.asked, "unreached", out.unreached, "err", msg)
		if _, err := s.record(ctx, w, rereviewRow(w, round, out)); err != nil {
			return err
		}
		return rateLimitOf(cause)
	}
	if errors.Is(cause, ghclient.ErrPaused) {
		return cause
	}
	tries := s.rereviewTries.count(w.ID, round.ref())
	s.log.Warn("ask for a new review", "watch", w.ID, "reviewers", logins, "try", tries, "err", msg)
	if tries < maxRereviewTries {
		return nil
	}
	failed := reviewRequest{refused: logins, cause: msg}
	_, err := s.record(ctx, w, rereviewRow(w, round, failed))
	return err
}

func rateLimitOf(cause error) error {
	if errors.Is(cause, ghclient.ErrPaused) {
		return cause
	}
	return nil
}

func rereviewRow(w store.Watch, round rereviewRound, out reviewRequest) store.Activity {
	payload := map[string]any{"reviewers": out.asked, "sha": round.headSHA}
	if len(out.refused) > 0 {
		payload["refused"] = out.refused
		payload["error"] = out.cause
	}
	if len(out.unreached) > 0 {
		payload["unreached"] = out.unreached
		payload["failure"] = out.failure
	}
	return store.Activity{
		Kind: store.ActivityReviewRequested, Ref: round.ref(), Actor: w.BotLogin,
		Summary: out.summary(round.headSHA),
		Payload: mustJSON(payload),
	}
}

func rereviewers(reviewed []string, botLogin string) []string {
	out := slices.Clone(reviewed)
	return slices.DeleteFunc(out, func(login string) bool { return strings.EqualFold(login, botLogin) })
}

type rereviewTries struct {
	registry[map[string]int]
}

func (t *rereviewTries) count(id int64, ref string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	rounds := t.m[id]
	if _, ok := rounds[ref]; !ok {
		rounds = map[string]int{}
		t.store(id, rounds)
	}
	rounds[ref]++
	return rounds[ref]
}
