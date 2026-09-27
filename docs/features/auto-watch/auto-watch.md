# Auto watch: design

`prd.md` holds the problem and the requirements. This page says how the
code meets them and records the decisions that the PRD left open.

## Decisions

- The Dependabot defaults are the merge scope `patch` and the approval
  `never` (open question 1).
- A pull request starts on its own only once. A watch that stopped with
  an error does not start again; the author starts it by hand (open
  question 2).
- `watch merge --approve` approves and merges. There is no separate
  `watch approve` command (open question 3).
- The notification kind of R37 is `auto`, one new mutable kind. It also
  carries the approval ask of R33. Merge results stay in `merge` (R39).
- **Approve and merge** is a button of the notification row in the app.
  The banner of the operating system only opens the app.
- A merge stops the watch (`StopMerged`), as before. Scene 2 of the
  mockup shows a merged watch that still runs; the code does not do
  that.
- The queue is a query over the stored pull requests, not rows of its
  own. R24 and R25 hold with no upkeep: a pull request that closes,
  merges or gets a watch by hand leaves the query, and a toggle that
  goes off empties it.

## Store

One migration adds:

- `repo_config`, one row for each repository, removed with it (R1). A
  toggle is a nullable time, `own_since` and `dependabot_since`; NULL
  is off (R10). The overrides are nullable or empty, and empty takes the
  setting of the daemon (R6).
- `auto_start_claims`, one row for each pull request that auto start
  took. The claim goes in before the worktree, so the daemon that loses
  a race (R19) writes a debug line and touches no worktree.
- `pull_requests.assignees`, `fork` and `update_type`, filled from the
  list response of the watcher with no new call.
- `watches.auto_reason`, `merge_when_ready` and `update_type`.
- The activity kinds `auto_started`, `approved` and `approval_asked`, the
  notification kind `auto`, and `notifications.action`.

## Auto start

`autostart.Starter.Run` runs after each pass of the repository watcher,
in the daemon only (R18). For each repository with a checkout and a
toggle on, it takes the open pull requests that:

- are of the author (author or assignee is the login of the token, R11)
  or of Dependabot (R12);
- GitHub created at or after the toggle time (R13);
- are not from a fork (R14; one log line for each pull request and run);
- are not drafts, unless drafts are included (R15);
- have no claim and never had a watch (R16).

The pull requests of the author start at once. The Dependabot ones start
oldest first until the active Dependabot watches of the repository reach
the limit (R21 to R23); the rest is the queue.

A start is `prwatch.Service.Start` with the checkout, the overrides, the
reason and, for Dependabot, the update type and merge when ready when
the update is in scope (R17, R30). A start that fails for another reason
than a lost race releases its claim, so the next pass tries again.

## Merge when ready and the Dependabot policy

Both run in the poll of a watch, under the lock of the watch.

- Before the readiness, the policy applies to a Dependabot watch that
  auto start began and whose update is within the scope (R35). `green`
  approves once for each head (R34, R36). `ask` records `approval_asked`
  when only a review is missing (R33). `never` does nothing (R32).
- After the readiness, a watch with merge when ready that is ready
  merges through the same `mergeNow` path as a merge of the author
  (R28). A failure records `merge_failed` with the ref
  `auto-merge@<head>`, so the daemon tries once for each head (R29).

`watch merge --approve` checks the author and the scope, approves, and
merges without the wait for the clock of readiness, because the
approval it just gave changed the pull request.

## Surfaces

- CLI: `repo config`, `repo queue`, the `AUTO START` column of
  `repo list`, `watch start --merge-when-ready`,
  `watch merge-rules --merge-when-ready`, `watch merge --approve`, and
  an `Auto:` line in `watch status`.
- App: the **Repository settings** panel of a repository, the `queued`
  badge with the place in the queue, the `auto`, update type and
  `merge when ready` badges of a watch, the merge when ready switch in
  the start dialog and the **Watch settings** panel, and the
  **Approve and merge** button of a notification.
- Skill: the section **Repository configuration** of `babysit-pr`, the
  merge when ready flags, `watch merge --approve`, and the evals
  `08-repo-config` and `09-approve-and-merge`.
- API: `GET` and `PATCH /repos/{id}/config`, `GET /repos/{id}/queue`,
  `mergeWhenReady` on the start and the `PATCH` of a watch, `approve` on
  the merge.

## Evidence

[The live run](<../../evidences/auto-watch. Live run of the auto watch feature.md>)
shows stories 1, 2, 3, 6, 7, 8 and 9 on the playground, with the defects
it found and fixed.
