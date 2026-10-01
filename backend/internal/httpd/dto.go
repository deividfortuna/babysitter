package httpd

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/deividfortuna/babysitter/internal/agent"
	"github.com/deividfortuna/babysitter/internal/checks"
	"github.com/deividfortuna/babysitter/internal/dependabot"
	"github.com/deividfortuna/babysitter/internal/prwatch"
	"github.com/deividfortuna/babysitter/internal/store"
)

type Health struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
}

type Repo struct {
	ID           int64      `json:"id"`
	Owner        string     `json:"owner"`
	Name         string     `json:"name"`
	FullName     string     `json:"fullName"`
	AddedAt      time.Time  `json:"addedAt"`
	LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty"`
	LastError    string     `json:"lastError"`
}

type RepoList struct {
	Repos []Repo `json:"repos"`
}

type AddRepoRequest struct {
	FullName string `json:"fullName"`
}

type PullRequest struct {
	Repo               string               `json:"repo"`
	Number             int                  `json:"number"`
	Title              string               `json:"title"`
	Author             string               `json:"author"`
	AuthorAvatarURL    string               `json:"authorAvatarUrl,omitempty" description:"The GitHub avatar of the author, absent until the next sync of the repository"`
	Dependabot         bool                 `json:"dependabot" description:"Dependabot opened the pull request and owns its branch"`
	State              store.PRState        `json:"state" enum:"open,closed,merged"`
	Draft              bool                 `json:"draft"`
	BaseRef            string               `json:"baseRef"`
	HeadRef            string               `json:"headRef"`
	HeadSHA            string               `json:"headSha"`
	HTMLURL            string               `json:"htmlUrl"`
	CreatedAt          time.Time            `json:"createdAt"`
	UpdatedAt          time.Time            `json:"updatedAt"`
	MergedAt           *time.Time           `json:"mergedAt,omitempty"`
	ClosedAt           *time.Time           `json:"closedAt,omitempty"`
	MergeableState     store.MergeableState `json:"mergeableState"`
	ReviewDecision     store.ReviewDecision `json:"reviewDecision" enum:"approved,changes_requested,review_required,none"`
	Approvals          int                  `json:"approvals"`
	ChangesRequested   int                  `json:"changesRequested"`
	RequestedReviewers []string             `json:"requestedReviewers"`
	Labels             []string             `json:"labels"`
	Additions          int                  `json:"additions"`
	Deletions          int                  `json:"deletions"`
	CIStatus           checks.CIStatus      `json:"ciStatus" enum:"success,failure,pending,none"`
	SyncedAt           time.Time            `json:"syncedAt"`
	Assignees          []string             `json:"assignees"`
	Fork               bool                 `json:"fork" description:"The head branch lives in another repository, so auto start skips the pull request"`
	UpdateType         dependabot.Level     `json:"updateType,omitempty" enum:",patch,minor,major" description:"The highest update of a Dependabot pull request"`
}

type PullRequestList struct {
	PullRequests []PullRequest `json:"pullRequests"`
}

type PullRequestQuery struct {
	Repo  string `query:"repo" description:"Only one repository, as owner/name"`
	State string `query:"state" enum:"open,all" description:"open (default) or all"`
}

type SyncAccepted struct {
	Accepted bool `json:"accepted"`
}

func repoFromStore(r store.Repo) Repo {
	return Repo{
		ID:           r.ID,
		Owner:        r.Owner,
		Name:         r.Name,
		FullName:     r.FullName(),
		AddedAt:      r.AddedAt,
		LastSyncedAt: r.LastSyncedAt,
		LastError:    r.LastError,
	}
}

func pullFromStore(p store.PullRequest) PullRequest {
	reviewers := p.RequestedReviewers
	if reviewers == nil {
		reviewers = []string{}
	}
	labels := p.Labels
	if labels == nil {
		labels = []string{}
	}
	assignees := p.Assignees
	if assignees == nil {
		assignees = []string{}
	}
	return PullRequest{
		Repo:               p.RepoFullName,
		Number:             p.Number,
		Title:              p.Title,
		Author:             p.Author,
		AuthorAvatarURL:    p.AuthorAvatarURL,
		Dependabot:         agent.IsDependabot(p.Author),
		State:              p.State,
		Draft:              p.Draft,
		BaseRef:            p.BaseRef,
		HeadRef:            p.HeadRef,
		HeadSHA:            p.HeadSHA,
		HTMLURL:            p.HTMLURL,
		CreatedAt:          p.CreatedAt,
		UpdatedAt:          p.UpdatedAt,
		MergedAt:           p.MergedAt,
		ClosedAt:           p.ClosedAt,
		MergeableState:     p.MergeableState,
		ReviewDecision:     p.ReviewDecision,
		Approvals:          p.Approvals,
		ChangesRequested:   p.ChangesRequested,
		RequestedReviewers: reviewers,
		Labels:             labels,
		Additions:          p.Additions,
		Deletions:          p.Deletions,
		CIStatus:           p.CIStatus,
		SyncedAt:           p.SyncedAt,
		Assignees:          assignees,
		Fork:               p.Fork,
		UpdateType:         p.UpdateType,
	}
}

