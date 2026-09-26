# Architecture

babysitter has two processes: a Go daemon and an Electron desktop app. The
CLI is the same binary as the daemon.

```text
babysitter/
  backend/    Go module: CLI, daemon, HTTP API, SQLite store, GitHub watcher
  frontend/   Electron app: main process, preload bridge, React renderer
  docs/       this file and other notes
```

## Backend

- `cmd/babysitter` is the entry point. `internal/cli` holds the cobra
  commands, including `daemon start`, `daemon stop` and `daemon status`.
- `internal/daemon` wires the long lived process: it opens the store,
  starts the watcher, binds the HTTP API on `127.0.0.1`, writes
  `running.json` and opens the supervisor socket.
- `internal/httpd` is the API. `router.go` lists the routes, `dto.go` is
  the wire contract, `events.go` streams the change feed.
- `internal/httpd/apispec` embeds `openapi.yaml`. `cmd/genspec` generates
  it from the Go types; run `go generate ./...` in `backend` after a change
  to a route or a DTO.
- `internal/events` is the in-process change feed. The store publishes an
  event after each write.
- `internal/runfile` reads and writes `running.json`.
- `internal/supervisor` is the socket that ties the daemon to the app.
- `internal/store`, `internal/watcher`, `internal/ghclient`,
  `internal/snapshot` and `internal/notify` are the original CLI packages.
- `internal/notify` also holds the notification center: `Center.Post`
  writes the row through the store and shows it, `Center.Present` says a
  client shows them instead. See **Notifications** below.
- `internal/prwatch` watches one pull request: it polls with the snapshot
  collector, turns the differences into activity rows, and types the
  actionable ones into the agent session of the watch. `internal/session`
  runs the agent in a pseudo terminal owned by the daemon: it types
  messages, keeps the recent output, and reports the exit.
  `internal/agent` holds the contract of an agent session, its state,
  the embedded prompts, and the messages the daemon types; `agent/claude`
  and `agent/copilot` build the command line of each CLI as an
  interactive session and the hooks that report what the agent does.
  `internal/worktree` makes and removes the worktree of the author's
  checkout the agent works in. `checks.TrimLog` cuts a raw GitHub
  Actions job log down to the step that failed and the lines around the
  error: no timestamps, no escape sequences, no steps that passed.
  `ghclient.RateGuard` is the rate limit handling both loops share.
  `ghclient.SharedRates` is the meter under the cache of every client:
  it reads the core budget from the headers of each answer, a 304
  included, and the secondary limit from a 403 or 429. `GET /ratelimit`
  answers from it, with the floor of the guard to say when the polls
  pause.
  Every client `ghclient.New` builds reads through one cache of the
  package: a GET carries the validator of the last answer, and GitHub
  answers 304 without counting the call against the rate limit.
  `ghclient.KeepResponses` puts the SQLite table `github_responses`
  behind that cache: the daemon and each CLI command that opens the
  database call it, so a process that starts again sends the
  validators of the last one. The table keeps the 4096 answers written
  last, and 64 MiB of bodies at most. The repository watcher reads the checks of a pull request when its head
  moves, while a check still runs, and once the TTL of the stored status
  runs out, not on every pass.

## Frontend

- `src/main.ts` is the Electron main process. It creates the window,
  owns the daemon through `src/main/daemon-supervisor.ts`, and updates
  the app through `src/main/app-updater.ts`.
- `src/preload.ts` exposes `window.babysitter`, the only bridge between the
  renderer and the main process: daemon status, a daemon restart, the app
  version, the system folder picker for the checkout of a watch, the
  notifications the app shows, and the updates of the app.
- `src/renderer` is the React app. `lib/api-client.ts` is the typed HTTP
  client, `lib/event-transport.ts` follows the change feed, `hooks` wrap
  the API in TanStack Query. The approval screen is
  `components/proposal-panel.tsx` with its dialogs in
  `components/proposal-dialogs.tsx`, over `hooks/useProposals.ts`. Its
  queries live under the `watches` key, so the `watch_proposal` and
  `watch_changed` events refetch them with the rest of a watch.
  `components/proposal-decision.tsx` holds the decision of the author
  for the page of one watch: the replies they edited or dropped, the
  requests, and the line that says what the last decision did. The
  panel and the approval mode of `components/watch-settings-panel.tsx`
  share it, so a switch to auto sends what the author left on the
  screen. A release writes to
  the store while the request is still open, and the refetch that
  follows takes the panel away before the answer; the line lives above
  the panel for that reason, and it goes when the next proposal asks.
- `src/api/schema.ts` is generated from `openapi.yaml` with
  openapi-typescript. Run `npm run api` at the repository root to refresh
  both files. The generator lives in `codegen/` at the repository root,
  with its own lockfile: it uses the compiler API of TypeScript 5, and
  the frontend type checks with TypeScript 7, which does not have that
  API.
