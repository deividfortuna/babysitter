package httpd

import (
	"net/http"

	"github.com/deividfortuna/babysitter/internal/prwatch"
)

func proposalPath(w http.ResponseWriter, r *http.Request) (int64, int, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return 0, 0, false
	}
	number, ok := pathID(w, r, "number")
	return id, int(number), ok
}

func (a *api) handleListProposals(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	views, err := a.watches.Proposals(r.Context(), id)
	if proposalErrors.write(w, err) {
		return
	}
	out := ProposalList{Proposals: make([]Proposal, 0, len(views))}
	for _, v := range views {
		out.Proposals = append(out.Proposals, proposalFromView(v))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) handleGetProposal(w http.ResponseWriter, r *http.Request) {
	id, number, ok := proposalPath(w, r)
	if !ok {
		return
	}
	d, err := a.watches.Proposal(r.Context(), id, number)
	if proposalErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, proposalDetailFrom(d))
}

func (a *api) handleApproveProposal(w http.ResponseWriter, r *http.Request) {
	id, number, ok := proposalPath(w, r)
	if !ok {
		return
	}
	var req ApproveRequest
	if err := readOptionalJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with the optional edits, drop, rejectPush and stopAsking fields")
		return
	}
	d := prwatch.Decision{Edits: map[int64]string{}, Drop: req.Drop, RejectPush: req.RejectPush, StopAsking: req.StopAsking}
	for _, e := range req.Edits {
		d.Edits[e.ReplyID] = e.Body
	}
	p, err := a.watches.Approve(r.Context(), id, number, d)
	if proposalErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, proposalFromStore(p))
}

func (a *api) handleRejectProposal(w http.ResponseWriter, r *http.Request) {
	id, number, ok := proposalPath(w, r)
	if !ok {
		return
	}
	var req RejectRequest
	if err := readOptionalJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with the optional reason and discard fields")
		return
	}
	p, err := a.watches.Reject(r.Context(), id, number, prwatch.Rejection{Reason: req.Reason, Discard: req.Discard})
	if proposalErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, proposalFromStore(p))
}

func (a *api) handleRetryProposal(w http.ResponseWriter, r *http.Request) {
	id, number, ok := proposalPath(w, r)
	if !ok {
		return
	}
	p, err := a.watches.Retry(r.Context(), id, number)
	if proposalErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, proposalFromStore(p))
}

func (a *api) handleSetApproval(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req ApprovalRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be JSON with the optional mode, autoApproveRebase and release fields")
		return
	}
	mode, _ := approvalModeIn(req.Mode)
	wt, err := a.watches.SetApproval(r.Context(), id, prwatch.ApprovalChange{Mode: mode, AutoApproveRebase: req.AutoApproveRebase, Release: req.Release})
	if proposalErrors.write(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, a.watchOut(r.Context(), wt))
}