type Watch struct {
	ID                int64                   `json:"id"`
	Repo              string                  `json:"repo"`
	Number            int                     `json:"number"`
	URL               string                  `json:"url"`
	Title             string                  `json:"title"`
	Author            string                  `json:"author"`
	AuthorAvatarURL   string                  `json:"authorAvatarUrl,omitempty" description:"The GitHub avatar of the author, absent until the next poll of an active watch"`
	Dependabot        bool                    `json:"dependabot" description:"Dependabot owns the branch of the pull request, so the daemon never pushes it and the agent only replies"`
	HeadRef           string                  `json:"headRef"`
	BaseRef           string                  `json:"baseRef"`
	Provider          string                  `json:"provider" enum:"claude,copilot,self" description:"The AI provider of the agent session the daemon runs; self means the session that started the watch is its agent and takes each message with the next route"`
	Model             string                  `json:"model"`
	Effort            string                  `json:"effort" description:"The effort level the agent works at; empty takes the default of the model"`
	SourceDir         string                  `json:"sourceDir"`
	WorktreeDir       string                  `json:"worktreeDir"`
	WorkBranch        string                  `json:"workBranch" description:"The private branch of the watch in its worktree"`
	Status            store.WatchStatus       `json:"status" enum:"active,stopped"`
	StopReason        store.StopReason        `json:"stopReason" enum:",user,merged,closed,lost_access,error"`
	IncludeExisting   bool                    `json:"includeExisting"`
	IncludeOwn        bool                    `json:"includeOwn"`
	StartedAt         time.Time               `json:"startedAt"`
	StoppedAt         *time.Time              `json:"stoppedAt,omitempty"`
	LastPollAt        *time.Time              `json:"lastPollAt,omitempty"`
	LastHeartbeatAt   *time.Time              `json:"lastHeartbeatAt,omitempty"`
	LastError         string                  `json:"lastError"`
	HeadSHA           string                  `json:"headSha"`
	PRState           store.PRState           `json:"prState"`
	MergeableState    store.MergeableState    `json:"mergeableState"`
	CheckStates       map[string]checks.State `json:"checkStates"`
	GreenSHA          string                  `json:"greenSha"`
	AgentSession      string                  `json:"agentSession" description:"The conversation id of the agent, kept across restarts of its session"`
	ApprovalsRequired int                     `json:"approvalsRequired" description:"How many approvals the pull request needs before the watch calls it ready to merge"`
	MergeMethod       string                  `json:"mergeMethod" enum:",squash,merge,rebase" description:"The merge method of the watch; empty takes the first one the repository allows"`
	ReadySince        *time.Time              `json:"readySince,omitempty" description:"Since when the pull request is ready to merge; absent while something blocks it, and until the readiness stood a whole poll interval"`
	ReadyBlockers     []string                `json:"readyBlockers" description:"What keeps the pull request from merging, one sentence each"`
	ApprovalMode      store.ApprovalMode      `json:"approvalMode" enum:"auto,manual" description:"Who releases the work of a turn of the agent: the daemon on its own, or the author"`
	AutoApproveRebase bool                    `json:"autoApproveRebase" description:"Approved work goes out after a clean rebase or merge onto a branch that moved, without asking again"`
	PendingProposal   int                     `json:"pendingProposal,omitempty" description:"The number of the proposal that waits on the author, absent when none waits"`
	TakenOverAt       *time.Time              `json:"takenOverAt,omitempty" description:"Since when the session is with the author in their terminal; absent while the daemon has it"`
	AutoReason        store.AutoReason        `json:"autoReason,omitempty" enum:",mine,assigned,dependabot" description:"Why auto start began the watch: the author opened the pull request, it is assigned to the author, or Dependabot opened it; absent for a watch started by hand"`
	MergeWhenReady    bool                    `json:"mergeWhenReady" description:"The daemon merges with the method of the watch as soon as the watch is ready to merge"`
	KeepWorktree      bool                    `json:"keepWorktree" description:"A stop leaves the worktree of the watch on disk unless the stop says otherwise"`
	BranchUpdate      store.BranchUpdate      `json:"branchUpdate" enum:"rebase,merge" description:"rebase: the branch is rebased onto its base; merge: the base is merged into the branch. The agent solves a conflict the same way"`
	BranchUpdater     prwatch.BranchUpdater   `json:"branchUpdater" enum:"dependabot,session,github,agent" description:"Who updates a branch that fell behind its base: Dependabot, the session of the author for a self watch, GitHub first, or the agent"`
	UpdateOnGitHub    bool                    `json:"updateOnGitHub" description:"When the branch falls behind its base, GitHub updates it first with the branch update, and the agent does it only when GitHub refuses"`
	UpdateType        dependabot.Level        `json:"updateType,omitempty" enum:",patch,minor,major" description:"The highest update of a Dependabot pull request; a type the daemon cannot read counts as major"`
	Session           Session                 `json:"session"`
	Summary           *WatchSummary           `json:"summary,omitempty"`
}