- `scripts/build-daemon.mjs` compiles the Go daemon into `frontend/daemon`.
  The `start`, `package` and `make` scripts run it first, and Forge ships
  the directory as an extra resource.

## How the processes talk

1. **Process lifecycle.** The main process reads `running.json` in the
   data directory. When it records a live daemon that answers `/healthz`,
   the app attaches to it. Otherwise the app spawns
   `babysitter daemon start --owner app` and waits for the file. The data
   directory is `BABYSITTER_DATA_DIR` or the user config directory plus
   `babysitter`, the same directory as the database.
2. **Supervisor link.** For a daemon it owns, the app holds one connection
   to the Unix socket named in `running.json`. Nothing is written on it.
   When the app exits, the socket closes, and the daemon stops after a
   five second grace period. A daemon started from the terminal has owner
   `cli` and is never linked, so it survives the app.
3. **REST over loopback.** The renderer calls the daemon directly at
   `http://127.0.0.1:<port>/api/v1` with openapi-fetch. The main process
   only tells it the port. The API has no authentication; the CORS
   middleware rejects every origin that is not loopback, so other local
   web pages cannot reach it.
4. **Change feed over SSE.** `GET /events` streams one frame per store
   write, tagged with the event type. The renderer treats each frame as a
   reason to refetch through the TanStack Query cache. The feed is live
   only; a client fetches a fresh snapshot after it connects.
5. **Electron IPC.** The preload bridge carries daemon status changes from
   the main process to the renderer, the notifications the renderer
   asks the main process to show, and the status and the settings of
   the updates of the app. Everything else goes over HTTP.

## Updates of the app

`src/main/app-updater.ts` wraps the `autoUpdater` of `electron-updater`.
It works only in a packaged macOS app that has
`Contents/Resources/app-update.yml`, which only a Developer ID build has.
In any other build the status is `unsupported`, and nothing checks.
`docs/release.md` tells how a release makes the feed.

The states are `idle`, `checking`, `available`, `not-available`,
`downloading`, `downloaded`, `installing` and `error`.

- The first check is 10 seconds after the start, then one each hour.
- A check that the app makes on its own and that fails goes to the log
  only. The status does not change. A check from **Check for updates**
  that fails shows the reason.
- Only one check runs at a time. A check that starts while another runs
  waits for its answer. A check that ends after a download started
  does not change the status.
- A download from **Download** that fails goes back to `available` with
  the reason, so the card shows it and offers **Download** again.
- The download is automatic unless the user turns it off. Then the
  sidebar card offers **Download**.
- The status becomes `downloaded` only when the `autoUpdater` of
  Electron says that Squirrel.Mac has the update. `electron-updater`
  says it earlier, when Squirrel has not staged the update yet.
- `allowDowngrade` is always off, because the store refuses a database
  from a newer schema. The code never sets `channel`: its setter turns
  downgrades on. The channel only sets `allowPrerelease`.
- The choices of the user live in `update-settings.json` in the data
  directory. With no saved channel, a prerelease build follows
  prereleases and a stable build follows stable releases.

**Restart to update** does these steps in this order:

1. The supervisor stops the daemon that the app owns, spawned or
   attached, and waits until `/healthz` stops answering, for 10 seconds
   at most. The shutdown request stays inside that time. If it did not
   wait, the new app could attach to the old daemon during its grace
   period and cancel its stop.
2. `quitAndInstall` closes the windows, and Squirrel starts ShipIt.
3. `before-quit` runs as for any quit. It calls `app.exit(0)` after 15
   seconds at most, so a slow stop cannot keep ShipIt waiting.

When the app has not quit 30 seconds after the click, the status goes
back to `downloaded` with a message, and the app starts the daemon
again.

A daemon with owner `cli`, from the terminal or from `service install`,
keeps the old binary until it restarts.

## Notifications

One row of the `notifications` table is one thing the user is told
about. `notify.Center` is the only way in: `prwatch` posts what the
watch reported (`prwatch/notify.go` maps an activity kind to a
notification kind and drops the quiet ones), and `POST /notifications`
posts what an agent sends with `babysitter notify`. The Center writes
the row through the store, which publishes `notification_added`, and
then shows it. A `notify` that the daemon does not take, whatever the
reason, falls through to the operating system and says why on stderr:
the command exists so the user hears about it, and a history row nobody
wrote is no answer to that. The post to the daemon takes half of the
`--timeout` budget, so a daemon that holds the connection and never
answers still leaves the other half for the tool that can reach the
user. A daemon that ran out of that half may still hold the row, and the
app may have drawn the banner from it already, so the command first
reads the newest row of the history, in half of what is left, and takes
it as recorded when it carries the title and the message it sent since
it started.

Who shows it is one question: the daemon and the app must never show
the same notification twice. A client that shows them itself opens the
change feed with `GET /events?present=1`, and the Center counts such a
stream as a presenter for as long as it is open. While there is one,
`Center.show` does nothing and the desktop app shows the notification
through Electron, as the app and with a click that brings the window up
on the watch. With no presenter, the Center hands the notification to
the operating system itself (`terminal-notifier`, `osascript` or
`notify-send`), which is what a daemon in a terminal does.

