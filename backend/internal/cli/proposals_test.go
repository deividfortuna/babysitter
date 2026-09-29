package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/deividfortuna/babysitter/internal/httpd"
	"github.com/deividfortuna/babysitter/internal/store"
)

const pendingProposalJSON = `{"number":2,"status":"pending","headSha":"abcdef1234567","baseSha":"abcdef1234567","workSha":"1a2b3c4d5e6f7","hasPush":true,
	"openedAt":"2026-09-07T12:04:00Z","pushRejected":false,
	"replies":[{"id":5,"inReplyTo":31,"body":"renamed it","dropped":false,
		"answers":{"id":7,"watchId":1,"kind":"review_comment","ref":"31","at":"2026-09-07T12:03:00Z","actor":"bob","summary":"bob commented on x.go:4: rename this","url":"","payload":{"body":"rename this"},"reported":true}},
		{"id":6,"body":"the failure is on main too","dropped":false}]}`

const failedProposalJSON = `{"number":1,"status":"failed","headSha":"abcdef1234567","baseSha":"abcdef1234567","workSha":"9f8e7d6c5b4a3","hasPush":true,
	"openedAt":"2026-09-07T12:01:00Z","pushRejected":false,"error":"push proposal 1: no credential","replies":[]}`

func (d *fakeDaemon) proposalRoutes() {
	d.mux.HandleFunc("GET /api/v1/watches/1/proposals", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"proposals":[%s,%s]}`, pendingProposalJSON, failedProposalJSON)
	})
	d.mux.HandleFunc("GET /api/v1/watches/1/proposals/2", func(w http.ResponseWriter, r *http.Request) {
		commit := ""
		if r.URL.Query().Get("commit") == "1a2b3c4" {
			commit = `"commit":"1a2b3c4d5e6f7",`
		}
		code := `"files":[{"path":"x.go","status":"M","added":3,"deleted":1}],"diff":"diff --git a/x.go b/x.go\n-old\n+new\n","truncated":false}`
		if r.URL.Query().Get("path") == "logo.png" {
			code = `"files":[{"path":"logo.png","status":"A","added":0,"deleted":0,"binary":true}],"diff":"","truncated":true}`
		}
		detail := strings.TrimSuffix(pendingProposalJSON, "}") +
			`,"commits":[{"sha":"1a2b3c4d5e6f7","subject":"Rename the thing"}],` + commit + code
		fmt.Fprint(w, detail)
	})
	record := func(kind string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			d.decisions = append(d.decisions, kind+" "+r.PathValue("number")+" "+strings.TrimSpace(string(body)))
			status := "released"
			if kind == "reject" {
				status = "rejected"
			}
			fmt.Fprintf(w, `{"number":%s,"status":%q,"headSha":"abcdef1234567","baseSha":"abcdef1234567","workSha":"1a2b3c4d5e6f7","hasPush":true,"openedAt":"2026-09-07T12:04:00Z","pushRejected":%v,"replies":[]}`,
				r.PathValue("number"), status, strings.Contains(string(body), `"rejectPush":true`))
		}
	}
	d.mux.HandleFunc("POST /api/v1/watches/1/proposals/{number}/approve", record("approve"))
	d.mux.HandleFunc("POST /api/v1/watches/1/proposals/{number}/reject", record("reject"))
	d.mux.HandleFunc("POST /api/v1/watches/1/approval", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)
		d.decisions = append(d.decisions, "approval "+string(raw))
		if body["mode"] == "auto" && body["release"] != true {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"error":{"code":"proposal_pending","message":"a proposal waits on your approval: proposal 2; switching to auto releases it, so confirm the release"}}`)
			return
		}
		one := strings.TrimSuffix(strings.TrimPrefix(d.watches, "["), "]")
		fmt.Fprint(w, strings.Replace(one, `"status":"active"`, fmt.Sprintf(`"status":"active","approvalMode":%q`, body["mode"]), 1))
	})
}

