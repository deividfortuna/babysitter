package prwatch

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-github/v91/github"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
)

func TestReplyGoesThroughTheDaemonAndStaysHidden(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.oldReviewComment()
	ctx := context.Background()
	req := fx.startRequest()
	req.IncludeOwn = new(true)
	w, err := fx.svc.Start(ctx, req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.reply(w, 31, "done, see 1a2b3c\nverified with go test ./...")
	fx.reply(w, 0, "The failure is on main too.")
	fx.hook(w, agent.EventStop, `{}`)
	if got := fx.posted(); len(got) != 2 || got[0] != "31:done, see 1a2b3c\nverified with go test ./..." || got[1] != "0:The failure is on main too." {
		t.Fatalf("posted = %q", got)
	}
	var reply ghfake.ReviewComment
	fx.update(func() { reply = fx.pr.ReviewComments[len(fx.pr.ReviewComments)-1] })
	rows := fx.activity(w)
	thread, general := rows[len(rows)-2], rows[len(rows)-1]
	if thread.Kind != store.ActivityReplied || reply.InReplyTo != 31 || thread.URL != reply.URL || thread.Actor != "alice" || thread.Summary != "the agent replied in the thread of comment 31: done, see 1a2b3c" {
		t.Fatalf("row = %+v", thread)
	}
	if general.Summary != "the agent commented: The failure is on main too." {
		t.Fatalf("row = %+v", general)
	}

	fx.update(func() {
		fx.pr.IssueComments = append(fx.pr.IssueComments, ghfake.Comment{ID: 14, Author: "alice", CreatedAt: ghfake.At("2026-09-07T12:07:00Z"), Body: "One more thought", URL: "https://c/14"})
	})
	fx.poll(w)
	equal(t, fx.kinds(w), []string{"watch_started", "session_started", "nudged", "replied", "replied", "comment", "nudged"})
	msgs := fx.host.last().messages()
	if last := msgs[len(msgs)-1]; !strings.Contains(last, "One more thought") || strings.Contains(last, "done, see 1a2b3c") {
		t.Fatalf("the agent got its own reply back:\n%s", last)
	}
}

func TestReplyCommandQuotesTheBinary(t *testing.T) {
	t.Parallel()
	w := store.Watch{ID: 7}
	spaced := &Service{exe: "/Users/alice/Library/Application Support/babysitter/babysitter"}
	if got := spaced.replyCommand(w); got != `'/Users/alice/Library/Application Support/babysitter/babysitter' watch reply 7` {
		t.Fatalf("reply command = %s", got)
	}
	plain := &Service{exe: "/usr/local/bin/babysitter"}
	if got := plain.replyCommand(w); got != "/usr/local/bin/babysitter watch reply 7" {
		t.Fatalf("reply command = %s", got)
	}
	if got := (&Service{}).replyCommand(w); got != "babysitter watch reply 7" {
		t.Fatalf("reply command without a binary = %s", got)
	}
}

func TestReplyRedactsASecretBeforeItIsPosted(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	token := "ghp_" + strings.Repeat("a", 36)
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)
	fx.reply(w, 0, "the job log has "+token+" in it")
	replies, err := fx.st.ProposalReplies(ctx, fx.proposal(w, 1).ID)
	if err != nil || len(replies) != 1 || strings.Contains(replies[0].Body, token) {
		t.Fatalf("recorded replies = %+v, %v", replies, err)
	}
	fx.hook(w, agent.EventStop, `{}`)
	row := fx.activity(w)[len(fx.activity(w))-1]
	for _, got := range append(fx.posted(), row.Summary, string(row.Payload)) {
		if strings.Contains(got, token) {
			t.Fatalf("the token reached %q", got)
		}
	}
	if posted := fx.posted(); len(posted) != 1 || !strings.Contains(posted[0], "[redacted]") {
		t.Fatalf("posted = %q", posted)
	}
}

func TestTheReplyOfASelfWatchIsPostedAtOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.oldReviewComment()
	w := fx.startSelf()
	out := fx.reply(w, 31, "renamed it")
	if out.Posted == nil || out.Posted.Kind != store.ActivityReplied || out.Posted.URL != fx.newestComment().URL || out.Proposal != 0 {
		t.Fatalf("Reply() = %+v", out)
	}
	if got := fx.posted(); len(got) != 1 || got[0] != "31:renamed it" {
		t.Fatalf("posted = %q", got)
	}
	if ps := fx.proposals(w); len(ps) != 0 {
		t.Fatalf("a self watch has proposals: %+v", ps)
	}
}

func TestReplyRefusals(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	if _, err := fx.svc.Reply(ctx, w.ID, ReplyRequest{Body: "  "}); !errors.Is(err, ErrEmptyReply) {
		t.Fatalf("empty reply error = %v", err)
	}
	if _, err := fx.svc.Reply(ctx, w.ID, ReplyRequest{InReplyTo: 404, Body: "hi"}); !errors.Is(err, ErrNoSuchComment) {
		t.Fatalf("reply to a missing comment error = %v", err)
	}
	if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Reply(ctx, w.ID, ReplyRequest{Body: "hi"}); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("reply on a stopped watch error = %v", err)
	}
	if got := fx.posted(); len(got) != 0 {
		t.Fatalf("posted = %q", got)
	}
	if ps := fx.proposals(w); len(ps) != 0 {
		t.Fatalf("a refused reply left %+v", ps)
	}
}