type Session struct {
	State     agent.State `json:"state" enum:"none,starting,idle,active,waiting_input,blocked,exited" description:"What the agent does now. none: no session; starting: the process runs and said nothing yet; idle: waits for a message; active: works; waiting_input: asked you a question; blocked: waits on a permission decision; exited: the process ended"`
	PID       int         `json:"pid"`
	StartedAt *time.Time  `json:"startedAt,omitempty"`
	SignalAt  *time.Time  `json:"signalAt,omitempty" description:"When the agent last reported what it does"`
	LogPath   string      `json:"logPath" description:"The file with everything the agent printed"`
}

type WatchSummary struct {
	PRState         store.PRState              `json:"prState"`
	HeadSHA         string                     `json:"headSha"`
	Checks          string                     `json:"checks"`
	MergeableState  store.MergeableState       `json:"mergeableState"`
	Activity        map[store.ActivityKind]int `json:"activity"`
	Messages        int                        `json:"messages" description:"How many messages the agent got"`
	Reason          store.StopReason           `json:"reason"`
	Detail          string                     `json:"detail,omitempty"`
	WorktreeRemoved bool                       `json:"worktreeRemoved" description:"The stop deleted the worktree of the watch"`
	WorkBranchLeft  string                     `json:"workBranchLeft,omitempty" description:"The private branch of the watch that stayed in the checkout of the author"`
}

type WatchList struct {
	Watches []Watch `json:"watches"`
}

type WatchQuery struct {
	Status string `query:"status" enum:"active,all" description:"active (default) or all"`
}

type StartWatchRequest struct {
	Target            string              `json:"target"`
	Repo              string              `json:"repo"`
	Provider          string              `json:"provider,omitempty" enum:",claude,copilot,self" description:"The AI provider that runs the agent session, or self when the caller's own session is the agent; empty takes the repository, then the daemon"`
	Model             string              `json:"model,omitempty" description:"The model of the provider; empty takes the model of the layer that gives the provider"`
	Effort            string              `json:"effort,omitempty" description:"The effort level of that model, one the providers route lists for it; empty takes the effort of the layer that gives the model"`
	SourceDir         string              `json:"sourceDir,omitempty" description:"A git checkout whose origin is the head repository of the pull request; absent makes the daemon clone the head repository into its data directory and use that clone, which needs a hosted provider and a target with the repository and the number"`
	IncludeExisting   *bool               `json:"includeExisting,omitempty" description:"Report the review items the pull request has already; absent takes the repository, then the daemon"`
	IncludeOwn        *bool               `json:"includeOwn,omitempty" description:"Report the comments of the token's own user; absent takes the repository, then the daemon"`
	ApprovalsRequired Optional[int]       `json:"approvalsRequired,omitzero" minimum:"0" nullable:"true" description:"How many approvals the pull request needs before the watch calls it ready to merge; absent takes the repository, then the daemon, 0 asks for none, and null asks for the rule of the base branch whatever the setting holds"`
	MergeMethod       *string             `json:"mergeMethod,omitempty" enum:",squash,merge,rebase" description:"The merge method of the watch: squash, merge, rebase, or empty for the first method the repository allows; absent takes the repository, then the daemon"`
	ApprovalMode      *string             `json:"approvalMode,omitempty" enum:"auto,manual" description:"Who releases the work of a turn of the agent; absent takes the repository, then the daemon, and a self watch runs in auto"`
	AutoApproveRebase *bool               `json:"autoApproveRebase,omitempty" description:"Approved work goes out after a clean rebase or merge onto a branch that moved, without asking again; absent takes the repository, then the daemon"`
	MergeWhenReady    *bool               `json:"mergeWhenReady,omitempty" description:"The daemon merges with the method of the watch as soon as the watch is ready to merge; absent means off"`
	KeepWorktree      *bool               `json:"keepWorktree,omitempty" description:"A stop leaves the worktree of the watch on disk; absent takes the repository, then the daemon"`
	BranchUpdate      *store.BranchUpdate `json:"branchUpdate,omitempty" enum:"rebase,merge" description:"rebase: the branch is rebased onto its base; merge: the base is merged into the branch. The agent solves a conflict the same way; absent takes the repository, then the daemon"`
	UpdateOnGitHub    *bool               `json:"updateOnGitHub,omitempty" description:"When the branch falls behind its base, GitHub updates it first with the branch update, and the agent does it only when GitHub refuses; absent takes the repository, then the daemon"`
}

type ReplyRequest struct {
	InReplyTo int64  `json:"inReplyTo,omitempty" description:"The comment the reply answers: a review comment, in its thread, or a comment on the conversation, on the conversation; absent posts a comment on the pull request"`
	Body      string `json:"body"`
}

