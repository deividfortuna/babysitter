import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildPullRequest, buildQueuedPullRequest, buildRepo, buildWatch } from "@test/fixtures";
import { expectViewTitle, focusOrder, renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi } from "@test/msw";
import { RepoView } from "./repo-view";

function renderView() {
  return renderWithProviders(<RepoView enabled name="octo/babysitter" onNavigate={vi.fn()} onWatchPull={vi.fn()} />);
}

test("announces that repository details are loading", async () => {
  serveApi();
  server.use(
    http.get(apiUrl("/api/v1/repos"), async () => new Promise<never>(() => undefined)),
    http.get(apiUrl("/api/v1/watches"), async () => new Promise<never>(() => undefined)),
    http.get(apiUrl("/api/v1/prs"), async () => new Promise<never>(() => undefined)),
  );

  renderView();
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });

  expect(screen.getByRole("status", { name: "Loading repository details" })).toBeVisible();
});

test("shows an open pull request and selects it to start watching", async () => {
  const pullRequest = buildPullRequest();
  const onWatchPull = vi.fn();
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [pullRequest] });
  const user = userEvent.setup();

  renderWithProviders(<RepoView enabled name="octo/babysitter" onNavigate={vi.fn()} onWatchPull={onWatchPull} />);

  expect(await screen.findByText("Add notifications")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Watch" }));

  expect(onWatchPull).toHaveBeenCalledWith(pullRequest);
});

test("heads the watched and the open pull requests like the other lists", async () => {
  serveApi({
    repos: [buildRepo()],
    watches: [buildWatch()],
    pullRequests: [buildPullRequest(), buildPullRequest({ number: 13, title: "Fix the flaky test" })],
  });

  renderView();

  const watching = await screen.findByRole("region", { name: "Watching · 1" });
  expect(within(watching).getByRole("button", { name: /Add notifications/ })).toBeVisible();
  const open = screen.getByRole("region", { name: "Open, not watched · 1" });
  expect(within(open).getByText("Fix the flaky test")).toBeVisible();
});

test("puts the watches that need you first in the watched list", async () => {
  serveApi({
    repos: [buildRepo()],
    watches: [
      buildWatch({ id: 1, number: 20, title: "Quiet newer" }),
      buildWatch({ id: 2, number: 10, title: "Waiting older", pendingProposal: 2 }),
    ],
  });

  renderView();

  const watching = await screen.findByRole("region", { name: "Watching · 2" });
  const titles = within(watching)
    .getAllByRole("listitem")
    .map((row) => row.textContent);
  expect(titles[0]).toContain("Waiting older");
  expect(titles[1]).toContain("Quiet newer");
  expect(within(watching).getByRole("button", { name: "Review proposal 2" })).toBeVisible();
});

test("keeps the title in the view header while the repository loads, shows, fails or is gone", async () => {
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [] });
  const shown = renderView();
  expectViewTitle("octo/babysitter");
  expect(await screen.findByRole("button", { name: "Sync now" })).toBeVisible();
  expectViewTitle("octo/babysitter");
  shown.unmount();

  serveApi({ repos: [], watches: [], pullRequests: [] });
  const gone = renderView();
  expect(await screen.findByText("octo/babysitter is not registered")).toBeVisible();
  expect(screen.getByRole("button", { name: "Show every watch" })).toBeVisible();
  expectViewTitle("octo/babysitter");
  gone.unmount();

  server.use(
    http.get(apiUrl("/api/v1/repos"), () =>
      HttpResponse.json({ error: { message: "GitHub token expired" } }, { status: 401 }),
    ),
  );
  renderView();

  expect(await screen.findByText("GitHub token expired")).toBeVisible();
  expectViewTitle("octo/babysitter");
});

test("the sync icon in the header asks the daemon to sync", async () => {
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [] });
  let synced = false;
  server.use(
    http.post(apiUrl("/api/v1/sync"), () => {
      synced = true;
      return HttpResponse.json({ accepted: true }, { status: 202 });
    }),
  );
  const user = userEvent.setup();

  renderView();
  const sync = await screen.findByRole("button", { name: "Sync now" });
  expect(sync).toHaveTextContent("");
  expect(sync).toHaveClass("size-7");
  await user.click(sync);

  await vi.waitFor(() => expect(synced).toBe(true));
});

