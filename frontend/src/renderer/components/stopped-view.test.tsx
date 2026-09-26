import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { buildStoppedWatch } from "@test/fixtures";
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