type ReplyResult struct {
	Posted   *Activity `json:"posted,omitempty" description:"The comment the reply made, when it was posted at once"`
	Proposal int       `json:"proposal,omitempty" description:"The number of the proposal the reply waits in, when it waits for the end of the turn"`
}

type Proposal struct {
	Number       int                  `json:"number" description:"Counts the proposals of one watch from 1"`
	Status       store.ProposalStatus `json:"status" enum:"open,pending,released,failed,rejected,declined,superseded"`
	HeadSHA      string               `json:"headSha" description:"The head of the pull request branch when the turn started; a turn that only added commits on top of it is rebased onto a head that moved"`
	BaseSHA      string               `json:"baseSha" description:"Where the work branch met that head when the turn started"`
	WorkSHA      string               `json:"workSha" description:"The work branch when the turn ended: what the daemon pushes"`
	HasPush      bool                 `json:"hasPush" description:"The turn made commits the daemon pushes"`
	OpenedAt     time.Time            `json:"openedAt"`
	EndedAt      *time.Time           `json:"endedAt,omitempty"`
	ReleasedAt   *time.Time           `json:"releasedAt,omitempty"`
	Error        string               `json:"error,omitempty" description:"Why the last release failed"`
	ApprovedAt   *time.Time           `json:"approvedAt,omitempty" description:"When the author approved it"`
	DecidedAt    *time.Time           `json:"decidedAt,omitempty" description:"When the author approved or rejected it"`
	PushRejected bool                 `json:"pushRejected" description:"The author let the replies go out without the commits"`
	RebasedFrom  string               `json:"rebasedFrom,omitempty" description:"The work before the daemon moved it onto a head that moved"`
	MovedBy      store.BranchUpdate   `json:"movedBy,omitempty" enum:"rebase,merge" description:"How the daemon moved the work onto the head. rebase: the work is on new commits; merge: the head is merged into the work and its commits stay"`
	Reason       string               `json:"reason,omitempty" description:"What the author said when they rejected it"`
	Replies      []ProposalReply      `json:"replies"`
}

type ProposalReply struct {
	ID        int64      `json:"id"`
	InReplyTo int64      `json:"inReplyTo,omitempty" description:"The comment the reply answers: a review comment, in its thread, or a comment on the conversation; absent comments on the pull request and answers no comment"`
	Body      string     `json:"body" description:"The text of the agent"`
	Edited    string     `json:"edited,omitempty" description:"The text of the author, which goes out in place of the body"`
	Dropped   bool       `json:"dropped" description:"The author took the reply out: it is never posted"`
	PostedURL string     `json:"postedUrl,omitempty" description:"The comment the reply made, once it went out"`
	PostedAt  *time.Time `json:"postedAt,omitempty"`
	Error     string     `json:"error,omitempty" description:"Why the last post failed"`
	DroppedAt *time.Time `json:"droppedAt,omitempty" description:"When the daemon gave up on the reply because GitHub can never take it; error says why"`
	Answers   *Activity  `json:"answers,omitempty" description:"The comment the reply answers, as the watch reported it"`
}

type ProposalList struct {
	Proposals []Proposal `json:"proposals"`
}

type ProposalDetail struct {
	Proposal
	Commits   []ProposalCommit `json:"commits"`
	Commit    string           `json:"commit,omitempty" description:"The commit the files and the diff are of; absent when they are of the whole work"`
	Base      string           `json:"base,omitempty" description:"The commit the files and the diff start from: the head of the proposal, or the parent of commit"`
	Files     []ProposalFile   `json:"files"`
	Diff      string           `json:"diff" description:"The plain unified diff of the work, or of the commit when commit is set"`
	Truncated bool             `json:"truncated" description:"The diff was longer than one megabyte, so it stops at the last whole file under that size; the files after it are in files and not in diff"`
	CodeError string           `json:"codeError,omitempty" description:"Why the commits, files and diff could not be read from the worktree; when set, they are empty and the work is unknown, not absent"`
}

type ProposalQuery struct {
	Commit string `query:"commit" description:"Only the files and the diff of this commit of the proposal, as a full SHA or a prefix of 7 characters or more"`
	Path   string `query:"path" description:"Only the file and the diff of this path, as the files list names it; with commit, of this path in that commit"`
}

type ProposalCommit struct {
	SHA      string `json:"sha"`
	Subject  string `json:"subject"`
	HeldBack bool   `json:"heldBack,omitempty" description:"The commit comes from before the turn: an earlier decision of the author kept it off the pull request, and this proposal carries it again"`
}

