# Pending issues

Issues found while the app runs. Each issue keeps its number for ever:
remove the item when it is fixed, and leave the numbers of the others
where they are. An evidence page takes the number of the issue it proves,
so `docs/evidences/18. …` belongs to issue 18. A page lives as long as
its issue does and goes with the fix.

| #   | Issue                                                                | Evidence |
| --- | -------------------------------------------------------------------- | -------- |
| 04  | prwatch cleanups, as a backlog: 04.4 left                             | none, code only |
| 05  | Nothing guards the page size of the thread comments                   | none |
| 18  | A proposal opens for a turn that already ended                       | [18](<docs/evidences/18. A proposal opens for a turn that already ended.md>), QA: not reproducible; rebase: not reproduced |
| 23  | A message typed right after the opening message reaches the agent cut | [23](<docs/evidences/23. A message typed right after the opening message reaches the agent cut.md>), QA: not reproduced; rebase: not reproduced |
| 25  | The app promises a reject it cannot do, and keeps the refusal on the next proposal | [25](<docs/evidences/25. The app promises a reject it cannot do, and keeps the refusal on the next proposal.md>), refusal fixed, dialog open; rebase: refusal fixed, dialog open |
| 29  | The merge blockers keep the proposal of the last poll until the next poll | none, seen in the check of 27 |
| 38  | The header of a watch cuts the title of the pull request to one letter | none, seen in the check of 30 to 37 |
| 42  | The installed service and the app daemon poll the same repositories twice | none, seen in the check of the rate limit |
| 46  | Two daemons can open the same database, and nothing stops the second | none, seen in the screenshot run of 45 |
| 52  | A message that waits on a hung fetch reaches the agent twice          | none, seen in the live run of the Ghostty terminal |
| 53  | Collapse all leaves a gap above the first file of a scrolled diff     | none, seen in the live run of the proposal diff |

## 04. prwatch cleanups, as a backlog

Found: 2026-09-20, review of `backend/internal/prwatch`, and triaged on
2026-09-22 as code only: design observations with no symptom a user can
reproduce. Seven of the eight are fixed on 2026-09-22 and each fix
carries a test in the suite CI runs. Only 04.8 had a symptom to
reproduce, and the test that shows it went in red; the other six change
no behaviour, and their tests pin the shape the code now has. One is
left, and it is the one that costs a rewrite of the package for no
change of behaviour.

- **04.4** The `self` provider is a mode flag branched at about 20 call
  sites through `hosted`, `runs` and `attended`. One per-watch channel
  interface, with the pty session and the pull channel as its two
  implementations, would hold the rule in one place.

  Read again on 2026-09-22 and sound: the branch stands at 17 sites over
  `message.go`, `next.go`, `lifecycle.go`, `poll.go`, `service.go` and
  `notify.go`, behind the three predicates of `service.go`. It is left
  because the change rewrites the whole message path of the package and
  no test can tell the two shapes apart, so it wants a branch and a
  review of its own.

## 05. Nothing guards the page size of the thread comments

`ghclient.FetchReviewState` reads `comments(first: 100)` of each review
thread and never asks for the next page. `ReviewThread.Authors` is
therefore the authors of the first 100 comments only.

Found again: 2026-09-26, code review of the re-review of answered
threads. The last comment of a thread comes from its own
`comments(last: 1)` read, so an answer is judged on the real last
comment. The list of authors is still cut at 100:

- A thread where the user of the token wrote the first 100 comments
  and a reviewer wrote after them counts as written only by that user.
  It stays unanswered, and holds the re-review, after the agent answers
  it.
- A reviewer that wrote only after comment 100 of an answered thread is
  not in `Threads.Reviewers`, so the daemon does not ask that reviewer.

Settle one of: page `comments` until `hasNextPage` is false, or ask for
the distinct authors in a query that does not depend on the page size.

## 18. A proposal opens for a turn that already ended

**Evidence:** [18](<docs/evidences/18. A proposal opens for a turn that already ended.md>).
**Not reproducible 2026-09-23:** the hook of the agent is synchronous,
so `startTurn` waits on the lock before the turn can end, and the mutex
wakes its waiters in order. Nothing changed. Close it, or keep the
guard as a cleanup.
**QA 2026-09-23: not reproduced either.** 45 turns of a fake agent that
ends a turn about 60 ms after the submit key left no `open` proposal.
**Regression check 2026-09-24 at `3c87078`: not reproduced.** 30 fast
turns left no `open` proposal. `227a845` reads the work branch in
`Hook` before the answer, and the hook stays synchronous.

Found: 2026-09-23, review of `test-battery`.
`backend/internal/prwatch/message.go:179`.

