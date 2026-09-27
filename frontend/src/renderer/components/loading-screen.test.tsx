import { render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { LoadingScreen } from "./loading-screen";

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

test("says the app reads the environment of the login shell", () => {
  render(<LoadingScreen status={{ state: "starting", step: "environment" }} />);

  expect(screen.getByRole("status")).toHaveTextContent("Reading your shell environment");
});

test("says the app starts the daemon for any other step", () => {
  const { rerender } = render(<LoadingScreen status={{ state: "starting", step: "daemon" }} />);
  expect(screen.getByRole("status")).toHaveTextContent("Starting the daemon");

  rerender(<LoadingScreen status={{ state: "starting" }} />);
  expect(screen.getByRole("status")).toHaveTextContent("Starting the daemon");
});

test("shows the name of the app", () => {
  render(<LoadingScreen status={{ state: "starting" }} />);

  expect(screen.getByRole("heading", { name: "Babysitter" })).toBeVisible();
});

test("on macOS the whole screen drags the window, which has no title bar", () => {
  platform.isMac = true;
  render(<LoadingScreen status={{ state: "starting" }} />);

  expect(screen.getByRole("main")).toHaveClass("app-drag");
});
