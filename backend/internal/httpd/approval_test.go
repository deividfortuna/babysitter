package httpd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deividfortuna/babysitter/internal/gitrelease"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
)

type approvalCalls struct {
	decisions  []prwatch.Decision
	rejections []prwatch.Rejection
	changes    []prwatch.ApprovalChange
}

func (f *fakeWatches) gated(ctx context.Context, id int64) (store.Watch, error) {
	w, err := f.st.GetWatch(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	if w.Provider == prwatch.ProviderSelf {
		return store.Watch{}, prwatch.ErrSelfWatch
	}
	return w, nil
}

func fakeProposal(watchID int64, status store.ProposalStatus) prwatch.ProposalView {
	answers := store.Activity{ID: 7, WatchID: watchID, Kind: store.ActivityReviewComment, Ref: "31", Actor: "bob", Summary: "bob commented on x.go:4: rename this", At: time.Now()}
	return prwatch.ProposalView{
		WatchID: watchID, Number: 1, Status: status, HeadSHA: "abc", BaseSHA: "abc", WorkSHA: "w1", HasPush: true, OpenedAt: time.Now(),
		Replies: []prwatch.ReplyView{{
			ID: 5, InReplyTo: 31, Body: "renamed it", RecordedAt: time.Now(),
			Answers: &answers,
		}},
	}
}

func (f *fakeWatches) Proposals(ctx context.Context, id int64) ([]prwatch.ProposalView, error) {
	if _, err := f.gated(ctx, id); err != nil {
		return nil, err
	}
	return []prwatch.ProposalView{fakeProposal(id, store.ProposalPending)}, nil
}

func (f *fakeWatches) Proposal(ctx context.Context, id int64, number int, q prwatch.CodeQuery) (prwatch.ProposalDetail, error) {
	if _, err := f.gated(ctx, id); err != nil {
		return prwatch.ProposalDetail{}, err
	}
	switch number {
	case 1:
		if q.Commit != "" && q.Commit != "w1" {
			return prwatch.ProposalDetail{}, prwatch.ErrUnknownCommit
		}
		if q.Path != "" && q.Path != "x.go" {
			return prwatch.ProposalDetail{}, prwatch.ErrUnknownFile
		}
		return prwatch.ProposalDetail{
			ProposalView: fakeProposal(id, store.ProposalPending),
			Commits:      []gitrelease.Commit{{SHA: "w1", Subject: "Rename the thing"}},
			Commit:       q.Commit,
			Files:        []gitrelease.File{{Path: "x.go", Status: "M", Added: 3, Deleted: 1}},
			Diff:         "diff --git a/x.go b/x.go\n",
		}, nil
	case 3:
		return prwatch.ProposalDetail{ProposalView: fakeProposal(id, store.ProposalPending), CodeError: "git log: exit status 128"}, nil
	}
	return prwatch.ProposalDetail{}, store.ErrProposalNotFound
}

func (f *fakeWatches) Approve(ctx context.Context, id int64, number int, d prwatch.Decision) (store.Proposal, error) {
	if _, err := f.gated(ctx, id); err != nil {
		return store.Proposal{}, err
	}
	if number == 2 {
		return store.Proposal{}, prwatch.ErrNotPending
	}
	f.mu.Lock()
	f.approval.decisions = append(f.approval.decisions, d)
	f.mu.Unlock()
	p := fakeProposal(id, store.ProposalReleased).Proposal
	p.PushRejected = d.RejectPush
	return p, nil
}

func (f *fakeWatches) Reject(ctx context.Context, id int64, number int, r prwatch.Rejection) (store.Proposal, error) {
	if _, err := f.gated(ctx, id); err != nil {
		return store.Proposal{}, err
	}
	f.mu.Lock()
	f.approval.rejections = append(f.approval.rejections, r)
	f.mu.Unlock()
	p := fakeProposal(id, store.ProposalRejected).Proposal
	p.Reason = r.Reason
	return p, nil
}

func (f *fakeWatches) SetApproval(ctx context.Context, id int64, c prwatch.ApprovalChange) (store.Watch, error) {
	w, err := f.gated(ctx, id)
	if err != nil {
		return store.Watch{}, err
	}
	switch {
	case c.Mode != nil && !c.Mode.Valid():
		return store.Watch{}, prwatch.ErrBadApprovalMode
	case c.Mode != nil && *c.Mode == store.ApprovalAuto && !c.Release:
		return store.Watch{}, fmt.Errorf("%w: proposal 1", prwatch.ErrProposalPending)
	}
	f.mu.Lock()
	f.approval.changes = append(f.approval.changes, c)
	f.mu.Unlock()
	mode, rebase := w.ApprovalMode, w.AutoApproveRebase
	if c.Mode != nil {
		mode = *c.Mode
	}
	if c.AutoApproveRebase != nil {
		rebase = *c.AutoApproveRebase
	}
	return f.st.SetWatchApproval(ctx, id, mode, rebase)
}

func TestTheSettingsCarryTheApprovalMode(t *testing.T) {
	t.Parallel()
	h, _, _, applied := newTestAPISettings(t)
	var got Settings
	if rec := call(t, h, http.MethodGet, "/settings", "", &got); rec.Code != http.StatusOK || got.ApprovalMode != "manual" || got.AutoApproveRebase {
		t.Fatalf("settings of a fresh daemon: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPut, "/settings", `{"approvalMode":"auto","autoApproveRebase":true}`, &got); rec.Code != http.StatusOK ||
		got.ApprovalMode != "auto" || !got.AutoApproveRebase || got.PollIntervalSeconds != 60 {
		t.Fatalf("put approval mode: %d %s", rec.Code, rec.Body)
	}
	if last := applied(); last[len(last)-1].ApprovalMode != store.ApprovalAuto {
		t.Fatalf("applied = %+v", last)
	}
	if rec := call(t, h, http.MethodPut, "/settings", `{"approvalMode":"sometimes"}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown mode: %d %s", rec.Code, rec.Body)
	}
}

func TestStartWatchHandsTheApprovalModeOver(t *testing.T) {
	t.Parallel()
	h, _, fw := newTestAPI(t)
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#3","sourceDir":"/src"}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#4","sourceDir":"/src","approvalMode":"manual","autoApproveRebase":true}`, nil); rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	got := fw.started()
	if got[0].ApprovalMode != nil || got[0].AutoApproveRebase != nil {
		t.Fatalf("start request = %+v, want the fields the body left out unset", got[0])
	}
	if got[1].ApprovalMode == nil || *got[1].ApprovalMode != store.ApprovalManual || got[1].AutoApproveRebase == nil || !*got[1].AutoApproveRebase {
		t.Fatalf("start request = %+v", got[1])
	}
	if rec := call(t, h, http.MethodPost, "/watches", `{"target":"octo/hello#5","sourceDir":"/src","approvalMode":"sometimes"}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown mode: %d %s", rec.Code, rec.Body)
	}
}

func TestAWatchShowsItsModeAndTheProposalThatWaits(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	ctx := context.Background()
	w, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now(), ApprovalMode: store.ApprovalManual})
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.OpenProposal(ctx, w.ID, "abc", "abc", "abc", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetProposalOutcome(ctx, p.ID, store.ProposalPending, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	var one Watch
	if rec := call(t, h, http.MethodGet, "/watches/1", "", &one); rec.Code != http.StatusOK || one.ApprovalMode != "manual" || one.PendingProposal != 1 {
		t.Fatalf("get watch: %d %s", rec.Code, rec.Body)
	}
	var list WatchList
	if rec := call(t, h, http.MethodGet, "/watches", "", &list); rec.Code != http.StatusOK || list.Watches[0].PendingProposal != 1 {
		t.Fatalf("list watches: %d %s", rec.Code, rec.Body)
	}
}

func TestAWatchSaysWhetherDependabotOwnsItsBranch(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	ctx := context.Background()
	for i, author := range []string{"dependabot[bot]", "dependabot-fan"} {
		if _, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 3 + i, Author: author, StartedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	for id, want := range map[int]bool{1: true, 2: false} {
		var got map[string]any
		if rec := call(t, h, http.MethodGet, fmt.Sprintf("/watches/%d", id), "", &got); rec.Code != http.StatusOK || got["dependabot"] != want {
			t.Errorf("watch %d of %v: dependabot = %v, want %v", id, got["author"], got["dependabot"], want)
		}
	}
}

func TestTheProposalRoutes(t *testing.T) {
	t.Parallel()
	h, st, fw := newTestAPI(t)
	ctx := context.Background()
	if _, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 3, StartedAt: time.Now(), ApprovalMode: store.ApprovalManual}); err != nil {
		t.Fatal(err)
	}
	self, err := st.CreateWatch(ctx, store.Watch{Owner: "octo", Name: "hello", Number: 4, Provider: prwatch.ProviderSelf, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	var list ProposalList
	if rec := call(t, h, http.MethodGet, "/watches/1/proposals", "", &list); rec.Code != http.StatusOK ||
		len(list.Proposals) != 1 || list.Proposals[0].Status != store.ProposalPending || len(list.Proposals[0].Replies) != 1 {
		t.Fatalf("list proposals: %d %s", rec.Code, rec.Body)
	}
	var d ProposalDetail
	if rec := call(t, h, http.MethodGet, "/watches/1/proposals/1", "", &d); rec.Code != http.StatusOK ||
		len(d.Commits) != 1 || d.Commits[0].Subject != "Rename the thing" || len(d.Files) != 1 || d.Files[0].Added != 3 ||
		!strings.Contains(d.Diff, "diff --git") || d.Replies[0].Answers == nil || d.Replies[0].Answers.Actor != "bob" || d.Replies[0].InReplyTo != 31 {
		t.Fatalf("proposal detail: %d %s", rec.Code, rec.Body)
	}
	var one ProposalDetail
	if rec := call(t, h, http.MethodGet, "/watches/1/proposals/1?commit=w1", "", &one); rec.Code != http.StatusOK || one.Commit != "w1" {
		t.Fatalf("proposal detail of one commit: %d %s", rec.Code, rec.Body)
	}
	var file ProposalDetail
	if rec := call(t, h, http.MethodGet, "/watches/1/proposals/1?path=x.go", "", &file); rec.Code != http.StatusOK || len(file.Files) != 1 {
		t.Fatalf("proposal detail of one file: %d %s", rec.Code, rec.Body)
	}
	var unread ProposalDetail
	if rec := call(t, h, http.MethodGet, "/watches/1/proposals/3", "", &unread); rec.Code != http.StatusOK ||
		unread.CodeError != "git log: exit status 128" || !strings.Contains(rec.Body.String(), `"codeError":"git log: exit status 128"`) ||
		len(unread.Commits) != 0 || len(unread.Replies) != 1 {
		t.Fatalf("proposal detail with unreadable code: %d %s", rec.Code, rec.Body)
	}

	var p Proposal
	body := `{"edits":[{"replyId":5,"body":"Renamed it, thanks."}],"drop":[6],"rejectPush":true,"stopAsking":true}`
	if rec := call(t, h, http.MethodPost, "/watches/1/proposals/1/approve", body, &p); rec.Code != http.StatusOK || p.Status != store.ProposalReleased || !p.PushRejected {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	decision := fw.approval.decisions[0]
	if decision.Edits[5] != "Renamed it, thanks." || len(decision.Drop) != 1 || decision.Drop[0] != 6 || !decision.RejectPush || !decision.StopAsking {
		t.Fatalf("decision = %+v", decision)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/proposals/1/approve", "", &p); rec.Code != http.StatusOK {
		t.Fatalf("approve with no body: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, h, http.MethodPost, "/watches/1/proposals/1/reject", `{"reason":"use a table test","discard":true}`, &p); rec.Code != http.StatusOK ||
		p.Status != store.ProposalRejected || p.Reason != "use a table test" {
		t.Fatalf("reject: %d %s", rec.Code, rec.Body)
	}
	if r := fw.approval.rejections[0]; r.Reason != "use a table test" || !r.Discard {
		t.Fatalf("rejection = %+v", r)
	}

	var w Watch
	if rec := call(t, h, http.MethodPost, "/watches/1/approval", `{"mode":"auto","release":true,"autoApproveRebase":true}`, &w); rec.Code != http.StatusOK ||
		w.ApprovalMode != "auto" || !w.AutoApproveRebase {
		t.Fatalf("set approval: %d %s", rec.Code, rec.Body)
	}

	for _, tc := range []struct {
		method, path, body string
		code               int
		word               string
	}{
		{http.MethodPost, "/watches/1/proposals/2/approve", "", http.StatusConflict, "not_pending"},
		{http.MethodPost, "/watches/1/approval", `{"mode":"auto"}`, http.StatusConflict, "proposal_pending"},
		{http.MethodPost, "/watches/1/approval", `{"mode":"sometimes"}`, http.StatusBadRequest, "bad_approval_mode"},
		{http.MethodGet, "/watches/1/proposals/9", "", http.StatusNotFound, "proposal_not_found"},
		{http.MethodGet, "/watches/1/proposals/1?commit=zzz", "", http.StatusNotFound, "commit_not_found"},
		{http.MethodGet, "/watches/1/proposals/1?path=nope.go", "", http.StatusNotFound, "file_not_found"},
		{http.MethodGet, "/watches/9/proposals", "", http.StatusNotFound, "watch_not_found"},
		{http.MethodGet, fmt.Sprintf("/watches/%d/proposals", self.ID), "", http.StatusBadRequest, "self_watch"},
		{http.MethodPost, fmt.Sprintf("/watches/%d/proposals/1/reject", self.ID), "", http.StatusBadRequest, "self_watch"},
		{http.MethodPost, "/watches/1/proposals/1/approve", `{"nope":1}`, http.StatusBadRequest, "bad_request"},
	} {
		if rec := call(t, h, tc.method, tc.path, tc.body, nil); rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.word) {
			t.Errorf("%s %s: %d %s, want %d %s", tc.method, tc.path, rec.Code, rec.Body, tc.code, tc.word)
		}
	}
}

func TestSendIsRefusedWhileAProposalWaits(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	seedWatch(t, st)
	if rec := call(t, h, http.MethodPost, "/watches/1/send", `{"message":"pending"}`, nil); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "proposal_pending") {
		t.Fatalf("send: %d %s", rec.Code, rec.Body)
	}
}

func TestSendIsRefusedWhileGitHubUpdatesTheBranch(t *testing.T) {
	t.Parallel()
	h, st, _ := newTestAPI(t)
	seedWatch(t, st)
	rec := call(t, h, http.MethodPost, "/watches/1/send", `{"message":"updating"}`, nil)
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "branch_updating" {
		t.Fatalf("send: %d %s", rec.Code, rec.Body)
	}
}
