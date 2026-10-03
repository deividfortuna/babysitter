package prwatch

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deividfortuna/babysitter/internal/ghclient/ghfake"
	"github.com/deividfortuna/babysitter/internal/store"
)

var changesFromBob = changesFrom(7, "bob")

func (fx *fixture) answered() {
	fx.update(func() { fx.pr.Reviews = []ghfake.Review{changesFromBob} })
}

// asked lists the reviewers of every review request GitHub received,
// refused ones included.
func (fx *fixture) asked() [][]string {
	fx.t.Helper()
	var out [][]string
	for _, a := range fx.api.Calls(ghfake.RouteRequestReviews) {
		var body struct {
			Reviewers []string `json:"reviewers"`
		}
		a.Decode(fx.t, &body)
		out = append(out, body.Reviewers)
	}
	return out
}

// threads gives the pull request n unresolved threads written by authors,
// whose last comment is lastID.
func threads(n int, lastID int64, authors ...string) []ghfake.Thread {
	out := make([]ghfake.Thread, n)
	for i := range out {
		out[i] = ghfake.Thread{Authors: authors, LastCommentID: lastID}
	}
	return out
}

// refuseReviews makes every review request answer status.
func (fx *fixture) refuseReviews(status int) {
	fx.api.Fail(ghfake.RouteRequestReviews, status, "Reviews may only be requested from collaborators.")
}

func (fx *fixture) reviewRequested(w store.Watch) store.Activity {
	fx.t.Helper()
	var out store.Activity
	for _, a := range fx.activity(w) {
		if a.Kind == store.ActivityReviewRequested {
			out = a
		}
	}
	return out
}

func TestRereviewAsksTheReviewerOfAnEarlierCommit(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.answered()
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	asked := fx.asked()
	if len(asked) != 1 || strings.Join(asked[0], ",") != "bob" {
		t.Fatalf("asked = %v, want bob once", asked)
	}
	row := fx.reviewRequested(w)
	if row.ID == 0 {
		t.Fatalf("no review_requested row in %v", fx.kinds(w))
	}
	if row.Ref != "rereview@abc" {
		t.Fatalf("ref = %q, want the head abc", row.Ref)
	}
	if !strings.Contains(row.Summary, "bob") {
		t.Fatalf("summary = %q, want bob in it", row.Summary)
	}

	fx.agentIdle(w)
	fx.poll(w)
	if asked := fx.asked(); len(asked) != 1 {
		t.Fatalf("asked = %v, want the one request", asked)
	}
}

var commentFromCopilot = reviewOf(8, "COMMENTED", "old", "copilot-pull-request-reviewer[bot]", "3 comments")

func TestRereviewAsksTheReviewerOfThreadsTheAgentAnswered(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() {
		fx.pr.Reviews = []ghfake.Review{commentFromCopilot}
		fx.pr.Threads = threads(3, 0, "copilot-pull-request-reviewer[bot]", "alice")
	})
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	asked := fx.asked()
	if len(asked) != 1 || strings.Join(asked[0], ",") != "copilot-pull-request-reviewer[bot]" {
		t.Fatalf("asked = %v, want copilot once", asked)
	}
}

func TestRereviewLeavesTheApproverOutWhenOnlyAnsweredThreadsAreLeft(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() {
		fx.pr.Reviews = []ghfake.Review{approvalFromBob, commentFromCarol}
		fx.pr.Threads = threads(1, 0, "carol", "alice")
	})
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	asked := fx.asked()
	if len(asked) != 1 || strings.Join(asked[0], ",") != "carol" {
		t.Fatalf("asked = %v, want carol alone: bob approved and wrote in no thread", asked)
	}
}

var commentFromCarol = reviewOf(10, "COMMENTED", "old", "carol", "1 comment")

func TestRereviewAsksOnlyTheThreadWritersThatReviewed(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() {
		fx.pr.Reviews = []ghfake.Review{commentFromBobOnTheHead}
		fx.pr.Threads = threads(1, 0, "bob", "dave", "alice")
	})
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	asked := fx.asked()
	if len(asked) != 1 || strings.Join(asked[0], ",") != "bob" {
		t.Fatalf("asked = %v, want bob alone: dave wrote in the thread but never reviewed", asked)
	}
}

func TestRereviewAsksAgainAfterANewAnswerOnTheSameHead(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() {
		fx.pr.Reviews = []ghfake.Review{commentFromBobOnTheHead}
		fx.pr.Threads = threads(1, 10, "bob", "alice")
	})
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)

	fx.update(func() {
		fx.pr.Requested = nil
		fx.pr.Threads = threads(len(fx.pr.Threads), 20, "bob", "alice", "bob", "alice")
	})
	fx.agentIdle(w)
	fx.poll(w)
	asked := fx.asked()
	if len(asked) != 2 {
		t.Fatalf("asked = %v, want bob once for each answer on head abc", asked)
	}

	fx.agentIdle(w)
	fx.poll(w)
	if asked := fx.asked(); len(asked) != 2 {
		t.Fatalf("asked = %v, want no third request for the same answer", asked)
	}
}

