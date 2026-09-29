package httpd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/redact"
	"github.com/deividfortuna/babysitter/internal/snapshot"
	"github.com/deividfortuna/babysitter/internal/store"
)

func (a *api) watchExists(w http.ResponseWriter, r *http.Request, id int64) bool {
	if _, err := a.store.GetWatch(r.Context(), id); errors.Is(err, store.ErrWatchNotFound) {
		writeError(w, http.StatusNotFound, "watch_not_found", err.Error())
		return false
	}
	return true
}

func (a *api) watchOut(ctx context.Context, w store.Watch) Watch {
	return a.watchWith(ctx, w, a.pendingProposals(ctx))
}

func (a *api) watchWith(ctx context.Context, w store.Watch, pending map[int64]int) Watch {
	info, err := a.watches.Session(ctx, w)
	if err != nil {
		info = prwatch.SessionInfo{State: agent.StateNone, AgentSession: w.AgentSession}
	}
	readySince, blockers := a.watches.Readiness(w, info.State)
	out := watchFromStore(w, info, readySince, blockers)
	out.PendingProposal = pending[w.ID]
	return out
}

func (a *api) pendingProposals(ctx context.Context) map[int64]int {
	pending, err := a.store.PendingProposals(ctx)
	if err != nil {
		return map[int64]int{}
	}
	return pending
}

func (a *api) handleListWatches(w http.ResponseWriter, r *http.Request) {
	opts := store.ListWatchesOptions{Status: store.WatchActive}
	switch r.URL.Query().Get("status") {
	case "", "active":
	case "all":
		opts.Status = ""
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "status must be active or all")
		return
	}
	watches, err := a.store.ListWatches(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_failed", err.Error())
		return
	}
	out := WatchList{Watches: make([]Watch, 0, len(watches))}
	pending := a.pendingProposals(r.Context())
	for _, wt := range watches {
		out.Watches = append(out.Watches, a.watchWith(r.Context(), wt, pending))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleStartWatch(w http.ResponseWriter, r *http.Request) {
	var req StartWatchRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with target and repo fields; the other fields are optional")
		return
	}
	target, err := snapshot.ParseTarget(req.Target, req.Repo)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_target", err.Error())
		return
	}
	if badApprovals(req.ApprovalsRequired) {
		writeError(w, http.StatusBadRequest, "bad_request", "approvalsRequired must be 0 or more, or null for the rule of the base branch")
		return
	}
	mode, ok := approvalModeIn(req.ApprovalMode)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_approval_mode", "approvalMode must be auto or manual")
		return
	}
	wt, err := a.watches.Start(r.Context(), prwatch.StartRequest{
		Target: target, Provider: req.Provider, Model: req.Model, Effort: req.Effort, SourceDir: req.SourceDir,
		IncludeExisting: req.IncludeExisting, IncludeOwn: req.IncludeOwn,
		ApprovalsRequired: approvalsIn(req.ApprovalsRequired),
		MergeMethod:       req.MergeMethod,
		ApprovalMode:      mode, AutoApproveRebase: req.AutoApproveRebase,
		MergeWhenReady: req.MergeWhenReady, KeepWorktree: req.KeepWorktree,
	})
	if errors.Is(err, store.ErrWatchExists) {
		writeError(w, http.StatusConflict, "watch_exists", "the pull request is already watched as watch "+strconv.FormatInt(wt.ID, 10))
		return
	}
	if startWatchErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, a.watchOut(r.Context(), wt))
}

func approvalModeIn(s *string) (*store.ApprovalMode, bool) {
	if s == nil {
		return nil, true
	}
	mode := store.ApprovalMode(*s)
	return &mode, mode.Valid()
}

func badApprovals(approvals Optional[int]) bool {
	return approvals.Value != nil && *approvals.Value < 0
}

func approvalsIn(approvals Optional[int]) prwatch.Approvals {
	return prwatch.Approvals{Set: approvals.Set, Count: approvals.Value}
}

func (a *api) handleGetWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	wt, err := a.store.GetWatch(r.Context(), id)
	if storeErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, a.watchOut(r.Context(), wt))
}

func (a *api) handleStopWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req StopWatchRequest
	if err := readOptionalJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with an optional keepWorktree field")
		return
	}
	wt, err := a.watches.Stop(r.Context(), id, prwatch.StopOptions{KeepWorktree: req.KeepWorktree})
	if stopWatchErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, a.watchOut(r.Context(), wt))
}

func (a *api) handleMergeWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req MergeWatchRequest
	if err := readOptionalJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with an optional method field, squash, merge or rebase, and an optional approve field")
		return
	}
	wt, err := a.watches.Merge(r.Context(), id, prwatch.MergeOptions{Method: req.Method, Approve: req.Approve})
	if mergeWatchErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, a.watchOut(r.Context(), wt))
}

func (a *api) handlePollWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if !a.watchExists(w, r, id) {
		return
	}
	if a.polls.take(id) {
		go a.pollUntilIdle(a.ctx, id)
	}
	writeJSON(w, http.StatusAccepted, SyncAccepted{Accepted: true})
}

func (a *api) pollUntilIdle(ctx context.Context, id int64) {
	for {
		if err := a.watches.Poll(ctx, id); err != nil {
			a.log.Warn("poll request failed", "watch", id, "err", err)
		}
		if !a.polls.done(id) {
			return
		}
	}
}

type pollFolder struct {
	mu     sync.Mutex
	queued map[int64]bool
}