func TestWatchProposalsListsAndShowsOne(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.proposalRoutes()

	out, err := runWatch(t, d, "proposals", "1")
	if err != nil {
		t.Fatalf("proposals = %v", err)
	}
	for _, want := range []string{"PROPOSAL", "2", "pending", "1a2b3c4", "2 replies", "failed", "push proposal 1: no credential"} {
		if !strings.Contains(out, want) {
			t.Errorf("the list lacks %q:\n%s", want, out)
		}
	}

	out, err = runWatch(t, d, "proposals", "1", "2")
	if err != nil {
		t.Fatalf("proposal 2 = %v", err)
	}
	for _, want := range []string{
		"Proposal:  2 of watch 1", "Status:    pending", "1a2b3c4 Rename the thing", "M x.go +3 -1",
		"reply 5 to comment 31 of bob:", "renamed it", "reply 6 on the pull request:", "babysitter watch approve 1 2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the detail lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "+new") || strings.Contains(out, "Commit:") {
		t.Fatalf("the diff or a commit shows without --diff or --commit:\n%s", out)
	}
	if out, err = runWatch(t, d, "proposals", "1", "2", "--commit", "1a2b3c4"); err != nil || !strings.Contains(out, "Commit:    1a2b3c4 only, for the files and the diff") {
		t.Fatalf("proposal 2 of one commit = %q, %v", out, err)
	}
	if out, err = runWatch(t, d, "proposals", "1", "2", "--diff"); err != nil || !strings.Contains(out, "diff --git a/x.go b/x.go\n-old\n+new") {
		t.Fatalf("proposal 2 with the diff = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "proposals", "1", "2", "--diff", "--file", "logo.png")
	if err != nil || !strings.Contains(out, "A logo.png binary") || !strings.Contains(out, "The diff of logo.png is longer than one megabyte") {
		t.Fatalf("proposal 2 of one binary file = %q, %v", out, err)
	}
}

func TestWatchApproveAndReject(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.proposalRoutes()

	out, err := runWatch(t, d, "approve", "1")
	if err != nil || out != "Proposal 2 of watch 1 went out: 1a2b3c4 is on fix.\n" {
		t.Fatalf("approve = %q, %v", out, err)
	}
	if _, err := runWatch(t, d, "approve", "1", "2", "--edit", "5=Renamed it, thanks.", "--drop", "6", "--reject-push", "--stop-asking"); err != nil {
		t.Fatalf("approve with a decision = %v", err)
	}
	if _, err := runWatch(t, d, "approve", "1", "2", "--edit", "five=x"); err == nil {
		t.Fatal("an edit of reply five went through")
	}
	out, err = runWatch(t, d, "reject", "1", "--reason", "use a table test", "--discard")
	if err != nil || !strings.Contains(out, "Rejected proposal 2 of watch 1") {
		t.Fatalf("reject = %q, %v", out, err)
	}
	want := []string{
		"approve 2 {}",
		`approve 2 {"edits":[{"replyId":5,"body":"Renamed it, thanks."}],"drop":[6],"rejectPush":true,"stopAsking":true}`,
		`reject 2 {"reason":"use a table test","discard":true}`,
	}
	if strings.Join(d.decisions, "\n") != strings.Join(want, "\n") {
		t.Fatalf("decisions = %q, want %q", d.decisions, want)
	}
}

func TestWatchModeSwitchesAndConfirmsTheRelease(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.proposalRoutes()

	if _, err := runWatch(t, d, "mode", "1", "auto"); err == nil || !strings.Contains(err.Error(), "--release") {
		t.Fatalf("a switch without the release = %v, want the flag named", err)
	}
	out, err := runWatch(t, d, "mode", "1", "auto", "--release")
	if err != nil || !strings.Contains(out, "Watch 1 runs in auto") {
		t.Fatalf("mode auto = %q, %v", out, err)
	}
	if out, err = runWatch(t, d, "mode", "1", "manual", "--auto-approve-rebase"); err != nil || !strings.Contains(out, "Watch 1 runs in manual") {
		t.Fatalf("mode manual = %q, %v", out, err)
	}
	if _, err := runWatch(t, d, "mode", "1", "sometimes"); err == nil {
		t.Fatal("mode sometimes went through")
	}
	want := []string{`approval {"mode":"auto"}`, `approval {"mode":"auto","release":true}`, `approval {"autoApproveRebase":true,"mode":"manual"}`}
	if strings.Join(d.decisions, "\n") != strings.Join(want, "\n") {
		t.Fatalf("decisions = %q, want %q", d.decisions, want)
	}
}

func TestWatchRetryTakesTheFailedProposal(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.proposalRoutes()
	out, err := runWatch(t, d, "retry", "1")
	if err != nil || !strings.Contains(out, "Proposal 1 of watch 1 went out") {
		t.Fatalf("retry = %q, %v", out, err)
	}
	if d.retries[0] != "1" {
		t.Fatalf("retries = %v", d.retries)
	}
}

func TestWatchStatusSaysWhoReleasesTheWork(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.watches = strings.Replace(d.watches, `"status":"active"`, `"status":"active","approvalMode":"manual","pendingProposal":2`, 1)
	out, err := runWatch(t, d, "status", "1")
	if err != nil || !strings.Contains(out, "Approval:  manual; proposal 2 waits on you: babysitter watch proposals 1 2") {
		t.Fatalf("status = %q, %v", out, err)
	}
}

func TestWatchStartTakesTheApprovalMode(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	if _, err := runWatch(t, d, "start", "octo/hello#3"); err != nil {
		t.Fatal(err)
	}
	if _, err := runWatch(t, d, "start", "octo/hello#3", "--approval-mode", "manual", "--auto-approve-rebase"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.starts[0]["approvalMode"]; ok {
		t.Fatalf("a start without the flag named a mode: %v", d.starts[0])
	}
	if d.starts[1]["approvalMode"] != "manual" || d.starts[1]["autoApproveRebase"] != true {
		t.Fatalf("start = %v", d.starts[1])
	}
}

func TestWatchStartSaysASelfWatchIgnoresTheApprovalFlags(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.watches = strings.Replace(d.watches, `"session":{"state":"idle","pid":4242`, `"provider":"self","session":{"state":"none","pid":0`, 1)

	out, err := runWatch(t, d, "start", "octo/hello#3", "--provider", "self", "--approval-mode", "manual", "--auto-approve-rebase")
	if err != nil || !strings.Contains(out, "ignores --approval-mode manual and --auto-approve-rebase") {
		t.Fatalf("start = %q, %v", out, err)
	}
	out, err = runWatch(t, d, "start", "octo/hello#3", "--provider", "self", "--approval-mode", "auto")
	if err != nil || strings.Contains(out, "ignores") {
		t.Fatalf("a self start that asked for auto = %q, %v", out, err)
	}
}

func TestSettingsSetTheApprovalMode(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.settings["approvalMode"] = "manual"
	d.settings["autoApproveRebase"] = false
	out, err := runAgainstDaemon(t, d, "settings", "set", "--approval-mode", "auto", "--auto-approve-rebase")
	if err != nil {
		t.Fatalf("settings set = %v", err)
	}
	if put := d.settingsPut[0]; put["approvalMode"] != "auto" || put["autoApproveRebase"] != true {
		t.Fatalf("put = %v", put)
	}
	if !strings.Contains(out, "Approval mode") || !strings.Contains(out, "Approve a clean rebase on its own") {
		t.Fatalf("settings = %q", out)
	}
}

func TestAReplyTheDaemonDroppedSaysWhy(t *testing.T) {
	t.Parallel()
	var r httpd.ProposalReply
	raw := `{"id":7,"inReplyTo":31,"body":"renamed it","dropped":false,"error":"no such review comment: 31","droppedAt":"2026-09-23T10:00:00Z"}`
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	if got, want := replyHeading(r), "reply 7 to comment 31, dropped: no such review comment: 31"; got != want {
		t.Fatalf("replyHeading() = %q, want %q", got, want)
	}
}

func TestAProposalSaysHowTheDaemonMovedIt(t *testing.T) {
	t.Parallel()
	cases := map[store.BranchUpdate]string{
		store.BranchRebase: "rebased from 4e7d0b8",
		store.BranchMerge:  "merged 7a1b2c3 into 4e7d0b8",
	}
	for by, want := range cases {
		p := httpd.Proposal{HeadSHA: "7a1b2c3d4e5f6", RebasedFrom: "4e7d0b8bbbbbb", MovedBy: by}
		if got := proposalNote(p); got != want {
			t.Errorf("proposalNote(moved by %s) = %q, want %q", by, got, want)
		}
	}
}

func TestTheListCountsTheRepliesThatGoOut(t *testing.T) {
	t.Parallel()
	var p httpd.Proposal
	raw := `{"number":2,"status":"released","replies":[
		{"id":5,"inReplyTo":31,"body":"renamed it","dropped":true},
		{"id":6,"inReplyTo":32,"body":"gone","dropped":false,"error":"no such comment","droppedAt":"2026-09-23T10:00:00Z"},
		{"id":7,"body":"the failure is on main too","dropped":false}]}`
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := (proposalListOutput{Proposals: []httpd.Proposal{p}}).writeText(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1 reply, 2 dropped") {
		t.Fatalf("list = %q", out.String())
	}
}

func TestWatchStatusNamesTheCleanRebaseWhileAProposalWaits(t *testing.T) {
	t.Parallel()
	d := newFakeDaemon()
	d.watches = strings.Replace(d.watches, `"status":"active"`, `"status":"active","approvalMode":"manual","autoApproveRebase":true,"pendingProposal":2`, 1)
	out, err := runWatch(t, d, "status", "1")
	want := "Approval:  manual, and approved work goes out after a clean rebase; proposal 2 waits on you: babysitter watch proposals 1 2"
	if err != nil || !strings.Contains(out, want) {
		t.Fatalf("status = %q, %v", out, err)
	}
}

func TestAProposalMarksTheCommitTheAuthorKeptOff(t *testing.T) {
	t.Parallel()
	var d httpd.ProposalDetail
	raw := `{"number":8,"status":"pending","headSha":"18a8da1","workSha":"472a1ca","hasPush":true,"replies":[],
		"commits":[{"sha":"e1197bc","subject":"Strip punctuation in slug","heldBack":true},{"sha":"472a1ca","subject":"Guard null"}],"files":[],"diff":""}`
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := (proposalOutput{watch: 1, ProposalDetail: d}).writeText(&out); err != nil {
		t.Fatal(err)
	}
	want := "Commits:   e1197bc Strip punctuation in slug (kept off by an earlier decision)\n           472a1ca Guard null\n"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("proposal = %q", out.String())
	}
}

func TestAProposalWhoseCodeCouldNotBeReadSaysSoInsteadOfNothing(t *testing.T) {
	t.Parallel()
	var d httpd.ProposalDetail
	raw := `{"number":8,"status":"pending","headSha":"18a8da1","workSha":"472a1ca","hasPush":true,"replies":[],
		"commits":[],"files":[],"diff":"","codeError":"git log: exit status 128"}`
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := (proposalOutput{watch: 1, ProposalDetail: d, diff: true}).writeText(&out); err != nil {
		t.Fatal(err)
	}
	want := "Code:      not read from the worktree: git log: exit status 128\n"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("proposal = %q", out.String())
	}
}