The app only claims the screen where the platform can draw a banner, and
only the main process knows: `useNotificationsPresent` answers `null`
until that round trip lands, and `App` opens no feed while it does. A
feed opened before the answer would leave the daemon presenting for a
moment, and a row that arrived there would be shown by the daemon and
again by the app when the feed came back with the claim. A probe that
fails answers `false`, because no answer at all would hold the feed
closed for good. The app that does not present shows nothing of its own
either, whatever the query caches of the other panes hold; its mark
still follows the rows, so a row the daemon showed is never shown twice.

The claim lives as long as the stream does, so a stream that drops hands
the showing back to the daemon until it is open again. The `ready` frame
carries `lastNotificationId`, the newest row the daemon held as it took
the claim. `Center.Present` reads that row and takes the claim under one
lock, which every post holds for reading, so a row lands either before
the mark, where the daemon draws it, or after the claim, where the client
draws it, and never between the two. The claim goes back without the
lock, and before the posts inside the handover draw: `handleEvents`
releases from a defer, so its write loop has already returned and a row
that lands there reaches no client, while the next stream marks it as
history. The daemon draws those rows, which shows a row the stream did
carry a second time. That is the side to err on. A daemon that cannot
read its own history takes no claim and keeps showing the notifications;
the frame then carries `lastNotificationId` as null, which tells the app
to show none of its own. `App` hands that id to `useNativeNotifications`
as the mark, and null while no feed carries the claim. The rows at or
below the mark are history: the daemon, or the app before its feed
dropped, showed them. The rows above it landed under the claim, so the
app shows them, whether they arrive in the first read after the connect
or in a later one. Without the mark a sleep and wake, or a
restart of the daemon from the app, shows every row of the gap a second
time; without the id a row that lands between the claim and the first
read is shown by nobody.

The three settings only govern the screen: `notificationsEnabled` off
stops the showing, `notificationSound` off silences it, and a kind in
`mutedNotificationKinds` reaches nobody; the row goes to the history
either way. The store keeps the muted kinds as a comma separated list,
and a kind the list leaves out is shown, so a kind the daemon gains
later arrives on. Whoever shows the notification keeps the settings:
the Center for the daemon through `Settings.ShowsNotification`,
`useNativeNotifications` for the app, which reads the same settings and
sends `silent` with the row. The `silent` column of the row is the other
half of it: `babysitter notify --silent` asks for no sound whoever draws
the banner, and the app only learns of it from the row, because the
Center shows nothing while the app presents. Settings the app has not read yet, or
cannot read at all, hold every notification back: a guess would show a
kind the user turned off. The renderer only shows a row that arrives
while it runs: the first read of the history sets its mark, so opening
the app replays nothing. A switch in the Notifications pane changes the
one kind it names and keeps the rest of the list, so an app older than
the daemon does not un-mute what it cannot name; the store puts the
list back in the order of the schema when it saves it. The kinds of the
renderer come from `schema.ts`, which the `items.enum` tag of the DTO
fills, so a kind added in Go and left out of `KIND_COPY` fails the
build.
A row of a watch goes with the watch, on the `ON DELETE CASCADE` of
`notifications.watch_id`. Nothing else deletes one, so the history has a
ceiling of its own: `AddNotification` keeps the newest
`store.DefaultNotificationCap` rows and drops what falls out of the
bottom. A watch that runs for months, one that stopped without being
removed and a row of `notify` that belongs to no watch would otherwise
grow the table for the life of the store. The trim runs after the
insert, and a trim that fails is logged and nothing more: the row was
recorded, so the caller hears that it was and the change feed carries
it.

The subtitle of a notification is the second line of the banner the
operating system draws. A row that names a pull request carries it
beside the row already, as `repo` and `number`, so `notify.body` leaves
it out of the text; a row of no pull request, from `babysitter notify
--subtitle`, has nowhere else for it and takes the whole sentence as its
body.

`shared/notifications.ts` holds the rules of how loud a notification is,
as pure functions the main process calls:

- A focused window shows nothing at all. The screen already says it.
- `shouldToast` takes no list of kinds, so a kind added to the daemon
  cannot lose its toast because a list here was not updated.
- `presentation` answers the banner, the bounce and the flash together.
  The bounce and the flash outlive the banner, so they only ever run
  beside one: a platform where `Notification.isSupported()` is false
  draws nothing and signals nothing, instead of bouncing a dock for a
  banner that was never there.
- `bounceType` bounces the dock of macOS `critical` for the `agent`
  kind, which keeps bouncing until the app comes up, and once for the
  rest. `shouldReplaceBounce` keeps a critical bounce from being
  downgraded by a later informational one, and
  `app.on("browser-window-focus")` cancels whichever runs.
