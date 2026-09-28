package httpd

import (
	"context"
	"errors"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
)

type WatchController interface {
	Start(ctx context.Context, req prwatch.StartRequest) (store.Watch, error)
	Stop(ctx context.Context, id int64, o prwatch.StopOptions) (store.Watch, error)
	Merge(ctx context.Context, id int64, o prwatch.MergeOptions) (store.Watch, error)
	Poll(ctx context.Context, id int64) error
	Send(ctx context.Context, id int64, message string) (store.Activity, error)
	Takeover(ctx context.Context, id int64, o prwatch.TakeoverOptions) (prwatch.Takeover, error)
	Handback(ctx context.Context, id int64, o prwatch.HandbackOptions) (store.Watch, error)
	Next(ctx context.Context, id int64, wait time.Duration) (prwatch.NextMessage, error)
	Reply(ctx context.Context, id int64, req prwatch.ReplyRequest) (prwatch.ReplyOutcome, error)
	Retry(ctx context.Context, id int64, number int) (store.Proposal, error)
	Proposals(ctx context.Context, id int64) ([]prwatch.ProposalView, error)
	Proposal(ctx context.Context, id int64, number int, q prwatch.CodeQuery) (prwatch.ProposalDetail, error)
	Approve(ctx context.Context, id int64, number int, d prwatch.Decision) (store.Proposal, error)
	Reject(ctx context.Context, id int64, number int, r prwatch.Rejection) (store.Proposal, error)
	SetApproval(ctx context.Context, id int64, c prwatch.ApprovalChange) (store.Watch, error)
	SetMergeRules(ctx context.Context, id int64, c prwatch.MergeRulesChange) (store.Watch, error)
	Output(ctx context.Context, id int64, lines int) (string, error)
	Resize(ctx context.Context, id int64, size prwatch.TerminalSize) error
	Session(ctx context.Context, w store.Watch) (prwatch.SessionInfo, error)
	Readiness(w store.Watch, state agent.State) (*time.Time, []string)
	Hook(ctx context.Context, id int64, event string, payload []byte) error
	Providers() []prwatch.Provider
}

type noopWatches struct{}

var errWatchUnavailable = errors.New("the watch service is not running in this daemon")

func (noopWatches) Start(context.Context, prwatch.StartRequest) (store.Watch, error) {
	return store.Watch{}, errWatchUnavailable
}

func (noopWatches) Stop(context.Context, int64, prwatch.StopOptions) (store.Watch, error) {
	return store.Watch{}, errWatchUnavailable
}

func (noopWatches) Merge(context.Context, int64, prwatch.MergeOptions) (store.Watch, error) {
	return store.Watch{}, errWatchUnavailable
}
func (noopWatches) Poll(context.Context, int64) error { return errWatchUnavailable }
func (noopWatches) Send(context.Context, int64, string) (store.Activity, error) {
	return store.Activity{}, errWatchUnavailable
}

func (noopWatches) Takeover(context.Context, int64, prwatch.TakeoverOptions) (prwatch.Takeover, error) {
	return prwatch.Takeover{}, errWatchUnavailable
}

func (noopWatches) Handback(context.Context, int64, prwatch.HandbackOptions) (store.Watch, error) {
	return store.Watch{}, errWatchUnavailable
}

func (noopWatches) Next(context.Context, int64, time.Duration) (prwatch.NextMessage, error) {
	return prwatch.NextMessage{}, errWatchUnavailable
}

func (noopWatches) Reply(context.Context, int64, prwatch.ReplyRequest) (prwatch.ReplyOutcome, error) {
	return prwatch.ReplyOutcome{}, errWatchUnavailable
}

func (noopWatches) Retry(context.Context, int64, int) (store.Proposal, error) {
	return store.Proposal{}, errWatchUnavailable
}

func (noopWatches) Proposals(context.Context, int64) ([]prwatch.ProposalView, error) {
	return nil, errWatchUnavailable
}

func (noopWatches) Proposal(context.Context, int64, int, prwatch.CodeQuery) (prwatch.ProposalDetail, error) {
	return prwatch.ProposalDetail{}, errWatchUnavailable
}

func (noopWatches) Approve(context.Context, int64, int, prwatch.Decision) (store.Proposal, error) {
	return store.Proposal{}, errWatchUnavailable
}

func (noopWatches) Reject(context.Context, int64, int, prwatch.Rejection) (store.Proposal, error) {
	return store.Proposal{}, errWatchUnavailable
}

func (noopWatches) SetApproval(context.Context, int64, prwatch.ApprovalChange) (store.Watch, error) {
	return store.Watch{}, errWatchUnavailable
}

func (noopWatches) SetMergeRules(context.Context, int64, prwatch.MergeRulesChange) (store.Watch, error) {
	return store.Watch{}, errWatchUnavailable
}

func (noopWatches) Output(context.Context, int64, int) (string, error) {
	return "", errWatchUnavailable
}