`startTurn` and `endTurn` run in two goroutines that wait on the same
watch lock. The poll holds the lock while it sends a message. The agent
answers fast: the `Active` hook starts `startTurn`, the `Stop` hook
starts `endTurn`. If `endTurn` gets the lock first, it finds no open
proposal and returns. Then `startTurn` opens one. The state is already
idle, so nothing closes it. The commits of the turn wait for the next
message, and `proposalBlocker` ignores open proposals, so the pull
request can show as ready to merge.

Fix: make `startTurn` check the turn it belongs to, as `endTurn` does
with `turnSeq`.

## 23. A message typed right after the opening message reaches the agent cut

**Evidence:** [23](<docs/evidences/23. A message typed right after the opening message reaches the agent cut.md>).
**Not verified:** seen once, no test yet.
**QA 2026-09-23: not reproduced** in two tries, a new watch that reports
the items that exist (1.37 s between the messages) and a restart with a
comment waiting (0.363 s, the gap of the first run). The agent got each
message whole. It stays open.
**Regression check 2026-09-24 at `3c87078`: not reproduced.** A restart
with a comment waiting typed the two messages 0.350 s apart into Claude
Code 2.1.280, and the agent got both whole.

Found: 2026-09-23, the evidence run of 13 to 22, watch 1 on
`deividfortuna/gha-playground#11`. The daemon started a new session and
typed the opening message and a message about one review comment 0.35
seconds apart. The agent got only `ground/pull/11`, the end of the
second message. The daemon marked the comment told, so no poll sends it
again, and the reviewer gets no answer.

`deliverRoutine` blocks only while the agent asks the author something,
so a routine message goes into a session that is busy with its first
turn. Decide: hold a routine message until the opening turn ends, or
type the untold rows into the opening message.

## 25. The app promises a reject it cannot do, and keeps the refusal on the next proposal

**Evidence:** [25](<docs/evidences/25. The app promises a reject it cannot do, and keeps the refusal on the next proposal.md>).
**Test:** `the refusal of a reject does not stay on the next proposal`,
red, then green.
**Stale refusal fixed 2026-09-23:** the context of the decision shows
the error of an approve, a reject or a retry only on the proposal the
request was about (`errorOf` in `proposal-decision.tsx`).
**Still open:** the dialog of a failed proposal whose push landed still
promises that nothing is pushed. The daemon does not say on the
proposal that its push landed, so the app cannot hide the reject yet.
**Regression check 2026-09-24 at `3c87078`: refusal fixed, dialog
open.** The refusal of proposal 5 was not on the panel of proposal 6.
The dialog still promises that nothing is pushed.

Found: 2026-09-23, the QA check of 17 on `gha-playground#12`.
`frontend/src/renderer/components/proposal-panel.tsx`.

The reject dialog of a failed proposal whose push landed says `Nothing
is pushed and nothing is posted` and that the work branch goes back to
the head of the turn. The daemon refuses the reject with a 409, since
the fix of 17. The panel then keeps that refusal: when the next turn
took the reply into proposal 5, the panel of proposal 5 still said
`part of proposal 4 is on GitHub ... retry it to send the rest`. The
alert reads the errors of the mutations of the watch, and nothing
resets them when the proposal changes.

Decide: hide the reject of a failed proposal whose push landed, or let
the daemon say so on the proposal. Reset the errors when the proposal
changes.

## 29. The merge blockers keep the proposal of the last poll until the next poll

Found: 2026-09-24, the live check of the fix of 27, on
`gha-playground#13`. `backend/internal/prwatch/ready.go`, `assess`.
The screenshot `27-fix-takeover-starts-on-the-push.png` shows it.

Only a poll writes `ready_blockers`. The poll after the approval of
proposal 13 stored `proposal 13 of the agent did not go out; retry with
...`. A turn then took proposal 13 over and offered proposal 14. Until
the next poll, the app and `watch status` still said that proposal 13
did not go out and named its retry, while the panel showed proposal 14.
With the default watch interval, that lasts minutes.

Fix: compute the blocker of the proposals when the watch is read, as
`Readiness` does with the state of the agent, or assess again when a
proposal opens, ends or changes state.

## 38. The header of a watch cuts the title of the pull request to one letter

Found: 2026-09-24, the live check of 30 to 37, on `gha-playground#15`,
at a window 1440 pixels wide. Not investigated.

With the approval mode, `approve a clean rebase`, `Stop watching` and
`Merge` in the header, the title *Add a text module (live check of 30 to
37)* shows as *A*. The screenshot of the fix of 30,
`docs/evidences/images/30-fix-one-conversation-answer.png`, shows it.
On `gha-playground#14`, with the same controls, the title kept its first
words. A stopped watch, with fewer controls, shows the whole title.

