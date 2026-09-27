import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vite-plus/test";
import { buildStoppedWatch } from "@test/fixtures";
import { StopSummaryDialog } from "./stop-summary-dialog";

const watch = buildStoppedWatch({
  id: 42,
  worktreeDir: "/Users/octo/code/babysitter",
  summary: { activity: { comment: 2, review: 1, commit: 3 }, messages: 4, detail: "Everything is ready." },
});

test("shows a stop receipt and lets the user watch another pull request", async () => {
  const onWatchAnother = vi.fn();
  const user = userEvent.setup();
  render(<StopSummaryDialog watch={watch} onClose={vi.fn()} onWatchAnother={onWatchAnother} />);

  expect(screen.getByRole("dialog", { name: "Stopped watching octo/babysitter#12" })).toHaveTextContent(
    "4 messages · 3 review items · 3 commits",
  );
  expect(screen.getByRole("dialog")).toHaveTextContent("Everything is ready.");
  expect(screen.getByRole("dialog")).toHaveTextContent("deleted from /Users/octo/code/babysitter");
  await user.click(screen.getByRole("button", { name: "Watch another PR" }));

  expect(onWatchAnother).toHaveBeenCalledOnce();
});
