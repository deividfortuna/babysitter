import { render, screen } from "@testing-library/react";
import { expect, test } from "vite-plus/test";
import { ChecksBadge, MergeableBadge, SessionBadge, StopBadge } from "./status-badges";

test("reports failing checks and merge conflicts", () => {
  render(
    <>
      <ChecksBadge watch={{ checkStates: { lint: "failed", test: "passed" }, greenSha: "", headSha: "abc123" }} />
      <MergeableBadge state="dirty" />
    </>,
  );

  expect(screen.getByText("1 failing check")).toBeVisible();
  expect(screen.getByText("1 failing check")).toHaveAttribute("title", "lint");
  expect(screen.getByText("conflicts")).toBeVisible();
});

test("marks a working agent with a live dot", () => {
  render(<SessionBadge state="active" />);

  expect(screen.getByText("agent working")).toBeVisible();
  expect(screen.getByTitle("The agent works right now")).toBeVisible();
});

test("leaves an idle agent without a live dot", () => {
  render(<SessionBadge state="idle" />);

  expect(screen.getByText("agent idle")).toBeVisible();
  expect(screen.queryByTitle("The agent works right now")).toBeNull();
});

test("shows a watch that stopped because it merged in green", () => {
  render(<StopBadge watch={{ stopReason: "merged" }} />);

  expect(screen.getByText("stopped · merged")).toHaveClass("text-success");
});

test("leaves a watch the author stopped in the neutral colour", () => {
  render(<StopBadge watch={{ stopReason: "user" }} />);

  expect(screen.getByText("stopped · user")).not.toHaveClass("text-success");
});