- `shouldSignalAttention` flashes the taskbar of Windows and Linux for
  the `agent` and `merge` kinds only.
- `isKindMuted` answers whether the user turned a kind off, and
  `NOTIFICATION_KINDS` is the order the settings pane lists them in.
- `clickTarget` says where a click lands: the watch of the row in the
  app, the pull request in the browser for a row of no watch, and
  otherwise the app alone. `clickPlan` puts that beside the row the
  renderer marks read, because the user read the row wherever the click
  lands, and `useNativeNotifications` does the mark. A click that raises
  goes through `showWindow()`, so a banner the user clicks after closing
  the last window brings the app back: macOS keeps the app alive with no
  window, and keeps the banner for hours. A click that only opens the
  browser raises nothing, so it reaches the renderer of a window that
  stands, or nobody; the queue would otherwise hold it for the window
  that comes up next, for a reason of its own.

The menu bar item of `main.ts` hangs off the same count. The renderer
reports it through `notifications:setBadge` after each read of the
history, and `setUnread` puts it on the dock, on the taskbar badge and
on the item at once. Its menu opens the app, asks the renderer for the
notification screen over `notifications:open`, and marks everything read
by calling `POST /notifications/read` on the daemon itself, so the menu
works while no window is open to do it.

The screen the menu asks for waits for a renderer that listens. A window
the click makes again loads its bundle first, so `openQueue`
(`main/pending-open.ts`) holds the channel, and what travels with it,
until the preload says `notifications:open-ready`, which it sends when
the renderer subscribes to `notifications:open`. The click of a banner
takes the same road, with the row it names as its payload. A window that
closes drops the sender and what it was asked for: the next window comes
up for a reason of its own, and that reason decides what it shows.

## Settings

One row in the `settings` table of the store holds the preferences of the
daemon. `GET /settings` reads it and `PUT /settings` writes the whole
document, so the desktop app reads the settings, edits them and sends
them back. A field the body leaves out is a bad request, as the schema
says, with one exception: `notificationsEnabled`, `notificationSound`
and `mutedNotificationKinds` are pointers on `settingsBody`, and an
absent one keeps what the daemon holds. Those three are the ones an app
older than the daemon never names, and the first two hold true, which a
zero value would have taken away in silence.

The settings hold two poll intervals and the defaults of a watch. A write
that the daemon accepts takes effect at once: `httpd` hands the saved
settings to `daemon.Run`, which calls `watcher.SetInterval` and
`prwatch.Service.SetInterval`. Both loops keep the interval behind a lock
and reset their ticker on the next turn, so nothing restarts and no poll
is forced.

The defaults belong to `prwatch`, not to the API: `Service.withDefaults`
fills `includeExisting`, `includeOwn`, `approvalsRequired`,
`mergeMethod`, `approvalMode` and `autoApproveRebase` on the way into
`Start`, and `Service.keepWorktree` answers
for every stop, including the ones the daemon makes itself after a merge,
a close or a lost token. `httpd` only turns the JSON into a request, so a
field the body leaves out stays nil and a field it carries always wins.
That is why the CLI sends a flag only when the user typed it.

Two of those fields need a value that says "not the setting": the empty
merge method, which is the first method the repository allows, and the
rule of the base branch. The merge method carries it as `*string`, where
`&""` is a choice and nil is silence. The approvals need three states, and
Go decodes an absent field and a JSON `null` into the same nil `*int`, so
`httpd.Optional[int]` carries the presence beside the value: absent takes
the setting, `null` asks for the rule of the base branch, and a number is
that number. `prwatch.Approvals` is the same shape inside the service.
The settings document needs none of it, because a `PUT` carries every
field: there `null` is the rule of the base branch.

`babysitter daemon start --interval` and `--watch-interval` hold for that
run only: `daemon.runningSettings` writes them over the stored settings
for the loops and stores nothing, so a daemon started by hand at another
rate leaves the preferences of the app alone. The stored value is what
`GET /settings` answers and what the app shows, so while such a daemon
runs the panel and the loop differ; the log line at start says so. A save
from the app wins over the flag, because `ApplySettings` retunes the
loops. A value outside the bounds (10 seconds to 24 hours) stops the
start.

`babysitter serve` resolves its interval the same way: without the flag
it reads the stored setting at the start and follows it from there, so a
babysitter installed as a service follows the Watching pane without a
restart. The pane writes that row from another process, so there is no
event to wait on: `cli.followInterval` reads it beside the loop and calls
`watcher.SetInterval` when it moved, never rarer than the shortest
interval the settings allow. With the flag, `serve` is pinned to it.
`service install` writes `--interval` into the service definition only
when the user typed it.

`babysitter settings get` and `babysitter settings set` are the same two
routes from the terminal. `set` reads the document, writes over the flags
the user typed and puts it back, so it needs no partial body.