## 42. The installed service and the app daemon poll the same repositories twice

Found: 2026-09-24, the check of the rate limit with a watch on
`deividfortuna/fipe-go#458`.

`babysitter service install` makes a launchd agent
(`com.deividfortuna.babysitter`) that runs `babysitter serve`. `serve`
runs the repository watcher. The daemon of the app runs the same
watcher on the same database. Nothing stops the two from running
together: `serve` writes no `running.json` and takes no lock, and the
daemon checks `running.json` only.

On the machine, pid 920 (`serve --interval 1m0s`, started about
2026-09-16) and pid 58085 (the app daemon) both polled the 5
repositories and their 33 open pull requests. pid 920 runs a binary
from before the response cache (`335aa1f`, 2026-09-19), so each of its
passes paid for every call: `fipe-proxy-lb` alone took 15 s a pass.
The account spent about 72 core calls a minute, about 4,300 an hour,
with a watch whose own poll costs close to nothing. The rate limit
card showed the spend of both processes as one budget.

The fix of 43 makes the cost smaller: both processes now read through
`github_responses`, so an answer that did not change costs neither of
them a call. Each process still keeps its own memory, so an answer that
changed costs one call in each process. pid 920 gets this only after it
starts again from a binary with the fix.

Settle one of:

- `serve` and `daemon start` refuse to run beside each other on the
  same database, with a lock on the database file.
- Remove `serve` and `service install`: the app daemon already runs the
  watcher.

## 46. Two daemons can open the same database, and nothing stops the second

Found: 2026-09-25, while I took the screenshot of the loading screen
of 45.

I started the development app with `BABYSITTER_DATA_DIR` set to a
scratch directory, to keep it away from the installed app. The data
directory moves `running.json` and the supervisor socket only; the
database stays at `~/Library/Application Support/babysitter/babysitter.db`,
as `babysitter daemon --help` says. The app found no daemon in its data
directory, so it spawned a second one on the database of the installed
app, which ran its own daemon at the same time. For 69 s, the second
daemon synced the 5 repositories twice and continued the Claude session
of watch 55 (`deividfortuna/gha-playground#15`):

```
17:56:20.513 msg="agent session continued (claude, pid 2303)" component=prwatch watch=55 pr=deividfortuna/gha-playground#15 kind=session_started
```

It logged no message to the agent, no push and no reply. Two agents on
one worktree, or two watch loops on one watch, can do all three.

`daemon start` can hold a lock next to the database (or an exclusive
SQLite lock row with the pid) and refuse to start while another live
process holds it. Issue 42 is the same class of problem for the launchd
service.

## 52. A message that waits on a hung fetch reaches the agent twice

Found: 2026-09-27, in the live run of the Ghostty terminal, on watch 1
of `deividfortuna/gha-playground#19`, approval mode `manual`.

I sent a message from the app. Before a message, the daemon fetches the
branch of the pull request. That fetch did not end: after about 95 s
the Send button was still disabled, with no word of what it waited on.
I reloaded the page, which cancelled the request, and the daemon logged:

```
19:30:58.183 level=WARN msg="read the pull request branch before a message" component=prwatch watch=1 err="git fetch -q --no-tags origin +refs/heads/live/auto-watch-mine:refs/remotes/origin/live/auto-watch-mine: git: context canceled"
```

I sent the same message again. It went through in about 4 s, and the
daemon logged one `you told the agent` row. But the pseudo terminal got
the text twice before the Enter, so the agent got it twice in one line:

```
\x1b[2G\xa0 Say in one short sentence what line 6 of greeting.md says. Change nothing.\x1b[78G\x1b[78GSay in one short sentence what line 6 of greeting.md says. Change nothing.
you: Say in one short sentence what line 6 of greeting.md says. Change nothing.Say in one short sentence what line 6 of greeting.md says. Change nothing.
```

So the cancelled send still typed its text after its fetch failed, and
did not submit it. The next message submitted both. A cancelled send
must type nothing, or the send must not wait on the fetch without a
limit. Issue 23 is in the same path of the daemon.

## 53. Collapse all leaves a gap above the first file of a scrolled diff

Found: 2026-09-28, in the live run of the proposal diff, on watch 1 of
the local harness, proposal 1 with 3 files.

When the diff is scrolled and Collapse all folds every file, the three
file headers are 40 px apart as they must be, but the first one starts
about 20 px under the toolbar of the diff. Before the scroll, the first
header touches the toolbar. The scroll position of the viewer of
`@pierre/diffs` seems to stay above 0 when its content gets shorter
than the panel. The page `docs/evidences/proposal-diff. Live run of the
proposal diff redesign.md` shows it in `pd-16-collapse-all.png`.