type ProposalFile struct {
	Path    string `json:"path"`
	Status  string `json:"status" description:"The letter git gives the change: A, M, D or T"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
	Binary  bool   `json:"binary,omitempty" description:"Git reads the file as binary: the diff has no lines for it, and added and deleted are 0"`
}

type ApproveRequest struct {
	Edits      []ReplyEdit `json:"edits,omitempty" description:"The replies the author rewrote"`
	Drop       []int64     `json:"drop,omitempty" description:"The ids of the replies the author took out"`
	RejectPush bool        `json:"rejectPush,omitempty" description:"Post the replies without pushing the commits"`
	StopAsking bool        `json:"stopAsking,omitempty" description:"Release the proposal and set the watch to auto"`
}

type ReplyEdit struct {
	ReplyID int64  `json:"replyId"`
	Body    string `json:"body"`
}

type RejectRequest struct {
	Reason  string `json:"reason,omitempty" description:"What the agent should do instead; it works on it"`
	Discard bool   `json:"discard,omitempty" description:"Reset the work branch to the head the turn started on, whatever the setting of the worktree says"`
}

type ApprovalRequest struct {
	Mode              *string `json:"mode,omitempty" enum:"auto,manual"`
	AutoApproveRebase *bool   `json:"autoApproveRebase,omitempty"`
	Release           bool    `json:"release,omitempty" description:"Confirm that a switch to auto releases the proposal that waits on the author"`
}

func proposalFromStore(p store.Proposal) Proposal {
	return Proposal{
		Number: p.Number, Status: p.Status, HeadSHA: p.HeadSHA, BaseSHA: p.BaseSHA, WorkSHA: p.WorkSHA, HasPush: p.HasPush,
		OpenedAt: p.OpenedAt, EndedAt: p.EndedAt, ReleasedAt: p.ReleasedAt, Error: p.Error,
		ApprovedAt: p.ApprovedAt, DecidedAt: p.DecidedAt, PushRejected: p.PushRejected, RebasedFrom: p.RebasedFrom, MovedBy: p.MovedBy, Reason: p.Reason,
		Replies: []ProposalReply{},
	}
}

func proposalFromView(v prwatch.ProposalView) Proposal {
	out := proposalFromStore(v.Proposal)
	for _, r := range v.Replies {
		reply := ProposalReply{
			ID: r.ID, InReplyTo: r.InReplyTo, Body: r.Body, Edited: r.Edited, Dropped: r.Dropped,
			PostedURL: r.PostedURL, PostedAt: r.PostedAt, Error: r.Error, DroppedAt: r.DroppedAt,
		}
		if r.Answers != nil {
			answers := activityFromStore(*r.Answers)
			reply.Answers = &answers
		}
		out.Replies = append(out.Replies, reply)
	}
	return out
}

func proposalDetailFrom(d prwatch.ProposalDetail) ProposalDetail {
	out := ProposalDetail{
		Proposal: proposalFromView(d.ProposalView), Commit: d.Commit, Base: d.Base, Diff: d.Diff, Truncated: d.Truncated, CodeError: d.CodeError,
		Commits: make([]ProposalCommit, 0, len(d.Commits)), Files: make([]ProposalFile, 0, len(d.Files)),
	}
	for _, c := range d.Commits {
		out.Commits = append(out.Commits, ProposalCommit{SHA: c.SHA, Subject: c.Subject, HeldBack: slices.Contains(d.HeldBack, c.SHA)})
	}
	for _, f := range d.Files {
		out.Files = append(out.Files, ProposalFile{Path: f.Path, Status: f.Status, Added: f.Added, Deleted: f.Deleted, Binary: f.Binary})
	}
	return out
}

type MergeWatchRequest struct {
	Method  string `json:"method,omitempty" enum:",squash,merge,rebase" description:"The merge method for this merge; empty takes the method of the watch"`
	Approve bool   `json:"approve,omitempty" description:"Approve the Dependabot update in the name of the author before the merge. The daemon refuses a pull request that is not of Dependabot or an update outside the merge scope of the repository"`
}

type UpdateWatchRequest struct {
	ApprovalsRequired Optional[int]       `json:"approvalsRequired,omitzero" minimum:"0" nullable:"true" description:"How many approvals the pull request needs before the watch calls it ready to merge; absent keeps what the watch has, 0 asks for none, and null reads the rule of the base branch again"`
	MergeMethod       *string             `json:"mergeMethod,omitempty" enum:",squash,merge,rebase" description:"The merge method of the watch: squash, merge, rebase, or empty for the first method the repository allows; absent keeps what the watch has"`
	MergeWhenReady    *bool               `json:"mergeWhenReady,omitempty" description:"The daemon merges as soon as the watch is ready to merge; absent keeps what the watch has"`
	BranchUpdate      *store.BranchUpdate `json:"branchUpdate,omitempty" enum:"rebase,merge" description:"rebase: the branch is rebased onto its base; merge: the base is merged into the branch. The agent solves a conflict the same way; absent keeps what the watch has"`
	UpdateOnGitHub    *bool               `json:"updateOnGitHub,omitempty" description:"When the branch falls behind its base, GitHub updates it first with the branch update, and the agent does it only when GitHub refuses; absent keeps what the watch has"`
}

type StopWatchRequest struct {
	KeepWorktree *bool `json:"keepWorktree,omitempty" description:"Leave the worktree of the watch on disk instead of deleting it; absent takes the rule the watch started with"`
}

type TakeoverRequest struct {
	PID   int  `json:"pid" description:"The pid of the process that takes the session; the hand-back waits until it ends"`
	Shell bool `json:"shell,omitempty" description:"The author runs a shell in the worktree and not the agent, so a watch with no conversation keeps none"`
}

type TakeoverResponse struct {
	Watch           Watch    `json:"watch"`
	WorktreeDir     string   `json:"worktreeDir" description:"The worktree the command of the author runs in"`
	WorkBranch      string   `json:"workBranch" description:"The private branch of the watch in the worktree"`
	HeadRef         string   `json:"headRef" description:"The branch of the pull request; push with git push origin HEAD:<headRef>"`
	Argv            []string `json:"argv" description:"The command that continues the conversation of the agent, with none of the rules of the daemon"`
	Declined        []int    `json:"declined" description:"The numbers of the proposals the takeover declined"`
	NewConversation bool     `json:"newConversation" description:"The watch had no conversation yet, so the command starts one"`
	BranchUpdating  bool     `json:"branchUpdating" description:"GitHub is updating the branch; the worktree is still on the old head, so the author fetches before they push"`
}

type HandbackRequest struct {
	Confirm bool `json:"confirm,omitempty" description:"Hand back although the worktree has commits or changes that are not on the pull request"`
	Force   bool `json:"force,omitempty" description:"Hand back although the process that took the session still runs"`
}

type WorkCommit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

type HandbackRefusal struct {
	Error   ErrorBody    `json:"error"`
	Commits []WorkCommit `json:"commits,omitempty" description:"The commits of the work branch that the pull request branch does not have; present with the code unconfirmed_work"`
	Files   []string     `json:"files,omitempty" description:"The changes of the worktree that are not committed, as git status --porcelain prints them; present with the code unconfirmed_work"`
}

type Settings struct {
	PollIntervalSeconds         int      `json:"pollIntervalSeconds" minimum:"10" maximum:"86400" description:"Time between passes of the repository watcher"`
	WatchIntervalSeconds        int      `json:"watchIntervalSeconds" minimum:"10" maximum:"86400" description:"Time between polls of a watched pull request"`
	WatchMaxIntervalSeconds     int      `json:"watchMaxIntervalSeconds" minimum:"10" maximum:"86400" description:"Longest time between polls of a watched pull request where nothing happens. After each quiet poll the time doubles up to this value; activity, running checks or a working agent bring it back to watchIntervalSeconds. The same value as watchIntervalSeconds keeps one fixed interval"`
	CheckMaxIntervalSeconds     int      `json:"checkMaxIntervalSeconds" minimum:"10" maximum:"86400" description:"Longest time between two reads of the pending checks of an open pull request by the repository watcher. The wait starts at one minute, or at pollIntervalSeconds or this value when one is shorter, and doubles after each read that finds the checks still pending, up to this value. A new head commit or a manual sync starts it again"`
	ApprovalsRequired           *int     `json:"approvalsRequired" minimum:"0" description:"How many approvals a new watch wants before it calls the pull request ready to merge; null takes the rule of the base branch"`
	MergeMethod                 string   `json:"mergeMethod" enum:",squash,merge,rebase" description:"The merge method of a new watch; empty takes the first one the repository allows"`
	IncludeExisting             bool     `json:"includeExisting" description:"A new watch reports the review items the pull request has already"`
	IncludeOwn                  bool     `json:"includeOwn" description:"A new watch reports the comments of the token's own user"`
	KeepWorktree                bool     `json:"keepWorktree" description:"A watch that stops leaves its worktree on disk"`
	NotificationsEnabled        bool     `json:"notificationsEnabled" description:"What happens on a watched pull request is shown as a notification of the operating system"`
	NotificationsBackgroundOnly bool     `json:"notificationsBackgroundOnly" description:"The app shows a notification only while none of its windows has the focus"`
	MutedNotificationKinds      []string `json:"mutedNotificationKinds" items.enum:"agent,review,checks,watch,merge,auto" description:"The notification kinds that reach nobody. The history keeps them either way"`
	SilentNotificationKinds     []string `json:"silentNotificationKinds" items.enum:"agent,review,checks,watch,merge,auto" description:"The notification kinds that arrive without a sound"`
	ApprovalMode                string   `json:"approvalMode" enum:"auto,manual" description:"Who releases the work of a turn of the agent of a new watch: the daemon on its own, or the author"`
	AutoApproveRebase           bool     `json:"autoApproveRebase" description:"Approved work goes out after a clean rebase or merge onto a branch that moved, without asking again"`
	Provider                    string   `json:"provider" enum:"claude,copilot" description:"The AI provider of a new watch"`
	Model                       string   `json:"model" description:"The model of that provider; empty takes the default of the provider"`
	Effort                      string   `json:"effort" description:"The effort level of that model, one the providers route lists for it; empty takes the default of the model"`
	BranchUpdate                string   `json:"branchUpdate" enum:"rebase,merge" description:"How a new watch updates a branch that fell behind its base. rebase: the branch is rebased onto its base; merge: the base is merged into the branch. The agent solves a conflict the same way"`
	UpdateOnGitHub              bool     `json:"updateOnGitHub" description:"A new watch asks GitHub to update a branch that fell behind its base, and the agent does it only when GitHub refuses"`
	ScreenReader                bool     `json:"screenReader" description:"The agent runs in the screen reader mode of its command line, which draws plain text in place of the full terminal interface. A change takes effect the next time an agent session starts"`
}

type Notification struct {
	ID        int64      `json:"id"`
	WatchID   int64      `json:"watchId,omitempty" description:"The watch the notification belongs to; absent for one that belongs to none"`
	Kind      string     `json:"kind" enum:"agent,review,checks,watch,merge,auto" description:"agent: the agent asks you for something; review: a review item nobody takes; checks: the checks of the pull request; watch: the life of a watch and of its session; merge: the pull request can merge, merged, or failed to; auto: a watch started on its own, or a Dependabot update waits on your approval"`
	Action    string     `json:"action,omitempty" enum:",approve_merge" description:"What the app offers to do from the notification. approve_merge: approve the Dependabot update in your name and merge it"`
	Repo      string     `json:"repo"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	URL       string     `json:"url"`
	Silent    bool       `json:"silent,omitempty" description:"The notification asked for no sound, whatever the settings hold"`
	CreatedAt time.Time  `json:"createdAt"`
	ReadAt    *time.Time `json:"readAt,omitempty" description:"When the user saw the notification; absent while it is unread"`
}

