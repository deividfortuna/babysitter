import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { http, HttpResponse } from "msw";
import { buildPullRequest, buildRepo } from "@test/fixtures";
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

  for (const button of within(screen.getByRole("banner")).getAllByRole("button")) {
    expect(button).toHaveAttribute("data-size", "sm");
  }
});