func newPollFolder() *pollFolder {
	return &pollFolder{queued: make(map[int64]bool)}
}

func (p *pollFolder) take(id int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, busy := p.queued[id]; busy {
		p.queued[id] = true
		return false
	}
	p.queued[id] = false
	return true
}

func (p *pollFolder) done(id int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.queued[id] {
		p.queued[id] = false
		return true
	}
	delete(p.queued, id)
	return false
}

func (a *api) handleListActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if !a.watchExists(w, r, id) {
		return
	}
	since, ok := queryInt(w, r, "since", 0)
	if !ok {
		return
	}
	limit, ok := queryCount(w, r, "limit", 0)
	if !ok {
		return
	}
	rows, err := a.store.ListActivity(r.Context(), id, since, limit)
	if storeErrors.write(w, err) {
		return
	}
	out := ActivityList{Activity: make([]Activity, 0, len(rows))}
	for _, row := range rows {
		out.Activity = append(out.Activity, activityFromStore(row))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleTakeoverWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req TakeoverRequest
	if err := readJSON(w, r, &req); err != nil || req.PID <= 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with the pid of the process that takes the session")
		return
	}
	tk, err := a.watches.Takeover(r.Context(), id, prwatch.TakeoverOptions{PID: req.PID, Shell: req.Shell})
	if takeoverErrors.write(w, err) {
		return
	}
	declined := tk.Declined
	if declined == nil {
		declined = []int{}
	}
	writeJSON(w, http.StatusOK, TakeoverResponse{
		Watch: a.watchOut(r.Context(), tk.Watch), WorktreeDir: tk.WorktreeDir, WorkBranch: tk.WorkBranch, HeadRef: tk.HeadRef,
		Argv: tk.Argv, Declined: declined, NewConversation: tk.NewConversation,
	})
}

func (a *api) handleHandbackWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req HandbackRequest
	if err := readOptionalJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with optional confirm and force fields")
		return
	}
	wt, err := a.watches.Handback(r.Context(), id, prwatch.HandbackOptions{Confirm: req.Confirm, Force: req.Force})
	if work, ok := errors.AsType[*prwatch.WorkError](err); ok {
		writeJSON(w, http.StatusConflict, handbackRefusal(work))
		return
	}
	if handbackErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, a.watchOut(r.Context(), wt))
}

func handbackRefusal(work *prwatch.WorkError) HandbackRefusal {
	commits := make([]WorkCommit, 0, len(work.Commits))
	for _, c := range work.Commits {
		commits = append(commits, WorkCommit{SHA: c.SHA, Subject: redact.Text(c.Subject)})
	}
	files := make([]string, 0, len(work.Files))
	for _, f := range work.Files {
		files = append(files, redact.Text(f))
	}
	return HandbackRefusal{
		Error:   ErrorBody{Code: "unconfirmed_work", Message: redact.Text(work.Error())},
		Commits: commits, Files: files,
	}
}

func (a *api) handleSendWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req SendMessageRequest
	if err := readJSON(w, r, &req); err != nil || req.Message == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with a message field")
		return
	}
	row, err := a.watches.Send(r.Context(), id, req.Message)
	if sendWatchErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, activityFromStore(row))
}

const MaxNextWait = 10 * time.Minute

func (a *api) handleNextWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	wait, err := nextWait(r.URL.Query().Get("wait"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	out, err := a.watches.Next(r.Context(), id, wait)
	if nextWatchErrors.write(w, err) {
		return
	}
	body := NextMessage{Watch: a.watchOut(r.Context(), out.Watch)}
	if out.Message != nil {
		row := activityFromStore(*out.Message)
		body.Message = &row
	}
	writeJSON(w, http.StatusOK, body)
}

func nextWait(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("wait must be a duration such as 5m, got %q", s)
	}
	return min(d, MaxNextWait), nil
}

func (a *api) handleReplyWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req ReplyRequest
	if err := readJSON(w, r, &req); err != nil || req.Body == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with a body field, and an inReplyTo comment id for a reply in a thread")
		return
	}
	out, err := a.watches.Reply(r.Context(), id, prwatch.ReplyRequest{InReplyTo: req.InReplyTo, Body: req.Body})
	if replyWatchErrors.write(w, err) {
		return
	}
	res := ReplyResult{Proposal: out.Proposal}
	if out.Posted != nil {
		row := activityFromStore(*out.Posted)
		res.Posted = &row
	}
	writeJSON(w, http.StatusCreated, res)
}

func (a *api) handleWatchOutput(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	lines, ok := queryCount(w, r, "lines", 200)
	if !ok {
		return
	}
	if lines < 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "lines must not be negative")
		return
	}
	out, err := a.watches.Output(r.Context(), id, lines)
	if outputErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, SessionOutput{Output: out})
}

func (a *api) handleResizeWatch(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req ResizeRequest
	if err := readJSON(w, r, &req); err != nil || !req.valid() {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with rows from 1 to 500 and cols from 1 to 1000")
		return
	}
	size := prwatch.TerminalSize{Rows: req.Rows, Cols: req.Cols}
	if resizeErrors.write(w, a.watches.Resize(r.Context(), id, size)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleWatchHook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req HookRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with event and payload fields")
		return
	}
	if !agent.ValidEvent(req.Event) {
		writeError(w, http.StatusBadRequest, "bad_request", "unknown hook event "+strconv.Quote(req.Event))
		return
	}
	payload, _ := json.Marshal(req.Payload)
	if hookErrors.write(w, a.watches.Hook(r.Context(), id, req.Event, payload)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
