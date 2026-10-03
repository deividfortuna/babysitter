import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildActivity, buildStoppedWatch, buildWatch } from "@test/fixtures";
import { chooseOption, renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi, type StopBody } from "@test/msw";
import { WatchDetail } from "./watch-detail";

function renderWatchDetail(onStopped = vi.fn()) {
  renderWithProviders(<WatchDetail id={42} enabled onNavigate={vi.fn()} onStopped={onStopped} onWatchPR={vi.fn()} />);
  return { onStopped };
}

test("stops an active watch and continues with the daemon's stop summary", async () => {
  const watch = buildWatch({ id: 42 });
  const stoppedWatch = buildStoppedWatch({ id: 42, summary: { messages: 1 } });
  const onStopped = vi.fn();
  const stopBodies: StopBody[] = [];
  serveApi({ watches: [watch], watchById: { 42: watch }, stoppedWatch: { 42: stoppedWatch }, stopBodies });
  const user = userEvent.setup();

  renderWatchDetail(onStopped);
  await user.click(await screen.findByRole("button", { name: "Stop watching" }));
  const dialog = await screen.findByRole("dialog", { name: "Stop watching octo/babysitter#12" });
  await user.click(within(dialog).getByRole("button", { name: "Stop watching" }));

  await waitFor(() => expect(onStopped.mock.calls[0]?.[0]).toEqual(stoppedWatch));
  expect(stopBodies[0]).toEqual({});
});

test("keeps the worktree when the author asks", async () => {
  const watch = buildWatch({ id: 42 });
  const stopBodies: StopBody[] = [];
  serveApi({
    watches: [watch],
    watchById: { 42: watch },
    stoppedWatch: { 42: buildStoppedWatch({ id: 42 }) },
    stopBodies,
  });
  const user = userEvent.setup();

  renderWatchDetail();
  await user.click(await screen.findByRole("button", { name: "Stop watching" }));
  const dialog = await screen.findByRole("dialog", { name: "Stop watching octo/babysitter#12" });
  await user.click(within(dialog).getByRole("checkbox", { name: "Keep the worktree on disk" }));
  await user.click(within(dialog).getByRole("button", { name: "Stop watching" }));

  await waitFor(() => expect(stopBodies[0]).toEqual({ keepWorktree: true }));
});

test("the archive of a stopped watch says the worktree is gone", async () => {
  const watch = buildStoppedWatch({ id: 42, summary: { worktreeRemoved: true } });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  expect(await screen.findByText(/deleted from/)).toBeInTheDocument();
  expect(screen.queryByText(/left in place/)).not.toBeInTheDocument();
});

test("the archive of a stopped watch names the branch that stayed behind", async () => {
  const watch = buildStoppedWatch({
    id: 42,
    sourceDir: "/home/me/babysitter",
    summary: { worktreeRemoved: true, workBranchLeft: "babysitter/fix" },
  });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  expect(await screen.findByText(/its branch babysitter\/fix stays in \/home\/me\/babysitter/)).toBeInTheDocument();
});

test("the archive of a stopped watch says the worktree stayed", async () => {
  const watch = buildStoppedWatch({ id: 42, summary: { worktreeRemoved: false } });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  expect(await screen.findByText(/left in place at/)).toBeInTheDocument();
});

test("shows what the agent does and sends it a message", async () => {
  const esc = String.fromCharCode(27);
  const watch = buildWatch({
    id: 42,
    session: { state: "waiting_input", pid: 7, startedAt: "2026-01-01T00:00:00Z", logPath: "" },
  });
  serveApi({
    watches: [watch],
    watchById: { 42: watch },
    watchOutput: { 42: `${esc}[32m❯${esc}[0m which option do you prefer?\n` },
  });
  let sent: Record<string, unknown> | null = null;
  server.use(
    http.post(apiUrl("/api/v1/watches/:id/send"), async ({ request }) => {
      sent = (await request.json()) as Record<string, unknown>;
      return HttpResponse.json(
        {
          id: 9,
          watchId: 42,
          kind: "nudged",
          ref: "0",
          at: "2026-01-01T00:00:00Z",
          actor: "",
          summary: "you told the agent: the first one",
          url: "",
          payload: {},
          reported: true,
        },
        { status: 201 },
      );
    }),
  );
  const user = userEvent.setup();

  renderWatchDetail();
  expect(await screen.findByText("agent asks you")).toBeVisible();
  expect(screen.getByText("The agent waits on you. Read what it printed and answer it below.")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "show the terminal" }));
  const terminal = await screen.findByRole("region", { name: "Terminal" });
  expect(await within(terminal).findByLabelText("Agent output")).toHaveTextContent("❯ which option do you prefer?");
  await user.type(screen.getByLabelText("Message to the agent"), "the first one");
  await user.click(screen.getByRole("button", { name: "Send" }));

  await waitFor(() => expect(sent).toEqual({ message: "the first one" }));
  await waitFor(() => expect(screen.getByLabelText("Message to the agent")).toHaveValue(""));
});

