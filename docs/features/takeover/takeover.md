# Takeover

A takeover moves the agent session of a watch from the pseudo terminal
of the daemon to the terminal of the author. A hand-back gives it back.
`prd.md` holds the requirements that this design numbers as R1 to R24.

```
            takeover                         handback
 daemon ───────────────► author terminal ───────────────► daemon
 pty session             same conversation                 pty session
 rules, hooks            no rules, no hooks                --resume
```

## The state

A watch that is taken over stays `active`, so the poll loop keeps
running (R10). Two new columns on `watches` hold the state:

- `taken_over_at`, a nullable time. A value means the session is with
  the author.
- `taken_over_pid`, the pid of the CLI process that took over.

`store.Watch` gets `TakenOverAt *time.Time` and `TakenOverPID int`, and
`store.SetWatchTakeover` writes both in one statement. The migration adds
both columns with no value. The DTO `httpd.Watch` gets `takenOverAt`, so
the app and `watch status` read it with no other call (R22).

Two new activity kinds record the moves: `taken_over` (R5) and
`handed_back` (R19). They do not notify.

## Takeover

### The route

`POST /api/v1/watches/{id}/takeover` with `{"pid": <int>}`, plus
`"shell": true` when the author runs a shell and not the agent.

`prwatch.Service.Takeover` does these steps under `s.locks.lock(id)`
(R4), in this order:

1. Refuse a self watch, a stopped watch and a watch that is already taken
   over (R9).
2. Build the command of the author, with a new session id when the
   watch has no conversation. A failure here changes nothing.
3. Write `taken_over_at` and `taken_over_pid`. The write comes before
   the steps that follow, so a poll that runs after the lock sees the
   state and does not start the session again.
4. `s.declineProposals(ctx, w)` (R3). The open proposal of the turn that
   the takeover cuts short is `Waiting()`, so it is declined with the
   `pending` and `failed` ones, in one `UPDATE`: all of them or none. A
   proposal that is not declined could still be approved, so a failed
   decline refuses the takeover. The decline comes before the stop: a
   refused takeover then leaves the session of the daemon running, and
   no open turn stays without a session to end it.
5. `s.handOverSession(ctx, w)`. It calls `beginStop`, so `watchExit`
   finds `stopping` and records no `session_exited` row and calls no
   `endTurn` (R2). When the process does not stop in `stopTimeout`, the
   session goes back to the daemon, the proposals of step 4 get their
   status back, and the takeover is refused with 409 `session_running`:
   two processes on one conversation corrupt it.
6. Store the new session id, when step 2 made one.
7. Record the `taken_over` row, with the declined numbers in its text as
   `stop` does with `declinedWord`.

When a step from 4 on fails, the write of step 3 and the session id of
step 6 are undone, so the watch stays with the daemon. After step 5 no
proposal waits, so the next message starts a new session.

The response holds what the CLI needs:

```json
{
  "watch": { "...": "..." },
  "worktreeDir": "/…/worktrees/acme-api-341",
  "workBranch": "babysitter/acme-api-341",
  "headRef": "fix-login",
  "argv": ["claude", "--resume", "3f0c…", "--model", "opus"],
  "declined": [4]
}
```

### The command of the author

The runner builds the command line, because the runner knows the flags
of its provider. `agent.Runner` gets one method:

```go
AuthorCommand(l Launch) ([]string, error)
```

- Claude: `claude --resume <session> --permission-mode manual`, plus
  `--model` when the watch has one. Claude files a session under the
  directory it ran in, so the CLI must run it in the worktree. A resume
  keeps the permission mode of the conversation, which is the `dontAsk`
  of the daemon, and that mode refuses every tool the rules do not allow.
  `manual` gives the author the prompts of a session of their own.
- Copilot: the flag that continues `<session>`, plus `--model`. Check
  the flag before cut 1: the runner uses `--session-id` today for a new
  session and for a resume.

The method adds nothing of `Command`: no permission mode of the daemon,
no tool rules, no system prompt, no `--settings`, no `GIT_CONFIG_*` env (R7). The trust
of the worktree that `acceptTrust` wrote for the daemon stays, so Claude
does not ask again.

A watch with no `AgentSession` yet gets a new conversation, and the
takeover response says so. A takeover with `shell` builds no command
and keeps the watch with no `AgentSession`: nobody starts that
conversation, so the hand-back must not resume it.

### The CLI

`babysitter watch takeover <watch> [--shell]` in `internal/cli/watch.go`,
through `onWatch` like the other commands (R1).

1. Call the route with `os.Getpid()`.
2. Print the banner to stderr (R6):

   ```
   The session of acme/api#341 is now yours.
     worktree  /…/worktrees/acme-api-341
     branch    babysitter/acme-api-341
     push      git push origin HEAD:fix-login
   Give it back with: babysitter watch handback acme/api#341
   ```

   A plain `git push` fails: the private branch tracks `origin/<head>`
   under another name, and `push.default=simple` refuses that.
3. `chdir` to the worktree and run `argv`, or `$SHELL` with `--shell`
   (R8). On macOS and Linux the CLI calls `syscall.Exec`, so the pid of
   step 1 is the pid of the agent. On Windows it starts a child with the
   terminal passed through, ignores Ctrl-C until the child exits, and
   exits with its code. There the pid of step 1 is the CLI, which lives
   as long as the child.

When the exec fails, the watch stays taken over with a pid that is dead,
so a hand-back works at once.

## While the author has the session

Each rule is one check of `w.TakenOverAt != nil`, behind a method with a
name: `s.withAuthor(w)`.

- **Messages (R11).** `tell` in `poll.go` returns the rows untold next to
  `holdsForProposal`, with the log line `the session is with you, the
  message waits for the hand-back`. The rows keep their state, as they
  do while a proposal waits.