type NotificationList struct {
	Notifications []Notification `json:"notifications"`
	UnreadCount   int            `json:"unreadCount" description:"How many notifications are unread, whatever the page holds"`
}

type NotificationQuery struct {
	Status string `query:"status" enum:"all,unread" description:"all (default) or unread"`
	Limit  string `query:"limit" description:"How many rows at most: 1 to 1000, 200 by default"`
}

type NewNotificationRequest struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Subtitle string `json:"subtitle,omitempty" description:"The second line of the notification the operating system shows"`
	URL      string `json:"url,omitempty"`
	Kind     string `json:"kind,omitempty" enum:",agent,review,checks,watch,merge,auto" description:"Empty means agent: the agent asks you for something"`
	WatchID  int64  `json:"watchId,omitempty"`
	Repo     string `json:"repo,omitempty"`
	Number   int    `json:"number,omitempty"`
	Silent   bool   `json:"silent,omitempty" description:"Do not make a sound"`
}

type ReadNotificationsRequest struct {
	IDs []int64 `json:"ids,omitempty"`
}

type NotificationsRead struct {
	UnreadCount int `json:"unreadCount"`
}

func notificationFromStore(n store.Notification) Notification {
	return Notification{
		ID:        n.ID,
		WatchID:   n.WatchID,
		Kind:      string(n.Kind),
		Repo:      n.Repo,
		Number:    n.Number,
		Title:     n.Title,
		Body:      n.Body,
		URL:       n.URL,
		Silent:    n.Silent,
		Action:    string(n.Action),
		CreatedAt: n.CreatedAt,
		ReadAt:    n.ReadAt,
	}
}

