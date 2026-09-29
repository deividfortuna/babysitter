import { render, screen } from "@testing-library/react";
import { expect, test } from "vite-plus/test";
import { SettingsRow } from "./settings-page";

test("a settings row keeps its own padding and adds the classes it is given", () => {
  render(<SettingsRow label="Keep the worktree" className="bg-muted" />);

  const row = screen.getByText("Keep the worktree").closest("[class*='group/row']");
  expect(row).toHaveClass("px-3.5", "py-3", "bg-muted");
});
