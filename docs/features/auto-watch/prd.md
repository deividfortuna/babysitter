# Auto watch: product requirements

This document holds the problem, the requirements and the stories that
show the feature is done. The design comes later in `auto-watch.md`.

`designs/` holds the app designs of the feature. Open
`designs/AutoWatch.dc.html` from a local server, as `designs/README.md`
tells.

## Problem

A watch starts only when the author asks for it. The author opens the
app or a terminal, finds the pull request, picks the checkout and starts
the watch. The author does this again for each pull request.

Two kinds of pull requests make this repetitive:

- **The pull requests of the author.** The author wants a watch on each
  one. The start is always the same.
- **The pull requests of Dependabot.** They arrive in groups, each one
  needs the same care, and most of them only need a green build and a
  merge.

The author wants each repository to say what babysitter does with its
pull requests, so that nobody starts these watches by hand.

## Who this is for

- **The author with many pull requests.** They open pull requests all
  day on a few repositories. They want a watch on each one without a
  step.
- **The maintainer of dependencies.** They want patch updates merged
  when the build is green, and they want to see the majors before they
  merge.
- **Both, on one machine.** The configuration belongs to the
  repository. A shared library gets manual approval. A small service
  gets auto approval and a merge on its own.

## Goals

1. A repository starts a watch on each new pull request of the author,
   with no step from the author.
2. A repository watches the pull requests of Dependabot and merges the
   ones within the scope that the author set.
3. The author controls the cost: the number of Dependabot sessions that
   run at the same time has a limit.
4. A stop from the author is final. Auto start never overrules it.
5. Nothing merges and nothing is approved in the name of the author
   unless the author turned that on for the repository.
6. The CLI, the app and the skill do the same things.

## Non-goals

- A clone that babysitter owns. The author gives the path of a checkout.
- A backfill. The pull requests that are open when a toggle goes on do
  not start.
- Renovate and other bots. Only Dependabot.
- A push to a Dependabot branch. R36 of the approval mode holds: the
  daemon never pushes to it, and the agent asks the bot to rebase.
- Different overrides for the two kinds of pull requests. One set of
  overrides serves the repository.
- A fix for issue 46. Two daemons on one database still both run auto
  start. R19 makes the lost race quiet.
- Auto start in `babysitter serve`. See R18.

## Requirements

### Repository configuration

- **R1** Each watched repository has a configuration. A new repository
  has an empty configuration, and an empty configuration starts
  nothing. When the repository is removed, its configuration and its
  queue are removed too.
- **R2** The configuration holds the path of a checkout. The author
  must give the path before they can turn on a toggle. The daemon
  refuses a path that is not a git checkout, or a checkout that has no
  remote for the repository.
- **R3** The toggle **Auto start my pull requests** starts a watch on
  each new pull request of the author.
- **R4** The toggle **Include drafts** lets R3 take a draft. It has no
  effect on Dependabot.
- **R5** The toggle **Auto watch Dependabot** starts a watch on each new
  pull request of Dependabot.
- **R6** The configuration holds overrides for the watches that it
  starts: provider, model, approval mode, merge method, approvals
  required and include existing. An empty override takes the setting
  of the daemon, as an absent field of `watch start` does.
- **R7** The **Dependabot merge scope** is the highest update that
  merges on its own: `patch`, `minor` or `major`. The default is
  `patch`.
- **R8** The **Dependabot approval** is `never`, `ask` or `green`. The
  default is `never`.
- **R9** The **Dependabot limit** is the number of Dependabot watches
  that run at the same time on the repository. The default is 1. The
  lowest value is 1.
- **R10** Each toggle records the time when it went on. A toggle that
  goes off and on again records a new time.

### Which pull requests start

- **R11** A pull request is of the author when the author wrote it or
  is an assignee. The author is the user of the token, the login that
  `GET /viewer` returns.
- **R12** A pull request of Dependabot is one whose author passes
  `agent.IsDependabot`.
- **R13** Auto start takes only a pull request that GitHub created at
  or after the time of R10. A pull request that opened while the daemon
  was down still starts.
- **R14** Auto start skips a pull request from a fork and writes a log
  line that names it. The agent cannot push to a fork.
- **R15** Without R4, auto start skips a draft. When the draft becomes
  ready for review, it starts on the next pass.
