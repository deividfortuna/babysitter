# babysitter

babysitter watches GitHub pull requests. It is one Go binary that is both
the CLI and a daemon, plus an Electron desktop app for macOS that runs the
daemon and shows its state.

A watch on a pull request runs a coding agent, Claude Code or Copilot CLI,
in a pseudo terminal inside a git worktree. The daemon polls the pull
request and types the new review comments, the failed checks with their
logs, and a branch that fell behind or conflicts into that session. The
agent fixes, commits and records its replies. The daemon pushes and posts
that work when you approve it, or at once in `auto` mode. See
[Watch a pull request](#watch-a-pull-request).

babysitter also polls the repositories you register and keeps the state of
their pull requests in a local SQLite database: title, author, review
decision, CI status, mergeable state and more. Run that poll once, in the
foreground, or as a background service of the operating system.

## Install

The desktop app runs on macOS 13 or later, on Apple silicon and on Intel.
It holds the CLI too: the daemon inside the app is the `babysitter`
binary.

With Homebrew, the cask installs the app in `/Applications` and links
the CLI on your `PATH`:

```sh
brew install deividfortuna/tap/babysitter
```

Or download the app from the latest release, open the DMG and move
`Babysitter.app` to `Applications`:

- [Apple silicon](https://github.com/deividfortuna/babysitter/releases/latest/download/babysitter-darwin-arm64.dmg)
- [Intel](https://github.com/deividfortuna/babysitter/releases/latest/download/babysitter-darwin-x64.dmg)

When macOS blocks the first launch, allow it in System Settings >
Privacy & Security, or remove the quarantine:

```sh
xattr -dr com.apple.quarantine /Applications/Babysitter.app
```

The app updates itself: it checks the releases once an hour, downloads
a new version, and asks you to restart. **Settings > Updates** turns the download off or follows
nightlies. See [docs/release.md](docs/release.md#updates-of-the-app).

For the CLI only, on Linux or macOS, each release also has a
`babysitter_<version>_<os>_<arch>.tar.gz` archive and `checksums.txt`.
`babysitter version` tells you when a newer release is out and how to
upgrade. It never upgrades by itself.

The downloads work only while the release assets are public. See
[docs/release.md](docs/release.md).

## Requirements

To build from source:

- Go 1.27 or later
- A C compiler. The SQLite driver, `mattn/go-sqlite3`, uses cgo. On macOS,
  install the Xcode command line tools. On Linux, install `gcc`.
- Node 26 or later for the desktop app and for `codegen/`. See
  `.node-version`.
- pnpm 12 or later. Each `package.json` sets the exact version in
  `devEngines.packageManager`, and pnpm switches to it.

To run:

- A GitHub token, from one of these sources, in this order:
  1. the `--token` flag
  2. the `GITHUB_TOKEN` environment variable
  3. the `gh` CLI, after `gh auth login`


## Desktop app

The repository also holds an Electron desktop app in `frontend/`. The app
runs the Go daemon for you and shows the watched repositories and their
open pull requests live. See [docs/architecture.md](docs/architecture.md)
for how the two processes talk.

```sh
pnpm start           # at the root: go install the CLI, then open the app

cd frontend
pnpm install
pnpm start           # compiles the daemon, then opens the app
pnpm run package     # builds a distributable app in frontend/out
pnpm run make        # also makes the DMG and the zip in frontend/out/make
```

`pnpm start`, `pnpm run package` and `pnpm run make` compile the daemon first
with `scripts/build-daemon.mjs` into `frontend/daemon/`. The script calls
`go build`, so Go and a C compiler are required, as for the CLI. Set
`BABYSITTER_DAEMON_BINARY` to make the app start another binary.

Releases hold only the macOS app. The Forge configuration also has
makers for Windows and Linux, but no workflow builds them. On Linux, use
the CLI archives.

`frontend/assets` holds the icons. `icon.svg` is the drawing of the app,
`icon-dark.svg` the same one on a dark tile, and `tray.svg` the black
silhouette of the menu bar. The loading screen shows the two icon SVGs.
The files below are made from those three:

| file | what reads it |
| --- | --- |
| `icon.icns` | the bundle and the DMG of macOS |
| `icon.ico` | the installer of Windows |
| `icon.png` | the window, the notifications and the menu bar of Windows and Linux, and the Linux packages |
| `trayTemplate.png`, `trayTemplate@2x.png` | the menu bar of macOS, which tints them itself |

`assets/agents` holds the logos of Claude Code and Copilot that the app
shows beside a watch. `assets/app-update.yml` tells the updater where the
releases are.

Render them with a browser and not with `qlmanage`, which fills the
transparent background with white and leaves white corners on the tile:

```sh
chrome --headless --screenshot=icon.png --window-size=512,512 \
  --default-background-color=00000000 --force-device-scale-factor=2 assets/icon.svg
sips -z <size> <size> icon.png --out icon.iconset/icon_<size>x<size>.png
iconutil -c icns icon.iconset -o assets/icon.icns
```

A notification of macOS carries no icon of its own: the system shows the
one of the bundle, and a second picture only lands beside the text.

To check the screens in a plain browser, open the Vite dev server that
`pnpm start` prints with `?daemon=http://127.0.0.1:<port>/api/v1`, the port
of a daemon started from the terminal. Without that parameter the page
shows the daemon-down screen.

One press of ⌘Q (Ctrl+Q on Linux and Windows) does not quit the app,
because a quit stops the daemon that the app started and the agents of
its watches. The app shows a hint. To quit, hold the shortcut for 1.2
seconds or press it again in 0.5 seconds. Quit in the application menu
or in the menu bar item quits at once. On macOS, when the app has no open
window, ⌘Q quits at once.

The app looks for a running daemon in the data directory and attaches to
it when it finds one. Otherwise it starts `babysitter daemon start` itself
and stops it when the app quits. A daemon you start from the terminal
survives the app:

```sh
cd backend
go run ./cmd/babysitter daemon start              # foreground, until Ctrl-C
go run ./cmd/babysitter daemon status             # pid, port and health
go run ./cmd/babysitter daemon stop
```

The data directory holds `running.json`, the supervisor socket, the
`worktrees/` of the watches, the `sessions/` logs of the agents, the
`logs/` of the daemon and the app (see [Logs](#logs)), the
`git-hooks/` the daemon installs, and the `update-settings.json` and
`theme.json` of the app. By default it holds `babysitter.db` too. It
comes from, in this order:

1. the `--data-dir` flag
2. the `BABYSITTER_DATA_DIR` environment variable
3. the directory of the default database path. `--db` and `BABYSITTER_DB`
   do not move it

### Settings

The Settings dialog of the app has three groups of pages:

- **App**: **Appearance** (the theme, light, dark or system, the rate
  limit card, and the screen reader mode of the agent), **Notifications**
  and **Updates**.
- **New watches**: **Agent** and **Review and merge**, what a watch
  starts with.
- **Daemon**: **Polling** and **Logs**.

Each page saves a change at once and shows "saved" in its header. A
number field saves when you stop typing. When you go to another page
before a save ends, the line under the navigation shows its result.

**Polling** holds the four intervals, which apply to the daemon as a
whole. **Relaxed**, **Balanced** and **Eager** set the four at once, and
Balanced holds the defaults. A bar shows about how many GitHub requests
an hour the intervals cost for the repositories and watches of now. "Set
the intervals by hand" shows the four fields. A watch poll interval
longer than the longest watch poll interval sets the longest to the same
value. **Agent** and **Review and merge** hold the defaults of a new
watch:

| Setting | What it does |
| --- | --- |
| Repository poll interval | Seconds between passes over the repositories you watch, 60 by default |
| Watch poll interval | Seconds between polls of a pull request under watch, 180 by default |
| Longest watch poll interval | Seconds a quiet pull request waits between polls at most, 900 by default. After each poll where nothing happens, the wait of that watch doubles up to this value. A new activity row, a check that runs or an agent that works brings it back to the watch poll interval. The same value as the watch poll interval keeps one fixed interval |
| Longest check read interval | Seconds the repository poll waits at most between two reads of the checks of an open pull request while they run, 900 by default. The wait starts at one minute, or at the repository poll interval or this value when one is shorter, and doubles after each read that finds the checks still pending. A new head commit or a manual sync starts it again |
| Agent | The provider and the model of a new watch, Claude and its default model by default |
| Effort | How much the model reasons before it acts: a level the model takes, such as `low`, `medium` or `high`. Empty takes the default of the model. A model can take no effort level, and then the field stays empty |
| Approval mode | Who releases the work of each turn of the agent of a new watch: `manual` holds it until you approve it, `auto` pushes and posts as soon as the turn ends. A new install asks, `manual`; an install that upgrades keeps `auto`. A `--provider self` watch always runs in auto |
| Approve a clean rebase or merge on its own | Work you approved does not ask again because the branch moved under it. A rebase or merge that conflicts always asks. No effect in auto |
| Approvals before ready to merge | What a new watch wants before it calls a pull request ready; empty takes the rule of the base branch |
| Merge method | The merge method of a new watch; empty takes the first one the repository allows |
| Report the review items that already exist | A new watch hands the agent what is on the pull request already |
| Report my own comments | A new watch reports the comments of your own user, for a repository you review yourself |
| Keep the worktree when a watch stops | The worktree of the agent stays on disk, however the watch ends |
| Branch behind its base | How a new watch updates a branch that fell behind its base: `rebase` (default) or `merge`, which adds a merge commit of the base. The agent solves a conflict with the same method |
| Update the branch on GitHub first | On by default. GitHub updates a branch that fell behind its base, with no turn of the agent. The agent does it only when GitHub refuses |

Each of these values comes from the first layer that sets it, from left
to right:

```
watch -> repository -> daemon
```

A value you give at `watch start`, or in the **Watch a pull request**
dialog, wins. A field you do not give takes the override of the
repository (see [Auto watch](#auto-watch)), and a field the repository
does not set takes the setting of the daemon. In the dialog and in the
**Repository settings** panel, a list that is not set shows
`Default (x)`, where `x` is the value the watch will take. A switch or a
number field shows that value itself. In the panel, a switch or a number
equal to the setting of the daemon stores no override, so it follows the
daemon. The daemon has no Default: it is the last layer.

A watch copies the values at its start, and a later change to the
repository or to the daemon does not reach a running watch. The
**Watch settings** panel of the watch, or `watch mode` and
`watch merge-rules`, change the approval mode, the clean rebase switch,
the approvals, the merge method and the branch update of one running
watch and leave the defaults alone. The worktree switch applies when the watch stops, and
the stop dialog or `watch stop --keep-worktree` can still say otherwise.

The **Notifications** page holds four more:

| Setting | What it does |
| --- | --- |
| Show notifications | What happens on a watched pull request reaches you as a notification of the system; the history keeps it either way |
| Only while the app is in the background | The app shows a notification only while none of its windows has the focus. Off shows it while you use the app too. On by default |
| What to tell you about | One switch per kind: Agent requests, Review comments, Checks, Watches, Merges and Auto watch. A kind you turn off stays in the history and only leaves the screen |
| Sound | One switch per kind. A kind with its sound off still shows, without a sound |

The daemon stores them in the database, so the CLI takes the same
defaults: a flag of `watch start` that you do not type is left out of the
request, and the repository, then the daemon, decides. An interval takes effect
at once, with no restart, and every watch starts again from the watch poll
interval. A new longest check read interval applies from the next read
of each pull request. The intervals accept 10 seconds
to 24 hours, and the longest watch poll interval cannot be shorter than
the watch poll interval.

Read and write them from the terminal too:

```sh
babysitter settings get                            # what the daemon holds
babysitter settings set --watch-interval 45s       # only the flags you type change
babysitter settings set --watch-max-interval 30m   # the longest wait of a quiet watch; the watch interval itself keeps one fixed interval
babysitter settings set --poll-interval 5m         # the repository poll interval
babysitter settings set --check-max-interval 5m    # the longest wait between reads of pending checks
babysitter settings set --approvals 2              # a number, 0 for none
babysitter settings set --approvals branch         # give the decision back to the base branch
babysitter settings set --merge-method rebase --keep-worktree
babysitter settings set --approval-mode auto       # push and post as soon as each turn ends
babysitter settings set --auto-approve-rebase      # approved work goes out again after a clean rebase or merge
babysitter settings set --include-existing --include-own
babysitter settings set --provider copilot --model auto   # the agent of a new watch; a new provider alone takes its default model
babysitter settings set --model opus --effort high        # the effort of that model; a new model alone takes its default effort
babysitter settings set --branch-update merge      # a branch behind its base gets a merge of the base, not a rebase
babysitter settings set --update-on-github=false   # the agent updates a branch behind its base, GitHub does not
babysitter settings set --screen-reader=false      # the agent draws its full terminal interface from the next session start
```

The notification flags of `settings set` are in
[Notifications](#notifications).

The **Updates** page belongs to the app, not to the daemon. The app
keeps it in `update-settings.json` in the data directory:

| Setting | What it does |
| --- | --- |
| Check for updates | Asks the releases now. The app also asks once an hour |
| Download updates automatically | On by default. Off, the app still checks and the sidebar offers **Download** |
| Channel | Stable, or Nightly for a build of `main` at most every 6 hours. A nightly build starts on Nightly. A change back to Stable does not install an older version |

A build that does not update itself shows only its version there, and
tells you to install a new version with Homebrew or from the release
page.

`daemon start --interval`, `--watch-interval`, `--watch-max-interval` and
`--check-max-interval` hold for that run only. A `--watch-interval` above the stored longest
interval raises the longest interval to it for that run. They do not change the stored settings, so the app keeps showing the
stored value while such a daemon runs at another rate; the daemon says so
in its log at start. Saving from the app wins over the flag. An interval
outside the bounds stops the start.

After a change to a route or a DTO in `backend/internal/httpd`, run
`pnpm run api` at the repository root. It regenerates `openapi.yaml` from
the Go types and `frontend/src/api/schema.ts` from the document. The
second step runs openapi-typescript from `codegen/`, so run `pnpm install`
in `codegen/` once before.

On macOS, if `go build` fails while linking with `tapi error: malformed
file` and `unknown architecture`, the command line tools are older than
the SDK they ship. Point cgo at the Xcode toolchain instead:

```sh
export CC=/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/clang
export CGO_CFLAGS="-isysroot $(xcrun --sdk macosx --show-sdk-path)"
export CGO_LDFLAGS="$CGO_CFLAGS"
```

## Usage

The examples use the installed binary. From `backend/`,
`go run ./cmd/babysitter` does the same.

```sh
babysitter version                                # print the version, and how to upgrade when a newer release is out
babysitter --version                              # print the version only
babysitter whoami                                 # print the authenticated user
babysitter repos                                  # list the 20 most recently updated repositories
babysitter repos --limit 0                        # list all of them

babysitter repo add owner/name                    # start to watch a repository
babysitter repo list                              # list the watched repositories, or repo ls
babysitter repo remove owner/name                 # stop to watch a repository, or repo rm
babysitter repo config owner/name                 # what the repository does with new pull requests, see Auto watch
babysitter repo config owner/name --auto-start-mine   # a watch on each new pull request of yours
babysitter repo queue owner/name                  # the Dependabot pull requests that wait for a place, oldest first

babysitter sync                                   # poll every watched repository once
babysitter serve --interval 60s                   # poll in a loop until Ctrl-C

babysitter prs                                    # list the open pull requests in the store
babysitter prs --state all                        # include merged and closed pull requests
babysitter prs --repo owner/name                  # only one repository
babysitter prs -o json                            # print as JSON

babysitter pr                                     # snapshot of the pull request of the current branch
babysitter pr 42 --repo owner/name                # snapshot by number
babysitter pr owner/name#42                       # same
babysitter pr https://github.com/owner/name/pull/42 -o json

babysitter daemon start                           # the daemon in the foreground, until Ctrl-C
babysitter daemon start --log-level debug         # also log the records a developer needs
babysitter daemon status                          # pid, port and health
babysitter daemon stop
babysitter daemon logs                            # the last 100 records of the daemon
babysitter daemon logs -f --level warn            # follow the warnings and errors until Ctrl-C
babysitter daemon logs --app -n 0                 # the whole log of the desktop app
babysitter daemon log-level                       # the level the running daemon logs at
babysitter daemon log-level debug                 # change it until the daemon stops

babysitter notify "PR #42 needs a reply to alice" # notification, through the daemon when one runs
babysitter notify --title "PR #42" --url https://github.com/owner/name/pull/42 "Reply to alice"
babysitter notifications list                     # the notification history of the daemon
babysitter notifications read                     # mark every unread notification as seen
babysitter notifications read 12 13               # mark only these rows

babysitter watch start                            # watch the pull request of the current branch, from its checkout
babysitter watch start owner/name#42              # watch by number, still from the checkout of its branch
babysitter watch start 42 --repo owner/name       # same
babysitter watch start owner/name#42 --no-checkout   # from anywhere: the daemon clones the head repository and makes the worktree from that clone
babysitter watch start --provider copilot         # choose the AI provider for this watch; without the flag the repository, then the daemon, decides
babysitter watch start --model sonnet             # choose the model of that provider; alone it runs on the provider the repository or the daemon gives
babysitter watch start --effort xhigh             # choose the effort of that model; alone it runs on the model the repository or the daemon gives
babysitter watch start --provider self            # no agent session in the daemon: your own coding agent session takes the messages
babysitter watch start --approvals 2              # approvals the pull request needs before it is ready to merge; without the flag the repository, then the daemon, decides, and 0 asks for none
babysitter watch start --approvals branch         # the rule of the base branch, whatever the settings hold
babysitter watch start --merge-method rebase      # merge method of the watch; without the flag the repository, then the daemon, decides, and an empty value, or settings that hold none, take the first method the repository allows
babysitter watch start --include-existing        # also hand the agent the review items already on the pull request
babysitter watch start --include-own             # also report the comments of your own user
babysitter watch start --merge-when-ready        # the daemon merges with the method of the watch as soon as the watch is ready
babysitter watch start --keep-worktree           # a stop leaves the worktree on disk; without the flag the repository, then the daemon, decides
babysitter watch start --branch-update merge      # rebase or merge: how the branch is updated when it falls behind its base, and how the agent solves a conflict
babysitter watch start --update-on-github=false   # the agent updates a branch behind its base; without the flag the repository, then the daemon, decides
babysitter watch list                             # the watched pull requests
babysitter watch list --all                       # stopped watches too
babysitter watch status 1                         # state, checks, the agent, and what blocks the merge
babysitter watch view 1                           # the pull request as the daemon last read it: reviewers, labels, size, checks, description
babysitter watch diff 1                           # the diff of the pull request, as gh pr diff shows it, through the cache of the daemon
babysitter watch activity 1                       # what happened on watch 1
babysitter watch activity 1 --since 20 --limit 50 # only the rows after row 20, at most 50
babysitter watch poll 1                           # ask the daemon to look at the pull request now
babysitter watch next 1 --wait 9m                 # take the next message of a self watch, after a wait when there is none
babysitter watch output 1                         # what the agent of watch 1 printed
babysitter watch output 1 --lines 50              # only the last 50 lines, 200 by default
babysitter watch send 1 "keep it to x.go"         # tell the agent something
babysitter watch reply 1 --to 31 "done, see 1a2b3c" # record a reply of the agent; the daemon posts it when the turn of the agent ends
babysitter watch retry 1                          # push and post the proposal of watch 1 whose release failed, again
babysitter watch proposals 1                      # the work of each turn of the agent of watch 1
babysitter watch proposals 1 3 --diff             # read proposal 3: commits, files, replies, and the diff
babysitter watch proposals 1 3 --diff --commit 1a2b3c4 # the files and the diff of one commit of proposal 3 only
babysitter watch proposals 1 3 --diff --file go.sum    # the diff of one file of proposal 3 only
babysitter watch approve 1                        # push and post the proposal that waits for you
babysitter watch approve 1 --edit 5="Fixed, thanks." --drop 6   # rewrite reply 5 and take reply 6 out first
babysitter watch approve 1 --stop-asking          # release it and run the watch in auto from now on
babysitter watch approve 1 --reject-push          # post the replies without the commits
babysitter watch reject 1 --reason "use a table test" --discard # nothing goes out; the agent works on your reason
babysitter watch mode 1 auto --release            # switch a running watch to auto, which releases what waits
babysitter watch mode 1 manual --auto-approve-rebase # back to manual, and let a clean rebase or merge of approved work go out on its own
babysitter watch merge-rules 1 --approvals 0      # change the approvals of a running watch; branch reads the rule of the base branch again
babysitter watch merge-rules 1 --merge-method rebase # change the merge method of a running watch; empty takes the first method the repository allows
babysitter watch merge-rules 1 --merge-when-ready # let the daemon merge watch 1 when it is ready; =false turns it off
babysitter watch merge-rules 1 --branch-update merge --update-on-github=false # change how a running watch updates a branch behind its base
babysitter watch start --approval-mode auto       # the approval mode of this watch; without the flag the settings decide
babysitter watch start --auto-approve-rebase      # approved work goes out again after a clean rebase or merge, without asking
babysitter watch stop 1                           # stop with a summary, and delete the worktree unless the settings keep it
babysitter watch stop 1 --keep-worktree           # stop but leave the worktree on disk; without the flag the rule the watch started with decides
babysitter watch merge 1                          # merge the pull request once the watch says it is ready, and stop
babysitter watch merge 1 --method squash          # with a merge method for this merge
babysitter watch merge 1 --approve                 # a Dependabot update in scope: approve in your name, then merge
babysitter watch takeover 1                       # continue the agent session in this terminal, with no rules
babysitter watch takeover 1 --shell               # a shell in the worktree instead of the agent
babysitter watch handback 1                       # give the session back; it asks first when you left work that is not pushed
babysitter watch handback 1 --yes                 # give it back with that work, without a question
babysitter watch handback 1 --force               # give it back although the agent of the takeover still runs

babysitter settings get                           # the settings of the running daemon
babysitter settings set --watch-interval 45s      # change one; the rest keep what they have

babysitter ratelimit                              # the GitHub requests the daemon has left, and when they reset
```

The `service` commands are in [Service](#service).

Global flags:

- `--token`: GitHub token
- `--timeout`: request timeout, default `30s`, `0` for none
- `--output`, `-o`: output format, `text` (default) or `json`
- `--db`: SQLite database path

The commands that talk to the daemon, `daemon`, `watch`, `settings`,
`notifications`, `notify` and `ratelimit`, also take `--data-dir`, the
directory where they look for the running daemon.

The database path comes from, in this order:

1. the `--db` flag
2. the `BABYSITTER_DB` environment variable
3. `<user config dir>/babysitter/babysitter.db`, which is
   `~/Library/Application Support/babysitter/babysitter.db` on macOS and
   `$XDG_CONFIG_HOME/babysitter/babysitter.db` on Linux, with
   `~/.config` when `XDG_CONFIG_HOME` is not set

## What the store holds

Each pull request row has these fields:

- number, GitHub id, title, author and the URL of its avatar, URL, base and
  head branch, head commit
- state: `open`, `merged` or `closed`
- draft flag, labels, requested reviewers, additions and deletions
- assignees, the fork flag (the head branch lives in another
  repository), and for Dependabot the update type: `patch`, `minor` or
  `major`, read from the title and the body
- `review_decision`: `approved`, `changes_requested`, `review_required` or
  `none`. Only the latest approval or change request of each reviewer
  counts: a later review that only comments leaves it standing. The author
  does not count. `review_required` needs a requested reviewer.
- `approvals` and `changes_requested`: how many reviewers approve, and
  how many ask for changes
- `ci_status`: `success`, `failure`, `pending` or `none`, from the check
  runs and the commit statuses of the head commit
- `mergeable_state`: the value GitHub reports, for example `clean`,
  `blocked`, `dirty` or `unknown`
- created, updated, merged, closed and synced timestamps

A pull request that is merged or closed is fetched one last time. After
that, babysitter does not poll it again.

The rows hang off `repos`, the watched repositories.
`repo_config` holds the auto watch configuration of each repository, and
`auto_start_claims` the pull requests that auto start took, one row each,
so that a pull request starts on its own only once. A claim older than
10 minutes with no watch is left by a daemon that stopped during the
start, and the next pass takes it over. Both go with the repository.

The `pr` command keeps three more tables: `pr_watch` records each pull
request it took a snapshot of, `pr_watch_seen` records the review items
it showed, and `pr_watch_retries` counts flaky retry cycles per head
commit. These tables do not depend on the watched repositories.

The `watch` command keeps `watches` (one row per watched pull request,
with the conversation id of its agent and the state of a takeover) and
`watch_activity` (what each
watch reported, one row per item, with the moment it reached the agent).
`proposals` and `proposal_replies` hold the work of each turn of the
agent and the replies it recorded. A watch takes its snapshots through
the same code as `pr`, so it reuses `pr_watch`, `pr_watch_seen` to show
each review item once, and `pr_watch_retries`.

The daemon keeps its preferences in `settings` and the notification
history in `notifications`.

`github_responses` is the disk copy of the cache of GitHub answers: the
ETag or `Last-Modified` of each GET, with its status, headers and body.
Every command that opens the store reads and writes it, `pr` and `sync`
included. See [Rate limit](#rate-limit).

## Snapshot

`pr` is the one shot mode of the
[babysit-pr](plugins/babysitter/skills/babysit-pr/SKILL.md)
skill. It fetches one pull request from GitHub, not from the store, and
prints what a babysitter needs to decide the next step. Use `-o json` to
feed an agent.

The target is a pull request URL, `owner/name#number`, or a number with
`--repo`. Without a target, `pr` runs `git` to read the current branch and
the remotes, and looks for a pull request with that head. The branch was
pushed to `origin`, so the owner of `origin` is the head owner. The pull
request lives in the repository of the `upstream` remote when there is one
(a fork), in `origin` otherwise. A bare number without `--repo` also takes
the repository from the remotes. Whatever the spelling of the target, the
snapshot keys the pull request by the owner and name GitHub returns.

The snapshot holds:

- `snapshot_at`: when the snapshot was taken
- `pr`: repository, number, `node_id`, URL, title, author, `author_avatar_url`,
  state, draft, merged and closed flags, head and base branch, the `head_repo` of a fork, head
  commit, `mergeable` (`null`
  while GitHub computes it), `mergeable_state`, `behind` (the branch
  needs the commits of its base, see below), `behind_by` (the commits
  of the base that the head does not have, read only while the state is
  `blocked`, else 0), `behind_err` when that compare failed,
  `review_decision`, the
  counts of `approvals` and `changes_requested`, the
  `requested_reviewers` still pending, the `reviewers_behind_head`
  who reviewed an earlier commit, the `reviewers` with the verdict
  that stands for each person who reviewed, `labels`, `assignees`,
  `milestone`, `auto_merge` (its merge method, absent when auto-merge is
  off), `additions`, `deletions`, `commits`, `changed_files`,
  `created_at`, the `body`, and for a pull request of Dependabot its
  `update_type`: `patch`, `minor` or `major`
- `checks`: the overall status and the counts of passed, failed, pending
  and skipped checks, `all_terminal`, and one item per check run or
  commit status of the head commit
- `failed_runs`: the GitHub Actions workflow runs of the head commit that
  failed. Only the latest run of each workflow counts
- `failed_jobs`: the failed jobs of every run that failed or still runs,
  each with its `logs_endpoint` for `gh api`
- `awaiting_approval`: the workflow runs of the head commit that wait for
  a person: an approval to run (status or conclusion `action_required`) or a
  deployment review (status `waiting`). They are not failures: there are
  no logs to read and `gh run rerun` cannot approve them.
- `new_review_items`: conversation comments, inline review comments and
  submitted reviews that no earlier snapshot of this pull request showed.
  Pending reviews and their inline comments do not appear until the
  reviewer submits them, and a review that only comments with an empty
  body does not appear at all. An inline comment gives its `path`, its
  `line`, the `side` of the diff that line is on (`LEFT` or `RIGHT`), and
  the `commit_id` the line refers to. When GitHub can no longer place an
  outdated comment on the current diff, `line` and `commit_id` are those
  of the commit the comment was written on.
- `threads`: the count of `unresolved` review threads, from the GraphQL
  API, and `err` when they could not be read. `unanswered` counts the
  unresolved threads where the last comment is not from the user of the
  token. `reviewers` names the other logins that wrote in the threads
  that user answered and that GitHub can ask for a review: each one
  submitted a review, has no draft review, and is not the author of the
  pull request. `last_answer` is the id of the newest of those answers.
  Only the daemon knows that user, so `babysitter pr` gives
  `unanswered` equal to `unresolved`, no `reviewers` and no
  `last_answer`.
- `actions`: what to do next, in order
- `retry_state`: retry cycles used for the head commit and the budget from
  `--max-flaky-retries` (default 3). `pr` does not rerun jobs. After
  `gh run rerun <run_id> --failed`, run `pr` once with `--record-retry`
  to count the cycle.

The actions are:

| Action | Meaning |
| --- | --- |
| `ready_to_merge` | CI is green, no new review items, nothing blocks the merge |
| `process_review_comment` | new review items need an answer or a change |
| `diagnose_ci_failure` | a check, a run or a job failed, read its logs |
| `retry_failed_checks` | every check ended, a run failed, and the retry budget has room |
| `stop_exhausted_retries` | every check ended, a run failed, and the retry budget is used |
| `stop_action_required` | a workflow run waits for approval, or a check waits for a person |
| `stop_pr_closed` | the pull request is merged or closed |
| `idle` | nothing to do now: checks are still running, or the merge waits on something only people can give, such as a review |

`ready_to_merge` needs every one of these:

- `checks.status` is `success` and every check ended. A head commit
  without any check is not green.
- no workflow run failed, with or without a check run
- no new review items
- the pull request is not a draft, and no workflow run waits for approval
- `mergeable` is `true` and `mergeable_state` is `clean`. Any other
  state, `unstable` and `has_hooks` included, blocks it.
- no reviewer requested changes, at least one approved, and no requested
  reviewer, person or team, is still pending
- no review thread is unresolved, and the threads could be read

A check run with conclusion `action_required` or status `waiting` counts
as pending, in the snapshot and in `ci_status`. A `stale` check run is not
a failure. A failed workflow run without a check run, for example a
`startup_failure`, is a failure.

`pr` records the review items it showed, and the retry of `--record-retry`,
only after it wrote the snapshot. A snapshot that never reaches its reader,
for example when the output pipe closes, does not hide the items or use
the budget.

A snapshot makes eight requests plus one for the jobs of each workflow
run that failed or still runs: the pull request detail, six REST calls,
and one GraphQL query for the review threads. Only the latest run of
each workflow counts, and a run that waits for approval costs nothing.
A list that spans more than one page costs one request per page; the
combined commit status reads one page of 100. Without a target, finding
the pull request of the branch costs one or two more. The requests after
the pull request detail run together. Each GET goes out with the ETag or
`Last-Modified` of its last answer, and a `304 Not Modified` is served
from the cache, so a snapshot of a pull request that did not change
costs less than the count above.

## Notifications

A notification is what babysitter tells you about: a review comment
nobody takes, a check that failed or turned green, a pull request that
can merge or whose merge failed, a watch that started or ended, an agent
session that ended, a release of the work of the agent that failed, a
proposal that waits on your approval, and whatever an agent asks you for
with `notify`. Each one is a row in the database and a notification of
the operating system.

Where it is shown depends on who is there:

- The desktop app shows it itself while it is open. It opens the change
  feed with `present=1`, and the daemon shows none of its own while such
  a stream is open, so nothing arrives twice. A click brings the app up
  on the watch it belongs to, or opens the pull request in the browser
  when it belongs to none and carries its link. The unread count sits on
  the dock icon.
  With **Only while the app is in the background** on, nothing is shown
  while a window of the app has the focus: the screen already says it.
  A notification of kind `agent`, what the agent
  asks for and a proposal that waits on you, keeps the dock of macOS
  bouncing until you come back, and every other kind bounces once. The
  kinds `agent` and `merge` flash the taskbar of Windows and Linux; a
  comment or a check does not flash.
- With no app, the daemon talks to the operating system itself, so a
  daemon started from a terminal notifies the same way.

Either way the row goes to the history, which the **Notifications**
screen of the app and `babysitter notifications` show. The app also
holds a menu bar item: it carries the unread count, opens the
notification screen on a click, and marks everything read from its menu,
which it does through the daemon so it works with no window open. A
click on a banner of a watch brings the app up, on that watch, and marks
that row as well; a window you closed is made again for it. A banner
that only opens a link in the browser marks its row when a window of the
app exists, and makes none.
The history keeps the newest 5000 rows: one of a watch goes when that
watch is removed, and the oldest fall out of the bottom as new ones
arrive.

Four settings govern them, in the **Notifications** page of the app or
from the terminal:

```sh
babysitter settings set --notifications=false               # only the history, nothing on screen
babysitter settings set --notifications-background-only     # the app shows nothing while one of its windows has the focus
babysitter settings set --mute-notifications review,checks  # every kind but these two
babysitter settings set --mute-notifications ""             # every kind again
babysitter settings set --silent-notifications watch,auto   # these two kinds without a sound
babysitter settings set --silent-notifications ""           # every kind with a sound again
babysitter settings set --notification-sound=false          # every kind without a sound
babysitter settings set --notification-sound                # every kind with a sound
```

`--mute-notifications` and `--silent-notifications` take the kinds of
the history: `agent`, `review`, `checks`, `watch`, `merge` and `auto`.
`auto` says that a watch started on its own, or that a Dependabot update
waits on your approval; in the app that row has an **Approve and merge**
button. A kind you mute stays in the history and only leaves the screen.
A silent kind still shows, without a sound. `--notification-sound`
writes the silent kinds too: `false` makes every kind silent and `true`
makes none silent, so do not give it with `--silent-notifications`.

`--notifications-background-only` applies to the app only. The daemon
shows a notification only while no app shows them, so it has no window
that can have the focus.

Read the history from the terminal:

```sh
babysitter notifications list                      # newest first, * marks an unread row
babysitter notifications list --unread --limit 20
babysitter notifications read                      # mark every unread row as seen
babysitter notifications read 12 13                # mark two rows
```

### notify

`notify` sends one notification. An agent that babysits a pull request
runs it when it needs the user, for example to reply to a review
comment, to approve a response, or to fix credentials. Put the flags
first. The words after the flags form the message, so a word that starts
with a dash is part of the message and not an option.

```sh
babysitter notify "PR #42 has a question from alice, reply needed"
babysitter notify --title "PR #42" --subtitle owner/name --url https://github.com/owner/name/pull/42 "Reply to alice"
babysitter notify --silent -o json "CI on PR #42 failed three times, help needed"
```

The notification goes to the daemon when one runs: the daemon records it
and shows it where the user is. With no daemon, with a daemon that does
not take it, or with `--local`, the command talks to the operating
system itself. When a daemon answered and did not take it, the command
says why on stderr.

Flags:

- `--title`: the first line, default `babysitter`
- `--subtitle`: the second line, for example the repository
- `--url`: opens on click with terminal-notifier. The other tools show it at
  the end of the message.
- `--silent`: no sound
- `--local`: show it here instead of giving it to the daemon
- `--data-dir`: where to look for the running daemon
- `--timeout`: the global flag also limits how long the command can take,
  for example when the first `osascript` notification waits for the
  permission prompt. The daemon takes half of it, so one that never
  answers still leaves time for the operating system. A daemon that
  answers late is asked whether it recorded the row before the command
  shows a second banner

The tool comes from the operating system:

- macOS: [terminal-notifier](https://github.com/julienXX/terminal-notifier)
  when it is on the PATH, for example after `brew install
  terminal-notifier`, otherwise `osascript`. The first notification from
  `osascript` asks for permission for Script Editor in System Settings.
- Linux: `notify-send` from libnotify
- Other systems, Windows included, have no tool, so there the command
  fails unless a daemon takes the notification

`-o json` prints `sent`, `recorded`, `backend`, `clickable`, `silent`
and the content as sent, so an agent can confirm the delivery.
`recorded` is true when the daemon took the notification; `backend` is
then `daemon`, and the message starts with the subtitle, as the daemon
stores it. `silent` is
false when `--silent` was set but `notify-send` is too old for the sound
hint; a notification the daemon records carries `--silent` on the row, so
the desktop app shows it without a sound too. The command fails when the
notification reaches neither the daemon nor a tool of the operating
system.

## Watch a pull request

`watch` hands one pull request to the daemon. The daemon checks it every
few minutes, reports what is new, and tells a coding agent that runs in
a live session on a worktree of your checkout. The agent fixes, commits
and replies, the way it does when you run it yourself. It does not push
and it does not post. A new install holds the work of each turn until
you read and approve it; nothing goes out under your account before
that. Set the approval mode to `auto`, for all watches or for one, and
the daemon pushes and posts as soon as the turn ends. You read what the
agent printed and tell it things.

Requirements:

- The daemon runs: `babysitter daemon start`, or the desktop app.
- The `claude` and `copilot` commands are on the PATH. Without one of
  them the daemon still reports activity, but a watch cannot pick that
  provider. Pass `--agent-bin none` to `daemon start` to turn Claude
  off on purpose, and `--copilot-bin` to run a Copilot CLI from elsewhere
  on disk. `--agent-model` and `--copilot-model` set the model of a
  watch that picks none; `watch start --model` and the model box of the
  app pick one for that watch. `watch start --effort` and the effort box
  of the app pick the effort level. The daemon gives it to Claude Code as
  `--effort` and to Copilot CLI as `--reasoning-effort`. The models of
  each provider, and the effort levels of each model, are in
  `backend/internal/prwatch/model-manifest.json`.
- The `origin` remote of the checkout is the head repository of the pull
  request. The start fails otherwise.
- The token can push to the head branch, and git can push it from the
  checkout without a prompt. The daemon pushes with the credential
  helper of your git configuration, and it has no terminal to ask on.
- `git user.name` and `git user.email` are set in the checkout. The
  agent commits with them, and with the signing configuration of the
  checkout.
- The agent runs on your Claude Code subscription or your Copilot
  subscription, depending on the provider you pick. A session stays open
  for the life of the watch and costs usage only when it gets a message.

You do not need a checkout of your own. When the start has none
(`watch start --no-checkout`, an empty checkout field in the app, or a
repository with auto start and no `--checkout`), the daemon clones the
head repository once into `<data dir>/checkouts/<owner>/<name>` and uses
that clone as the checkout. The clone keeps no files of its own; each
watch gets its worktree from it. The clone fetches and pushes on
github.com with `gh auth git-credential`, so `gh` must be logged in.
If your git configuration has
`url.git@github.com:.insteadOf https://github.com/`, the clone uses SSH
with your keys instead, and `gh` does not need to be logged in. The
agent commits with the `user.name` and `user.email` of your global git
configuration. The target must name the repository and the number, and
the provider must be `claude` or `copilot`: a `self` watch works in your
own checkout.

Run `watch start` in the checkout of the repository. The daemon fetches
the head branch there and makes a git
worktree of that checkout in `<data dir>/worktrees/<owner>-<name>-<n>`,
in lower case,
on a private branch `babysitter/<head branch>` that tracks the pull
request branch, and starts the agent there: Claude Code as an
interactive session in a pseudo terminal the daemon owns, or Copilot
CLI the same way. The first message names the pull request, the
worktree and the branch the daemon pushes, and tells the agent to read
the pull request and wait. Your own checkout is never touched. What already
exists on the pull request is the baseline: only later activity goes to
the agent. A branch that is already behind its base or in conflict with
it, or a check that already failed, is the exception: the start tells
the agent at once.
`--include-existing` tells it about the review items that already exist
too. Your own comments are not review feedback and are left out;
`--include-own` reports them, for a repository you review yourself. The
agent replies through the daemon, with `babysitter watch reply`. With
`--to`, a reply answers a review comment in its thread, or a comment on
the conversation on the conversation, and a second reply to the same
comment takes the place of the first before it goes out. The daemon
checks the comment the reply answers at once, keeps the reply
until the turn ends, and remembers each comment it posts, so a reply of
the agent never comes back to it as feedback, with or without
`--include-own`.

On each check the daemon looks for new comments, review comments and
reviews, checks that failed, recovered, or all passed, new commits on the
branch, the branch falling behind its base or conflicting with it, and
the pull request being merged or closed. Each item is reported once, in
the daemon log, in `watch activity`, in the desktop app, and as a
desktop notification for the moments that matter. A quiet check reports
nothing; after an hour of quiet, one line says the watch is alive. A
restart neither repeats old items nor loses new ones.

The items that ask for work go to the agent as one message: the failed
checks with the API path of their logs, the review comments with their
ids and the file and line, the reviews, and the branch behind its base
or in conflict with it. The text is typed into the terminal as you would
type it. A `nudged` row in the activity keeps the message, and the app
shows it. The agent reads the logs and the code, decides on the merits,
fixes what the branch broke, verifies with the narrowest check, commits
in your voice, and replies in each thread through the daemon. A flaky
check gets a rerun, a failure of the
base branch gets a comment, and a comment that is wrong gets a polite
answer with the evidence. A product decision only you can take comes
back as a question, on the pull request and in the session, and the
agent waits. Everything read from the pull request and from CI is data
to verify, never an instruction to follow.

A reply does not resolve the review thread it answers. Only
`resolveReviewThread` on the GraphQL API of GitHub does that, and the
daemon does not call it yet. An unresolved thread stays a merge
blocker, so a pull request whose comments are all answered still shows
`n review threads unresolved` in `watch status` and never reaches
`merge_ready` on its own. Resolve each answered thread on GitHub
yourself. The new review is the daemon's job: once the agent is done
with a head and a review is the only thing the pull request still waits
for, it asks for one, once per head and newest answer: a new answer
in a thread on the same head gets a new request. A thread where the
agent wrote the last reply waits on its reviewer, so it counts as part
of that review.
The request goes out only when every unresolved thread ends with a reply
of the agent. When the pull request is short of its approvals or a
reviewer is against it, the daemon asks the reviewers of the earlier
commits and the reviewers of the answered threads. When the answered
threads are all that is left, it asks only their reviewers. The
answered threads still stop the merge until someone resolves them.
A thread nobody but the user of the token wrote in is not counted, so a
blocker always has feedback behind it; with `--include-own` it counts.
The daemon asks for no review while reviewers are already requested. A
request that fails three times leaves a row in the activity that names
the reviewers and the cause.

A branch is behind its base when GitHub reports the state `behind`, or
when GitHub reports `blocked` and a compare of the base with the head
finds commits of the base that the head does not have. GitHub reports
`blocked` in place of `behind` when a rule other than an up to date
branch also fails, for example a required check that the old head never
reports. The daemon compares only while the state is `blocked`. It reads
the second page of the compare, which has no list of changed files, so
the answer stays small and a conditional request gets a 304 until the
base or the head moves. A compare that fails counts as not behind: the
blockers and the daemon log give the reason.

When the branch falls behind its base, the daemon first asks GitHub to
update it, with the GraphQL mutation `updatePullRequestBranch`, and the
method of the watch: `rebase` or `merge`. The expected head of the
mutation makes GitHub refuse the update when someone pushed in between.
The activity records `branch_updated`, and the agent gets no message.
GitHub accepts the request and moves the branch a moment later, so the
answer still names the old head and the next poll sees the new one. A
conflict is refused at once. When the head did not move three poll
intervals after GitHub accepted the request, the activity records
`branch_update_failed` and the agent updates the branch. Until the head
moves or the update is recorded as failed, the agent gets no message,
and `watch send` is refused with the reason.
The daemon waits while work is in flight: while the agent works, while a
proposal is open or waits for you, or while the session is with you. A
rebase on GitHub would move the branch under that work, so the daemon
asks GitHub after the work is done, and until then the agent gets no
message about the branch.
The work branch of the worktree follows the new head before the next
message. It follows any rewrite of the pull request branch, by GitHub,
by you or by a force push, when it sits on a head that the daemon saw
before, because then it holds no work that the branch lacks. GitHub gets one try for each head, and a new head that is
still behind gets a new try. When GitHub refuses, for example on a
conflict, the activity records `branch_update_failed` with the reason,
and the agent updates the branch. Only a GraphQL error of the type
`UNPROCESSABLE`, `FORBIDDEN` or `NOT_FOUND` is a refusal. A network
error, a timeout, a 5xx answer, a rate limit or an error of another
type is not a refusal: the next poll tries again, and after three
failures for the same head the agent updates the branch. When a
proposal of the agent failed, the daemon does not ask GitHub: the agent
gets the branch behind its base, and its next turn also solves the
failed proposal.

The agent updates the branch in the worktree with the method of the
watch: it rebases onto the base, or it merges the base into the branch.
It resolves each conflict on the merits of both sides, and commits.
When the pull request branch moves under work that did not go out yet,
the daemon moves the work with the same method: it rebases the new
commits of the work, or, with `merge`, it merges the branch into the
work and pushes without force. A conflict goes back to the agent.
GitHub cannot solve a conflict, so a branch in conflict goes to the agent
at once. With `--update-on-github=false`, the agent also updates a branch
that is only behind. A `--provider self` watch never asks GitHub: your
own session pushes the branch, and a rebase on GitHub would move the
branch under it. A pull request opened by Dependabot is different:
the branch belongs to the bot, which drops the pull request or opens it
again when somebody else pushes to it. The agent comments
`@dependabot rebase` instead, and the daemon never pushes and never
rebases that branch. A commit the agent makes there stays in the
worktree until the next message: before it, the worktree moves to what
the bot pushed.

### Auto watch

A repository can start watches on its own. Its configuration says what
babysitter does with each new pull request. An empty configuration starts
nothing, and the configuration goes when you remove the repository.

```sh
babysitter repo config acme/billing --checkout ~/code/billing   # the checkout each worktree comes from; optional
babysitter repo config acme/billing --auto-start-mine           # a watch on each new pull request you opened or that is assigned to you
babysitter repo config acme/billing --include-drafts            # also your drafts
babysitter repo config acme/billing --auto-watch-dependabot     # a watch on each new pull request of Dependabot
babysitter repo config acme/billing --dependabot-scope minor    # patch (default), minor or major: the highest update that merges on its own
babysitter repo config acme/billing --dependabot-approval ask   # never (default), ask or green
babysitter repo config acme/billing --dependabot-limit 2        # Dependabot watches at the same time, 1 by default
babysitter repo config acme/billing --merge-method squash --approval-mode manual   # the overrides of each watch on the repository
babysitter repo config acme/billing --provider claude --model opus --effort max   # the agent of each watch on the repository
babysitter repo config acme/billing --keep-worktree --include-own=false            # a switch takes an override too
babysitter repo config acme/billing --branch-update merge --update-on-github=false # how the watches on the repository update a branch behind its base
babysitter repo config acme/billing --approvals default --reset-overrides           # back to the settings of the daemon
babysitter repo config acme/billing --auto-start-mine=false     # turn a toggle off; the watches that run go on
babysitter repo queue acme/billing                              # the Dependabot pull requests that wait
babysitter repo list                                            # the AUTO START column shows the toggles that are on
```

The app has the same settings in the **Repository settings** panel of a
repository, which the panel icon in its header opens.

The rules:

- The checkout must be a git checkout whose `origin` is the repository.
  It is optional: without it, the daemon clones the repository into
  `<data dir>/checkouts/<owner>/<name>` and makes each worktree from
  that clone.
- A toggle records when it went on. It takes only the pull requests that
  GitHub created from then on, also one that opened while the daemon was
  down. It starts no watch on the pull requests that were open before.
- A pull request is yours when you wrote it or it is assigned to you, as
  `GET /user` of the token says. A draft waits until it is ready for
  review, unless drafts are included. A pull request from a fork never
  starts: the agent cannot push to it, and the log names it once.
- Auto start takes a pull request only once. A pull request that has or
  had a watch, also one you stopped or one that stopped with an error,
  does not start again on its own. Start it by hand.
- The overrides apply to each watch on the repository, one you start by
  hand and one that auto start begins: a field the watch start does not
  give takes the override, and a field without an override takes the
  setting of the daemon. The panel calls this group **Watch defaults**.
  A list without an override shows `Default (x)`; a switch or a number
  shows the value a watch takes, and stores an override only while it
  differs from the daemon.
- A watch that auto start began is the same as `watch start` with no
  flag. It records an `auto_started` row, and the
  notification kind `auto` says why it started. The watch shows an
  **auto** badge in the app and an `Auto:` line in `watch status`.
- Auto start runs in the daemon, after each pass of the repository
  watcher. `babysitter serve` runs the watcher and starts no watch. When
  two daemons share one database, only one of them starts a pull
  request; the other writes a debug line.

**The Dependabot queue.** At most `--dependabot-limit` Dependabot watches
run on the repository at the same time, also the ones you started by
hand. A new update beyond that waits in the queue, which the store keeps
and `repo queue` and the app show. When a place is free, the oldest one
starts on the next pass. A pull request leaves the queue when it closes,
merges, or when you start a watch on it by hand. The queue is empty
while the toggle is off. Each merge moves the base, so the next update
is often behind and waits for `@dependabot rebase` and a new build.

**Merge when ready.** Each watch has this option, off by default. Set it
with `watch start --merge-when-ready` or `watch merge-rules
--merge-when-ready`, or with the switch of the start dialog and of the
**Watch settings** panel. When the watch records `merge_ready`, the
daemon merges with the method of the watch, through the same path as
`watch merge`. Everything that keeps a watch from ready also keeps it
from this merge. A merge that fails records `merge_failed` and notifies;
the daemon tries once for each head, and a new head tries again. On a
Dependabot watch that auto start began, the option is on when the update
is within the scope, and off when it is not: a major update in a `minor`
scope stops at ready to merge and waits for you. The daemon reads the
update type from the title and the body of the pull request, a grouped
pull request takes its highest update, and a type it cannot read counts
as `major`.

**Dependabot approval.** With `never`, the daemon submits no review; when
the base branch needs one, the watch shows the blocker and waits. With
`ask`, a green update in scope whose only blocker is a missing review
records `approval_asked` and notifies you. The notification row in the
app has **Approve and merge**; from the terminal it is
`babysitter watch merge <watch> --approve`. With `green`, the daemon
submits an approving review in your name when the build is green and the
update is in scope, once for each head, with a body that names the
Dependabot policy of babysitter, and records an `approved` row. The
daemon never approves an update outside the scope, and never a pull
request that is not of Dependabot.

### The daemon pushes and posts

A turn starts when the agent starts to work on a message, and it ends
when the agent goes idle or its session ends. A question to you in the
middle of a turn keeps the turn open. The work of one turn is one
proposal: the commits on the work branch and the replies the agent
recorded. A turn that made no commit and no reply leaves no proposal.

A git hook the daemon installs refuses every push of the session, and
the refusal tells the agent to commit and let the turn end. When the
turn ends, the daemon pushes the work branch, then posts the replies in
the order the agent made them. The replies describe the code, so a push
that fails posts none of them. A push that landed says nothing to the
agent.

A second git hook adds this trailer to each commit of the session:

```
Co-authored-by: babysitter <335241182+babysitter-orchestrator@users.noreply.github.com>
```

The hook adds the trailer one time only, also when the agent amends a
commit. The session of a takeover has no hooks, so your
commits do not get the trailer.

The daemon decides how to push. Before it types a message into an idle
session, it fetches the pull request branch and fast-forwards the work
branch to it, so a commit somebody else pushed is under the work of the
turn. At the start of the turn it records the head of the pull request
branch. A work branch that contains the current head goes out as a
plain push. Anything else rewrites the branch, and goes out with
`--force-with-lease=refs/heads/<branch>:<head it read>`, never a bare
`--force-with-lease`: the fetch before the push moves the ref a bare
lease reads, and the lease would pass whatever moved. When somebody
pushed while the agent worked, and the turn only added commits on top
of the head it started from, none of them a merge, the daemon rebases
those commits onto the new head and pushes them. A turn that takes over
the commits of a failed release starts from the head that release
started from. A rewrite that would drop a commit the work branch never
had does not go out. A watch with the branch update `merge` never
pushes with force: a rewrite fails its release and names the commits of
the branch that the work lacks.
The push runs with `--no-verify`, because the session never ran the
hooks of the repository either, and with `GIT_TERMINAL_PROMPT=0` and
`GCM_INTERACTIVE=never`.

A release that fails is an `agent_failed` row in the activity, with the
reason and the command that retries it, and a notification of kind
`watch`. `babysitter watch retry <watch> [proposal]` pushes and posts
again. In `auto`, or with **Approve a clean rebase or merge on its own**, it
rebases a turn that only added commits onto a pull request branch that
moved; otherwise the next poll rebases it and asks you again. Each
reply that names one of the old commits then names the rebased one.
With the branch update `merge`, the daemon merges the pull request
branch into the work instead: no commit is rewritten, the replies stay
as they were, and a merge that conflicts goes to the agent the same
way. In `manual`, a retry of work you never approved is refused. Work the daemon cannot rebase or merge goes to the agent: a rebase or merge
that conflicts, at once, and a rewrite that lacks commits of the pull
request branch, on a retry. These are the only failed pushes the agent
hears about. The row of a rebase or merge that conflicts names no retry, because
only the agent can resolve it. Its next turn brings the work up to the
branch and takes over the replies that did not go out. The agent can
record a reply again for the same comment, and the new reply takes the
place of the old one, so the thread gets one answer. A reply GitHub can
never take, such as one to a comment that was deleted, is dropped with
an `agent_failed` row, and the replies after it still go out. A comment
counts as deleted only when GitHub does not find it but still finds the
pull request: a token that lost access gets a 404 too, and its replies
wait for a retry. Work that did not go out blocks the merge until it
does, in both modes.

### Approve the work of the agent

In `manual`, the turn ends and nothing goes out. The proposal waits for
you: a `proposal` row in the activity, a notification of kind `agent`,
and the blocker `proposal N waits on your approval` in `watch status`
and the app, so the watch never calls the pull request ready before the
work you are about to read. `watch proposals <watch> <n>` prints the
commits, the changed files, and each reply with the comment it
answers; `--diff` adds the plain unified diff, and `--commit <sha>`
keeps the files and the diff to one commit of the proposal. A diff
longer than one megabyte stops at the last whole file under that size;
`--file <path>` reads one file, including a file after the cut. Then:

- `watch approve <watch>` pushes and posts it, under your account.
- `--edit <reply id>=<text>` rewrites a reply first, and `--drop <reply
  id>` takes one out. A dropped reply to a comment brings the
  comment back to the agent on a later poll. The agent hears what you
  changed in a message that asks for nothing.
- `--reject-push` posts the replies without the commits, which stay on
  the work branch. They come back only under a new commit of the agent.
- `--stop-asking` releases the proposal and runs the watch in `auto`
  from then on. Your other watches still ask.
- `watch reject <watch> --reason <text>` sends nothing out, and the
  agent works on your reason. Without `--reason`, the agent only hears
  that nothing went out. Without `--discard`, the commits stay on the
  work branch for the agent to build on. `--discard` resets the work
  branch to the head the turn started on, whatever the worktree setting
  says. Without a proposal number, the command takes the proposal that
  waits, or else the one that failed. A failed proposal whose push
  landed or whose reply posted is partly on GitHub, and the reject is
  refused: retry it to send the rest.
- `watch mode <watch> auto --release` switches a running watch to
  `auto` and releases what waits; without `--release` the command
  refuses and names the proposal. `watch mode <watch> manual` switches
  back, and `--auto-approve-rebase` sets the clean rebase switch of the
  watch.

While a proposal waits, the agent takes no message: the comments,
reviews and checks that arrive stay untold, and the message after your
decision carries all of them. `watch send` is refused with the reason.
When the pull request branch moves while you read, the daemon rebases
the proposal itself. A clean rebase comes back to you marked rebased,
with the commits it now names, and a reply that named an old commit
names the new one; with **Approve a clean rebase or merge on its own**, work you
approved goes out without asking again. With the branch update `merge`,
the daemon merges the branch into the proposal instead: it comes back
marked merged, and its commits and replies stay as they were. A rebase
or a merge that conflicts goes to the agent, and in `manual` its resolution asks you, because nobody read
it. Work that rewrote the branch is not rebased: its release fails, and
its retry goes to the agent. Stop declines what waits. A watch started
with `--provider self` has no gate: your own session pushes and posts,
and `watch start` says so when you give it `--approval-mode manual` or
`--auto-approve-rebase`.

The app shows the same on the page of the watch. A proposal that waits
has its own section, with the commits, the changed files, the plain
unified diff of the file you pick, and each reply beside the comment it
answers, in a box you can edit. **Approve** (**Approve and post** when
the proposal holds only replies), **Approve and stop asking**, **Reject
proposal** and **Reject push** are under it; a drop, a rejected push, a
rejection, **Approve and stop asking** and a switch to auto each ask
first. The
**Watch settings** button at the far right of the header, after
**Merge**, opens a panel docked on the right with the copy of the
defaults of that watch: the approval mode, **Approve a clean rebase or merge on
its own**, the approvals before ready to merge and the merge method.
Each one saves when you change it, and an empty approvals field reads
the rule of the base branch again. The dialog that starts a watch sets
all of them. A switch to auto from the panel approves the proposal as
you left it on the screen, with the replies you edited and without the
ones you dropped. A self watch shows a fixed `auto` badge there. The
list marks the watch **approval needed**. A failed release has
**Retry the push**, or **Retry** when it holds only replies, and
**Reject proposal** in the section, and **Retry** in its row of the
timeline. After each decision, a line under the header says
what went out, until the next proposal asks.

The daemon knows what the agent does. The agent reports each turn
through its hooks, and `watch list`, `watch status` and the app show
the state: `none` before the first session (`no session` in
`watch status`), `starting`, `idle`, `active`, `waiting_input` when the
agent asked you a question, `blocked` when it waits on a permission
decision, `exited` when its process ended. A message about the pull
request waits while the agent needs you, so it never answers in your
place. A session that exited starts again with the next message, on the
same conversation, so nothing it learned is lost. A daemon that
restarts starts the session of every watch at once, on the same
conversation. Claude Code takes its hooks on the
command line. Copilot CLI loads them from a plugin in
`<data dir>/agent-plugins/<watch id>`, which the daemon writes before the
start with `--plugin-dir`. The plugin is outside the worktree, so the
agent cannot change its hooks.

`watch output` prints the last lines the agent printed, as its terminal
drew them. `watch send` types a message into the session: an answer to
a question, or something the pull request did not ask for. It is
refused while the agent waits on a permission decision, where Enter
would decide for you, while a proposal waits on you, while the session
is with you after a takeover, and on a watch that stopped or whose
provider is not available. The app shows the same terminal and the same
message box on the page of the watch.

`watch view` prints the pull request from the last snapshot of the
daemon: state, branches, mergeability, reviewers, labels, assignees,
milestone, auto-merge, size, checks and the description. It makes no
call to GitHub and marks no review item as seen. After a restart of the
daemon, it answers only after the first poll of the watch. `watch diff`
prints the diff of the pull request as GitHub serves it, the same diff
as `gh pr diff`: from where the head left the base to the pushed head.
Commits that are not pushed yet are not in it. The daemon reads it
through its cache of conditional requests. The ETag of a diff is a hash
of the diff, so comments and reviews do not change it: a diff that did
not change gets a 304 and costs no API budget. The cache keeps a diff of
up to 4 MiB; a larger diff costs one call each time. GitHub serves no diff
for a very large pull request; `watch diff` then says so, and `git diff`
in the checkout is the way to read it. The agent reads the pull request
with these two commands when its session starts.

### Take the session over

When the agent is stuck, continue its conversation in your own
terminal:

```sh
babysitter watch takeover acme/api#341
```

The takeover ends the session of the daemon and declines every proposal
that waits, so nothing of that work goes out. Then the command prints
the worktree, the work branch and the push command, and runs the agent
of the watch in the worktree on the same conversation. That session is
yours: it has no tool rules, no system prompt of babysitter, no hooks
and no `pre-push`, so it can push and run any tool. Claude Code starts
it in `manual` permission mode, so it asks you before a tool instead of
the `dontAsk` of the daemon that the conversation would keep. Push with
`git push origin HEAD:<head branch>`: a plain `git push` fails, because
the work branch has another name. `--shell` runs your shell in the
worktree instead of the agent. A watch with no conversation yet starts
a new one, and the command says so. When GitHub is updating the branch,
the takeover goes through and the command says that your worktree is
still on the old head, so fetch before you push. The takeover of a self watch, of a
stopped watch or of a watch that is already taken over is refused, and
so is the takeover of a watch whose provider the daemon does not have,
unless you pass `--shell`.

While the session is with you, the daemon keeps polling and records
activity, but it types nothing into a session: new review comments and
failed checks wait. `watch send` is refused, a hook of your session
changes nothing, and the watch has the blocker `the session is with
you`, so it is not ready to merge and asks for no re-review. `watch
status` shows `with you since <time>`, and `watch list` shows `with you`
in the AGENT column. A stop for any reason keeps the worktree and its
branch.

Give the session back when you are done:

```sh
babysitter watch handback acme/api#341
```

The hand-back is refused while the agent you started with `takeover`
still runs, because two processes on one conversation corrupt it; quit
it first, or pass `--force`. When the work branch has commits that the
pull request does not have, or the worktree has changes that are not
committed, the command lists them and asks first; `--yes` skips the
question. The daemon pushes those commits with the work of the next
turn. On a Dependabot watch the daemon never pushes, so the hand-back
refuses such commits until you push them. After the hand-back, the agent
continues the same conversation: a first message tells it what you did,
and the next one carries the activity that waited. In the app,
**Continue in terminal** opens a dialog with the takeover command and a
copy button, and a watch that is taken over has a **Hand back** button.
The app has no `--force`: it tells you when the agent of the takeover
still runs.

A watch started with `--provider self` has no agent session in the
daemon and no worktree: the coding agent session that starts it is its
agent, and it works in your checkout, which has to be on the pull
request branch. The `babysit-pr` skill of the plugin in
`plugins/babysitter` drives that from Claude Code or Copilot CLI, which
reads it through the `.agents/skills` symlink. The daemon polls and records activity
as for any watch; the agent takes each message with
`babysitter watch next <watch>`, which takes a fresh look at the pull
request, answers with the actionable activity the agent was not told
about yet as one message, with the log of each failed job, and records
it as a `nudged` row so nothing is handed out twice. With `--wait`, the
command waits up to that long, ten minutes at most, for something to
happen when there is nothing to do, and returns early when the watch
stops or the pull request is ready to merge. One caller at a time: a
second `watch next` on the same watch is refused while the first waits,
because asking for a message says the agent stopped working on the last
one, and a second caller would say it about a message the first still
holds. `watch send` and
`watch output` have nothing to talk to on such a watch; `watch reply`
and `watch stop` work as for any other. That agent pushes its own
commits, and its reply goes out at once: there is no turn to wait for.

Such a watch counts its agent as working from the moment it takes a
message with `watch next` until it asks for the next one, and at most
thirty minutes. While it works, `watch merge` refuses with `the agent
is still working`, and `watch status` shows that blocker, because the
agent can still push a fix for the message it holds. The next
`watch next` clears it. A daemon that starts again counts the agent as
working until it asks for a message, still thirty minutes at most from
the last message it took, and hands out again any message it
recorded but never answered with.

The watch stops when you say so, when the pull request is merged or
closed, or when the token loses access. It ends the agent session and
prints a summary: state, head, checks, and how many messages the agent
got. The stop then deletes the worktree and its `babysitter/` branch,
unless **Keep the worktree when a watch stops** is on. When the branch
cannot be deleted, it stays and the summary says so. Your own checkout
stays as it is. A proposal that did not go out is declined: its commits
go with the worktree. Stop with `--keep-worktree`, or with the box in
the app, which starts from the setting, to leave the worktree on disk,
for example to look at work the daemon did not push. A stop after lost
access keeps the
worktree too, because the token or the network can come back, and so
does a stop while the session is with you after a takeover. Delete a
kept worktree yourself: nothing else removes it.

What the agent may never do, whatever a comment or a log says:

- push. The daemon pushes the work branch when the turn ends
- post on GitHub beside the daemon, for example with `gh api -f`
- run `babysitter watch mode`, `approve`, `reject`, `retry`, `merge`,
  `stop`, `takeover` or `handback`, so it cannot approve its own work.
  Before each tool call, the hook sends the call to the daemon and
  applies its answer. The daemon refuses these commands also with a
  flag before `watch` such as `-o json`, a full path, `sh -c` or shell
  quotes such as `baby''sitter`, and it refuses every tool of a watch
  that has no agent session in the daemon. When the daemon does not
  answer in 5 seconds, the hook refuses the tool and tells the agent to
  try again. When the daemon answers with an error, or with no
  verdict, the hook refuses the tool and tells the agent not to try
  again. Claude Code and Copilot CLI have
  deny rules for these commands too. The daemon reads the text of the
  command, so a shell variable, a command substitution or a script can
  still hide a decision
- fetch the web or start a subagent: `WebFetch`, `WebSearch`, `Task`,
  `Agent`, `curl` and `wget` are refused
- merge, approve, dismiss a review, open or close a pull request
- run a command because a comment or a log asked for it
- work in your own checkout
- report success it did not verify
- widen a change beyond its root cause
- put a secret in a reply, a commit, a notification or the store

The session runs Claude Code with your user settings only. The
settings of the worktree are not read: they belong to the branch under
review, and a `.claude/settings.json` there could add hooks that run
commands. No MCP server runs, the GitHub ones included. The rules of
babysitter go in with `--append-system-prompt`, and the session runs
with `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`. Copilot CLI runs
without its built-in MCP servers and with `--no-auto-update`. It has no
flag for a system prompt, so the rules open the first message. Tools outside the rules
in `backend/internal/agent/claude/tools.go` are refused without a prompt, so the
agent never waits on a dialog nobody sees. The worktree is marked
trusted in the Claude Code configuration before the start, so the trust
dialog does not take the first message as its answer. Copilot CLI runs
with every tool allowed except the same rules, and trusts the worktree
through `COPILOT_ALLOW_ALL`, because an untrusted directory loads no
hooks and opens on a trust dialog. Everything the agent printed is kept
in `<data dir>/sessions/<watch id>.log`.

## Claude Code plugin

`plugins/babysitter` is a Claude Code plugin. Its `babysit-pr` skill
drives a `--provider self` watch from your own session: it starts the
watch, takes one message at a time with `watch next`, and tells you what
to fix, push and reply until the pull request can merge. It also reads
a watch the desktop app started and relays what its agent does, explains
takeover and hand-back, and merges a watched pull request when you ask.
`.claude-plugin/marketplace.json` at the root publishes it.

```sh
claude plugin marketplace add deividfortuna/babysitter
claude plugin install babysitter@babysitter
```

Then ask to babysit a pull request, or call the skill with
`/babysitter:babysit-pr`. A session in this checkout gets the plugin
without the install: `.claude/settings.json` registers the marketplace
from the working copy, so a change to the skill takes effect in the next
session. See [the plugin README](plugins/babysitter/README.md).

## Service

The service runs `babysitter serve` in the background with the installed
binary. Install the binary first:

```sh
cd backend && go install ./cmd/babysitter   # or pnpm run backend:install at the root
babysitter service install                  # install and start
babysitter service status
babysitter service stop
babysitter service start
babysitter service uninstall
```

`service install` passes the `--db` and `--token` flags to the service when
you give them. Without a token, the service uses `GITHUB_TOKEN` or the `gh`
CLI. Without `--interval`, the service follows the poll interval and the
longest check read interval of the settings, so the Polling page of the
app and `babysitter settings set --poll-interval` and
`--check-max-interval` reach it; with `--interval` it polls at that rate
for as long as it stays installed, and takes the longest check read
interval of the settings when it starts.

`service install` takes an interval between 10 seconds and 24 hours, the
bound of the settings, because the definition it writes holds that value
until somebody installs the service again. A definition of an earlier
install that holds a value outside the bound keeps its service running:
`serve` polls at the nearest rate the bound allows and says so in the
log.

On macOS, the service is a launchd user agent:

- unit: `~/Library/LaunchAgents/com.deividfortuna.babysitter.plist`
- log: `~/Library/Logs/babysitter/babysitter.log`
- The agent needs a logged in GUI session. If `gh` keeps the token in the
  keychain and the agent cannot read it, set `GITHUB_TOKEN` or run
  `gh auth login --insecure-storage`.

On Linux, the service is a systemd user unit:

- unit: `~/.config/systemd/user/babysitter.service`
- log: `journalctl --user -u babysitter`
- To keep the service running after you log out, run
  `loginctl enable-linger $USER`.

If you rebuild the binary in a new location, run `service install` again.

The service supports macOS and Linux only. On other systems, Windows
included, the `service` commands fail.

## Rate limit

Each repository poll costs one request per page of 100 open pull
requests of each repository. The checks of an open pull request cost two
more requests, read only when its head commit is new, when the wait since
the last read has passed, or on a manual sync while its CI is pending.
While CI is pending, the wait starts at one minute, or at the repository
poll interval or the longest check read interval when one is shorter, and
doubles after each read up to the longest check read interval. When CI has finished,
the wait is ten minutes. The time of the last read is
kept in memory, so a process that starts reads the checks of every open
pull request again. A pull request that changed, that is new to the
store, or whose mergeable state is still unknown costs two more, and one
that left the open list costs one more to see how it ended.

Each watch takes a full snapshot of its pull request on every watch
poll, see [Snapshot](#snapshot). A watch where nothing happens polls less
often: its wait doubles after each quiet poll, up to the longest watch
poll interval. A kick, such as the start of a watch or a decision on a
proposal, polls only the watch it concerns. A watch poll also reads the log of each
failed job it hands to the agent, and the rule of the base branch when
the approvals of the watch follow it.

Every GET goes out with the ETag or `Last-Modified` of its last answer.
GitHub answers `304 Not Modified` when nothing changed, and babysitter
serves the body from the cache it keeps in the database.

When fewer than 10 requests remain in the hour, babysitter waits for the
limit to reset. If you watch many active repositories or pull requests,
raise the intervals in the Polling page of the settings, or with
`babysitter settings set --poll-interval`, `--watch-interval`,
`--watch-max-interval` and `--check-max-interval`. `daemon start
--interval`, `--watch-interval`, `--watch-max-interval` and
`--check-max-interval` hold for that run only.

The daemon reads the budget from the headers of each GitHub answer.
`babysitter ratelimit` prints it, and the sidebar of the desktop app
shows it above the account when more than half of the budget is used,
or when the daemon slows down or pauses its polls. "Always show the
GitHub rate limit" in the Appearance page of the settings shows the card
all the time. The card says when the budget is nearly
used, when the polls pause until the reset, and when GitHub asked for a
slow down with its secondary limit. "Poll less often" opens the Polling
page of the settings.

## Logs

The daemon and the desktop app each write a log to the `logs/` folder
of the data directory, one JSON record per line:

- `daemon.log`: what the daemon did, from each poll to each failure.
  Each call of the hook of an agent adds one record with the watch,
  the event, the decision, the tool, the command on one line with
  secrets removed, and the rule that refused the tool. A refused tool
  is an info record; every other call is a debug record.
  A daemon started from a terminal also prints it as text on stderr.
- `app.log`: what the app did to start, attach to and stop the daemon,
  the updates it checked, and the lines the daemon printed that are not
  records, such as a panic.

A file that reaches 5 MiB moves to `daemon.log.1` or `app.log.1`, and
the one before it goes. The daemon keeps its last 2000 records in
memory too, for the API and the app.

The daemon logs at info level. Debug adds the records that only a
developer needs, such as each HTTP request. `daemon start --log-level
debug` starts at that level, and `babysitter daemon log-level debug`
changes the level of a running daemon until it stops.

`babysitter daemon logs` prints the last records: from the running
daemon, or from `daemon.log` when no daemon runs. `-f` follows the
daemon until Ctrl-C, `--level` hides the records below a level, and
`--app` prints `app.log`.

In the desktop app, the Logs page of the settings has the same
view: a live log of the daemon or of the app, a level filter, a text
filter, a copy button, and a button that opens the `logs/` folder. Its
"Debug logs" switch changes the level of the daemon.

The API serves the records at `GET /api/v1/logs`, streams them as
server-sent events at `GET /api/v1/logs/stream`, and reads and changes
the level at `/api/v1/logs/level`.

## Layout

The repository is a Go workspace with the backend module and a pnpm
project for the desktop app. Paths below are relative to `backend/`.

- `cmd/babysitter/main.go`: the entry point
- `cmd/genspec`: generates the OpenAPI document from the Go types
- `internal/cli`: the cobra command tree
- `internal/daemon`: wires the watcher, the API, the run file and the supervisor socket
- `internal/httpd`: the loopback HTTP API
- `internal/httpd/apispec`: the OpenAPI document, `openapi.yaml`, which `go generate` writes with `cmd/genspec` and the API serves
- `internal/events`: the in-process change feed the API streams
- `internal/logbook`: the log of the daemon: the slog handler, the records it keeps in memory, the rotated file and the level
- `internal/runfile`: `running.json`, the handshake the desktop app reads
- `internal/supervisor`: the socket that stops the daemon when the app quits
- `internal/processalive`: checks whether a pid still runs
- `internal/ghclient`: token lookup, the go-github client with its shared conditional-request cache, and `RateGuard` and `RateMeter` for the rate limit
- `internal/ghclient/ghfake`: the fake GitHub of the tests
- `internal/store`: the SQLite schema and queries
- `internal/watcher`: the poll loop and the rules for review and CI state
- `internal/snapshot`: the `pr` snapshot, its target parsing and action rules
- `internal/checks`: reduces check runs, commit statuses and workflow runs to the states the app uses, and trims a failed job log to the lines that matter
- `internal/prwatch`: the watch of one pull request: the poll, the activity diff, the messages to the agent session
- `internal/session`: runs the agent in a pseudo terminal the daemon owns: types messages, keeps the output, reports the exit
- `internal/agent`: the contract of an agent session, its state, the git hooks, the messages the daemon types, and the rules that decide if the agent may run a tool: the parse of a tool call into the programs it runs, rules on the state of the watch, and rules on a command, each with the reason the agent reads; the prompts live in `prompts/`. `agent/claude` and `agent/copilot` build the command line of each CLI as an interactive session, with its hooks and its tool rules
- `internal/worktree`: the git write operations of a watch, in its own worktree
- `internal/gitrepo`: reads the current branch, the remotes and git configuration values with git
- `internal/gitrelease`: the git the daemon runs to release the work of the agent: reads the pull request branch, compares it with the work branch, and pushes with the credential helper of the author
- `internal/service`: launchd and systemd integration
- `internal/execx`: runs external commands, such as the operating system tools of `service` and `notify`, git and the agent CLIs, and reports their exit code
- `internal/notify`: the notification history and where each one is shown, and desktop notifications with terminal-notifier, osascript or notify-send
- `internal/redact`: takes anything that looks like a secret out of the text the daemon writes
- `internal/textx`: the summary text of a comment, a check, an error, a count or a commit id
- `internal/timex`: time helpers shared by the loops of the daemon
- `internal/testutil`: the waits and the test logger that the Go tests share

The Claude Code plugin lives in `plugins/babysitter`, with the
`babysit-pr` skill and its references under `skills/`; `.agents/skills`
is a symlink to that directory. The desktop app
lives in `frontend/`: `src/main.ts` and `src/main/` are the
Electron main process, `src/preload.ts` the bridge, `src/renderer/` the
React app, and `src/api/schema.ts` the generated API types. `codegen/`
holds openapi-typescript, which writes `src/api/schema.ts`. See
[docs/architecture.md](docs/architecture.md).

## Tests

At the root, one command runs the same checks as CI: golangci-lint,
`go test`, tsc, `vp fmt`, `vp lint`, `vp test`, actionlint and zizmor:

```sh
pnpm run check
```

Or one part at a time:

```sh
pnpm run backend:lint
pnpm run backend:test
pnpm run frontend:typecheck
pnpm run frontend:format:check
pnpm run frontend:lint
pnpm run frontend:test
pnpm run actions:lint
```

CI (`.github/workflows/pr.yaml`) also runs the Go tests with `-race` and
coverage, the Vitest coverage thresholds of `frontend/vite.config.ts`,
govulncheck, and a check of the GoReleaser configuration. It fails when
`go mod tidy`, `go generate` for the OpenAPI document, or `pnpm run api:ts`
change a file.

## Release

`.github/workflows/release.yaml` releases the CLI, the desktop app and
the Homebrew cask on three channels:

| Channel | How it starts | What it publishes |
| --- | --- | --- |
| Nightly | A schedule runs every 30 minutes. It releases only when `main` has new commits and 6 hours have passed since the last nightly. You can also start it by hand with `channel=nightly` from `main` | A GitHub prerelease `vX.Y.Z-nightly.YYYYMMDD.<run>` with the `nightly-mac.yml` update feed |
| Stable | Start it by hand with `channel=stable` from `main`, or push a `vX.Y.Z` tag on a commit of `main`. The version must be newer than the latest stable release | The GitHub "latest" release with the `latest-mac.yml` update feed, and the Homebrew cask |
| Preview | Start it by hand with `channel=preview` from any branch. This is the default input | A test build for maintainers, `vX.Y.Z-preview.YYYYMMDD.<run>`. It has no update feed and no cask, and its notes tell users not to install it |

A stable release that you start by hand ships the commit of the latest
nightly, so stable only gets a build that nightly users already ran.
Its version is the one that nightly previews, or the `version` input.

The workflow has these jobs:

1. `resolve` runs `frontend/scripts/resolve-release.mjs`. It selects the
   channel, the commit and the version, and for a scheduled run it
   decides if a nightly is due.
2. `cli` runs [GoReleaser](https://goreleaser.com) inside the
   `goreleaser-cross` image, because the SQLite driver needs a C compiler
   for each target. GoReleaser runs `go mod tidy` and `go test` first,
   then makes archives for Linux and macOS on amd64 and arm64, and a
   checksum file.
3. `desktop` makes the app on a macOS runner of each arch, because cgo
   does not cross compile there. It sets the version of the app and of
   the daemon, checks the arch and the version of the daemon, and keeps
   the DMG and the zip, each under its versioned name and as
   `babysitter-darwin-<arch>`.
4. `publish` writes the update feed of the channel and makes the GitHub
   release with all the files. A release that fails before this job
   leaves no release.
5. `homebrew` writes `Casks/babysitter.rb` in the tap after a stable
   release. It runs only when the repository variable `HOMEBREW_TAP` is
   set.

```sh
git tag v0.1.0
git push origin v0.1.0
```

The version of a nightly or a preview is the next patch after the latest
stable release. To release a minor or a major version, set it in
`frontend/package.json` in a pull request. When that version is higher,
the nightlies use it. The workflow does not change `main` after a
release.

The secrets of the release and the tap are in
[docs/release.md](docs/release.md).
