# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

ALWAYS follow the rules in `docs/rules/`. Today there is one: never touch
`frontend/src/renderer/components/ui`. Only the shadcn CLI writes there.

Issues found while the app runs go into `PENDING_ISSUES.md` at the repository
root. Each issue takes the next free number and keeps it: remove the item
when it is fixed and leave the numbers of the others where they are. An
evidence page in `docs/evidences/` takes the number of the issue it
proves, the issue links the page from the index table and from its own
section, and the page goes with the fix.

## What this is

babysitter watches GitHub pull requests. It is one Go binary that is both
the CLI and a daemon, plus an Electron desktop app that runs the daemon and
shows its state. A watch on a pull request runs a coding agent (Claude Code
or Copilot CLI) in a pseudo terminal inside a git worktree, and the daemon
types review comments and failed check logs into that session so the agent
fixes, pushes and replies on its own.

`README.md` documents every CLI command and the snapshot format.
`docs/architecture.md` documents the two processes, the daemon endpoints
and the watch flow in detail. Read it before changing `httpd`, `prwatch`,
`session` or `daemon`.

## Layout

- `backend/`: Go module `github.com/deividfortuna/babysitter`, Go 1.27,
  cgo for SQLite. `go.work` at the root points to it.
- `frontend/`: Electron Forge + Vite+ + React 19 + Tailwind 4 + shadcn.
  Node 26, see `.node-version`. TypeScript 7 type checks it. Vite+ (`vp`)
  runs Vitest, Oxlint and Oxfmt; `frontend/vite.config.ts` holds their
  `test`, `lint` and `fmt` blocks. Forge builds with the
  `vite.{main,preload,renderer}.config.mts` files, not `vite.config.ts`.
  pnpm 12 installs it. `frontend/pnpm-workspace.yaml` holds the pnpm
  settings: `nodeLinker: hoisted`, which Forge needs, the `vite` and
  `vitest` overrides, and `allowBuilds`, the only packages whose install
  scripts run.
- `codegen/`: openapi-typescript, which writes `frontend/src/api/schema.ts`
  from `openapi.yaml`. It has its own lockfile because it uses the compiler
  API of TypeScript 5, which TypeScript 7 does not have.
- `docs/`: architecture and rules.
- `plugins/babysitter` is the Claude Code plugin. Its `babysit-pr` skill
  drives a watch from a coding agent session. `.claude-plugin/marketplace.json`
  at the root publishes it, and `.claude/settings.json` registers that
  marketplace from the checkout, so a session in this repository loads the
  plugin as `babysitter@babysitter`. `.agents/skills` is a symlink to
  `plugins/babysitter/skills`, for the agents that read that path.

## Commands

Run from the repository root unless noted.

```sh
pnpm run check             # golangci-lint, go test, tsc, vp fmt, vp lint, vp test, actionlint, zizmor: what CI runs
pnpm run backend:lint      # golangci-lint, rules in backend/.golangci.yml
pnpm run backend:test      # go test ./...
pnpm run frontend:typecheck
pnpm run frontend:format   # vp fmt with the Tailwind class sorter, rules in the fmt block of frontend/vite.config.ts
pnpm run frontend:lint     # type-aware vp lint with @shadcn/lint and better-tailwindcss, rules in the lint block of frontend/vite.config.ts
pnpm run frontend:test
pnpm run actions:lint      # actionlint and zizmor on .github/workflows
pnpm run api               # regenerate openapi.yaml and frontend/src/api/schema.ts, needs pnpm install in codegen/
pnpm start                 # go install the binary, then open the Electron app
```

Backend, from `backend/`:

```sh
go test ./internal/prwatch/                    # one package
go test ./internal/prwatch/ -run TestDiff      # one test
go test -race -count=1 ./...                   # what CI runs
golangci-lint run ./...                        # vet, staticcheck, errcheck, gosec and more
go generate ./internal/httpd/apispec/          # rewrite openapi.yaml
go test -tags agentlive -run TestLive -v ./internal/agent/   # real claude and copilot refuse the author decisions; uses model credits
go run ./cmd/babysitter daemon start           # daemon in the foreground
```

Frontend, from `frontend/`:

```sh
pnpm run test src/renderer/hooks/useRepos.test.tsx      # one test file
pnpm run test:coverage                                  # CI enforces thresholds in vite.config.ts
pnpm run package                                        # distributable in frontend/out
```

