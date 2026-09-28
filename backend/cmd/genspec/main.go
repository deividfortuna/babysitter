package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go"
	"github.com/swaggest/openapi-go/openapi3"

	"github.com/deividfortuna/babysitter/internal/httpd"
)

func main() {
	out := flag.String("out", "internal/httpd/apispec/openapi.yaml", "output path")
	flag.Parse()

	data, err := build()
	if err != nil {
		fmt.Fprintln(os.Stderr, "genspec:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "genspec:", err)
		os.Exit(1)
	}
}

type idParam struct {
	ID int64 `path:"id" description:"Repository id"`
}

type watchIDParam struct {
	ID int64 `path:"id" description:"Watch id"`
}

type sendParams struct {
	watchIDParam
	httpd.SendMessageRequest
}

type replyParams struct {
	watchIDParam
	httpd.ReplyRequest
}

type proposalParams struct {
	watchIDParam
	Number int `path:"number" description:"Proposal number, counted per watch from 1"`
}

type approveParams struct {
	proposalParams
	httpd.ApproveRequest
}

type rejectParams struct {
	proposalParams
	httpd.RejectRequest
}

type approvalParams struct {
	watchIDParam
	httpd.ApprovalRequest
}

type updateWatchParams struct {
	watchIDParam
	httpd.UpdateWatchRequest
}

type outputParams struct {
	watchIDParam
	httpd.OutputQuery
}

type resizeParams struct {
	watchIDParam
	httpd.ResizeRequest
}

type hookParams struct {
	watchIDParam
	httpd.HookRequest
}

type nextParams struct {
	watchIDParam
	httpd.NextQuery
}

type stopWatchParams struct {
	watchIDParam
	httpd.StopWatchRequest
}

type takeoverParams struct {
	watchIDParam
	httpd.TakeoverRequest
}

type mergeWatchParams struct {
	watchIDParam
	httpd.MergeWatchRequest
}

type activityParams struct {
	watchIDParam
	httpd.ActivityQuery
}

type handbackParams struct {
	watchIDParam
	httpd.HandbackRequest
}

type repoConfigParams struct {
	idParam
	httpd.UpdateRepoConfigRequest
}

type operation struct {
	method, path, id, summary string
	req                       any
	resp                      any
	status                    int
	stream                    bool
	errors                    []int
	errorBodies               map[int]any
}

func (op operation) errorBody(status int) any {
	if body, ok := op.errorBodies[status]; ok {
		return body
	}
	return httpd.APIError{}
}

func build() ([]byte, error) {
	r := openapi3.NewReflector()
	r.DefaultOptions = append(r.DefaultOptions, jsonschema.InterceptProp(func(p jsonschema.InterceptPropParams) error {
		if !p.Processed || p.ParentSchema == nil {
			return nil
		}
		tag := p.Field.Tag.Get("json")
		if tag == "" || tag == "-" || strings.Contains(tag, "omitempty") || strings.Contains(tag, "omitzero") {
			return nil
		}
		if slices.Contains(p.ParentSchema.Required, p.Name) {
			return nil
		}
		p.ParentSchema.Required = append(p.ParentSchema.Required, p.Name)
		return nil
	}))
	r.AddTypeMapping(httpd.Optional[int]{}, new(int))
	r.Spec = &openapi3.Spec{Openapi: "3.0.3"}
	r.Spec.Info.
		WithTitle("babysitter daemon API").
		WithVersion("1.0.0").
		WithDescription("Loopback API of the babysitter daemon. Generated from the Go types in internal/httpd; do not edit by hand.")
	r.Spec.WithServers(openapi3.Server{
		URL:       "http://127.0.0.1:{port}" + httpd.Prefix,
		Variables: map[string]openapi3.ServerVariable{"port": {Default: "0"}},
	})

	ops := []operation{
		{method: http.MethodGet, path: "/healthz", id: "getHealth", summary: "Liveness of the daemon", resp: httpd.Health{}, status: http.StatusOK},
		{method: http.MethodGet, path: "/readyz", id: "getReady", summary: "Readiness of the daemon", resp: httpd.Health{}, status: http.StatusOK},
		{method: http.MethodGet, path: "/events", id: "streamEvents", summary: "Change feed as server-sent events", status: http.StatusOK, stream: true},
		{method: http.MethodGet, path: "/settings", id: "getSettings", summary: "The settings of the daemon", resp: httpd.Settings{}, status: http.StatusOK},
		{method: http.MethodPut, path: "/settings", id: "putSettings", summary: "Write the settings of the daemon", req: httpd.Settings{}, resp: httpd.Settings{}, status: http.StatusOK, errors: []int{http.StatusBadRequest}},
		{method: http.MethodGet, path: "/repos", id: "listRepos", summary: "List the watched repositories", resp: httpd.RepoList{}, status: http.StatusOK},
		{method: http.MethodPost, path: "/repos", id: "addRepo", summary: "Watch a repository", req: httpd.AddRepoRequest{}, resp: httpd.Repo{}, status: http.StatusCreated, errors: []int{http.StatusBadRequest, http.StatusConflict}},
		{method: http.MethodDelete, path: "/repos/{id}", id: "removeRepo", summary: "Stop watching a repository", req: idParam{}, status: http.StatusNoContent, errors: []int{http.StatusNotFound}},
		{method: http.MethodGet, path: "/repos/{id}/config", id: "getRepoConfig", summary: "What babysitter does with the new pull requests of a repository", req: idParam{}, resp: httpd.RepoConfig{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound}},
		{method: http.MethodPatch, path: "/repos/{id}/config", id: "updateRepoConfig", summary: "Change the configuration of a repository. A toggle that goes on records the time; only pull requests created from then on start.", req: repoConfigParams{}, resp: httpd.RepoConfig{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound}},
		{method: http.MethodGet, path: "/repos/{id}/queue", id: "getRepoQueue", summary: "The Dependabot pull requests that wait for a place, oldest first", req: idParam{}, resp: httpd.RepoQueue{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound}},
		{method: http.MethodGet, path: "/prs", id: "listPullRequests", summary: "List the stored pull requests", req: httpd.PullRequestQuery{}, resp: httpd.PullRequestList{}, status: http.StatusOK, errors: []int{http.StatusBadRequest}},
		{method: http.MethodGet, path: "/notifications", id: "listNotifications", summary: "The notifications the daemon recorded, newest first", req: httpd.NotificationQuery{}, resp: httpd.NotificationList{}, status: http.StatusOK, errors: []int{http.StatusBadRequest}},
		{method: http.MethodPost, path: "/notifications", id: "addNotification", summary: "Record one notification and show it", req: httpd.NewNotificationRequest{}, resp: httpd.Notification{}, status: http.StatusCreated, errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/notifications/read", id: "readNotifications", summary: "Mark notifications as seen; no ids marks every unread one", req: httpd.ReadNotificationsRequest{}, resp: httpd.NotificationsRead{}, status: http.StatusOK, errors: []int{http.StatusBadRequest}},
		{method: http.MethodGet, path: "/providers", id: "listProviders", summary: "The AI providers and the models they offer", resp: httpd.ProviderList{}, status: http.StatusOK},
		{method: http.MethodGet, path: "/viewer", id: "getViewer", summary: "The GitHub account the daemon acts as", resp: httpd.Viewer{}, status: http.StatusOK, errors: []int{http.StatusServiceUnavailable}},
		{method: http.MethodGet, path: "/ratelimit", id: "getRateLimit", summary: "The GitHub API budget the token of the daemon has left", resp: httpd.RateLimit{}, status: http.StatusOK},
		{method: http.MethodGet, path: "/logs", id: "listLogs", summary: "The last records the daemon logged in this run, oldest first", req: httpd.LogQuery{}, resp: httpd.LogList{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable}},
		{method: http.MethodGet, path: "/logs/stream", id: "streamLogs", summary: "The log of the daemon as server-sent events: a ready frame, the kept records after the seq, then one log frame per new record", req: httpd.LogStreamQuery{}, status: http.StatusOK, stream: true, errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable}},
		{method: http.MethodGet, path: "/logs/level", id: "getLogLevel", summary: "The lowest level the daemon records", resp: httpd.LogLevel{}, status: http.StatusOK, errors: []int{http.StatusServiceUnavailable}},
		{method: http.MethodPut, path: "/logs/level", id: "putLogLevel", summary: "Change the lowest level the daemon records until it stops", req: httpd.LogLevel{}, resp: httpd.LogLevel{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/sync", id: "requestSync", summary: "Ask the watcher for a sync pass now", resp: httpd.SyncAccepted{}, status: http.StatusAccepted},
		{method: http.MethodGet, path: "/watches", id: "listWatches", summary: "List the watched pull requests", req: httpd.WatchQuery{}, resp: httpd.WatchList{}, status: http.StatusOK, errors: []int{http.StatusBadRequest}},
		{method: http.MethodPost, path: "/watches", id: "startWatch", summary: "Watch a pull request", req: httpd.StartWatchRequest{}, resp: httpd.Watch{}, status: http.StatusCreated, errors: []int{http.StatusBadRequest, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodGet, path: "/watches/{id}", id: "getWatch", summary: "One watched pull request", req: watchIDParam{}, resp: httpd.Watch{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound}},
		{method: http.MethodPatch, path: "/watches/{id}", id: "updateWatch", summary: "Change the approvals, the merge method and merge when ready of a watch while it runs. The other fields of a watch cannot change.", req: updateWatchParams{}, resp: httpd.Watch{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/stop", id: "stopWatch", summary: "Stop watching a pull request", req: stopWatchParams{}, resp: httpd.Watch{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/merge", id: "mergeWatch", summary: "Merge the pull request of a watch and stop the watch", req: mergeWatchParams{}, resp: httpd.Watch{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/poll", id: "pollWatch", summary: "Poll a watched pull request now", req: watchIDParam{}, resp: httpd.SyncAccepted{}, status: http.StatusAccepted, errors: []int{http.StatusBadRequest, http.StatusNotFound}},
		{method: http.MethodGet, path: "/watches/{id}/activity", id: "listWatchActivity", summary: "Activity of a watched pull request", req: activityParams{}, resp: httpd.ActivityList{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound}},
		{method: http.MethodPost, path: "/watches/{id}/send", id: "sendWatchMessage", summary: "Send a message to the agent of a watched pull request", req: sendParams{}, resp: httpd.Activity{}, status: http.StatusCreated, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/takeover", id: "takeoverWatch", summary: "Move the agent session of a watch to the terminal of the author: the session of the daemon ends and the proposals that wait are declined", req: takeoverParams{}, resp: httpd.TakeoverResponse{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/handback", id: "handbackWatch", summary: "Give the agent session of a watch back to the daemon. A 409 with the code unconfirmed_work lists the commits and the changes to confirm.", req: handbackParams{}, resp: httpd.Watch{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}, errorBodies: map[int]any{http.StatusConflict: httpd.HandbackRefusal{}}},
		{method: http.MethodPost, path: "/watches/{id}/next", id: "nextWatchMessage", summary: "Hand the agent of a self watch its next message, after a wait when there is none", req: nextParams{}, resp: httpd.NextMessage{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/reply", id: "replyOnWatch", summary: "Take a reply of the agent on a watched pull request; the daemon posts it when the turn ends, or at once for a self watch", req: replyParams{}, resp: httpd.ReplyResult{}, status: http.StatusCreated, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity, http.StatusServiceUnavailable}},
		{method: http.MethodGet, path: "/watches/{id}/proposals", id: "listProposals", summary: "The proposals of a watch, newest first", req: watchIDParam{}, resp: httpd.ProposalList{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable}},
		{method: http.MethodGet, path: "/watches/{id}/proposals/{number}", id: "getProposal", summary: "One proposal with its commits, files, diff and replies", req: proposalParams{}, resp: httpd.ProposalDetail{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/proposals/{number}/approve", id: "approveProposal", summary: "Release a proposal that waits on the author, with the replies they changed", req: approveParams{}, resp: httpd.Proposal{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/proposals/{number}/reject", id: "rejectProposal", summary: "Reject a proposal: nothing of it goes out, and the reason goes to the agent", req: rejectParams{}, resp: httpd.Proposal{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/approval", id: "setWatchApproval", summary: "Change who releases the turns of a watch while it runs", req: approvalParams{}, resp: httpd.Watch{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/proposals/{number}/retry", id: "retryProposal", summary: "Release a failed proposal of a watch again, rebased onto a pull request branch that moved", req: proposalParams{}, resp: httpd.Proposal{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodGet, path: "/watches/{id}/output", id: "getWatchOutput", summary: "The last lines the agent of a watched pull request printed", req: outputParams{}, resp: httpd.SessionOutput{}, status: http.StatusOK, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/resize", id: "resizeWatchTerminal", summary: "Set the size of the terminal of the agent of a watched pull request. A session that starts later takes the same size.", req: resizeParams{}, status: http.StatusNoContent, errors: []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/watches/{id}/hook", id: "reportWatchHook", summary: "Report an event of the agent of a watched pull request. The hook command of the agent calls it.", req: hookParams{}, status: http.StatusNoContent, errors: []int{http.StatusBadRequest, http.StatusServiceUnavailable}},
		{method: http.MethodPost, path: "/control/shutdown", id: "shutdown", summary: "Stop the daemon. Rejected for browser origins.", status: http.StatusAccepted, errors: []int{http.StatusForbidden}},
	}

	for _, op := range ops {
		oc, err := r.NewOperationContext(op.method, httpd.Prefix+op.path)
		if err != nil {
			return nil, err
		}
		oc.SetID(op.id)
		oc.SetSummary(op.summary)
		if op.req != nil {
			oc.AddReqStructure(op.req)
		}
		switch {
		case op.resp != nil:
			oc.AddRespStructure(op.resp, openapi.WithHTTPStatus(op.status))
		case op.stream:
			oc.AddRespStructure(nil, openapi.WithHTTPStatus(op.status), openapi.WithContentType("text/event-stream"))
		default:
			oc.AddRespStructure(nil, openapi.WithHTTPStatus(op.status))
		}
		for _, status := range op.errors {
			oc.AddRespStructure(op.errorBody(status), openapi.WithHTTPStatus(status))
		}
		oc.AddRespStructure(httpd.APIError{}, openapi.WithHTTPStatus(http.StatusInternalServerError))
		if err := r.AddOperation(oc); err != nil {
			return nil, err
		}
	}
	return r.Spec.MarshalYAML()
}
