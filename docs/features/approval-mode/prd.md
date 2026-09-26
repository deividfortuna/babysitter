# Approval mode: product requirements

`approval-mode.md` holds the design. [`design-mocks.html`](design-mocks.html)
holds the mocks of the approval screen and the settings. This document
holds the problem, the requirements and the tests that say the feature is
done.

## Problem

A watch acts in the name of the author. It pushes to the pull request
branch and it posts replies under the author's account, and nobody sees
either one before it lands. The author carries the result: a reviewer
reads a reply the author never wrote, and a teammate pulls a commit the
author never read.

So an author who does not fully trust the agent does not start a watch
at all. The feature they want is not a better agent. It is a gate they
control.

## Who this is for

- **The author who wants a gate.** They want the work done and read
  before it becomes public. They are the default.
- **The author who trusts the watch.** They run many pull requests and
  they do not want a screen between the agent and the branch. They keep
  what they have today.
- **Both, on one machine.** Trust belongs to the pull request, not to
  the person. The same author gates a change to a shared library and
  lets a dependency bump go.

## Goals

1. Nothing becomes public without the mode that the author chose.
2. The author reads the code and the words before they are released.
3. What goes out is what the author read.
4. The author changes a reply that is nearly right, instead of throwing
   the turn away.
5. The choice follows the pull request, and it changes while the watch
   runs.
6. An install that upgrades keeps the behavior it has.

## Non-goals

- A review tool. The screen shows what is about to be released. It is
  not a place to write code.
- A sandbox. The hardening in `approval-mode.md` does not hold against a
  session that works at getting around it.
- A gate on the merge. `prwatch.Service.Merge` is already an act of the
  author. The daemon still owes that author a readiness answer that
  counts the unreleased work, which is R34.
- A gate on what the agent reads, or on `gh run rerun`. Nothing about
  those is public, and the design gives up `GH_CONFIG_DIR` to keep them.
- A second reviewer. The gate belongs to the author of the watch.
- A gate on a self watch. Its agent is the session of the author, in the
  checkout of the author, so there is nothing to gate. See R33.
- A release for a Dependabot watch. The bot owns that branch and a push
  from anybody else takes the pull request from it. See R36.

## Requirements

### Release

- **R1** The agent does not push and does not post, in either mode.
- **R2** In `auto` the daemon pushes and posts when the turn ends.
- **R3** In `manual` the daemon releases nothing until the author
  approves.
- **R4** The daemon records a proposal in both modes, with one shape. It
  names the head SHA of the pull request branch and the SHA of the work
  branch, which is what R5 pins the lease to and what R13 reads for
  staleness.
- **R5** The daemon decides the push without asking the agent. A work
  branch that contains the remote head goes out plain. Anything else
  goes out with the lease pinned to the head SHA the daemon read and
  checked it against, `--force-with-lease=refs/heads/<head ref>:<sha>`. A bare
  `--force-with-lease` is never used: the daemon fetches before it
  compares, and the fetch makes a bare lease pass whatever moved. The
  push uses the credential helper of the author, the one the session
  uses today, and a credential that fails is a failed push under R23.
- **R36** A Dependabot watch has no push. `agent.IsDependabot` on the
  author decides it, since R1 takes away the empty `PushRef` that says
  so today. Its proposal carries replies only, the daemon never pushes
  and never rebases that branch, and `behind` and `conflict` stay
  routine rows that reach the agent, which comments `@dependabot
  rebase`.
- **R37** `watch reply` records the reply and answers the same way in
  both modes, with no comment id and no URL. The comment the daemon
  posts is marked seen at the release, where it is posted. The comment
  the reply answers was marked seen by the poll that reported it, so a
  drop forgets that mark, and the next poll delivers the comment again.
- **R38** Before a message goes into an idle session, the daemon brings
  the work branch up to the pull request branch when that is a
  fast-forward. The turn records where the work branch met the head.
  When the head moved during a turn that only added commits on top of
  it, the release rebases those commits onto the new head. A rewrite
  that would drop a commit the work branch never had does not go out:
  it fails under R23. A branch the daemon never pushes follows the head
  whatever it holds.

### Choice

- **R6** One global setting, and a copy on each watch that the author
  changes while the watch runs.
- **R7** A fresh database defaults to `manual`.
- **R8** An install that upgrades keeps `auto`. A migration script
  cannot tell a fresh database from an upgraded one, because migration
  16 inserts the settings row for both. `migrate` reads
  `PRAGMA user_version` once before the chain and seeds the defaults of
  a fresh install only when that version was 0.
- **R9** A flip to `auto` with a pending proposal takes the shape of
  R32: it names what is about to be released and asks to confirm.
- **R30** `auto approve rebase` is a second setting of the same shape,
  off in a fresh database, with no meaning in `auto`.