type SendMessageRequest struct {
	Message string `json:"message"`
}

const (
	MaxTerminalRows = 500
	MaxTerminalCols = 1000
)

type ResizeRequest struct {
	Rows uint16 `json:"rows" minimum:"1" maximum:"500" description:"Rows of the terminal of the agent, 1 to 500"`
	Cols uint16 `json:"cols" minimum:"1" maximum:"1000" description:"Columns of the terminal of the agent, 1 to 1000"`
}

func (r ResizeRequest) valid() bool {
	return r.Rows >= 1 && r.Rows <= MaxTerminalRows && r.Cols >= 1 && r.Cols <= MaxTerminalCols
}

type SessionOutput struct {
	Output string `json:"output"`
}

type NextMessage struct {
	Watch   Watch     `json:"watch"`
	Message *Activity `json:"message,omitempty" description:"The nudged row of the message the agent takes now, with the text in its payload; absent when there is nothing to do"`
}

type NextQuery struct {
	Wait string `query:"wait" description:"How long to wait for a message when there is none, as a Go duration such as 5m; default 0"`
}

type OutputQuery struct {
	Lines int `query:"lines" description:"At most this many lines from the end, default 200"`
}

type HookRequest struct {
	Event   string         `json:"event" enum:"session-start,user-prompt-submit,pre-tool-use,post-tool-use,post-tool-use-failure,permission-request,stop,notification,session-end"`
	Payload map[string]any `json:"payload"`
}

