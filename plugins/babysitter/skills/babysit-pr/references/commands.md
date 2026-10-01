# The other commands

Add `-o json` when you read the output yourself. The text form is for
the user.

## Read a watch

```bash
babysitter watch list                  # watched pull requests, provider, agent state, head, checks, last poll
babysitter watch status <watch>        # one watch, with what blocks the merge
babysitter watch view <watch>          # the pull request as the daemon last read it: reviewers, labels, size, checks, description
babysitter watch diff <watch>          # the diff of the pull request, as gh pr diff shows it, through the cache of the daemon
babysitter watch activity <watch>      # everything reported, oldest first; --since <id> for the rest
babysitter watch poll <watch>          # ask the daemon to look now, for example after a push
```

## Activity kinds

`comment`, `review_comment`, `review`, `check_failed`,
`check_recovered`, `checks_green`, `commit`, `behind`, `conflict`,
`merged`, `closed`, `heartbeat`, `watch_started`, `watch_stopped`,
`session_started`, `session_exited`, `nudged`, `replied`,
`agent_failed`, `merge_ready`, `merge_failed`, `review_requested`,
`proposal`, `taken_over`, `handed_back`, `auto_started`, `approved`,
`approval_asked`, `branch_updated`, `branch_update_failed`.

`taken_over` and `handed_back` say that the user moved the session of a
watch of the app to their terminal and gave it back. Read
`references/app-watches.md`.

Six of them are easy to read wrong:

- `nudged`: one message to the agent, with the text in its payload. On a
  self watch, this is a message that you took.
- `replied`: a reply posted through the daemon. On a watch of the app,
  the daemon posts the replies of a turn when that turn ends, after it
  pushes the commits.
- `agent_failed`: on a watch of the app, also a push or a post of the
  daemon that failed. The payload names the proposal and the
  `watch retry` command.
- `proposal`: on a watch of the app in `manual`, the work of a turn
  that waits for the user, or that the daemon rebased and offers again.
  Read `references/app-watches.md`.
- `commit`: a new head on the branch. Your push, or the work of a
  different author.
- `merge_ready`: checks green, enough approvals, nobody requesting
  changes, every review thread resolved, and nothing pending from the
  agent.

Two come from a branch that fell behind its base, on a watch of the app:

- `branch_updated`: GitHub accepted the request to update the branch
  with the method of the watch, and the agent got no message. GitHub
  moves the branch a moment later, so the head can still be the old
  one. When it moves, a `commit` with the new head follows.
- `branch_update_failed`: GitHub did not update the branch, and the
  agent updates it. GitHub refused the request, three requests for the
  same head failed, or GitHub accepted the request and the head did not
  move three poll intervals later. In the last case this row follows a
  `branch_updated` for the same head. The payload holds the reason.

Three come from auto watch (`babysitter repo config`):

- `auto_started`: the repository started the watch on its own. The
  payload says why: `mine`, `assigned` or `dependabot`.
- `approved`: the daemon submitted an approving review in the name of
  the user, by the Dependabot policy of the repository or by
  `watch merge --approve`.
- `approval_asked`: a Dependabot update in scope is green and only a
  review is missing. The user answers with
  `babysitter watch merge <watch> --approve` or the button of the app.

## Stop a watch of the app

```bash
babysitter watch stop <watch> --keep-worktree   # leave the worktree of its agent on disk
```

## Notifications

The daemon notifies the user of the events of a watch: the start, a
failed check, all checks green, ready to merge, a merge by merge when
ready, a failed merge, and the stop. The kind `auto` says that a watch
started on its own, or that a Dependabot update waits on the approval
of the user. Each one is a row of the history, which the desktop app shows and
`babysitter notifications list` prints. You do not need
`babysitter notify` for a watched pull request. Use it for a different
thing that you stop on while the user is away:

```bash
babysitter notify --title "PR #42" --url <pull request> "Reply to alice"
```

It goes to the daemon when one runs, so it lands in the same history and
reaches the user where they are.

## One snapshot, no watch

```bash
babysitter pr <target> -o json
```

This prints the state, the checks, the failed jobs with their log
endpoints, the review items that no earlier snapshot showed, and the
recommended actions. Use it for a quick look when the user does not want
a watch.

Do not run it on a pull request that is watched. It marks the review
items as shown, and the watch then does not hand them over.
