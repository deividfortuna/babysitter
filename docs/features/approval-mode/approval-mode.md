# Approval mode

A watch acts on a pull request in the name of its author. Approval mode
says who releases that act: the daemon on its own, or the author.

- `auto`: the daemon pushes and posts as soon as a turn ends.
- `manual`: the daemon records a proposal and waits for the author.

## The agent never pushes and never posts

In both modes the agent commits on the work branch and calls
`babysitter watch reply`. The daemon performs both public actions. The
`pre-push` hook of `internal/agent/tools.go` refuses every push of the
session, and now it refuses them all, so `PushRef`, `BABYSITTER_PUSH_REF`
and the branch arm of the hook go away with nothing to allow. The
refusal names what to do instead, because the agent reads it and acts on
it: the daemon pushes this branch, commit and let the turn end.

This gives four things at once.

- The mode flips on a running watch with no session restart, because
  nothing about the mode reaches the session.
- A proposal in `auto` carries the same record as one in `manual`,
  instead of a record the daemon reconstructs from a changed `HeadSHA`.
- One prompt serves both modes. The mode is invisible to the agent.
- The push and the reply take the same shape: the agent records an
  intent, the daemon performs it.

The mode is invisible, the push is not. Six places tell the agent to
push today, and each one changes:

- `open.md:12`, the first push instruction the agent ever reads:
  `Push with git push origin HEAD:<branch>`.
- the first line of `system.md`: `you commit, push and reply yourself;
  nobody reviews your work before it lands`.
- rule 4 of `system.md`, which opens with `Before you push, verify the
  change`.
- rule 6, which is the push rule.
- rule 7, which tells the agent to reply after the fix is pushed.
- the two lines of `nudge.md`, 60 and 66, that spell out
  `--force-with-lease` for a branch that fell behind or conflicts.

The package comment at the head of `claude/tools.go` and
`copilot/tools.go` says the agent pushes the pull request branch. It
changes with the rules below it.

### The daemon chooses the push

The daemon fetches the pull request branch and compares it with the work
branch. A work branch that contains the remote head is a fast-forward,
and it goes out plain. Anything else is a rewrite, and it goes out with
the lease pinned to the head the daemon read and checked it against:

```sh
git push --force-with-lease=refs/heads/<head ref>:<head sha it read> origin HEAD:<head ref>
```

A bare `--force-with-lease` is never used. It reads the remote-tracking
ref of the worktree, and the daemon has to fetch before it can compare
or rebase, so the fetch satisfies the lease and the push behaves as
`--force`. That would clobber the concurrent push this feature exists to
catch. The pinned lease refuses the push when the branch moved after
the daemon read it.

The lease does not catch a commit that landed before the daemon read
the head: the head has it, and a rewrite pinned to that head passes.
Two things close that gap. Before the daemon types a message into an
idle session, it fetches the pull request branch and fast-forwards the
work branch to it, so the work of the turn starts on top of that
commit. And the turn records where the work branch met the head, the
merge base; when `git cherry <work> <head> <merge base>` lists a commit
with no equivalent in the work, the work does not go out as it is. A
turn that only added commits on top of the head it started from is
rebased onto the new head, and a conflict goes to the agent. A rewrite
fails as any push fails, and its retry hands it to the agent, which
brings those commits into it.

The push runs with `--no-verify`. The session never ran the hooks of
the repository, because the daemon pointed `core.hooksPath` at its own
hook, and a hook that needs the dependencies of the project fails in a
worktree that has none installed.

### The credential of the push

The push runs with the credential helper of the author, the same one the
session uses today. Nothing is set up for it: the daemon shells out to
plain `git`, and `git` reads the helper from the config of the author.
The daemon posts the reply with the token of `ghclient.Token` and pushes
with the helper, so the two public acts can come from two identities on
a machine configured that way.

That is the cheap choice and it has one cost. The session has a
terminal, the daemon has none, so a helper that wants to prompt fails
instead of asking, and the environment of a daemon the app spawned is
not the environment of one started from a terminal. A push that fails
for the credential is a failed push like any other: it goes in the
activity with a retry, and the author sees it there.

## A Dependabot watch has no push

Dependabot owns the branch of the pull requests it opens. A push from
anybody else makes the bot stop updating the pull request or replace it,
which is why `pushRef` in `internal/prwatch/live.go` already answers
empty for a Dependabot author, and why `open.md` tells the agent not to
push that branch and not to rebase it.

The empty `PushRef` was that signal, and the section above takes it away
by refusing every push for everybody. So the signal moves onto the
watch, through `agent.IsDependabot(w.Author)`, the call `pullRequestOf`
already makes.