test("shows the daemon error when Sync fails", async () => {
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [] });
  server.use(
    http.post(apiUrl("/api/v1/sync"), () =>
      HttpResponse.json({ error: { message: "the daemon is shutting down" } }, { status: 503 }),
    ),
  );
  const user = userEvent.setup();

  renderView();
  await user.click(await screen.findByRole("button", { name: "Sync now" }));

  expect(await screen.findByText("the daemon is shutting down")).toBeVisible();
});

test("a queued Dependabot update shows its place and its update type", async () => {
  serveApi({
    repos: [buildRepo()],
    watches: [],
    pullRequests: [
      buildPullRequest({ number: 30, title: "Bump x/net", author: "dependabot[bot]" }),
      buildPullRequest({ number: 31, title: "Bump the aws group", author: "dependabot[bot]" }),
      buildPullRequest({ number: 32, title: "Bump actions/checkout", author: "dependabot[bot]" }),
      buildPullRequest({ number: 12, title: "Add notifications" }),
    ],
    queue: [
      buildQueuedPullRequest({ number: 30, position: 1, updateType: "minor" }),
      buildQueuedPullRequest({ number: 31, position: 2, updateType: "major" }),
      buildQueuedPullRequest({ number: 32, position: 3, updateType: "major" }),
    ],
  });

  renderView();

  const first = rowOf(await screen.findByText("Bump x/net"));
  expect(await within(first).findByText("queued")).toBeVisible();
  expect(within(first).getByText("next in queue · minor")).toBeVisible();
  expect(within(rowOf(screen.getByText("Bump the aws group"))).getByText("2nd in queue · major")).toBeVisible();
  expect(within(rowOf(screen.getByText("Bump actions/checkout"))).getByText("3rd in queue · major")).toBeVisible();
  const mine = rowOf(screen.getByText("Add notifications"));
  expect(within(mine).queryByText("queued")).toBeNull();
  expect(within(first).getByRole("button", { name: "Watch" })).toBeEnabled();
});

test("the panel icon opens the repository settings, and the same icon in the same place closes them", async () => {
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [] });
  const user = userEvent.setup();

  renderView();
  const toggle = await screen.findByRole("button", { name: "Repository settings" });
  await user.click(toggle);

  const panel = screen.getByRole("complementary", { name: "Repository settings" });
  expect(
    within(panel).getByText(
      "What babysitter does with new pull requests of octo/babysitter. Nothing starts until you turn it on.",
    ),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "Repository settings" })).toBe(toggle);

  await user.click(toggle);
  expect(screen.queryByRole("complementary", { name: "Repository settings" })).toBeNull();
});

test("the panel icon comes before the pull requests and the repository settings in the focus order", async () => {
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [buildPullRequest()] });
  const user = userEvent.setup();

  renderView();
  const toggle = await screen.findByRole("button", { name: "Repository settings" });
  await user.click(toggle);
  const panel = screen.getByRole("complementary", { name: "Repository settings" });
  await within(panel).findByRole("button", { name: /Watch defaults/ });

  const watch = screen.getByRole("button", { name: "Watch" });
  const order = await focusOrder(user);
  const firstAfter = order.findIndex((element) => element === watch || panel.contains(element));
  expect(firstAfter).toBeGreaterThanOrEqual(0);
  expect(order.indexOf(toggle)).toBeGreaterThanOrEqual(0);
  expect(order.indexOf(toggle)).toBeLessThan(firstAfter);
});

test("the header ends with a no-drag space under the panel icon, so a click on the icon does not drag the window", async () => {
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [buildPullRequest()] });

  renderView();
  await screen.findByRole("button", { name: "Repository settings" });

  const space = screen.getByRole("banner").lastElementChild;
  expect(space).toHaveAttribute("data-slot", "panel-toggle-space");
});

function rowOf(title: HTMLElement): HTMLElement {
  const row = title.closest<HTMLElement>('[role="listitem"]');
  if (!row) throw new Error("the pull request has no row");
  return row;
}
