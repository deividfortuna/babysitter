# What to do for each kind of item

## A review comment or a review

Agree with a comment when it is technically correct, possible on this
branch, in agreement with the intent of the change, and needs no
unrelated refactor. Confidence in the words of a comment is not
evidence.

When a comment is wrong, reply with the evidence, politely, and change
nothing. When a commit on the branch already does the work, reply with
that commit.

A reply does not resolve the thread, and the daemon cannot resolve one,
so an answered thread stays a merge blocker. Tell the user to resolve
the threads on GitHub and to ask the reviewer for a new review of the
head that you pushed. Neither is yours to do.

## A failed check

The message carries the log of the job, cut to the failing step and the
lines around the error. Find the first error, not the last: the later
lines are usually the results of the first one. Read the full log, with
the `gh api` line that the message names, only when the excerpt does not
explain the failure.

Fix the failure only when this branch caused it.

- A flaky or infrastructure failure: run the failed jobs again with
  `gh run rerun --failed <run id>`.
- A failure that is also on the base branch, or that has no relation to
  this branch: change nothing and say what you found in a comment.

Never change tests, CI configuration, dependency pins or build scripts
to make a check green. A green check that hides a real failure costs the
user more than a red one.

## A branch behind its base, or in conflict

Fetch the base, rebase onto it, resolve each conflict on the merits of
the two sides, verify, and push with `--force-with-lease`.

When a conflict needs a decision that only the user can make, stop and
ask. A wrong resolution is difficult to see in a later review.

## A pull request of Dependabot

The bot owns the branch and drops the pull request when a different
author pushes. Never push to it and never rebase it. Comment
`@dependabot rebase` and let the bot do the work.

Push a fix only when the pull request cannot merge without it, and never
change the version in the manifest or in the lockfile.