The approval mode is the one setting whose default depends on the
history of the database. Migration 22 adds it with `auto`, so an install
that upgrades keeps what it did, and `store.migrate` reads
`PRAGMA user_version` before the chain: a database that had no schema
takes the fresh seed, `manual`, in the transaction of its last
migration. A watch copies the mode at the start, and
`POST /watches/{id}/approval` changes the copy while it runs. A self
watch always records `auto`.

The approvals and the merge method are copies too.
`PATCH /watches/{id}` changes them while the watch runs, with the same
three states for the approvals as the start: absent keeps the copy,
`null` reads the rule of the base branch again, and a number is that
number. They are the only fields of a watch that can change: the others
come from GitHub or from the start, and a body that names one gets a 400
with the code `field_not_changeable`. The approval mode stays on its own
route, because a switch to auto can release a proposal.
`prwatch.Service.SetMergeRules` writes the row and, when the count
changed, kicks a pass: the blockers of the watch are from the last poll
and only a new snapshot can judge the new count.

## Daemon endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/healthz`, `/readyz` | Liveness, version, pid |
| GET | `/api/v1/openapi.yaml` | The embedded contract |
| GET | `/api/v1/events` | Change feed as server-sent events; `?present=1` takes the showing of the notifications over from the daemon |
| GET, PUT | `/api/v1/settings` | The settings of the daemon; a field the PUT leaves out keeps its value |
| GET, POST | `/api/v1/repos` | List or watch repositories |
| DELETE | `/api/v1/repos/{id}` | Stop watching a repository |
| GET | `/api/v1/prs` | Stored pull requests, `?repo=` and `?state=` |
| GET, POST | `/api/v1/notifications` | The notification history, `?status=unread` and `?limit=`, or record one |
| POST | `/api/v1/notifications/read` | Mark notifications as seen; no ids marks every unread one |
| POST | `/api/v1/sync` | Ask the watcher for a pass now |
| GET | `/api/v1/providers` | The AI providers and the models they offer |
| GET | `/api/v1/viewer` | The GitHub account the token belongs to, held for half an hour |
| GET | `/api/v1/ratelimit` | The core GitHub budget the token has left, and whether the polls wait for a reset or a retry |
| GET, POST | `/api/v1/watches` | List the watched pull requests, or start one |
| GET | `/api/v1/watches/{id}` | One watch |
| PATCH | `/api/v1/watches/{id}` | Change the approvals and the merge method of a running watch; `null` approvals read the rule of the base branch again, and any other field is refused |
| POST | `/api/v1/watches/{id}/stop` | Stop a watch, returns its summary |
| POST | `/api/v1/watches/{id}/merge` | Merge the pull request of a watch when it is ready, returns the stopped watch |
| POST | `/api/v1/watches/{id}/poll` | Poll a watch now |
| GET | `/api/v1/watches/{id}/activity` | The activity of a watch, `?since=` and `?limit=` |
| POST | `/api/v1/watches/{id}/send` | Type a message from the author into the agent session |
| POST | `/api/v1/watches/{id}/takeover` | Move the agent session to the terminal of the author: end the session of the daemon, decline the proposals that wait, and return the command that continues the conversation |
| POST | `/api/v1/watches/{id}/handback` | Give the session back to the daemon; a 409 `unconfirmed_work` lists the commits and the changes that are not on the pull request until `confirm` |
| POST | `/api/v1/watches/{id}/next` | Hand the agent of a self watch its next message, `?wait=` for a long poll |
| POST | `/api/v1/watches/{id}/reply` | Record a reply of the agent in the proposal of its turn; a self watch posts it at once |
| GET | `/api/v1/watches/{id}/proposals` | The proposals of a watch, newest first, with their replies |
| GET | `/api/v1/watches/{id}/proposals/{number}` | One proposal with its commits, files, plain unified diff and replies |
| POST | `/api/v1/watches/{id}/proposals/{number}/approve` | Release a proposal that waits, with the replies the author edited or dropped |
| POST | `/api/v1/watches/{id}/proposals/{number}/reject` | Reject a proposal, with a reason for the agent and an optional discard |
| POST | `/api/v1/watches/{id}/proposals/{number}/retry` | Push and post a failed proposal again, rebased onto a pull request branch that moved |
| POST | `/api/v1/watches/{id}/approval` | Change the approval mode of a running watch; a switch to auto releases what waits only with `release` |
| GET | `/api/v1/watches/{id}/output` | The last lines the agent printed, `?lines=` |
| POST | `/api/v1/watches/{id}/hook` | An event of the agent, reported by its hook command |
| POST | `/api/v1/control/shutdown` | Stop the daemon, rejected for browser origins |

## Watch flow