test("opens the terminal in a panel at the bottom and closes it", async () => {
  const watch = buildWatch({ id: 42 });
  serveApi({ watches: [watch], watchById: { 42: watch }, watchOutput: { 42: "hello from the agent\n" } });
  const user = userEvent.setup();

  renderWatchDetail();
  await user.click(await screen.findByRole("button", { name: "show the terminal" }));
  const terminal = await screen.findByRole("region", { name: "Terminal" });
  expect(within(terminal).getByRole("separator", { name: "Resize terminal" })).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "hide the terminal" })).toBeVisible();

  await user.click(within(terminal).getByRole("button", { name: "Close the terminal" }));

  expect(screen.queryByRole("region", { name: "Terminal" })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "show the terminal" })).toBeVisible();
});

test("Ctrl+` shows and hides the terminal", async () => {
  const watch = buildWatch({ id: 42 });
  serveApi({ watches: [watch], watchById: { 42: watch }, watchOutput: { 42: "hello from the agent\n" } });
  const user = userEvent.setup();

  renderWatchDetail();
  await screen.findByRole("button", { name: "show the terminal" });
  await user.keyboard("{Control>}[Backquote]{/Control}");
  expect(await screen.findByRole("region", { name: "Terminal" })).toBeInTheDocument();

  await user.keyboard("{Control>}[Backquote]{/Control}");
  expect(screen.queryByRole("region", { name: "Terminal" })).not.toBeInTheDocument();
});

test("Ctrl+` does nothing on a self watch", async () => {
  const watch = buildWatch({ id: 42, provider: "self", sourceDir: "/home/alice/hello" });
  serveApi({ watches: [watch], watchById: { 42: watch } });
  const user = userEvent.setup();

  renderWatchDetail();
  await screen.findByText("your own session");
  await user.keyboard("{Control>}[Backquote]{/Control}");

  expect(screen.queryByRole("region", { name: "Terminal" })).not.toBeInTheDocument();
});

test("a self watch has no terminal and no message box", async () => {
  const watch = buildWatch({ id: 42, provider: "self", sourceDir: "/home/alice/hello" });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();
  expect(await screen.findByText("your own session")).toBeVisible();
  expect(
    screen.getByText(/The coding session that started this watch is its agent, in \/home\/alice\/hello/),
  ).toBeVisible();
  expect(screen.getByText("babysitter watch next 42")).toBeVisible();
  expect(screen.queryByRole("button", { name: "show the terminal" })).not.toBeInTheDocument();
  expect(screen.queryByLabelText("Message to the agent")).not.toBeInTheDocument();
});

test("shows the message a nudge carried", async () => {
  const watch = buildWatch({ id: 42 });
  serveApi({
    watches: [watch],
    watchById: { 42: watch },
    watchActivity: {
      42: [
        buildActivity({
          id: 5,
          watchId: 42,
          kind: "nudged",
          actor: "",
          url: "",
          summary: "told the agent about 1 comment",
          payload: { message: "The following 1 unresolved review comment is on PR #12" },
        }),
      ],
    },
  });
  const user = userEvent.setup();

  renderWatchDetail();
  expect(await screen.findByText("told the agent about 1 comment")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "the message" }));

  expect(await screen.findByText(/unresolved review comment is on PR #12/)).toBeVisible();
});

