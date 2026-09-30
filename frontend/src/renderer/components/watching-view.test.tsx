import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildPullRequest, buildWatch } from "@test/fixtures";
import { expectViewTitle, renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi } from "@test/msw";
import { WatchingView } from "./watching-view";

test("shows the first-run screen when no pull requests are watched", async () => {
  serveApi();
  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  expect(await screen.findByText("No pull request is watched")).toBeVisible();
});

test("announces that watched pull requests are loading", async () => {
  serveApi();
  server.use(http.get(apiUrl("/api/v1/watches"), async () => new Promise<never>(() => undefined)));

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

  expect(screen.getByRole("status", { name: "Loading watched pull requests" })).toBeVisible();
});

test("groups active pull requests by repository and opens a selected pull request", async () => {
  const watch = buildWatch();
  const onNavigate = vi.fn();
  const user = userEvent.setup();
  serveApi({ watches: [watch] });

  renderWithProviders(<WatchingView enabled onNavigate={onNavigate} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  expect(await screen.findByText("octo/babysitter · 1")).toBeVisible();
  await screen.findByText("Add notifications");
  await user.click(await screen.findByRole("button", { name: /Add notifications/ }));

  expect(onNavigate).toHaveBeenCalledWith({ kind: "watch", id: 42 });
});

test("pins the watches that wait on the author above the repositories", async () => {
  serveApi({
    watches: [
      buildWatch({ id: 1, number: 1, title: "Quiet one" }),
      buildWatch({ id: 2, number: 2, title: "Loud one", session: { state: "blocked", pid: 8, logPath: "" } }),
    ],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  expect(await screen.findByText("2 active")).toBeVisible();
  const needsYou = screen.getByRole("region", { name: "Needs you · 1" });
  expect(within(needsYou).getByText("agent waits on a permission")).toBeVisible();
  const repo = screen.getByRole("region", { name: "octo/babysitter · 1" });
  expect(within(repo).getByText("agent idle")).toBeVisible();
  expect(within(repo).queryByText("Loud one")).toBeNull();
});

test("a watch that needs you names its next step and opens the watch from it", async () => {
  const onNavigate = vi.fn();
  const user = userEvent.setup();
  serveApi({
    watches: [
      buildWatch({ id: 1, number: 1, title: "Asks", session: { state: "waiting_input", pid: 8, logPath: "" } }),
      buildWatch({ id: 2, number: 2, title: "Exited", session: { state: "exited", pid: 0, logPath: "" } }),
      buildWatch({ id: 3, number: 3, title: "Idle one" }),
    ],
  });

  renderWithProviders(<WatchingView enabled onNavigate={onNavigate} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  const needsYou = await screen.findByRole("region", { name: "Needs you · 2" });
  expect(within(needsYou).getByRole("button", { name: "Message the agent" })).toBeVisible();
  expect(within(needsYou).getAllByRole("link", { name: "Open on GitHub" })).toHaveLength(2);
  await user.click(within(needsYou).getByRole("button", { name: "Answer the agent" }));
  expect(onNavigate).toHaveBeenCalledWith({ kind: "watch", id: 1 });
  const idle = screen.getByRole("button", { name: /Idle one/ }).closest('[role="listitem"]') as HTMLElement;
  expect(within(idle).queryByRole("link", { name: "Open on GitHub" })).toBeNull();
});

test("the state chips count the watches and keep only the chosen state", async () => {
  const user = userEvent.setup();
  serveApi({
    watches: [
      buildWatch({ id: 1, number: 1, title: "Asks", session: { state: "waiting_input", pid: 8, logPath: "" } }),
      buildWatch({ id: 2, number: 2, title: "Works", session: { state: "active", pid: 8, logPath: "" } }),
      buildWatch({ id: 3, number: 3, title: "Ready", readySince: "2026-01-01T00:00:00Z" }),
      buildWatch({ id: 4, number: 4, title: "Mine", takenOverAt: "2026-01-01T00:00:00Z" }),
    ],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  const chips = await screen.findByRole("radiogroup", { name: "Filter by state" });
  expect(within(chips).getByRole("radio", { name: "All 4" })).toBeChecked();
  expect(within(chips).getByRole("radio", { name: "Needs you 1" })).toBeVisible();
  expect(within(chips).getByRole("radio", { name: "Agent working 1" })).toBeVisible();
  expect(within(chips).getByRole("radio", { name: "Ready to merge 1" })).toBeVisible();
  expect(within(chips).getByRole("radio", { name: "With you 1" })).toBeVisible();

  await user.click(within(chips).getByRole("radio", { name: "Ready to merge 1" }));

  expect(screen.getByRole("button", { name: /Ready/ })).toBeVisible();
  expect(screen.queryByText("Asks")).toBeNull();
  expect(screen.queryByText("Works")).toBeNull();
  expect(screen.queryByText("Mine")).toBeNull();
});

test("the search keeps the watches whose title, number or author holds the text", async () => {
  const user = userEvent.setup();
  serveApi({
    watches: [
      buildWatch({ id: 1, number: 482, title: "Retry webhooks", author: "deividfortuna" }),
      buildWatch({ id: 2, number: 1203, title: "Paginate the audit log", author: "sgreen" }),
    ],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);
  const search = await screen.findByRole("searchbox", { name: "Filter by title, number, author, repository or label" });

  await user.type(search, "#482");
  expect(screen.getByText("Retry webhooks")).toBeVisible();
  expect(screen.queryByText("Paginate the audit log")).toBeNull();

  await user.clear(search);
  await user.type(search, "SGREEN");
  expect(screen.getByText("Paginate the audit log")).toBeVisible();
  expect(screen.queryByText("Retry webhooks")).toBeNull();
});

test("offers to clear the filters when no watch matches them", async () => {
  const user = userEvent.setup();
  serveApi({ watches: [buildWatch({ id: 1, number: 1, title: "Quiet one" })] });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);
  await user.click(await screen.findByRole("radio", { name: "With you 0" }));

  expect(screen.getByText("No watch matches")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Clear the filters" }));

  expect(screen.getByRole("radio", { name: "All 1" })).toBeChecked();
  expect(screen.getByText("Quiet one")).toBeVisible();
});

test("shows a live dot on the row of a pull request whose agent works", async () => {
  serveApi({
    watches: [
      buildWatch({ id: 1, number: 1, title: "Working", session: { ...buildWatch().session, state: "active" } }),
      buildWatch({ id: 2, number: 2, title: "Quiet" }),
    ],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  const working = await screen.findByRole("button", { name: /Working/ });
  expect(within(working).getByTitle("The agent works right now")).toBeVisible();
  const quiet = screen.getByRole("button", { name: /Quiet/ });
  expect(within(quiet).queryByTitle("The agent works right now")).toBeNull();
});

test("keeps the title in the view header", async () => {
  serveApi({ watches: [buildWatch()] });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  expect(await screen.findByRole("banner")).toContainElement(
    screen.getByRole("heading", { level: 1, name: "Watched pull requests" }),
  );
});

test("keeps the title in the view header while watches load, fail or are none", async () => {
  serveApi();
  const { unmount } = renderWithProviders(
    <WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />,
  );
  expectViewTitle("Watched pull requests");
  expect(await screen.findByText("No pull request is watched")).toBeVisible();
  expectViewTitle("Watched pull requests");
  unmount();

  server.use(
    http.get(apiUrl("/api/v1/watches"), () =>
      HttpResponse.json({ error: { message: "daemon gone" } }, { status: 500 }),
    ),
  );
  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  expect(await screen.findByText("daemon gone")).toBeVisible();
  expectViewTitle("Watched pull requests");
});

test("a row names a watch that auto start began, its update type and merge when ready", async () => {
  serveApi({
    watches: [
      buildWatch({
        id: 1,
        number: 30,
        title: "Bump stripe-go",
        dependabot: true,
        autoReason: "dependabot",
        updateType: "patch",
        mergeWhenReady: true,
      }),
      buildWatch({ id: 2, number: 31, title: "Retry webhooks" }),
    ],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  const auto = await screen.findByRole("button", { name: /Bump stripe-go/ });
  expect(within(auto).getByText("auto")).toHaveAttribute(
    "title",
    "Auto start began it because Dependabot opened the pull request.",
  );
  expect(within(auto).getByText("patch")).toBeVisible();
  expect(within(auto).getByText("merge when ready")).toBeVisible();
  const manual = screen.getByRole("button", { name: /Retry webhooks/ });
  expect(within(manual).queryByText("auto")).toBeNull();
  expect(within(manual).queryByText("merge when ready")).toBeNull();
});

test("a row shows the author, the labels and the changed lines of its pull request", async () => {
  serveApi({
    watches: [buildWatch({ id: 1, number: 12, title: "Add notifications", author: "dependabot[bot]" })],
    pullRequests: [buildPullRequest({ number: 12, labels: ["billing", "bug"], additions: 42, deletions: 7 })],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  const row = await screen.findByRole("button", { name: /Add notifications/ });
  expect(await within(row).findByText("+42")).toBeVisible();
  expect(within(row).getByText("−7")).toBeVisible();
  expect(within(row).getByText("billing")).toBeVisible();
  expect(within(row).getByText("bug")).toBeVisible();
  expect(within(row).getByText("dependabot")).toBeVisible();
});

test("a row puts the state that matters most first and names the failing checks", async () => {
  serveApi({
    watches: [
      buildWatch({
        id: 1,
        number: 1,
        title: "Broken one",
        lastError: "GitHub said 502",
        checkStates: { lint: "failed", test: "passed" },
      }),
      buildWatch({ id: 2, number: 2, title: "Ready one", readySince: "2026-01-01T00:00:00Z" }),
      buildWatch({ id: 3, number: 3, title: "Waiting one", pendingProposal: 4, lastError: "GitHub said 502" }),
    ],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  const broken = await screen.findByRole("button", { name: /Broken one/ });
  expect(within(broken).getByText("error")).toHaveAttribute("title", "GitHub said 502");
  expect(within(broken).getByText("1 failing check: lint")).toBeInTheDocument();
  const ready = screen.getByRole("button", { name: /Ready one/ });
  expect(within(ready).getByText("ready to merge")).toBeVisible();
  expect(within(ready).queryByText("agent idle")).toBeNull();
  const waiting = screen.getByRole("button", { name: /Waiting one/ });
  expect(within(waiting).getByText("approval needed")).toBeVisible();
  expect(within(waiting).queryByText("error")).toBeNull();
});

test("a row shows conflicts but not a clean merge state", async () => {
  serveApi({
    watches: [
      buildWatch({ id: 1, number: 1, title: "Conflicting", mergeableState: "dirty" }),
      buildWatch({ id: 2, number: 2, title: "Clean one", mergeableState: "clean" }),
    ],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  const conflicting = await screen.findByRole("button", { name: /Conflicting/ });
  expect(within(conflicting).getByText("conflicts")).toHaveClass("text-destructive");
  const clean = screen.getByRole("button", { name: /Clean one/ });
  expect(within(clean).queryByText("clean")).toBeNull();
});

test("keeps the rows and names the failure when the pull requests do not load", async () => {
  serveApi({ watches: [buildWatch({ id: 1, number: 12, title: "Add notifications" })] });
  server.use(
    http.get(apiUrl("/api/v1/prs"), () =>
      HttpResponse.json({ error: { message: "pull request store gone" } }, { status: 500 }),
    ),
  );

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  const alert = await screen.findByRole("alert");
  expect(within(alert).getByText("Labels and changed lines did not load")).toBeVisible();
  expect(within(alert).getByText("pull request store gone")).toBeVisible();
  expect(screen.getByRole("button", { name: /Add notifications/ })).toBeVisible();
});

test("the search also finds a watch by its repository and its labels", async () => {
  const user = userEvent.setup();
  serveApi({
    watches: [
      buildWatch({ id: 1, number: 12, title: "", repo: "octo/api" }),
      buildWatch({ id: 2, number: 13, title: "Labelled one", repo: "octo/web" }),
    ],
    pullRequests: [buildPullRequest({ number: 13, repo: "octo/web", labels: ["billing"] })],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);
  const search = await screen.findByRole("searchbox", {
    name: "Filter by title, number, author, repository or label",
  });

  await user.type(search, "octo/api");
  expect(screen.getByRole("button", { name: /octo\/api#12/ })).toBeVisible();
  expect(screen.queryByText("Labelled one")).toBeNull();

  await user.clear(search);
  await user.type(search, "BILLING");
  expect(await screen.findByText("Labelled one")).toBeVisible();
  expect(screen.queryByRole("button", { name: /octo\/api#12/ })).toBeNull();
});

test("the chip counts follow the search text", async () => {
  const user = userEvent.setup();
  serveApi({
    watches: [
      buildWatch({ id: 1, number: 1, title: "Retry webhooks", pendingProposal: 2 }),
      buildWatch({ id: 2, number: 2, title: "Paginate the log", pendingProposal: 3 }),
      buildWatch({ id: 3, number: 3, title: "Quiet webhooks" }),
    ],
  });

  renderWithProviders(<WatchingView enabled onNavigate={vi.fn()} onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);
  expect(await screen.findByRole("radio", { name: "Needs you 2" })).toBeVisible();

  await user.type(screen.getByRole("searchbox"), "webhooks");

  expect(screen.getByRole("radio", { name: "All 2" })).toBeChecked();
  expect(screen.getByRole("radio", { name: "Needs you 1" })).toBeVisible();
  expect(screen.getByRole("region", { name: "Needs you · 1" })).toBeVisible();
});