- **R16** Auto start takes a pull request only once. It skips a pull
  request that has, or had, a watch of any origin. So a watch that the
  author stops never starts again on its own, and neither does a watch
  that stopped with an error.
- **R17** An auto start is the same as a `watch start` with the
  overrides of R6. It makes a worktree from the checkout of R2.
- **R18** Auto start runs in the daemon, after each pass of the
  repository watcher. `babysitter serve` runs the watcher and starts no
  watch.
- **R19** When the unique index of active watches refuses a start, the
  daemon writes a debug line and goes on. The author sees no error.
- **R20** When a toggle goes off, auto start stops for that kind. The
  watches that run go on to their end.

### The Dependabot queue

- **R21** A pull request of Dependabot that R5 takes when the limit of
  R9 is full goes into the queue. The store keeps the queue.
- **R22** The limit counts every active watch on a Dependabot pull
  request of the repository, also one that the author started by hand.
- **R23** When a place becomes free, the oldest pull request of the
  queue starts on the next pass. The order is the time of creation on
  GitHub.
- **R24** A pull request leaves the queue when it closes, when it
  merges, or when the author starts a watch on it by hand.
- **R25** When the toggle of R5 goes off, the queue is emptied.
- **R26** The pull requests of the author never go into a queue.

### Merge when ready

- **R27** Each watch has the option **Merge when ready**. It is off by
  default. The author sets it at the start and changes it while the
  watch runs, as they do with the other merge rules.
- **R28** When the option is on and the watch records `merge_ready`,
  the daemon merges with the method of the watch. It uses the same path
  as a merge from the author. Everything that keeps a watch from ready
  also keeps it from this merge: a proposal that waits, a session with
  the author, a review that is missing.
- **R29** A merge that fails records `merge_failed` and notifies. The
  daemon tries once for each head SHA. A new head tries again.
- **R30** On a watch that R5 started, the option is on when the update
  is within the scope of R7, and off when it is not. A watch with the
  option off stops at ready to merge and waits for the author.
- **R31** The daemon reads the update type from the title and the body
  of the pull request. A grouped pull request takes its highest update.
  A type that the daemon cannot read counts as `major`. The watch keeps
  the type, so the app and the CLI show it.

### Dependabot approval

- **R32** With `never`, the daemon submits no review. When the base
  branch needs a review, the watch shows the blocker and waits.
- **R33** With `ask`, the daemon notifies when the build is green, the
  update is within the scope, and the only blocker is a missing review.
  The notification has an **Approve and merge** action. The CLI does
  the same with `babysitter watch merge <watch> --approve`.
- **R34** With `green`, the daemon submits an approving review in the
  name of the author when the build is green and the update is within
  the scope. The body of the review says that the Dependabot policy of
  babysitter approved it. The daemon approves once for each head SHA.
- **R35** The daemon never approves an update outside the scope, and
  never approves a pull request that is not of Dependabot.
- **R36** Each review that the daemon submits records an activity row.

### Notifications

- **R37** The new notification kind `watch_auto_started` tells the
  author that a watch started on its own, and why: my pull request,
  assigned to me, or Dependabot. It is on by default, and the author
  can mute it as any other kind.
- **R38** The watch records an `auto_started` activity row with the
  same reason.
- **R39** A merge of R28 notifies with the kind of the merge that
  exists.

### Reach

- **R40** `babysitter repo config <owner/name>` shows the configuration.
  Its flags change it, with the same names as the flags of
  `watch start` for the overrides. `repo list` shows the toggles that
  are on.
- **R41** `babysitter repo queue <owner/name>` lists the queue.
- **R42** `watch start --merge-when-ready` and
  `watch merge-rules --merge-when-ready` set the option of R27.
- **R43** The app has a configuration panel for each repository, with
  the folder picker of the preload for R2. The list of pull requests
  shows a **queued** badge, and a watch shows an **auto** badge when auto
  start began it.
- **R44** The `babysit-pr` skill reads and changes the configuration,
  reads the queue, and sets the option of R27 on a watch.
- **R45** `README.md` documents the commands, the configuration and the
  queue.

## Stories and what proves them