1. `babysitter watch start` posts the target, the checkout directory,
   and the chosen AI provider with its model. An empty model leaves the
   choice to the command of that provider. The service takes a baseline
   snapshot and checks that the pull request is open, that the checkout
   is the head repository, and that the daemon has a command for the
   provider. Then it does these steps at the same time: it checks that
   the token can push, reads the login of the token, the git identity
   of the checkout and the approvals the base branch requires, and
   fetches the head branch. A failed check stops the fetch. When all the
   steps pass, it makes the worktree, records the watch, and starts
   the agent session: the interactive command of the provider in a
   pseudo terminal, in the worktree, with the standing rules in its
   system prompt and a hook command that reports every turn to the
   daemon. The first message names the pull request and tells the agent
   to read it and wait. A branch that is already behind its base or in
   conflict with it, or a check that already failed, is told at once,
   because a later poll sees no change of state.
2. On each interval the service takes a snapshot, diffs it against the
   stored state (`prwatch.Diff`), inserts each new activity row once
   (unique on watch, kind and ref), reports the rows (log, notification,
   `watch_activity` event), and marks them reported. A row a dead daemon
   recorded but did not report is reported on the next start.
3. The actionable rows the agent was not told about yet go to it as one
   message: the failed checks with the log of each failed job, the
   review comments with their ids, the reviews, the branch behind its
   base or in conflict with it. The daemon reads each log itself, four
   logs at a time, and
   `checks.TrimLog` cuts it down to the failure, so the message carries
   the error instead of an endpoint the agent has to fetch. The daemon
   types the message into the terminal, chunk by chunk, then Enter after
   a pause. The rows are marked told after the send, and a `nudged` row
   keeps the text. `agent.Sanitize` removes from the text of the pull
   request the characters that Claude Code removes from a prompt, and
   writes every line break as `\n`: a prompt that Claude Code changes
   stays in its input box until a second Enter, and the daemon presses
   Enter only once. A message the session cannot take stays untold and
   comes back on the next poll: the agent asked the author a question,
   or waits on a permission decision, and Enter would answer in the
   author's place. A failed check of an earlier head is dropped: nobody
   can fix it.
4. The agent does the work: it edits the worktree, verifies, commits
   with the identity of the checkout, and replies with `babysitter watch
   reply` (`POST /watches/{id}/reply`). It does not push and it does not
   post. A `pre-push` hook the daemon installs refuses every push of the
   session, and the tool rules deny merges, `gh pr comment`, `gh api`
   with a method, a field or an input, the destructive `gh` calls, and
   the commands of babysitter that decide for the author (`watch mode`,
   `approve`, `reject`, `retry`, `merge`, `stop`, `takeover`,
   `handback`), under the bare word,
   under the path the daemon names, and under any line that holds
   `babysitter` before the subcommand, which takes a global flag, another
   path and `sh -c`. A reply whose text names one of these commands is
   refused too, and the agent rewords it.
   The reply is checked against GitHub at once, so a wrong comment id
   reaches the agent while it can fix it, and it waits in the proposal
   of the turn.