var commentFromBobOnTheHead = reviewOf(9, "COMMENTED", "abc", "bob", "2 comments")

func TestRereviewAsksTheReviewerOfAThreadAnsweredWithoutAPush(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() {
		fx.pr.Reviews = []ghfake.Review{commentFromBobOnTheHead}
		fx.pr.Threads = threads(2, 0, "bob", "alice")
	})
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	asked := fx.asked()
	if len(asked) != 1 || strings.Join(asked[0], ",") != "bob" {
		t.Fatalf("asked = %v, want bob, whose threads the agent answered on the head he reviewed", asked)
	}
}

func TestRereviewAsksOncePerHead(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.answered()
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)

	fx.update(func() { fx.pr.Requested = nil })
	fx.agentIdle(w)
	fx.poll(w)
	if asked := fx.asked(); len(asked) != 1 {
		t.Fatalf("asked = %v, want the one request", asked)
	}
}

func TestRereviewAsksNobody(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		why   string
		busy  bool
		setup func(fx *fixture)
	}{
		{"the pull request has its approval", "it is ready to merge", false, (*fixture).good},
		{"a reviewer was asked already", "the pull request waits on dave", false, func(fx *fixture) {
			fx.answered()
			fx.update(func() { fx.pr.Requested = []string{"dave"} })
		}},
		{"a check failed", "a review is not the only thing left", false, func(fx *fixture) {
			fx.answered()
			fx.update(func() {
				fx.pr.CheckRuns = []ghfake.CheckRun{{ID: 1, Name: "build", Status: "completed", Conclusion: "failure", CheckSuiteID: 501}}
			})
		}},
		{"a thread waits on the agent", "the reviewer wrote last in it", false, func(fx *fixture) {
			fx.answered()
			fx.update(func() { fx.pr.Threads = threads(1, 0, "alice", "bob") })
		}},
		{"nobody reviewed yet", "there is no login to ask", false, func(*fixture) {}},
		{"the agent works", "it may still push", true, (*fixture).answered},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			c.setup(fx)
			w := fx.startWorking()
			if !c.busy {
				fx.agentIdle(w)
			}

			fx.poll(w)
			if asked := fx.asked(); len(asked) != 0 {
				t.Fatalf("asked = %v, want none: %s", asked, c.why)
			}
		})
	}
}

func TestRereviewRecordsARefusedRequestOnce(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.answered()
	fx.refuseReviews(http.StatusUnprocessableEntity)
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	row := fx.reviewRequested(w)
	if row.ID == 0 || !strings.Contains(row.Summary, "could not ask bob") {
		t.Fatalf("summary = %q, want the refusal against bob", row.Summary)
	}
	if got := fx.watch(w).LastError; got != "" {
		t.Fatalf("LastError = %q, want the poll to go on", got)
	}

	fx.agentIdle(w)
	fx.poll(w)
	if asked := fx.asked(); len(asked) != 1 {
		t.Fatalf("asked = %v, want the one request: a refusal repeats", asked)
	}
}

func TestRereviewRetriesAFailedRequest(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.answered()
	fx.refuseReviews(http.StatusInternalServerError)
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	if row := fx.reviewRequested(w); row.ID != 0 {
		t.Fatalf("a failure that may pass recorded %q", row.Summary)
	}
	fx.agentIdle(w)
	fx.poll(w)
	if asked := fx.asked(); len(asked) != 2 {
		t.Fatalf("asked = %v, want two tries", asked)
	}
}

func TestRereviewers(t *testing.T) {
	t.Parallel()
	reviewed := []string{"Bob", "carol", "babysitter"}
	got := rereviewers(reviewed, "BABYSITTER")
	if strings.Join(got, ",") != "Bob,carol" {
		t.Fatalf("rereviewers() = %v, want Bob and carol", got)
	}
	if strings.Join(reviewed, ",") != "Bob,carol,babysitter" {
		t.Fatalf("rereviewers() changed the snapshot: %v", reviewed)
	}
	if got := rereviewers(nil, "alice"); len(got) != 0 {
		t.Fatalf("rereviewers(none) = %v, want none", got)
	}
}

var changesFromContractor = reviewOf(8, "CHANGES_REQUESTED", "old", "ex-contractor", "fix that")