- **R33** A self watch has no gate. Both settings have no meaning on it,
  `watch start --provider self` records `auto`, and `watch proposals`,
  `watch approve` and `watch reject` refuse a self watch with the
  reason.

### The proposal

- **R10** One proposal covers one turn. A turn starts when the agent
  starts to work on a message, whoever sent it, and ends when the agent
  goes idle or its session ends. A question to the author in the middle
  of a turn keeps the proposal open.
- **R11** A message that asks for nothing leaves no proposal: a turn
  with no commit and no reply leaves nothing behind.
- **R12** A watch has one pending proposal at a time. While one is
  pending the daemon sends no routine message, and `watch send` is
  refused with the reason.
- **R13** A proposal goes stale when the pull request branch moves away
  from the head SHA of R4, and the release refuses when the work branch
  is not the SHA the proposal names.
- **R14** While a proposal is pending the untold rows stay untold, and
  the message after the decision carries all of them at once. Nothing is
  held in the stored state of the watch: a frozen `HeadSHA` makes
  `staleCheck` drop every check of the real head without a word.
- **R15** Stop declines what is pending.
- **R31** The daemon rebases a stale proposal itself, and it is the only
  actor that can, because `behind` and `conflict` are routine rows. A
  clean rebase obeys `auto approve rebase`. A conflicted rebase is
  aborted, handed to the agent, and the proposal it opens always asks.
- **R34** A pending proposal is a ready blocker. `assess` stores it with
  the other blockers of the watch, so `Readiness` does not call the pull
  request ready while work waits on the author, and no `merge_ready` row
  and no merge offer reaches them before the work they are about to
  approve. A stored blocker is one poll old, which is the lag every
  other stored blocker has.

### The decision

- **R16** The author reads the commits, the list of changed files, the
  diff and the reply of each comment. The diff is plain unified text.
- **R17** The author edits a reply before it goes out.
- **R18** The author drops a reply, after a confirmation.
- **R19** The author rejects the push, after a warning and a
  confirmation.
- **R20** The author rejects the proposal, with an optional reason and a
  `discard the commits` box that is off by default. The box resets the
  work branch whatever `KeepWorktree` says.
- **R32** The author approves and stops asking, which releases the
  proposal and sets that watch to `auto`.

### Back to the agent

- **R21** A reply that was edited or dropped comes back as its own
  message that asks for nothing.
- **R22** A comment whose reply was dropped is delivered again.
- **R23** A push that failed shows in the activity with a retry, as an
  `agent_failed` row. That kind exists, so a failed push needs no
  rebuild of `watch_activity`, and it notifies as `watch` already.
- **R24** The agent hears about a failed push only when the daemon
  cannot rebase the work: a rebase that conflicts, and a rewrite that
  lacks commits of the head when the author retries it.
- **R25** A push that landed says nothing to the agent.

### Reach

- **R26** The CLI carries `watch proposals`, `watch approve` and
  `watch reject`.
- **R27** The app carries the setting and the approval screen, as
  [`design-mocks.html`](design-mocks.html) shows them.
- **R28** The skill reaches the feature through the same CLI commands,
  for a watch the app started. The watches the skill starts are self
  watches, and R33 leaves them as they are.
- **R29** Two notifications, and no new notification kind: an approval
  is needed is `agent`, and a push failed is `watch`. Nothing maps to
  `agent` in `notificationKinds` today, so a proposal that opens is a
  new activity kind with an entry in that map, and it costs the rebuild
  of `watch_activity` that each new kind costs. Both obey the mute rules
  that exist, and R34 is what carries a waiting proposal to an author
  who muted them.

### Hardening

- **R35** `gh api` posts with `-f` and `-F` and needs no `--method` or
  `-X`, so both providers deny those two patterns as well.

## Stories and what proves them

**An author gates a pull request.** They start a watch with the default.
A reviewer leaves a comment. The agent fixes it and the turn ends.
Nothing is on GitHub. The app says an approval is needed, and the watch
says it waits on the author instead of saying it can merge.
*Proves R1, R3, R10, R29, R34.*

**An author releases the work.** They read the commit, they read the
reply, they approve. The commit is on the branch and the reply is in the
thread, both under their account.
*Proves R2, R16.*

**An author fixes the words.** The change is right and the reply is
blunt. They edit the reply, they approve, and the thread carries the text
they wrote. The agent hears that the reply changed and it starts no
turn.
*Proves R17, R21, R11.*

**An author refuses a claim.** A comment is wrong and the agent says so.
The author drops the reply. Nothing is posted, the comment was never
marked seen, and it comes back to the agent on a later poll.
*Proves R18, R22, R37.*