func (noopWatches) Resize(context.Context, int64, prwatch.TerminalSize) error {
	return errWatchUnavailable
}
func (noopWatches) Hook(context.Context, int64, string, []byte) error { return errWatchUnavailable }

func (noopWatches) Session(context.Context, store.Watch) (prwatch.SessionInfo, error) {
	return prwatch.SessionInfo{State: agent.StateNone}, nil
}

func (noopWatches) Readiness(w store.Watch, _ agent.State) (*time.Time, []string) {
	return w.ReadySince, append([]string{}, w.ReadyBlockers...)
}

func (noopWatches) Providers() []prwatch.Provider { return prwatch.Catalog() }

var (
	startWatchErrors = newErrorMap("watch_failed",
		unavailable("watch_unavailable", errWatchUnavailable),
		badRequest("watch_rejected",
			prwatch.ErrNotOpen, prwatch.ErrNoPushAccess, prwatch.ErrNoIdentity, prwatch.ErrWrongRepo, prwatch.ErrWrongBranch,
			prwatch.ErrBadProvider, prwatch.ErrBadModel, prwatch.ErrNoAgent, prwatch.ErrBadMergeMethod, prwatch.ErrBadApprovalMode),
	)
	nextWatchErrors = newErrorMap("next_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		badRequest("hosted_watch", prwatch.ErrHostedWatch),
		conflict("next_in_flight", prwatch.ErrNextInFlight),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	mergeWatchErrors = newErrorMap("merge_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		conflict("watch_stopped", prwatch.ErrWatchStopped),
		conflict("not_ready", prwatch.ErrNotReady),
		unprocessable("merge_refused", prwatch.ErrMergeRefused),
		unprocessable("approve_refused", prwatch.ErrNotDependabot, prwatch.ErrOutOfScope),
		badRequest("bad_merge_method", prwatch.ErrBadMergeMethod),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	updateWatchErrors = newErrorMap("watch_update_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		conflict("watch_stopped", prwatch.ErrWatchStopped),
		badRequest("bad_merge_method", prwatch.ErrBadMergeMethod),
		badRequest("bad_approvals", prwatch.ErrBadApprovals),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	stopWatchErrors = newErrorMap("watch_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	sendWatchErrors = newErrorMap("send_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		conflict("watch_stopped", prwatch.ErrWatchStopped),
		conflict("agent_busy", prwatch.ErrAgentBusy),
		conflict("proposal_pending", prwatch.ErrProposalPending),
		conflict("taken_over", prwatch.ErrTakenOver),
		badRequest("no_agent", prwatch.ErrNoAgent),
		badRequest("self_watch", prwatch.ErrSelfWatch),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	takeoverErrors = newErrorMap("takeover_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		conflict("watch_stopped", prwatch.ErrWatchStopped),
		conflict("taken_over", prwatch.ErrTakenOver),
		conflict("session_running", prwatch.ErrSessionRunning),
		badRequest("no_agent", prwatch.ErrNoAgent),
		badRequest("self_watch", prwatch.ErrSelfWatch),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	handbackErrors = newErrorMap("handback_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		conflict("watch_stopped", prwatch.ErrWatchStopped),
		conflict("not_taken_over", prwatch.ErrNotTakenOver),
		conflict("author_running", prwatch.ErrAuthorRunning),
		conflict("unpushed_commits", prwatch.ErrUnpushedCommits),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	replyWatchErrors = newErrorMap("reply_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		conflict("watch_stopped", prwatch.ErrWatchStopped),
		badRequest("bad_request", prwatch.ErrEmptyReply),
		unprocessable("no_such_comment", prwatch.ErrNoSuchComment),
		conflict("proposal_pending", prwatch.ErrProposalPending),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	proposalErrors = newErrorMap("proposal_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		notFound("proposal_not_found", store.ErrProposalNotFound),
		notFound("reply_not_found", store.ErrReplyNotFound),
		notFound("commit_not_found", prwatch.ErrUnknownCommit),
		notFound("file_not_found", prwatch.ErrUnknownFile),
		conflict("watch_stopped", prwatch.ErrWatchStopped),
		conflict("not_pending", prwatch.ErrNotPending),
		conflict("proposal_pending", prwatch.ErrProposalPending),
		conflict("nothing_to_retry", prwatch.ErrNothingToRetry),
		badRequest("self_watch", prwatch.ErrSelfWatch),
		badRequest("bad_approval_mode", prwatch.ErrBadApprovalMode),
		badRequest("bad_request", prwatch.ErrEmptyReply),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	outputErrors = newErrorMap("output_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		badRequest("self_watch", prwatch.ErrSelfWatch),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	resizeErrors = newErrorMap("resize_failed",
		notFound("watch_not_found", store.ErrWatchNotFound),
		conflict("watch_stopped", prwatch.ErrWatchStopped),
		badRequest("self_watch", prwatch.ErrSelfWatch),
		unavailable("watch_unavailable", errWatchUnavailable),
	)
	hookErrors = newErrorMap("hook_failed",
		unavailable("watch_unavailable", errWatchUnavailable),
	)
)