- **Send (R12).** `Service.Send` refuses with `ErrTakenOver`: `the
  session is with you; run babysitter watch handback first`. The HTTP
  layer maps it to 409.
- **Hooks (R13).** `Service.Hook` already returns when `s.sessions.get`
  finds no live session, and the takeover removes it. A test keeps this
  true, because the Copilot hooks file stays in the worktree and the
  author's Copilot session reads it.
- **Readiness (R14).** `blockers` in `ready.go` adds `the session is with
  you`. `assess` then stores no ready clock, sends no `merge_ready` row
  and asks for no re-review, and `Merge` refuses as it does for any
  blocker.
- **Stop (R15).** `stop` keeps the worktree when the watch is taken over,
  for any reason, as it does after lost access. `worktreeWord` says `kept:
  the session was with you`. The stop clears nothing else: a stopped
  watch has no hand-back.
- **Other routes.** `approve`, `reject` and `retry` find no proposal that
  waits, because step 4 declined them all. `watch mode` still works: the
  mode applies from the next turn.

## Hand-back

### The route

`POST /api/v1/watches/{id}/handback` with `{"confirm": bool, "force":
bool}`.

`prwatch.Service.Handback` does these steps under the lock:

1. Refuse a watch that is not taken over.
2. Unless `force`, refuse while `taken_over_pid` is alive (R18). The
   answer is 409 with `the session is still open in your terminal (pid
   N)`. The check is `kill(pid, 0)` on Unix and `OpenProcess` on
   Windows, behind one function in `internal/session`.
3. Read the worktree (R17):
   - `git rev-list <remote head>..<work branch>` for commits that the
     pull request branch does not have, with their subjects.
   - `git status --porcelain` for changes that are not committed.

   When either list has items and `confirm` is false, answer 409 with
   both lists and change nothing.
4. Store the remote head as `handback_start`, then clear
   `taken_over_at` and `taken_over_pid`, and record the `handed_back`
   row with the counts of both lists. When a write fails, the session
   stays with the author, so the hand-back can be done again.
5. Send `prompts/handback.md` through `deliver`, which starts the
   session with `--resume` through `ensureSession`. The prompt says that
   the author worked in the worktree, names where the work branch
   stands, lists the commits of step 3, and tells the agent to read them
   before it goes on. The author can have used `--shell`, so the
   transcript alone does not tell the agent what changed. The text is
   stored in the payload of the `handed_back` row, and the row counts as
   told only when the message goes out. When the session does not
   start, each poll sends the text of the last untold row again, before
   the rows that waited. A hand-back that is done answers with success,
   although the watch cannot be read after it.
6. Kick a poll. `tell` sends the rows that waited as one message (R19).

### Commits of the author

The commits that step 3 lists sit on the work branch below the next
turn. The next turn records where the work branch stands when it opens,
so a turn that adds nothing would leave them there with no push. The
first proposal after a hand-back starts from the head of the pull
request branch instead, so its push carries the commits the author
confirmed. `Handback` stores that start on the watch, and the turn
clears it only after its proposal opens, so a turn that fails to open
keeps it for the next try. A test proves both: a turn that adds nothing pushes the
author's commits, and a turn that adds one commit pushes all of them in
one push.

## Surfaces

### CLI

- `watch takeover <watch> [--shell]`.
- `watch handback <watch> [--yes] [--force]`. On a 409 with lists, it
  prints them and asks `Hand back with this work? [y/N]`, then calls the
  route again with `confirm`. `--yes` sends `confirm` at once.
- `watch status` prints `session: with you since <time>`, and `watch
  list` shows `with you` in the session column.

### App

- `watch-detail.tsx` gets a "Continue in terminal" action when the watch
  has a session and is not taken over. It opens `takeover-dialog.tsx`: a
  read-only field with `babysitter watch takeover <owner/name#number>`
  and a copy button (R20). The dialog uses the shadcn `Dialog` already in
  `components/ui`; nothing there changes.
- `status-badges.tsx` shows `With you` for a watch with `takenOverAt`.
- `agent-terminal.tsx` shows the log of the last session, read only, and
  replaces its input with a Hand back button (R21). A 409 with lists
  opens `handback-dialog.tsx`, which shows the commits and the files and
  confirms. A 409 for a live pid shows the reason and offers nothing
  else.
- The activity list shows the `taken_over` and `handed_back` rows.

### Skill

`plugins/babysitter/skills/babysit-pr/SKILL.md` gets a section: when the
author wants to continue the session in their terminal, give them the
takeover command; when they say they are done, run `babysitter watch
handback`. The skill never runs `takeover` (R23). The deny rules of a
watch session block `takeover` and `handback`, as they block `approve`:
only the author moves the session.

### API

Both routes go in `internal/httpd/router.go`, with DTOs in `dto.go`.
`npm run api` writes `openapi.yaml` and `frontend/src/api/schema.ts`.

## Order of work

Each step starts with a test that fails.

1. Store: the migration, `SetWatchTakeover`, the two activity kinds,
   and `takenOverAt` in the DTO.
2. `AuthorCommand` for Claude and Copilot, after the Copilot flag check.
3. `Service.Takeover` and its route: stories 1, 2 and 3 of `prd.md`.
4. The rules while taken over: stories 4, 5 and 6, and the refusal of
   `send`.
5. `Service.Handback`, the pid check, the worktree read, `handback.md`,
   and the start of the first proposal after it: stories 7, 8 and 9.
6. The CLI commands, the output of `status` and `list`, and `README.md`.
7. The app: story 10.
8. The skill, and the deny rules for `takeover` and `handback`.
9. The live run on the playground of stories 1, 2, 7 and 9.
