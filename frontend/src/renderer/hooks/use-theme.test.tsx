import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, test, vi } from "vitest";
import { stubMatchMedia } from "@test/test-utils";
import { bridge } from "@/lib/bridge";
import { ThemeProvider, useTheme } from "./use-theme";

function Probe() {
  const { preference, theme, setPreference } = useTheme();
  return (
    <div>
      <span data-testid="state">{`${preference}/${theme}`}</span>
      <button onClick={() => setPreference("light")}>go light</button>
    </div>
  );
}

function renderProbe() {
  return render(
    <ThemeProvider>
      <Probe />
    </ThemeProvider>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
});

test("starts on the system preference and resolves it against the colour scheme", () => {
  stubMatchMedia(true);

  renderProbe();

  expect(screen.getByTestId("state")).toHaveTextContent("system/dark");
  expect(document.documentElement).toHaveClass("dark");
});

test("starts on the stored preference and ignores the colour scheme", () => {
  stubMatchMedia(true);
  window.localStorage.setItem("theme", "light");

  renderProbe();

  expect(screen.getByTestId("state")).toHaveTextContent("light/light");
  expect(document.documentElement).not.toHaveClass("dark");
});

test("a new preference lands on the root and in storage", async () => {
  stubMatchMedia(true);
  const user = userEvent.setup();

  renderProbe();
  await user.click(screen.getByRole("button", { name: "go light" }));

  expect(screen.getByTestId("state")).toHaveTextContent("light/light");
  expect(document.documentElement).not.toHaveClass("dark");
  expect(window.localStorage.getItem("theme")).toBe("light");
});

test("follows the system when the colour scheme changes under the system preference", () => {
  const media = stubMatchMedia(false);

  renderProbe();
  expect(document.documentElement).not.toHaveClass("dark");

  act(() => media.set(true));

  expect(screen.getByTestId("state")).toHaveTextContent("system/dark");
  expect(document.documentElement).toHaveClass("dark");
});

test("hands the preference to the native chrome", async () => {
  const follow = vi.spyOn(bridge.theme, "follow").mockImplementation(() => undefined);
  const user = userEvent.setup();

  renderProbe();
  await user.click(screen.getByRole("button", { name: "go light" }));

  expect(follow).toHaveBeenCalledWith("system");
  expect(follow).toHaveBeenLastCalledWith("light");
});
