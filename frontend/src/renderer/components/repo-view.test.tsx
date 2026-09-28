import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { http, HttpResponse } from "msw";
import { buildPullRequest, buildQueuedPullRequest, buildRepo, buildWatch } from "@test/fixtures";
import { expectViewTitle, renderWithProviders } from "@test/test-utils";
import { apiUrl, server, serveApi } from "@test/msw";
import { RepoView } from "./repo-view";

function renderView() {
  return renderWithProviders(
    <RepoView enabled name="octo/babysitter" onNavigate={vi.fn()} onWatchPR={vi.fn()} onWatchPull={vi.fn()} />,
  );
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

test("shows the daemon error when repository details cannot load", async () => {
  serveApi({ watches: [], pullRequests: [] });
  server.use(
    http.get(apiUrl("/api/v1/repos"), () =>
      HttpResponse.json({ error: { message: "GitHub token expired" } }, { status: 401 }),
    ),
  );

  renderView();

  expect(await screen.findByText("GitHub token expired")).toBeVisible();
});

test("explains when the selected repository is no longer registered", async () => {
  serveApi({ repos: [], watches: [], pullRequests: [] });

  renderView();

  expect(await screen.findByText("octo/babysitter is not registered")).toBeVisible();
  expect(screen.getByRole("button", { name: "Show every watch" })).toBeVisible();
});

test("shows an open pull request and selects it to start watching", async () => {
  const pullRequest = buildPullRequest();
  const onWatchPull = vi.fn();
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [pullRequest] });
  const user = userEvent.setup();

  renderWithProviders(
    <RepoView enabled name="octo/babysitter" onNavigate={vi.fn()} onWatchPR={vi.fn()} onWatchPull={onWatchPull} />,
  );

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

test("keeps the title in the view header", async () => {
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [] });

  renderView();

  expect(await screen.findByRole("banner")).toContainElement(
    screen.getByRole("heading", { level: 1, name: "octo/babysitter" }),
  );
});

test("keeps the title in the view header while the repository loads, fails or is gone", async () => {
  serveApi({ repos: [], watches: [], pullRequests: [] });
  const { unmount } = renderView();
  expectViewTitle("octo/babysitter");
  expect(await screen.findByText("octo/babysitter is not registered")).toBeVisible();
  expectViewTitle("octo/babysitter");
  unmount();

  server.use(
    http.get(apiUrl("/api/v1/repos"), () =>
      HttpResponse.json({ error: { message: "GitHub token expired" } }, { status: 401 }),
    ),
  );
  renderView();

  expect(await screen.findByText("GitHub token expired")).toBeVisible();
  expectViewTitle("octo/babysitter");
});

test("sizes the header buttons like the other views", async () => {
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [] });

  renderView();
  await screen.findByRole("button", { name: "Watch by URL" });

  const settings = screen.getByRole("button", { name: "Repository settings" });
  for (const button of within(screen.getByRole("banner")).getAllByRole("button")) {
    if (button === settings) continue;
    expect(button).toHaveAttribute("data-size", "sm");
  }
  expect(settings).toHaveClass("size-7");
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

test("the panel icon in the header opens the repository settings, and the panel closes them", async () => {
  serveApi({ repos: [buildRepo()], watches: [], pullRequests: [] });
  const user = userEvent.setup();

  renderView();
  await user.click(await screen.findByRole("button", { name: "Repository settings" }));

  const panel = screen.getByRole("complementary", { name: "Repository settings" });
  expect(
    within(panel).getByText(
      "What babysitter does with new pull requests of octo/babysitter. Nothing starts until you turn it on.",
    ),
  ).toBeVisible();
  expect(screen.queryByRole("button", { name: "Repository settings" })).toBeNull();

  await user.click(within(panel).getByRole("button", { name: "Close repository settings" }));
  expect(screen.queryByRole("complementary", { name: "Repository settings" })).toBeNull();
});

function rowOf(title: HTMLElement): HTMLElement {
  const row = title.closest<HTMLElement>('[role="listitem"]');
  if (!row) throw new Error("the pull request has no row");
  return row;
}