CI (`.github/workflows/pr.yaml`) fails when `go mod tidy`, `go generate`
for the OpenAPI document, or `pnpm run api:ts` produce a diff. After a change
to a route in `internal/httpd/router.go` or a DTO in `internal/httpd/dto.go`,
run `pnpm run api` and commit both generated files.

On macOS, when `go build` fails to link with `tapi error: malformed file`,
point cgo at the Xcode toolchain as the README shows.

## Architecture in short

**Backend packages.** `cmd/babysitter` is the entry point, `internal/cli`
holds the cobra commands. `internal/daemon` wires the long lived process:
store, repository watcher, HTTP API on loopback, `running.json`, supervisor
socket. `internal/store` is SQLite and publishes an event on
`internal/events` after each write; `internal/httpd/events.go` streams that
feed as SSE. `internal/snapshot` fetches one pull request from GitHub and
decides the next action. `internal/prwatch` is the watch loop: snapshot,
`Diff` against stored state, activity rows, message to the agent, merge
readiness. `internal/session` owns the pseudo terminal of the agent.
`internal/agent` holds the session contract, the embedded prompts in
`prompts/`, and one subpackage per provider (`claude`, `copilot`) that
builds the command line and the hooks. `internal/worktree` makes and removes
the worktree the agent works in. `internal/ghclient` wraps go-github with
one shared conditional-request cache and `RateGuard`. `internal/logbook` is
the slog handler of the daemon: it keeps the last records in memory, writes
`<dataDir>/logs/daemon.log` with rotation, and holds the level that
`PUT /logs/level` changes while the daemon runs.

**Frontend.** `src/main.ts` is the Electron main process and owns the daemon
through `src/main/daemon-supervisor.ts`: it attaches to a live daemon found
in `running.json` or spawns `babysitter daemon start --owner app`.
`src/preload.ts` exposes `window.babysitter`, the only IPC bridge, and it
carries little more than daemon status and the app log.
`src/main/app-log.ts` is the log of the main process, in
`<dataDir>/logs/app.log`; log there, not with `console.log`. The renderer talks to the daemon
directly over HTTP with openapi-fetch (`src/renderer/lib/api-client.ts`) and
refetches TanStack Query caches on each SSE frame
(`src/renderer/lib/event-transport.ts`). `src/api/schema.ts` is generated;
never edit it by hand. `scripts/build-daemon.mjs` compiles the Go binary
into `frontend/daemon/` before `start`, `package` and `make`.
The agent terminal is read only. `src/renderer/lib/ghostty/` parses the
output with libghostty-vt compiled to WebAssembly and draws it on a canvas.
It opens in a panel at the bottom of the watch view, and the user drags
the top edge of the panel to change its height. The grid fills that panel,
from 10 to 60 rows, and the app sends that size to `POST /watches/{id}/resize`,
which resizes the pseudo terminal of the agent.
The WASM is inlined in a lazy chunk, because the packaged app loads from
`file://`, where `fetch` fails; the CSP allows it with `'wasm-unsafe-eval'`.
`pnpm run build:ghostty-wasm` rebuilds it at the revision in `vendor/VERSION`.

**Tests.** Go tests sit next to the code and share two helper packages.
`internal/testutil` waits (`Eventually`, `Within`, `Settle`) and gives the
code under test a logger that writes to the test output (`Logger`); a test
never writes its own deadline loop or discards the logs.
`internal/ghclient/ghfake` is the GitHub of every test: `ghfake.New()` keeps
repositories and pull requests as typed state, applies the writes, records
every request as an `Action`, and takes reactors (`React`, `Fail`,
`Observe`, `GraphQLError`) for the failures a test needs. `ghfake.Serve`
starts a raw handler as GitHub, for the `ghclient` tests of exact wire
behavior such as ETags and pagination. A test builds state, not JSON, and
never starts its own GitHub server. Frontend tests use Vitest through
`vp test`, import it from `vite-plus/test`, and run with jsdom, Testing
Library and msw; `src/test/setup.ts` starts the msw server
with `onUnhandledRequest: "error"`, so every HTTP call a test makes needs a
handler in `src/test/msw.ts` or in the test. Fixtures live in
`src/test/fixtures.ts`.

To see the renderer in a plain browser, open the Vite dev server that
`pnpm start` prints with `?daemon=http://127.0.0.1:<port>/api/v1`, where the
port is that of a daemon started from the terminal.