1. **The author opens a pull request.** Auto start is on. The author
   opens a pull request. On the next pass a watch starts with the
   overrides of the repository, the watch has an `auto_started` row, and
   the author gets a `watch_auto_started` notification.
   *Proves R3, R6, R11, R17, R37, R38.*
2. **A pull request from before the toggle.** A pull request of the
   author opened before auto start went on. It never starts.
   *Proves R10, R13.*
3. **The author stops the watch.** The author stops a watch that auto
   start began. The pull request stays open. The next passes start
   nothing. *Proves R16.*
4. **A draft.** Include drafts is off. The author opens a draft. Nothing
   starts. The author marks it ready. The next pass starts the watch.
   *Proves R4, R15.*
5. **A pull request from a fork.** A teammate assigns to the author a
   pull request from a fork. Nothing starts, and the log names it.
   *Proves R11, R14.*
6. **Monday morning.** The limit is 1. Dependabot opens four pull
   requests. One watch starts, and three pull requests go into the
   queue, oldest first. `repo queue` and the app show them.
   *Proves R5, R9, R21, R23, R41, R43.*
7. **A patch goes out.** The scope is `patch` and the approval is
   `green`. The watch of a patch update sees a green build. The daemon
   approves, the watch reaches ready, and the daemon merges. The next
   pull request of the queue starts.
   *Proves R23, R28, R30, R34, R36, R39.*
8. **A major waits.** The scope is `minor`. A grouped pull request holds
   a patch and a major. The watch keeps it green and stops at ready to
   merge. The daemon does not approve it.
   *Proves R30, R31, R35.*
9. **The author approves from the notification.** The approval is
   `ask`. A minor update is green and misses only a review. The
   notification shows Approve and merge. A click approves and merges.
   `watch merge --approve` does the same from the CLI.
   *Proves R33.*
10. **The toggle goes off.** Two Dependabot watches run and two pull
    requests wait. The author turns the toggle off. The two watches go
    on, and the queue is empty.
    *Proves R20, R25.*
11. **A merge fails.** Merge when ready is on, and the merge fails. The
    watch records `merge_failed` and notifies. The daemon does not try
    again until the head moves.
    *Proves R29.*
12. **Two daemons.** Two daemons run on one database and both see a new
    pull request. One watch starts. The other daemon writes a debug
    line only. *Proves R19.*
13. **`serve` runs.** The launchd service runs `serve` on the same
    database. It starts no watch. *Proves R18.*

## Done

The feature ships when every requirement holds, `npm run check` passes,
each story has a test that fails without the change, and stories 1, 6,
7 and 8 run live in the app on the playground.

Cut 1 is the configuration and auto start of the pull requests of the
author, in the daemon and the CLI: R1 to R4, R6, R10 to R20, R37, R38,
R40 and R45.

Cut 2 is merge when ready: R27 to R29, R39 and R42.

Cut 3 is Dependabot: R5, R7 to R9, R21 to R26, R30 to R36 and R41.

Cut 4 is the app: R43.

Cut 5 is the skill: R44.

The rule of `docs/rules` asks for the three surfaces together, so the
feature ships when all five cuts are in.

## Risks

- **The daemon approves in the name of the author.** With `green`, a
  review with the name of the author lands on GitHub with no click. The
  body of R34 and the row of R36 keep it visible, and the default is
  `never`.
- **The queue is slow.** With a limit of 1, each merge moves the base,
  so the next pull request is behind and waits for `@dependabot rebase`
  and a new build. Expect one merge for each build.
- **The update type is text.** Dependabot writes it in the title and
  the body, and the format can change. R31 counts an unknown type as
  `major`, which is the safe side of the error.
- **The assignees are not stored.** `pull_requests` has no column for
  them. The list of pull requests returns them, so this is a migration
  and no new call to GitHub.
- **Two daemons.** Issue 46 stays open. The unique index keeps one
  active watch for each pull request, but two daemons still do each
  pass twice.
- **Merge when ready changes a non-goal of approval mode.** That page
  says the merge is an act of the author. The option keeps it so: the
  author turns it on, for each watch or for each repository.

## Open questions

- The defaults of R7 and R8, `patch` and `never`, are the safe choice.
  Confirm them.
- R16 also blocks a pull request whose watch stopped with an error.
  Confirm that the author starts it again by hand.
- R33 adds `--approve` to `watch merge`. A separate `watch approve`
  command is the other choice.
