import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { FirstRun } from "./first-run";

test("starts watching or adding a repository from the first-run screen", async () => {
  const onWatchPR = vi.fn();
  const onAddRepo = vi.fn();
  const user = userEvent.setup();

  render(<FirstRun onWatchPR={onWatchPR} onAddRepo={onAddRepo} />);

  await user.click(screen.getByRole("button", { name: "Watch a pull request" }));
  await user.click(screen.getByRole("button", { name: "Add a repository" }));

  expect(onWatchPR).toHaveBeenCalledOnce();
  expect(onAddRepo).toHaveBeenCalledOnce();
});

test("the first-run screen does not promise a push without approval", () => {
  render(<FirstRun onWatchPR={vi.fn()} onAddRepo={vi.fn()} />);

  expect(screen.queryByText(/pushes and replies on its own/)).toBeNull();
});