- The proposal of a Dependabot watch carries replies only. There is no
  push part to approve or to reject.
- The daemon never pushes and never rebases that branch, so the stale
  and rebase path below does not run on it.
- `behind` and `conflict` stay routine rows and still reach the agent,
  which comments `@dependabot rebase` as it does today.

A commit the agent makes on such a watch stays on the work branch and
nothing releases it. That is what happens today, because the hook
refuses the push and no other actor takes it, and this feature does not
change it.

## A self watch has no gate

A watch of provider `self` has no session in the daemon, no worktree and
no work branch. Its agent is the coding agent session of the author, in
the author's own checkout, and the author reads every change while it is
made. There is nothing to gate it with: no session the daemon launched,
no hooks directory, no branch the daemon owns.

So a self watch runs in `auto` whatever the settings say.
`watch start --provider self` records `auto` on the watch, and
`watch proposals`, `watch approve` and `watch reject` answer a self
watch with that reason. The `babysit-pr` skill needs no change. The
skill reaches the feature only for a watch the app started, which is
what `references/app-watches.md` already covers.

## Settings

One global setting and a per-watch override. A watch copies the global
value when it starts, as it already does for `ApprovalsRequired` and
`MergeMethod`, and the author changes the copy while the watch runs.

A fresh database defaults to `manual`, and an install that upgrades
keeps `auto`. A migration script cannot tell the two apart. `migrateTo`
applies the chain from `PRAGMA user_version`, and migration 16 inserts
the settings row, so a fresh database has that row by the time any later
migration runs. The schema version before the chain is the only thing
that separates them: `migrate` reads it once, applies the chain, then
seeds the defaults of a fresh install when that version was 0. The new
column carries `auto` as its default, so the upgrade needs nothing more
than the `ALTER TABLE`.

`auto approve rebase` is a second setting of the same shape, global with
a copy on the watch. It means one thing: work that the author approved
does not ask again because the branch moved under it. It has no meaning
in `auto` or on a self watch, and it is off in a fresh database.

## A proposal

One proposal covers one turn. A turn starts when the agent starts to
work on a message, whoever sent it, and it ends when the agent goes
idle or its session ends, so a question to the author in the middle of
a turn keeps the proposal open. The hook of the agent says both, and
the daemon counts the messages it typed, so an idle signal read before
the last message ends nothing. A message that asks for nothing leaves
nothing: a turn with no commit and no reply deletes its proposal, and
its number is free again. A daemon that stops in the middle of a turn
closes that turn when it starts again, because the session that made
it is gone.

A proposal holds:

- the SHA of the work branch it was built on,
- the reply text of each comment the agent answered,
- the head SHA of the pull request branch it was built on.

It goes stale when the pull request branch moves away from that head
SHA. A release reads the work branch again and refuses when it is not
the SHA the proposal names, so what goes out is what the author read.

A watch has one pending proposal at a time and many in sequence. A
proposal is approved, the push lands, a check breaks, and the next turn
opens the next proposal.

### What `watch reply` records

`watch reply` records the reply and answers the same way in both modes:
the reply is on the watch, with no comment id and no URL, because
neither exists yet. Today it prints `Posted on the pull request of watch
N: <url>` (`internal/cli/watch.go`), and the daemon posts at the end of
the turn now, so that line is true for nobody. One wording keeps the
mode invisible to the agent, which is what the first section buys.

`Reply` marked the comment it posted as seen, with
`MarkReviewItemsSeen`, so the next poll never brings it back as
feedback. That mark moves to the release, where the comment is posted.
The comment the reply answers is a different item: the poll that
reported it marked it seen already. So a drop forgets that mark, one
item of `pr_watch_seen`, and the next poll reports the comment again.
Its activity row exists already, and the unique key keeps a second row
out, so the poll takes the told mark off that row, and the next message
carries it. A comment on the conversation has no thread, and the agent
answers it with `--to` and its id all the same: the reply records the
kind of the comment it answers, the daemon posts it on the
conversation, and a drop forgets that comment the same way. A reply
with no `--to` answers no comment the daemon knows, so a drop of it
brings nothing back, and the message after the decision says so. The window between the reply and the decision is
closed already: a watch with a pending proposal sends no routine
message.

The comment a reply answers is checked when the agent records the
reply, with a read of the review comment and, when that is not found,
of the conversation comment. A wrong id reaches the agent while it can
still fix it, the way it did when the reply was posted at once.

### While one is pending

The daemon keeps polling and keeps writing activity. No activity row is
held back. The rows the agent was not told about already
carry `nudged_at IS NULL`, and `tell` marks a row only after the message
goes in. What changes is that a watch with a pending proposal takes no
routine message, the same way a watch whose agent waits on the author
takes none today. The rows stay untold, and the message after the
decision carries all of them at once.

