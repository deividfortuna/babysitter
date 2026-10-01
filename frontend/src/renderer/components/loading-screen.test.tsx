import { render, screen } from "@testing-library/react";
import { expect, test } from "vite-plus/test";
import { LoadingScreen } from "./loading-screen";

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

test("the whole screen drags the window, which has no title bar", () => {
  render(<LoadingScreen status={{ state: "starting" }} />);

  expect(screen.getByRole("main")).toHaveClass("app-drag");
});