func TestRereviewAsksTheGoodLoginsWhenOneIsRefused(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.update(func() {
		fx.pr.Reviews = []ghfake.Review{changesFromBob, changesFromContractor}
		fx.pr.Refuse = []string{"ex-contractor"}
	})
	w := fx.start()
	fx.agentIdle(w)

	fx.poll(w)
	asked := fx.asked()
	if len(asked) != 3 {
		t.Fatalf("asked = %v, want the batch and then one call per login", asked)
	}
	if strings.Join(asked[1], ",") != "bob" || strings.Join(asked[2], ",") != "ex-contractor" {
		t.Fatalf("asked = %v, want bob and ex-contractor asked apart", asked)
	}
	row := fx.reviewRequested(w)
	if !strings.Contains(row.Summary, "asked bob") || !strings.Contains(row.Summary, "ex-contractor") {
		t.Fatalf("summary = %q, want bob asked and ex-contractor refused", row.Summary)
	}
}

func TestRereviewGivesUpOnAHeadThatKeepsFailing(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.answered()
	fx.refuseReviews(http.StatusForbidden)
	w := fx.start()

	for range maxRereviewTries + 2 {
		fx.agentIdle(w)
		fx.poll(w)
	}
	if asked := fx.asked(); len(asked) != maxRereviewTries {
		t.Fatalf("asked = %d times, want %d", len(asked), maxRereviewTries)
	}
	row := fx.reviewRequested(w)
	if row.ID == 0 || !strings.Contains(row.Summary, "could not ask bob") {
		t.Fatalf("summary = %q, want the failure recorded against the head", row.Summary)
	}
}

func TestRereviewRecordsTheRequestsItSentBeforeItFailed(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	var daveFails atomic.Bool
	daveFails.Store(true)
	fx.api.React(ghfake.RouteRequestReviews, func(a ghfake.Action) (ghfake.Response, bool) {
		var body struct {
			Reviewers []string `json:"reviewers"`
		}
		a.Decode(t, &body)
		return ghfake.Response{Status: http.StatusInternalServerError, Message: "Server Error"},
			daveFails.Load() && slices.Contains(body.Reviewers, "dave") && !slices.Contains(body.Reviewers, "carol-gone")
	})
	fx.update(func() {
		fx.pr.Reviews = []ghfake.Review{changesFromBob, changesFrom(8, "carol-gone"), changesFrom(9, "dave")}
		fx.pr.Refuse = []string{"carol-gone"}
	})
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)

	row := fx.reviewRequested(w)
	if row.ID == 0 {
		t.Fatalf("no review_requested row in %v", fx.kinds(w))
	}
	for _, want := range []string{"asked bob", "could not ask carol-gone", "did not reach dave"} {
		if !strings.Contains(row.Summary, want) {
			t.Fatalf("summary = %q, want %q in it", row.Summary, want)
		}
	}

	daveFails.Store(false)
	fx.update(func() { fx.pr.Requested = nil })
	fx.api.Reset()
	fx.agentIdle(w)
	fx.poll(w)
	if asked := fx.asked(); len(asked) != 0 {
		t.Fatalf("asked = %v, want nothing: the head was asked before", asked)
	}
}

func changesFrom(id int64, login string) ghfake.Review {
	return reviewOf(id, "CHANGES_REQUESTED", "old", login, "fix this")
}

// reviewOf is a review of pull request 3 submitted on September 2nd.
func reviewOf(id int64, state, commit, login, body string) ghfake.Review {
	return ghfake.Review{
		ID: id, State: state, CommitID: commit, Author: login, Body: body, SubmittedAt: ghfake.At("2026-09-02T00:00:00Z"),
		URL: fmt.Sprintf("https://github.com/octo/hello/pull/3#pullrequestreview-%d", id),
	}
}

func TestStopForgetsTheFailedTriesOfTheWatch(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.answered()
	fx.refuseReviews(http.StatusInternalServerError)
	w := fx.start()
	fx.agentIdle(w)
	fx.poll(w)
	if len(fx.svc.rereviewTries.get(w.ID)) == 0 {
		t.Fatal("no try was counted; this test needs a failing request")
	}

	if _, err := fx.svc.Stop(context.Background(), w.ID, StopOptions{}); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if heads := fx.svc.rereviewTries.get(w.ID); len(heads) != 0 {
		t.Fatalf("rereviewTries = %v after the stop, want none", heads)
	}
}

func TestTheFailedTriesKeepTheHeadOfNowAlone(t *testing.T) {
	t.Parallel()
	fx := newFixture(t)
	fx.answered()
	fx.refuseReviews(http.StatusInternalServerError)
	w := fx.start()
	fx.agentIdle(w)

	for _, sha := range []string{"abc", "def", "ghi"} {
		fx.update(func() { fx.pr.HeadSHA = sha })
		fx.agentIdle(w)
		fx.poll(w)
	}

	heads := fx.svc.rereviewTries.get(w.ID)
	if len(heads) != 1 {
		t.Fatalf("rereviewTries = %v, want the head of now alone", heads)
	}
	if _, ok := heads["rereview@ghi"]; !ok {
		t.Fatalf("rereviewTries = %v, want the counter of head ghi", heads)
	}
}