That is also what keeps one proposal pending at a time, with no second
rule. The agent starts no turn while a proposal waits, so no second
proposal can open. A message from the author with `watch send` is
refused while one is pending, and the reason names it: an author who
wants a different answer rejects the proposal and says why.

Holding the stored state of the watch still would be the wrong way to
reach the same place. `staleCheck` compares the SHA in a check row with
`HeadSHA` of the watch, and `actionable` marks every row of another head
nudged without a word. A frozen `HeadSHA` drops every check of the real
head in silence.

The watch does not look frozen. `assess` puts a pending proposal in the
blockers it stores on the watch, so `watch status` and the app both say
the watch waits on the author, and `Readiness` stops calling the pull
request ready. Without that rule an idle agent and a green check write
`merge_ready`, and the author merges the pull request without the work
they are about to approve. The same sentence makes the agent not done
with the head, so the daemon asks no reviewer for a new review either.
A proposal whose release failed is a blocker too, in both modes: its
work is not on the branch.

The blocker is stored, not laid over the stored list the way the agent
state is in `agentBusyWord`, so it is one poll old. A proposal that
opens right after a poll leaves the watch green until the next one, and
a proposal approved right after a poll leaves the blocker standing for
the same time. Every stored blocker has that lag, and the notification
is what reaches the author inside it.

### What the author does

The screen shows the commits, the list of changed files, the diff and the
reply text of each comment. The diff is the plain unified text that
`git diff` prints. There is no side by side, no rich viewer and no
comments on it. [`design-mocks.html`](design-mocks.html) shows the
screen. The author can:

- edit a reply before it goes out,
- drop a reply, after a confirmation,
- reject the push, after a warning and a confirmation,
- reject the proposal, with an optional reason and a `discard the
  commits` box that is off by default,
- approve and stop asking, which releases the proposal and sets the
  watch to `auto`. It is the per-watch override, in the place where the
  author decides they have read enough.

`discard the commits` resets the work branch whatever `KeepWorktree`
says. A box that leaves the commits behind on a kept worktree would tell
the author one thing and do another.

### Stale and rebase

A proposal goes stale when the pull request branch moves. The daemon
rebases the work branch itself on the next poll and does not ask the
agent. It does so for a proposal that waits on the author, and for one
the author approved whose push failed because the branch moved between
the read and the push.

- The rebase is clean, the author approved the work, and `auto approve
  rebase` is on: the daemon pushes. Nothing asks.
- Any other clean rebase: the proposal is offered again, marked
  rebased, with the work branch SHA it now names, and the author
  approves the push. An approval of the old commits is not an approval
  of the new ones.
- The rebase conflicts: the daemon aborts it and hands the work to the
  agent. The agent resolves the conflict and opens a new proposal, which
  asks whatever the setting says. The resolution is code that nobody
  read.

The daemon is the only actor that can rebase a pending proposal, and the
rule above is what makes that true. `behind` and `conflict` are routine
rows, and a watch with a pending proposal sends no routine message.
Without that, `nudge.md` would tell the agent to rebase the same branch
and rewrite the commits the author is reading.

### What the agent hears

- A reply the author edited or dropped comes back as its own message.
  It says what the author did and asks for nothing, so it opens no
  proposal.
- A comment whose reply was dropped is delivered again.
- A push that failed shows in the activity as `agent_failed`, with the
  reason and a retry. It reaches the agent only when a rebase conflicts.
- A push that landed says nothing.

`agent_failed` is a row that exists, so a failed push needs no new
activity kind and no rebuild of `watch_activity`, and
`notificationKinds` sends it to the author as `watch` already. The cost
is that the row says the agent failed when the daemon did. The summary
and the payload say which.

### Stop

Stop declines what is pending. The commits go with the worktree. With
`KeepWorktree` on they stay on the work branch of the author's checkout
with nothing pointing at them. A new watch on the same pull request does
the work again.

### Flip to auto

A flip to `auto` with a pending proposal takes the shape of `approve and
stop asking`: the author reads what is about to be released, confirms,
and the proposal goes out. One decision, one confirmation, whether the
author reaches it from the proposal or from the setting.

## Enforcement

The boundary is that the daemon owns the push and the reply. The
`pre-push` hook refuses every push of the session, with no branch left
to allow, and the reply already goes through `babysitter watch reply`.

The rest is hardening, in the same class as `deniedTools` and with the
same limit: the patterns match the head of a command, and a shell has
many ways to write the same call.