func TestReplyToAConversationCommentOfAnotherPullRequestIsRefused(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()

	_, err := fx.svc.Reply(context.Background(), w.ID, ReplyRequest{InReplyTo: 556, Body: "answered"})
	if !errors.Is(err, ErrNoSuchComment) || !strings.Contains(err.Error(), "another pull request") {
		t.Fatalf("Reply() error = %v, want ErrNoSuchComment for another pull request", err)
	}
}

func TestASelfWatchAnswersAConversationCommentOnTheConversation(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.conversation()
	req := fx.startRequest()
	req.Provider = ProviderSelf
	w, err := fx.svc.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	out, err := fx.svc.Reply(context.Background(), w.ID, ReplyRequest{InReplyTo: 422, Body: "answered"})
	if err != nil || out.Posted == nil {
		t.Fatalf("Reply() = %+v, %v", out, err)
	}
	if got := fx.posted(); !slices.Equal(got, []string{"0:answered"}) {
		t.Fatalf("posted = %q", got)
	}
}

func TestAReplyToADeletedCommentDoesNotHoldBackTheOthers(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.turn(w)
	fx.rel.commit("abc", "w1")
	fx.reply(w, 31, "renamed it")
	fx.reply(w, 0, "also fixed the lint")
	fx.update(func() { fx.repo.Deleted = []int64{31} })

	fx.hook(w, agent.EventStop, `{}`)
	if got := fx.posted(); len(got) != 1 || got[0] != "0:also fixed the lint" {
		t.Fatalf("posted = %q", got)
	}
	p := fx.proposal(w, 1)
	if p.Status != store.ProposalReleased {
		t.Fatalf("proposal = %+v", p)
	}
	replies, err := fx.st.ProposalReplies(ctx, p.ID)
	if err != nil || len(replies) != 2 || replies[0].DroppedAt == nil || !strings.Contains(replies[0].Error, "no such comment") {
		t.Fatalf("replies = %+v, %v", replies, err)
	}
	var dropped bool
	for _, row := range fx.activity(w) {
		dropped = dropped || row.Kind == store.ActivityAgentFailed && strings.Contains(row.Summary, "comment 31")
	}
	if !dropped {
		t.Fatalf("no row says the reply to comment 31 was dropped: %v", fx.kinds(w))
	}
}

func TestReplyToACommentOnAnotherPullRequestIsRefused(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	w := fx.start()
	fx.hook(w, agent.EventUserPromptSubmit, `{}`)

	_, err := fx.svc.Reply(context.Background(), w.ID, ReplyRequest{InReplyTo: 555, Body: "renamed it"})
	if !errors.Is(err, ErrNoSuchComment) || !strings.Contains(err.Error(), "another pull request") {
		t.Fatalf("Reply() error = %v, want ErrNoSuchComment for another pull request", err)
	}
	replies, _ := fx.st.ProposalReplies(context.Background(), fx.proposal(w, 1).ID)
	if len(replies) != 0 {
		t.Fatalf("recorded replies = %+v", replies)
	}
}

func TestAReplyThatRacesAStopLeavesNoProposal(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	newClient := fx.svc.newClient
	var once sync.Once
	fx.svc.newClient = func(ctx context.Context) (*github.Client, error) {
		once.Do(func() {
			if _, err := fx.svc.Stop(ctx, w.ID, StopOptions{}); err != nil {
				t.Error(err)
			}
		})
		return newClient(ctx)
	}

	if _, err := fx.svc.Reply(ctx, w.ID, ReplyRequest{Body: "done"}); !errors.Is(err, ErrWatchStopped) {
		t.Fatalf("Reply() error = %v, want ErrWatchStopped", err)
	}
	for _, p := range fx.proposals(w) {
		if p.Status.Waiting() {
			t.Fatalf("the stopped watch kept %+v", p)
		}
	}
}

func TestAReplyIsKeptWhenThePullRequestIsOutOfReach(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	ctx := context.Background()
	w := fx.start()
	fx.turn(w)
	fx.reply(w, 31, "renamed it")
	fx.gone("Not Found")

	fx.hook(w, agent.EventStop, `{}`)
	p := fx.proposal(w, 1)
	replies, err := fx.st.ProposalReplies(ctx, p.ID)
	if p.Status != store.ProposalFailed || err != nil || len(replies) != 1 || replies[0].DroppedAt != nil {
		t.Fatalf("proposal = %+v, replies = %+v, %v", p, replies, err)
	}

	fx.gone("")
	if p, err := fx.svc.Retry(ctx, w.ID, 1); err != nil || p.Status != store.ProposalReleased {
		t.Fatalf("Retry() = %+v, %v", p, err)
	}
	if got := fx.posted(); len(got) != 1 || got[0] != "31:renamed it" {
		t.Fatalf("posted = %q", got)
	}
}