test("sends a commit row to the commit, not to the pull request", async () => {
  const watch = buildWatch({ id: 42 });
  serveApi({
    watches: [watch],
    watchById: { 42: watch },
    watchActivity: {
      42: [
        buildActivity({
          id: 6,
          watchId: 42,
          kind: "commit",
          actor: "",
          url: "https://github.com/octo/babysitter/pull/12",
          summary: "new commit e48bd70 on fix",
          payload: { sha: "e48bd705a60995ec8356f0f73548d5f3e63b576c" },
        }),
      ],
    },
  });

  renderWatchDetail();

  expect(await screen.findByRole("link", { name: "new commit e48bd70 on fix" })).toHaveAttribute(
    "href",
    "https://github.com/octo/babysitter/commit/e48bd705a60995ec8356f0f73548d5f3e63b576c",
  );
});

test("names what blocks the merge and keeps the merge button off", async () => {
  const watch = buildWatch({ id: 42, readyBlockers: ["no approval yet", "2 checks pending"] });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  const list = await screen.findByRole("list", { name: "What blocks the merge" });
  expect(
    within(list)
      .getAllByRole("listitem")
      .map((li) => li.textContent),
  ).toEqual(["no approval yet", "2 checks pending"]);
  expect(screen.getByRole("button", { name: "Merge" })).toBeDisabled();
  expect(screen.getByText("not ready to merge")).toBeInTheDocument();
});

