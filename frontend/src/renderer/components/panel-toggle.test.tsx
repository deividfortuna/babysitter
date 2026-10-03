import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { renderWithProviders } from "@test/test-utils";
import { PanelToggle } from "./panel-toggle";

const platform = vi.hoisted(() => ({ isMac: false }));

vi.mock("@/lib/platform", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/platform")>()),
  get isMac() {
    return platform.isMac;
  },
}));

afterEach(() => {
  platform.isMac = false;
});

function Toggle() {
  const [open, setOpen] = useState(false);
  return <PanelToggle label="Watch settings" open={open} onOpenChange={setOpen} />;
}

test("the same button opens and closes the panel", async () => {
  const user = userEvent.setup();
  renderWithProviders(<Toggle />);

  const toggle = screen.getByRole("button", { name: "Watch settings" });
  expect(toggle).toHaveAttribute("aria-expanded", "false");

  await user.click(toggle);
  expect(toggle).toHaveAttribute("aria-expanded", "true");

  await user.click(toggle);
  expect(toggle).toHaveAttribute("aria-expanded", "false");
});

test("off macOS stays clear of the window buttons", () => {
  renderWithProviders(<Toggle />);

  expect(screen.getByRole("button", { name: "Watch settings" })).toHaveClass("absolute", "right-window-controls");
});

test("on macOS keeps the inset of the header", () => {
  platform.isMac = true;

  renderWithProviders(<Toggle />);

  expect(screen.getByRole("button", { name: "Watch settings" })).toHaveClass("absolute", "right-5");
});