5. The work of one turn is one proposal (`prwatch/turn.go`,
   `store/proposals.go`). The agent that goes `active` opens it, and it
   records the head of the pull request branch, where the work branch
   meets it, and where the work branch stands. The agent that goes
   `idle`, or a session that exits, closes it; an idle signal read
   before the last message typed in closes nothing. A turn that leaves
   the work branch where it found it has no push, even when the branch
   holds commits the author kept off, and with no reply it leaves no
   proposal. A turn that commits on top of commits the author kept off
   waits on the author in both modes, because its push carries them. The
   work branch is read before the fetch of the pull request branch, so a
   commit made while the fetch waits is part of the turn. The turn after
   a failed proposal starts where the failed one started, so it carries
   those commits. Before the daemon types a message into an idle session, it
   fetches the pull request branch and fast-forwards the work branch to
   it (`syncWork`), so a commit somebody else pushed is under the work
   of the turn.

   The daemon then releases the proposal (`prwatch/release.go`, with the
   git of `internal/gitrelease`). It pushes the work branch: plain when
   the work contains the head of the pull request branch, and otherwise
   with the lease pinned to the head it read,
   `--force-with-lease=refs/heads/<branch>:<sha>`, never a bare lease,
   which the fetch before the push would satisfy. When `git cherry`
   against the head lists a commit the work branch never had, a turn
   that only added commits on top of the head it started from, none of
   them a merge, is rebased onto the head and pushed, and a rewrite does
   not go out. A turn that takes over the commits of a failed proposal
   starts from the head of that proposal, and a hand-off to the agent
   moves that head to the one the agent was told about. The
   push uses `--no-verify` and the credential
   helper of the author, with `GIT_TERMINAL_PROMPT=0`. Then the daemon
   posts the replies in order and marks each comment it made seen, so
   no later poll reports it back as feedback. The next poll sees the
   push as a `commit` row; each reply is a `replied` row. A branch
   Dependabot owns is never pushed or rebased: its proposal carries
   replies only, and before each message its worktree moves to what the
   bot pushed. A reply GitHub can never take, such as one to a comment
   that was deleted, is dropped with an `agent_failed` row, and the
   replies after it still go out. A 404 drops a reply only when a read
   of the comment also answers 404 and a read of the pull request does
   not, since a token that lost access gets a 404 too.

   A release that fails marks the proposal `failed` and records an
   `agent_failed` row with the reason and the retry command, which
   notifies as `watch`. `POST /watches/{id}/proposals/{n}/retry`
   releases it again. While the watch asks, a retry of work the author
   never approved is refused. Work the daemon cannot rebase is handed to
   the agent as a message (`prompts/conflict.md`): a rebase that
   conflicts, at once, and a rewrite that lacks commits of the head, on
   a retry. The row of a rebase that conflicts names no retry: a retry
   conflicts the same way. The next turn of the agent takes over the
   failed proposal and its replies, and a retry of it is then refused. A reply
   the agent records for a comment, a review comment or a comment on
   the conversation, takes the place of the reply to that comment that
   waits in the proposal (`AddProposalReplyTo`), so
   the answer of the resolution does not go out beside the one of the
   work that never landed. An open or failed proposal is a ready
   blocker, so the pull request does not merge without it. The start
   and the end of a turn run in the order the hooks reported them. A
   daemon that starts again closes the turns
   a shutdown cut short, and a stop declines what did not go out.

   A watch in `manual` releases nothing at the end of the turn
   (`prwatch/proposal.go`). The proposal is `pending`, a `proposal` row
   notifies the author as `agent`, and `assess` stores the blocker
   `proposal N waits on your approval`, so no `merge_ready` row and no
   re-review request goes out before the work the author reads; a
   `failed` proposal blocks the merge the same way in both modes. While
   a proposal waits, `tell` holds every routine message, `send` and a
   new turn are refused, and the rows stay untold until the decision.
   `approve` writes the edits and the drops, releases, forgets the seen
   mark of each comment a dropped reply answered (the next poll finds its row and
   takes the told mark off), tells the agent what changed in a message
   that asks for nothing (`prompts/decision.md`), and kicks a poll.
   `reject` forgets the comments of the unposted replies, resets the
   work branch when asked, and sends the reason as a message of the
   author. It refuses a failed proposal that is partly on GitHub: a
   push that landed, or a reply that posted. On each poll, `rebaseStale`
   rebases a pending proposal, or an approved one that failed and whose
   work the pull request branch does not hold, onto a pull request
   branch that moved: a
   clean rebase of approved work goes out when the watch approves a
   clean rebase on its own, any other clean rebase is offered again, and
   a conflict goes to the agent. Without that setting, the release of
   approved work refuses to rebase and leaves it to `rebaseStale`, and
   a rewrite is never rebased. A clean rebase gives the commits new
   SHAs, so `renameReplies` writes them into each reply that waits and
   names an old one, full or abbreviated; the rebase in a release does
   the same. The commits pair up in order, or by subject when the
   rebase dropped one the base already had.
6. The agent reports what it does through its hooks: `babysitter watch
   hook <event>` posts each event to the daemon, which keeps the state
   of the session (`starting`, `idle`, `active`, `waiting_input`,
   `blocked`) and publishes `watch_session`. Claude Code takes the hooks
   on its command line; Copilot CLI reads them from
   `.github/hooks/babysitter.json` in the worktree, which the daemon
   writes before the start and hides from git. When the
   process exits, a `session_exited` row says so, and the next message
   starts the session again on the same conversation. A session that
   exits right after a resume had no conversation to continue, so the
   one after that is new.
7. The author reads the session with `watch output`, or the terminal
   panel of the app, and talks to it with `watch send`, which is refused
   only while the agent waits on a permission decision.
   A watch of provider `self` has no session and no worktree: the
   coding agent session that started it, through the `babysit-pr`
   skill of the plugin in `plugins/babysitter`, is its agent and works
   in the author's checkout, which the
   start requires to be on the head branch. Steps 3, 5 and 6 do not
   run for it: that agent pushes its own commits, and its reply goes
   out at once. Instead the agent calls `babysitter watch next`
   (`POST /watches/{id}/next`): the daemon polls the watch, composes
   the same message from the untold rows, records the `nudged` row and
   marks the rows told, and answers with it. A caller that gave up by
   then gets the message taken back: the daemon deletes the `nudged`
   row and the rows go to the next call. With `?wait=` the route
   subscribes to the change feed and holds the answer until a row
   arrives, the watch stops, the daemon calls the pull request ready
   to merge, or the wait ends, ten minutes at most. The agent of such
   a watch counts as working from the message it takes until it asks
   for the next one, and at most half an hour: a session that ends in
   between says nothing, and the deadline hands the watch back. `send`
   and `output` are refused on such a watch; `reply`, `merge` and
   `stop` work as for any other.

   The author can take the session over (`prwatch/takeover.go`). Under
   the lock of the watch, `Takeover` writes `taken_over_at` and
   `taken_over_pid` first, so a poll after the lock does not start the
   session again, then ends the session through `handOverSession`, so no
   `session_exited` row and no `endTurn` follow; a session that does not
   stop goes back to the daemon and the takeover is refused. It declines every
   proposal that waits, and records a `taken_over` row. When one of these
   steps fails, the takeover is undone and refused. The response
   carries the command of `agent.Runner.AuthorCommand`: the provider,
   the model and `--resume <session>`, with none of the rules, hooks or
   prompts of the daemon. The CLI replaces itself with that command in
   the worktree, so the pid it sent is the pid of the agent. While the
   watch is taken over, `tell` holds every message, `send` is refused,
   `recover` starts no session, a hook finds no live session and
   changes nothing, the blocker `the session is with you` keeps the pull
   request from ready to merge and from a re-review request, and a stop
   keeps the worktree. `Handback` (`prwatch/handback.go`) refuses while
   the pid is alive unless `force`, reads the commits of the work branch
   that the head does not have and `git status --porcelain`, and
   refuses with both lists until `confirm`; a Dependabot watch refuses
   such commits, since the daemon never pushes them. It then stores the
   head as the start of the next turn, so the push of that turn carries
   the commits of the author, clears the takeover, records a
   `handed_back` row, resumes the session with `prompts/handback.md`,
   and kicks a poll that tells the agent what waited. The row keeps the
   text of that prompt, and a poll sends it again while it has not gone
   out.
