# Watches that the desktop app started

A watch of the app runs its own agent session in a pseudo terminal, in a
worktree of the checkout. That agent takes the messages, commits and
records its replies. It does not push and it does not post. The work of
each turn is one proposal. In `auto`, the daemon pushes and posts it
when the turn ends. In `manual`, it waits for the user. Do none of that
for it, and do not edit the checkout on its behalf. You read and relay.

```bash
babysitter watch output <watch>          # the last lines that agent printed, as its terminal drew them
babysitter watch send <watch> <message>  # type a message into its session, as the user would
babysitter watch retry <watch>           # push and post the proposal whose release failed, again
```

## A proposal that waits for the user

`watch status` says `Approval: manual; proposal N waits on you`, and the
merge is blocked until the user decides. While it waits, the agent takes
no message, and `watch send` is refused.

```bash
babysitter watch proposals <watch>             # every proposal, newest first
babysitter watch proposals <watch> <n> --diff  # the commits, the files, each reply with its comment, and the diff
babysitter watch proposals <watch> <n> --diff --commit <sha>  # the files and the diff of one commit only
babysitter watch proposals <watch> <n> --diff --file <path>   # the diff of one file only, also one after the one-megabyte cut
babysitter watch approve <watch>               # push and post it under the account of the user
babysitter watch approve <watch> --edit <reply id>=<text> --drop <reply id> --reject-push
babysitter watch approve <watch> --stop-asking # release it and run the watch in auto from now on
babysitter watch reject <watch> --reason <text> [--discard]
babysitter watch mode <watch> auto --release   # switch to auto, which releases what waits
babysitter watch mode <watch> manual
```

Show the user the proposal, then do what they decide. Never approve,
edit, drop or reject on your own judgement: the gate exists because the
user reads the work before it goes out under their account. A dropped
reply to a comment brings that comment back to the agent. A
reason of a rejection goes to the agent, which works on it.

## A release that failed

The work of one turn is one proposal. When the daemon cannot push it or
post its replies, `watch activity` has an `agent_failed` row with the
reason and the `watch retry` command. Tell the user the reason. Retry
only when the user says so, or when the reason is gone: a credential
that git could not read, for example, needs the user first. A retry
rebases a turn that only added commits onto a branch that moved, or merges the branch into it when the watch merges. Work
the daemon cannot rebase or merge goes to that agent, which brings it up to the
branch in its next turn. In `manual`, the daemon refuses a retry of work
the user never approved. It refuses a reject of a failed proposal whose
push landed or whose reply posted: part of it is on GitHub, and a retry
sends the rest.

The output carries escape sequences. Read it with `-o json` and remove
them, or show it in a terminal.

## Agent states

`none`, `starting`, `idle`, `active`, `waiting`, `waiting_input`,
`blocked`, `exited`. They are in `watch list` and in `watch status`.

`waiting` means that the agent ended its turn, but its background work
still runs and can wake it. The watch does not merge in that state.

`waiting_input` means that the agent asked the user a question.
`blocked` means that it waits for a permission decision. The two need
the user.

## What to send

Send only what the user said or clearly meant. An invented answer to a
question that the agent asked becomes work that nobody wants.

A send is refused while the agent waits for a permission decision,
because the Enter key would decide in place of the user. Show the user
`watch output` and let them decide.

## Take the session over

When the user wants to continue the session of that agent in their own
terminal, give them the command. Do not run it yourself: it replaces
its process with the agent, so it needs a terminal of its own.

```bash
babysitter watch takeover <owner/name#number>          # the same conversation, in the worktree, with no rules
babysitter watch takeover <owner/name#number> --shell  # a shell in the worktree instead
```

The takeover declines every proposal that waits. While the session is
with the user, `watch status` says `Agent: with you since <time>`, the
merge is blocked with `the session is with you`, and `watch send` is
refused. The daemon still polls, and holds what it finds for the
hand-back.

When the user says they are done, hand the session back:

```bash
babysitter watch handback <watch>
```

When the user left commits or changes that are not on the pull request,
the command lists them and asks. Show the user the list and pass
`--yes` only when they say so. The hand-back is refused while the agent
of the takeover still runs. Tell the user to quit it; pass `--force`
only when they say it is gone.
