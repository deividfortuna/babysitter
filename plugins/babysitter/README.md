# babysitter plugin

The `babysit-pr` skill of [babysitter](https://github.com/deividfortuna/babysitter)
for Claude Code.

babysitter is a daemon that watches a GitHub pull request. It polls the
pull request and hands over one thing at a time: a new review comment, a
review, a check that failed with its log, or a branch that fell behind
its base or conflicts with it. The skill drives that loop from your
session: you fix, push and reply through the daemon until the pull
request can merge.

## Requirements

- The `babysitter` binary on the `PATH`. Install it with
  `go install github.com/deividfortuna/babysitter/cmd/babysitter@latest`,
  or use the desktop app.
- A running daemon: `babysitter daemon start`, or the desktop app.
- `gh` with access to the repository of the pull request.

## Install

```sh
claude plugin marketplace add deividfortuna/babysitter
claude plugin install babysitter@babysitter
```

## Use

Ask to watch, babysit or keep moving a pull request, or call the skill:

```
/babysitter:babysit-pr
```

The skill starts the watch with `--provider self`, so the work stays in
your session and in your checkout, which has to be on the head branch of
the pull request. It also reads and relays a watch that the desktop app
started, and merges a watched pull request when you ask.

## Contents

- `skills/babysit-pr/SKILL.md`: the watch loop and the rules of the work.
- `skills/babysit-pr/references/playbooks.md`: what to do for a review
  comment, a failed check, a branch behind or in conflict, and a pull
  request of Dependabot.
- `skills/babysit-pr/references/commands.md`: the other commands of a
  watch and the activity kinds.
- `skills/babysit-pr/references/app-watches.md`: how to read and relay a
  watch that the desktop app started.
