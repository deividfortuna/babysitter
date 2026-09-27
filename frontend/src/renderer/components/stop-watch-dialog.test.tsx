import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildWatch } from "@test/fixtures";
import { serveApi, type StopBody } from "@test/msw";
import { renderWithProviders } from "@test/test-utils";
import { StopWatchDialog } from "./stop-watch-dialog";

const WORKTREE = "/tmp/worktrees/octo-hello-3";

function renderDialog(keepWorktree = true) {
  const stopBodies: StopBody[] = [];
  serveApi({ stopBodies });
  const watch = buildWatch({ id: 7, worktreeDir: WORKTREE, keepWorktree });
  renderWithProviders(<StopWatchDialog open onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />);
  return stopBodies;
}

test("the box opens on the worktree rule the watch started with", async () => {
  const bodies = renderDialog();
  const user = userEvent.setup();

  expect(screen.getByLabelText("Keep the worktree on disk")).toBeChecked();
  expect(screen.getByText(/stays at \/tmp\/worktrees\/octo-hello-3/)).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Stop watching" }));

  await waitFor(() => expect(bodies).toHaveLength(1));
  expect(bodies[0]).not.toHaveProperty("keepWorktree");
});

test("a watch that removes its worktree opens with the box clear", () => {
  renderDialog(false);

  expect(screen.getByLabelText("Keep the worktree on disk")).not.toBeChecked();
  expect(screen.getByText(/are deleted/i)).toBeVisible();
});

test("opens again on the rule of the watch after the author ticked the box", async () => {
  serveApi({});
  const watch = buildWatch({ id: 7, worktreeDir: WORKTREE, keepWorktree: true });
  const user = userEvent.setup();
  const { rerender } = renderWithProviders(
    <StopWatchDialog open onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />,
  );
  await user.click(screen.getByLabelText("Keep the worktree on disk"));
  expect(screen.getByLabelText("Keep the worktree on disk")).not.toBeChecked();

  rerender(<StopWatchDialog open={false} onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />);
  rerender(<StopWatchDialog open onOpenChange={vi.fn()} watch={watch} onStopped={vi.fn()} />);

  expect(screen.getByLabelText("Keep the worktree on disk")).toBeChecked();
});

test("the box the author ticks beats the rule of the watch", async () => {
  const bodies = renderDialog();
  const user = userEvent.setup();

  await user.click(screen.getByLabelText("Keep the worktree on disk"));
  await user.click(screen.getByRole("button", { name: "Stop watching" }));

  await waitFor(() => expect(bodies).toHaveLength(1));
  expect(bodies[0].keepWorktree).toBe(false);
});