The `gh api` rules need two patterns more in both providers. `gh api`
sends a POST as soon as the call carries `-f` or `-F`, so the rules that
deny `--method` and `-X` leave `gh api repos/o/n/pulls/N/comments -f
body=...` open. That call posts a comment today, in both modes.

Two things this design does not use.

`GH_CONFIG_DIR` at an empty directory takes the credential of `gh` away
from the session. It takes `gh run rerun --failed` with it, which rule
10 of the prompt names, and the job log reads that the `copilot` rules
leave open on purpose. Both are reads and reruns, and nothing here gates
those. It also hardens little: `ghclient.Token` reads `GITHUB_TOKEN`
first, the session inherits the environment, and `gh` reads the same
variable.

A blocking `PreToolUse` hook needs the endpoint to answer with a
decision and the hook command to speak the deny protocol of each
provider. It fails open when the daemon does not answer, because a
session must not freeze on a daemon that is down, so it stops nothing
that closes the connection. For Copilot CLI it is weaker again: the hook
file lives at `.github/hooks/babysitter.json` inside the worktree, and
the agent may write there. The cost is a protocol on both sides, and the
gain is a guardrail the agent can delete.

`gh auth:*` is in `deniedTools` already, and a session that wants the
token reads the config file of the author itself, or takes
`GITHUB_TOKEN` out of the environment it inherits. The session runs as
the author. None of the hardening holds against a session that works at
getting around it.

## Notifications

Two notifications, and no new notification kind. An approval is needed
is `agent`, which is the agent asking the author for something it cannot
do alone. A push that failed is `watch`, which is the life of a watch.
The schema, the openapi enum and the mute list of the app all stay as
they are.

Neither reaches the author on its own. `notificationKinds` in
`internal/prwatch/notify.go` maps an activity kind to a notification
kind, and nothing in it maps to `agent`: the only producer of that kind
is the `notify` command, through `internal/httpd`. So a proposal that
opens is a new activity kind with an entry in that map. It is the
timeline row the app needs anyway, and it costs one rebuild of
`watch_activity`, the rebuild migrations 13, 14 and 15 each paid to add
one kind. A failed push rides on `agent_failed`, which is in the map
already.

Both follow the mute rules that are already there, and a muted kind is
why the notification is not the only thing that carries a waiting
proposal. The ready blocker cannot be turned off.

## Surfaces

- `internal/store`: the two settings columns, the proposals table, the
  fresh seed of `migrate`, and the rebuild of `watch_activity` for the
  proposal activity kind.
- a new package for the git the release needs: fetch, the fast-forward
  check, the push, the rebase and its abort, the reset of `discard the
  commits`, and the log, the file list and the diff the author reads.
  `internal/worktree` keeps `Create` and `Remove` and nothing else:
  `worktree.Manager` is the fake seam of the `prwatch` tests, and a
  second interface beside it costs less than a wider one.
- `internal/prwatch`: the turn boundary, the routine message a pending
  proposal holds, the stored ready blocker, the Dependabot rule, and the
  push and the reply the daemon now owns.
- `internal/agent`: the prompt in six places, `PushRef` and
  `BABYSITTER_PUSH_REF` going away with the branch arm of the hook, and
  the two `gh api` patterns in `claude/tools.go` and `copilot/tools.go`.
- `internal/httpd`: the proposal routes. Run `npm run api` after.
- `internal/cli`: `watch proposals`, `watch approve`, `watch reject`,
  `watch mode` for the per-watch override, `watch retry`, and the new
  wording of `watch reply`.
- `frontend`: the setting and the approval screen, as
  [`design-mocks.html`](design-mocks.html) shows them.
- `README.md` and `docs/architecture.md`.

## Order of work

The push moves out of the agent for `auto` as well as `manual`, so every
watch that exists runs through the new path. That is the risk, not the
screen. The cuts are ordered so the risky part ships alone.

1. The push and the reply move to the daemon, and every watch behaves as
   `auto`. The proposals table, the turn boundary, the pinned lease and
   the fast-forward before a message, the Dependabot rule, the seen mark
   at the release, and the prompt. A push that fails records
   `agent_failed` with a retry, which notifies as `watch`: the cut that
   can break a push is the cut that reports it. The retry is one
   command, `watch retry`. A turn that only added commits is rebased
   onto a branch that moved, and work the daemon cannot rebase goes to
   the agent. An open or failed proposal blocks the merge. No setting
   and no gate.
2. `manual`: the two settings and their migration, the gate, the stored
   ready blocker, the stale and rebase path, the proposal activity kind
   with its notification, and `watch proposals`, `watch approve` and
   `watch reject`. Proven from a terminal.
3. The approval screen and the setting in the app.
