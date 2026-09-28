import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildPullRequest, buildStoppedWatch } from "@test/fixtures";
import { expectViewTitle, renderWithProviders } from "@test/test-utils";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi } from "@test/msw";
import { StoppedView } from "./stopped-view";

test("opens an archived watch", async () => {
  serveApi({ watches: [buildStoppedWatch({ id: 42, number: 12, title: "Ship notifications" })] });
  const onNavigate = vi.fn();
  const user = userEvent.setup();

  renderWithProviders(<StoppedView enabled onNavigate={onNavigate} />);

  await user.click(await screen.findByRole("button", { name: /Ship notifications/ }));

  expect(onNavigate).toHaveBeenCalledWith({ kind: "watch", id: 42 });
});

test("keeps the title in the view header", async () => {
  serveApi({ watches: [buildStoppedWatch()] });

  renderWithProviders(<StoppedView enabled onNavigate={vi.fn()} />);

  expect(await screen.findByRole("banner")).toContainElement(
    screen.getByRole("heading", { level: 1, name: "Stopped" }),
  );
});

test("keeps the title in the view header while stopped watches load, fail or are none", async () => {
  serveApi({ watches: [] });
  const { unmount } = renderWithProviders(<StoppedView enabled onNavigate={vi.fn()} />);
  expectViewTitle("Stopped");
  expect(await screen.findByText("Nothing stopped yet")).toBeVisible();
  expectViewTitle("Stopped");
  unmount();

  server.use(
    http.get(apiUrl("/api/v1/watches"), () =>
      HttpResponse.json({ error: { message: "daemon gone" } }, { status: 500 }),
    ),
  );
  renderWithProviders(<StoppedView enabled onNavigate={vi.fn()} />);

  expect(await screen.findByText("daemon gone")).toBeVisible();
  expectViewTitle("Stopped");
});

test("shows a watch that stopped because it merged in green", async () => {
  serveApi({ watches: [{ ...buildStoppedWatch(), stopReason: "merged" as const }] });

  renderWithProviders(<StoppedView enabled onNavigate={vi.fn()} />);

  expect(await screen.findByText("stopped · merged")).toHaveClass("text-success");
});

test("groups stopped watches into today and earlier", async () => {
  const now = new Date();
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 0, 1).toISOString();
  serveApi({
    watches: [
      buildStoppedWatch({ id: 1, number: 1, title: "Old one", stoppedAt: "2026-01-01T01:00:00Z" }),
      buildStoppedWatch({ id: 2, number: 2, title: "Fresh one", stoppedAt: startOfToday }),
    ],
  });

  renderWithProviders(<StoppedView enabled onNavigate={vi.fn()} />);

  const today = await screen.findByRole("region", { name: "Today" });
  expect(within(today).getByRole("button", { name: /Fresh one/ })).toBeVisible();
  const earlier = screen.getByRole("region", { name: "Earlier" });
  expect(within(earlier).getByRole("button", { name: /Old one/ })).toBeVisible();
});

test("a stopped row reads the labels and changed lines of its merged pull request", async () => {
  serveApi({ watches: [buildStoppedWatch({ id: 1, number: 12, title: "Ship notifications" })] });
  server.use(
    http.get(apiUrl("/api/v1/prs"), ({ request }) => {
      const state = new URL(request.url).searchParams.get("state");
      const pull = buildPullRequest({
        number: 12,
        state: "merged",
        labels: ["frontend"],
        additions: 154,
        deletions: 9,
      });
      return HttpResponse.json({ pullRequests: state === "all" ? [pull] : [] });
    }),
  );

  renderWithProviders(<StoppedView enabled onNavigate={vi.fn()} />);

  const row = await screen.findByRole("button", { name: /Ship notifications/ });
  expect(await within(row).findByText("+154")).toBeVisible();
  expect(within(row).getByText("frontend")).toBeVisible();
  expect(within(row).getByText("watched 1h 0m · 3 messages to the agent")).toBeVisible();
});

test("keeps the rows and names the failure when the pull requests do not load", async () => {
  serveApi({ watches: [buildStoppedWatch({ id: 1, number: 12, title: "Ship notifications" })] });
  server.use(
    http.get(apiUrl("/api/v1/prs"), () =>
      HttpResponse.json({ error: { message: "pull request store gone" } }, { status: 500 }),
    ),
  );

  renderWithProviders(<StoppedView enabled onNavigate={vi.fn()} />);

  const alert = await screen.findByRole("alert");
  expect(within(alert).getByText("Labels and changed lines did not load")).toBeVisible();
  expect(within(alert).getByText("pull request store gone")).toBeVisible();
  expect(screen.getByRole("button", { name: /Ship notifications/ })).toBeVisible();
});