**A Dependabot pull request.** A check fails on a version bump. The
agent commits a fix and the turn ends. No push proposal opens, the
daemon pushes nothing and rebases nothing, and the branch stays with the
bot. The branch falls behind later, the agent hears about it, and it
comments `@dependabot rebase`.
*Proves R36.*

**An author keeps a slow watch.** They take an hour to read. The branch
moved while they read. The daemon rebases the work, the rebase is clean,
and the proposal comes back marked rebased with the work branch SHA it
now names. They approve it and the push lands with the lease pinned to
the head SHA the proposal was rebased onto.
*Proves R13, R31, R5.*

**Somebody else pushes first.** The author approves, and between the
read of the branch and the push a teammate moves the pull request
branch. The pinned lease refuses the push. The activity carries the
failure with a retry, and nothing of the teammate is lost.
*Proves R5, R23.*

**A rebase that conflicts.** The same watch, and this time the rebase
does not apply. The daemon aborts it and hands the work to the agent.
The agent resolves the conflict and opens a new proposal, which asks
even though `auto approve rebase` is on, because nobody has read the
resolution. The activity carries the failed push and a retry, and a
notification reaches the author.
*Proves R31, R23, R24, R29.*

**An author stops reading this one.** They have approved four turns of
the same pull request and they trust it. They press approve and stop
asking. That proposal is released and the watch runs in `auto` from
there. Their other watches still ask.
*Proves R32, R6.*

**An author stops waiting.** They trust the watch and flip it to `auto`.
The app names the pending proposal, they confirm, and it is released.
The next turn needs no screen.
*Proves R6, R9, R2.*

**An author who does not want a gate.** They upgrade an install that
runs today. Every watch behaves as it did. A colleague installs the same
build on a clean machine and gets a watch that asks.
*Proves R8, R7.*

**Two comments arrive during a decision.** The author takes an hour.
Both comments reach the agent in one message after the decision, neither
is lost, and a check that failed on the head that arrived in that hour
reaches the agent too.
*Proves R14, R12.*

**A check breaks while the author reads.** The pull request branch moved
and its checks failed. The agent is told nothing until the decision, so
it opens no second proposal, and the failed check is in the message that
follows the decision.
*Proves R12, R14.*

**The session drives the watch.** The author runs `babysit-pr` in their
own checkout on a fresh install. The watch starts, the session pushes
and replies as it does today, and no proposal waits for anybody.
*Proves R33, R28.*

## Done

The feature ships when every requirement above holds, `npm run check`
passes, and each story has a test that fails without the change.

Cut 1 is the push and the reply moving to the daemon, with every watch
behaving as `auto`: R1, R2, R4, R5, R10, R11, R23, R24, R25, R33, R35,
R36, R37 and R38, plus the prompt. R23 is in this cut because this cut
is the one that can break a push, and a failure nobody records is the
worst shape of the risk below. Its retry is one command,
`watch retry`. The clean rebase of R31 is in this cut too, as `auto`
approves it: a turn that only added commits is rebased onto a branch
that moved, so a push of a teammate does not fail every turn after it.
The blocker of R34 is in this cut for an open or failed proposal, so a
merge never leaves work that did not go out behind. Nothing else an
author sees changes.

Cut 2 is `manual` from a terminal: R3, R6 to R9, R12 to R22, R26, R28 to
R32 and R34, with R32 as a flag on `watch approve`.

Cut 3 is R27, and it puts R32 on the screen.

## Risks

- **The push leaves the agent for `auto` too.** Every watch that runs
  today goes through the new path, so a defect reaches the authors who
  did not ask for this feature. Cut 1 carries that risk alone, with no
  gate and no screen behind it to confuse a report.
- **A pull request waits on a person.** A proposal never expires. A
  watch that is forgotten does nothing, and the author may have muted
  the notification, so the ready blocker of R34 is what has to say it in
  the list and in `watch status`.
- **The default changes what a new install does.** An author who expects
  the watch of the README finds a watch that waits. `README.md` has to
  say so first.
- **The fresh seed is a new step in `migrate`.** It runs once, on a
  database at version 0, and it is the only thing that separates R7 from
  R8. A test starts from an empty file and from a database seeded at the
  version before the change, and reads the setting from both.
- **The daemon pushes without a terminal.** R5 keeps the credential
  helper of the author, and the session that used it had a terminal
  while the daemon has none. A helper that wants to prompt fails, and a
  daemon the app spawned does not carry the environment of one started
  from a terminal. The first push of cut 1 on a machine with a
  prompting helper is what finds this.
- **The ready blocker is one poll old.** R34 stores it, so a proposal
  that opens right after a poll leaves the watch looking ready until the
  next one. A merge offer inside that window is the case to watch, and
  the interval is the size of it.
- **The hardening reads as a boundary.** It is not one, and
  `approval-mode.md` says that in the place where somebody would assume
  otherwise.
