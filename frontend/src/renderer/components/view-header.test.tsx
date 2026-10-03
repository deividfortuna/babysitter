import { render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import {
  ViewHeader,
  ViewHeaderActions,
  ViewHeaderButton,
  clearsWindowButtonsWhenSidebarCollapses,
} from "./view-header";

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

test("sticks to the top of the view so the title stays in the top bar", () => {
  render(<ViewHeader>Stopped</ViewHeader>);

  expect(screen.getByRole("banner")).toHaveClass("sticky", "top-0", "min-h-titlebar");
});

test("drags the window and clears the navigation buttons when the sidebar collapses", () => {
  render(<ViewHeader>Stopped</ViewHeader>);

  expect(screen.getByRole("banner")).toHaveClass("app-drag", clearsWindowButtonsWhenSidebarCollapses);
});

test("off macOS keeps its content out from under the window buttons", () => {
  render(<ViewHeader>Stopped</ViewHeader>);

  expect(screen.getByRole("banner")).toHaveClass("pr-window-controls");
  expect(screen.getByRole("banner")).not.toHaveClass("pr-5");
});

test("on macOS keeps the usual inset, since the window buttons sit at the left", () => {
  platform.isMac = true;

  render(<ViewHeader>Stopped</ViewHeader>);

  expect(screen.getByRole("banner")).toHaveClass("pr-5");
  expect(screen.getByRole("banner")).not.toHaveClass("pr-window-controls");
});

test("sizes every button of the header the same", () => {
  render(
    <ViewHeader>
      Stopped
      <ViewHeaderActions>
        <ViewHeaderButton>Mark all as read</ViewHeaderButton>
        <ViewHeaderButton variant="outline">Stop watching</ViewHeaderButton>
      </ViewHeaderActions>
    </ViewHeader>,
  );

  for (const button of screen.getAllByRole("button")) {
    expect(button).toHaveAttribute("data-size", "xs");
  }
});
