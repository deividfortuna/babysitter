import { render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { ViewHeader, clearsWindowButtonsWhenSidebarCollapses } from "./view-header";

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

  expect(screen.getByRole("banner")).toHaveClass("sticky", "top-0");
});

test("on macOS drags the window and clears the window buttons when the sidebar collapses", () => {
  platform.isMac = true;

  render(<ViewHeader>Stopped</ViewHeader>);

  expect(screen.getByRole("banner")).toHaveClass("app-drag", clearsWindowButtonsWhenSidebarCollapses);
});

test("elsewhere leaves dragging and the header padding to the native window frame", () => {
  render(<ViewHeader>Stopped</ViewHeader>);

  expect(screen.getByRole("banner")).not.toHaveClass("app-drag");
  expect(screen.getByRole("banner")).not.toHaveClass(clearsWindowButtonsWhenSidebarCollapses);
});