8. Once the agent is done with a head, the daemon asks the previous
   reviewers for a new review, the same call as the re-request review
   button (`prwatch.rereview`), so nobody has to notice the push. It
   asks only when a review is the only thing left
   (`snapshot.WaitsOnlyForReview`): the agent is idle with nothing left
   to be told, no request is pending yet, and the pull request is short
   of its approvals, has a reviewer against it, or has unresolved
   threads that all end with a reply from the token's own user, while
   everything else about it is good. `snapshot.Rereviewers` picks the
   logins. When the pull request is short of its approvals or has a
   reviewer against it, they are the ones that reviewed an earlier
   commit. The logins that wrote in the answered threads are always
   added, and when those threads are all that is left, they are the
   only ones asked, so an approver is not asked again. Of those thread
   logins, `snapshot.requestable` keeps only the ones GitHub can ask: a
   login that submitted a review, has no draft review, and is not the
   author. The GraphQL thread reads its last comment on its own
   (`comments(last: 1)`), so a reviewer that wrote after the first 100
   comments, or a deleted account, does not pass as an answer. The
   list of authors still comes from the first 100 comments only (issue
   05): a reviewer that wrote only after them is not asked. The author
   and the token's own user are left out. One `review_requested` row
   per round says who was asked. A round is the head plus the id of the
   newest answer (`rereview@<head>#<comment id>`, or `rereview@<head>`
   with no answer), so a request GitHub drops is not made again in the
   same round, and a new answer on the same head starts a new round. A
   request GitHub refuses gets its own row against the round, because
   the next poll would fail the same way; any other failure is logged
   and left to the next poll.

   A base branch with no rule asks for no approval, so the review owes
   the pull request nothing and the daemon asks nobody, however far
   behind the reviewers are. That is on purpose: a pending request is a
   merge blocker, so asking on a branch that needs no review would hold
   a pull request that is good to merge, for as long as the reviewer
   takes to answer. Set the approvals of the watch to ask for a review
   on such a branch. Answered threads are the exception: they already
   stop the merge, and only their reviewer can say the answer is good,
   so the daemon asks that reviewer on any branch.
9. After every poll the daemon says what keeps the pull request from
   merging (`snapshot.Blockers` plus what the daemon knows: a message
   the agent was not told yet, an agent that works or waits on the
   author) and keeps it on the watch row. The first poll without a
   blocker starts a clock, a new head starts it again, and after one
   whole interval without a blocker a `merge_ready` row tells the
   author once per head. The daemon never merges on its own: the author
   merges from the app or with `babysitter watch merge`, the daemon
   takes a fresh snapshot, assesses it as a poll does and refuses unless
   that assessment calls the pull request ready, so the merge waits for
   the same clock every other reader waits for; it merges with the head
   commit of that snapshot so a push in between makes GitHub refuse,
   records a `merged` row and stops the watch. A merge GitHub refuses is
   a `merge_failed` row and the watch goes on.
10. Merged, closed, or three access errors in a row stop the watch with a
   summary. A stop ends the agent session, declines the proposal that
   did not go out, then deletes the worktree and its private branch,
   whatever commits it holds, so the worktrees of the stopped watches do not
   fill the disk. A stop the author asks for takes `keepWorktree` to
   leave it on disk instead, and a stop after lost access always keeps
   it: the token or the network can come back, and the work in the
   worktree must survive until the author decides. A stop while the
   session is with the author keeps it too. The stop is written
   before the worktree goes, so a write that fails never leaves an
   active watch without its directory. The summary says which of the
   two happened. A daemon that stops ends every session without a word;
   the next daemon continues each conversation.
