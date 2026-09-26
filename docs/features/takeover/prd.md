# Takeover: product requirements

`takeover.md` holds the design. This document holds the problem, the
requirements and the tests that show the feature is done.

`design/` holds the app designs of the feature

`evidence.md` holds the live run of the stories.

## Problem

A watch runs the agent in a pseudo terminal that the daemon owns. The
author can read that terminal and type into it, but the session keeps
the rules of the daemon: it cannot push, it cannot call the `gh`
commands that the tool rules deny, and every message the daemon types
goes into it.

Sometimes the agent cannot fix the problem. It tries the same fix again,
it waits on a permission that the rules refuse, or the fix needs a tool
that the session does not have. Today the author has two choices: stop
the watch and lose it, or leave the agent stuck. The author wants to
continue the same conversation in their own terminal, with full control,
and then give the watch back.

## Who this is for

- **The author with a stuck agent.** They want the conversation and the
  worktree that the agent used, not a new start.
- **The author who fixes by hand.** They want a shell in the worktree and
  the right push command.

## Goals

1. One short command moves the session from the daemon to the author's
   terminal.
2. The author's session is the same conversation, in the same worktree.
3. While the author has the session, the daemon does not type into it,
   push, post or remove the worktree.
4. The author gives the watch back with one command or one button, and
   the agent continues with what the author did.

## Non-goals

- Rules in the author's session. It belongs to the author. It has no
  tool rules, no system prompt of babysitter, no hooks and no `pre-push`.
- Opening a terminal from the app. The app shows the command with a copy
  button. A launcher for each terminal comes later.
- An automatic hand-back when the author quits the agent. Only the
  author gives the watch back.
- Takeover of a self watch. It has no session and no worktree.

## Requirements

### Takeover

- **R1** `babysitter watch takeover <watch>` takes the watch by id, pull
  request URL or `owner/name#number`, as the other `watch` commands do.
- **R2** The takeover ends the session of the daemon. The session end
  does not close the open turn and records no `session_exited` row.
- **R3** The takeover declines every proposal that waits: open,
  `pending` or `failed`. Nothing of the declined work goes out.
- **R4** The takeover takes the lock of the watch, so it waits for a
  release that is in progress.
- **R5** The takeover records a `taken_over` activity row.
- **R6** The CLI prints the worktree, the work branch and the push
  command `git push origin HEAD:<head ref>`, then runs the agent of the
  watch in the worktree on the same conversation.
- **R7** The command line of the author's session has none of the flags
  of the daemon: no permission mode, no tool rules, no system prompt, no
  hook settings. It keeps the provider, the model and the conversation.
- **R8** `takeover --shell` runs the shell of the author in the worktree
  instead of the agent.
- **R9** The takeover of a self watch, a stopped watch or a watch that is
  already taken over is refused with a reason.

### While the author has the session

- **R10** The daemon polls and records activity as usual.
- **R11** The daemon types nothing into a session. The actionable rows
  stay untold, as they do while a proposal waits.
- **R12** `watch send` is refused with a reason that names `handback`.
- **R13** A hook event for the watch changes nothing. This covers the
  Copilot hooks file that stays in the worktree.
- **R14** The watch has the ready blocker `the session is with you`. No
  `merge_ready` row and no re-review request go out, and a merge is
  refused.
- **R15** A stop of any reason keeps the worktree and its branch. The
  stop summary says why.

### Hand-back

- **R16** `babysitter watch handback <watch>` and the Hand back button of
  the app give the watch back through one route.
- **R17** Before the hand-back, the daemon reads the worktree. When the
  work branch has commits that the pull request branch does not have, or
  the worktree has changes that are not committed, the route refuses and
  returns the list. The CLI shows the list and asks for a confirmation;
  `--yes` skips the question. The app shows the list in a confirmation
  dialog.
- **R18** The hand-back is refused while the author's process is still
  alive, because two processes on one conversation corrupt it.
  `--force` skips this check.
- **R19** The hand-back records a `handed_back` activity row, clears the
  blocker of R14 and starts a poll. The next message continues the same
  conversation and carries the rows that were held.

### Reach

- **R20** The app shows a "Continue in terminal" action on a watch that
  has a session. It opens a dialog with the command and a copy button.
- **R21** The app shows a watch that is taken over with its own badge and
  a Hand back button in place of the terminal input.
- **R22** `watch status` and `watch list` show that the watch is taken
  over.
- **R23** The `babysit-pr` skill gives the author the takeover command
  and runs `watch handback` when the author asks. The skill never runs
  `takeover`, because the command needs a terminal of its own.
- **R24** `README.md` documents both commands.

## Stories and what proves them

1. **The author takes over a stuck agent.** The agent waits on a
   permission. The author runs `takeover`. The pty process is gone, the
   watch shows the blocker, a `taken_over` row exists, and the CLI runs
   the agent with `--resume <session>` in the worktree.
   *Proves R1, R2, R5, R6, R7, R14.*
2. **The takeover comes in the middle of a turn.** The agent is `active`
   with an open proposal and one commit. After the takeover the proposal
   is `declined`, nothing is pushed, and no reply is posted.
   *Proves R2, R3.*
3. **A proposal waits in manual.** A `pending` proposal exists. After the
   takeover it is `declined` and `approve` on it is refused.
   *Proves R3.*
4. **A review comes in while the author works.** A poll finds a new
   review comment. The row is recorded and stays untold. The session is
   not started again. *Proves R10, R11.*
5. **The author's Copilot session sends a hook.** The daemon receives
   `watch hook` for the watch. The watch state does not change and no
   turn opens. *Proves R13.*
6. **The pull request merges while the author works.** The poll sees the
   merge and stops the watch. The worktree and its branch stay on disk,
   and the summary says the session was with the author. *Proves R15.*
7. **The author hands back with unpushed work.** The work branch is two
   commits ahead of the head and one file is modified. `handback` without
   `--yes` refuses and lists both. With `--yes` it succeeds.
   *Proves R17.*
8. **The author hands back with the agent still open.** The process of
   the takeover is alive. The hand-back is refused and names its pid.
   *Proves R18.*
9. **The agent continues.** After the hand-back, the held review comment
   goes out in one message to a session started with `--resume`.
   *Proves R19.*
10. **The app flow.** The dialog shows `babysitter watch takeover
    <owner/name#number>` and copies it. After the takeover, the watch
    shows the badge and the Hand back button, and the button opens the
    confirmation of R17 when the route returns a list.
    *Proves R20, R21.*

## Done

The feature ships when every requirement holds, `npm run check` passes,
each story has a test that fails without the change, and stories 1, 2, 7
and 9 run live in the app on the playground.

Cut 1 is the daemon and the CLI: R1 to R19, R22 and R24.

Cut 2 is the app: R20 and R21.

Cut 3 is the skill: R23.

## Risks

- **A pid that the system gives again.** R18 reads a pid. A new process
  with the same pid makes the hand-back refuse until the author uses
  `--force`. That is the safe side of the error.
- **The Copilot resume flag.** The runner starts Copilot with
  `--session-id` for a new session and for a resume. Check that a
  session started by hand with that flag continues the conversation
  before cut 1 ships.
- **The author pushes a rewrite.** The author has no rules. A force push
  from the author is a new head for the daemon, and the next turn starts
  from it. The design does not try to stop this.
