import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { buildSettings, buildWatch } from "@test/fixtures";
import { http, HttpResponse } from "msw";
import { apiUrl, server, serveApi, type StopBody } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import { StopWatchDialog } from "./stop-watch-dialog";

function renderDialog() {
  const stopBodies: StopBody[] = [];
  serveApi({ settings: buildSettings({ keepWorktree: true }), stopBodies });
  const watch = buildWatch({ id: 7, worktreeDir: "/tmp/worktrees/octo-hello-3" });
  renderWithProviders(<StopWatchDialog open onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />);
  return stopBodies;
}

test("the box opens on the worktree setting of the daemon", async () => {
  const bodies = renderDialog();
  const user = userEvent.setup();

  await waitFor(() => expect(screen.getByLabelText("Keep the worktree on disk")).toBeChecked());
  await user.click(screen.getByRole("button", { name: "Stop watching" }));

  await waitFor(() => expect(bodies).toHaveLength(1));
  expect(bodies[0]).not.toHaveProperty("keepWorktree");
});

test("opens again on the worktree setting after the author ticked the box", async () => {
  serveApi({ settings: buildSettings({ keepWorktree: true }) });
  const watch = buildWatch({ id: 7, worktreeDir: "/tmp/worktrees/octo-hello-3" });
  const user = userEvent.setup();
  const { rerender } = renderWithProviders(
    <StopWatchDialog open onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />,
  );
  await waitFor(() => expect(screen.getByLabelText("Keep the worktree on disk")).toBeChecked());
  await user.click(screen.getByLabelText("Keep the worktree on disk"));
  expect(screen.getByLabelText("Keep the worktree on disk")).not.toBeChecked();

  rerender(<StopWatchDialog open={false} onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />);
  rerender(<StopWatchDialog open onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />);

  await waitFor(() => expect(screen.getByLabelText("Keep the worktree on disk")).toBeChecked());
});

test("a stop before the settings arrive says nothing about the worktree", async () => {
  const bodies = renderDialog();
  const user = userEvent.setup();

  await user.click(screen.getByRole("button", { name: "Stop watching" }));

  await waitFor(() => expect(bodies).toHaveLength(1));
  expect(bodies[0]).not.toHaveProperty("keepWorktree");
});

test("the box the author ticks beats the setting", async () => {
  const bodies = renderDialog();
  const user = userEvent.setup();

  await waitFor(() => expect(screen.getByLabelText("Keep the worktree on disk")).toBeChecked());
  await user.click(screen.getByLabelText("Keep the worktree on disk"));
  await user.click(screen.getByRole("button", { name: "Stop watching" }));

  await waitFor(() => expect(bodies).toHaveLength(1));
  expect(bodies[0].keepWorktree).toBe(false);
});

test("says the daemon decides while its worktree setting is on the way", async () => {
  renderDialog();

  expect(screen.getByText(/the setting of your daemon decides/i)).toBeVisible();
  expect(screen.queryByText(/are deleted/i)).toBeNull();

  await waitFor(() => expect(screen.getByLabelText("Keep the worktree on disk")).toBeChecked());
  expect(screen.getByText(/stays at \/tmp\/worktrees\/octo-hello-3/)).toBeVisible();
});

test("holds the stop until the worktree setting has landed", async () => {
  let answer: (() => void) | undefined;
  const landed = new Promise<void>((resolve) => {
    answer = resolve;
  });
  serveApi({ settings: buildSettings({ keepWorktree: true }) });
  server.use(
    http.get(apiUrl("/api/v1/settings"), async () => {
      await landed;
      return HttpResponse.json(buildSettings({ keepWorktree: true }));
    }),
  );
  const watch = buildWatch({ id: 7, worktreeDir: "/tmp/worktrees/octo-hello-3" });
  renderWithProviders(<StopWatchDialog open onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />);

  expect(screen.getByRole("button", { name: "Stop watching" })).toBeDisabled();

  answer?.();
  await waitFor(() => expect(screen.getByRole("button", { name: "Stop watching" })).toBeEnabled());
});