test("merges a ready pull request with the method the author picks", async () => {
  const watch = buildWatch({ id: 42, readySince: "2026-01-01T00:10:00Z", mergeMethod: "squash" });
  const merged = buildStoppedWatch({
    id: 42,
    stopReason: "merged",
    summary: { reason: "merged", detail: "rebase", prState: "merged" },
  });
  const onStopped = vi.fn();
  serveApi({ watches: [watch], watchById: { 42: watch } });
  let body: Record<string, unknown> | null = null;
  server.use(
    http.post(apiUrl("/api/v1/watches/:id/merge"), async ({ request }) => {
      body = (await request.json()) as Record<string, unknown>;
      return HttpResponse.json(merged);
    }),
  );
  const user = userEvent.setup();

  renderWatchDetail(onStopped);
  expect(await screen.findByText(/Ready to merge since/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Merge" }));
  const dialog = await screen.findByRole("dialog", { name: "Merge octo/babysitter#12" });
  const method = within(dialog).getByLabelText("Merge method");
  expect(method).toHaveTextContent("Squash");
  await chooseOption(user, method, "Rebase");
  await user.click(within(dialog).getByRole("button", { name: "Merge" }));

  await waitFor(() => expect(onStopped.mock.calls[0]?.[0]).toEqual(merged));
  expect(body).toEqual({ method: "rebase" });
});

test("shows why the daemon refused a merge", async () => {
  const watch = buildWatch({ id: 42, readySince: "2026-01-01T00:10:00Z" });
  serveApi({ watches: [watch], watchById: { 42: watch } });
  server.use(
    http.post(apiUrl("/api/v1/watches/:id/merge"), () =>
      HttpResponse.json(
        { error: { code: "not_ready", message: "the pull request is not ready to merge: the agent is still working" } },
        { status: 409 },
      ),
    ),
  );
  const user = userEvent.setup();

  renderWatchDetail();
  await user.click(await screen.findByRole("button", { name: "Merge" }));
  const dialog = await screen.findByRole("dialog", { name: "Merge octo/babysitter#12" });
  await user.click(within(dialog).getByRole("button", { name: "Merge" }));

  expect(await within(dialog).findByText(/the agent is still working/)).toBeInTheDocument();
});

test("names the reviewers the daemon asked for a new review", async () => {
  const watch = buildWatch({ id: 42 });
  serveApi({
    watches: [watch],
    watchById: { 42: watch },
    watchActivity: {
      42: [
        buildActivity({
          id: 7,
          watchId: 42,
          kind: "review_requested",
          actor: "babysitter",
          url: "https://github.com/octo/babysitter/pull/12",
          summary: "asked copilot-pull-request-reviewer[bot] for a new review of e48bd70",
          payload: { reviewers: ["copilot-pull-request-reviewer[bot]"], sha: "e48bd70" },
        }),
      ],
    },
  });

  renderWatchDetail();

  expect(
    await screen.findByRole("link", {
      name: "asked copilot-pull-request-reviewer[bot] for a new review of e48bd70",
    }),
  ).toHaveAttribute("href", "https://github.com/octo/babysitter/pull/12");
  expect(screen.getByText("review requested", { exact: false })).toBeVisible();
});

test("shows a live dot in the agent panel while the agent works", async () => {
  const base = buildWatch({ id: 42 });
  const watch = buildWatch({ id: 42, session: { ...base.session, state: "active" } });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  expect(await screen.findByText("agent working")).toBeVisible();
  expect(screen.getByTitle("The agent works right now")).toBeVisible();
});

test("leaves the agent panel without a live dot while the agent is idle", async () => {
  const watch = buildWatch({ id: 42 });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  expect(await screen.findByText("agent idle")).toBeVisible();
  expect(screen.queryByTitle("The agent works right now")).toBeNull();
});

test("keeps the title in the view header", async () => {
  const watch = buildWatch({ id: 42 });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  const title = await screen.findByRole("heading", { level: 1 });

  expect(screen.getByRole("banner")).toContainElement(title);
});

test("keeps a header to drag the window while the watch loads or fails", async () => {
  serveApi({ watches: [] });
  server.use(
    http.get(apiUrl("/api/v1/watches/:id"), () =>
      HttpResponse.json({ error: { message: "no such watch" } }, { status: 404 }),
    ),
  );

  renderWatchDetail();
  expect(screen.getByRole("banner")).toBeInTheDocument();

  expect(await screen.findByText("no such watch")).toBeVisible();
  expect(screen.getByRole("banner")).toBeInTheDocument();
});

test("shows why a watch stopped under the title, with the other badges", async () => {
  const watch = buildStoppedWatch({ id: 42, mergeableState: "clean" });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();
  const stopped = await screen.findByText("stopped · user");

  expect(stopped.parentElement).toContainElement(screen.getByText("clean"));
  expect(stopped.parentElement).not.toContainElement(screen.getByRole("heading", { level: 1 }));
});

test("shows a watch that stopped because it merged in green", async () => {
  const watch = { ...buildStoppedWatch({ id: 42 }), stopReason: "merged" as const };
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  expect(await screen.findByText("stopped · merged")).toHaveClass("text-success");
});

test("the header names why auto start began the watch, and the activity marks the rows of auto start", async () => {
  const watch = buildWatch({
    id: 42,
    dependabot: true,
    autoReason: "dependabot",
    updateType: "minor",
    mergeWhenReady: true,
  });
  serveApi({
    watches: [watch],
    watchById: { 42: watch },
    watchActivity: {
      42: [
        buildActivity({ id: 1, kind: "auto_started", summary: "Started on its own: Dependabot opened it", url: "" }),
        buildActivity({ id: 2, kind: "approved", summary: "Approved on your behalf", url: "" }),
        buildActivity({ id: 3, kind: "approval_asked", summary: "Asked you to approve", url: "" }),
      ],
    },
  });

  renderWatchDetail();

  await screen.findByRole("heading", { level: 1, name: "Add notifications" });
  const header = screen.getByRole("banner");
  expect(within(header).getByText("auto")).toHaveAttribute(
    "title",
    "Auto start began it because Dependabot opened the pull request.",
  );
  expect(within(header).getByText("minor")).toBeVisible();
  expect(within(header).getByText("merge when ready")).toBeVisible();
  expect(await screen.findByText("Started on its own: Dependabot opened it")).toBeVisible();
  expect(screen.getByText(/^auto started/)).toBeVisible();
  expect(screen.getByText(/^approved ·/)).toBeVisible();
  expect(screen.getByText(/^approval asked/)).toBeVisible();
});

test("the ready banner says the daemon merges when merge when ready is on", async () => {
  const watch = buildWatch({
    id: 42,
    readySince: "2026-01-01T00:10:00Z",
    mergeWhenReady: true,
    mergeMethod: "squash",
  });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  expect(await screen.findByText(/Merge when ready is on, so the daemon merges it now with squash\./)).toBeVisible();
});

test("the ready banner leaves the merge to the author when merge when ready is off", async () => {
  const watch = buildWatch({ id: 42, readySince: "2026-01-01T00:10:00Z" });
  serveApi({ watches: [watch], watchById: { 42: watch } });

  renderWatchDetail();

  expect(await screen.findByText(/Merge it when it suits you\./)).toBeVisible();
  expect(screen.queryByText(/the daemon merges it now/)).toBeNull();
});