type HookResponse struct {
	Decision string `json:"decision" enum:"allow,deny" description:"For pre-tool-use, whether the agent may run the tool; every other event gets allow"`
	Reason   string `json:"reason,omitempty" description:"Why the daemon refuses the tool, for the agent to read"`
}

const (
	HookAllow = "allow"
	HookDeny  = "deny"
)

func hookResponse(v prwatch.Verdict) HookResponse {
	if v.Deny {
		return HookResponse{Decision: HookDeny, Reason: v.Reason}
	}
	return HookResponse{Decision: HookAllow}
}

type Activity struct {
	ID         int64              `json:"id"`
	WatchID    int64              `json:"watchId"`
	Kind       store.ActivityKind `json:"kind" enum:"comment,review_comment,review,check_failed,check_recovered,checks_green,commit,behind,conflict,merged,closed,heartbeat,watch_started,watch_stopped,session_started,session_exited,nudged,agent_failed,merge_ready,merge_failed,replied,review_requested,proposal,taken_over,handed_back,auto_started,approved,approval_asked,branch_updated,branch_update_failed"`
	Ref        string             `json:"ref"`
	At         time.Time          `json:"at"`
	Actor      string             `json:"actor"`
	Summary    string             `json:"summary"`
	URL        string             `json:"url"`
	Payload    map[string]any     `json:"payload"`
	Reported   bool               `json:"reported"`
	ReportedAt *time.Time         `json:"reportedAt,omitempty"`
	NudgedAt   *time.Time         `json:"nudgedAt,omitempty" description:"When the row reached the agent"`
}

type ActivityList struct {
	Activity []Activity `json:"activity"`
}

type ActivityQuery struct {
	Since int64 `query:"since" description:"Only rows with an id above this"`
	Limit int   `query:"limit" description:"At most this many rows, default 200"`
}

func watchFromStore(w store.Watch, s prwatch.SessionInfo, readySince *time.Time, blockers []string) Watch {
	states := w.CheckStates
	if states == nil {
		states = map[string]checks.State{}
	}
	out := Watch{
		ID: w.ID, Repo: w.Repo(), Number: w.Number, URL: w.URL, Title: w.Title, Author: w.Author, AuthorAvatarURL: w.AuthorAvatarURL,
		Dependabot: agent.IsDependabot(w.Author),
		HeadRef:    w.HeadRef, BaseRef: w.BaseRef, Provider: w.Provider, Model: w.Model, Effort: w.Effort, SourceDir: w.SourceDir, WorktreeDir: w.WorktreeDir,
		WorkBranch: w.WorkBranch, Status: w.Status, StopReason: w.StopReason, IncludeExisting: w.IncludeExisting, IncludeOwn: w.IncludeOwn,
		StartedAt: w.StartedAt, StoppedAt: w.StoppedAt, LastPollAt: w.LastPollAt, LastHeartbeatAt: w.LastHeartbeatAt,
		LastError: w.LastError, HeadSHA: w.HeadSHA, PRState: w.PRState, MergeableState: w.MergeableState,
		CheckStates: states, GreenSHA: w.GreenSHA, AgentSession: w.AgentSession,
		ApprovalsRequired: w.ApprovalsRequired, MergeMethod: w.MergeMethod, ReadySince: readySince, ReadyBlockers: blockers,
		ApprovalMode: w.ApprovalMode, AutoApproveRebase: w.AutoApproveRebase, TakenOverAt: w.TakenOverAt,
		AutoReason: w.AutoReason, MergeWhenReady: w.MergeWhenReady, KeepWorktree: w.KeepWorktree, UpdateType: w.UpdateType,
		BranchUpdate: w.BranchUpdate, UpdateOnGitHub: w.UpdateOnGitHub, BranchUpdater: prwatch.BranchUpdaterOf(w),
		Session: Session{State: s.State, PID: s.PID, StartedAt: s.StartedAt, SignalAt: s.SignalAt, LogPath: s.LogPath},
	}
	if out.Session.State == "" {
		out.Session.State = agent.StateNone
	}
	if w.Status == store.WatchStopped && len(w.Summary) > 2 {
		var sum WatchSummary
		if err := json.Unmarshal(w.Summary, &sum); err == nil {
			if sum.Activity == nil {
				sum.Activity = map[store.ActivityKind]int{}
			}
			out.Summary = &sum
		}
	}
	return out
}

func activityFromStore(a store.Activity) Activity {
	payload := map[string]any{}
	_ = json.Unmarshal(a.Payload, &payload)
	return Activity{
		ID: a.ID, WatchID: a.WatchID, Kind: a.Kind, Ref: a.Ref, At: a.At, Actor: a.Actor, Summary: a.Summary,
		URL: a.URL, Payload: payload, Reported: a.Reported, ReportedAt: a.ReportedAt, NudgedAt: a.NudgedAt,
	}
}
