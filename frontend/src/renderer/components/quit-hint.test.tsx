import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vite-plus/test";
import { bridge } from "@/lib/bridge";
import type { QuitShortcutHint } from "../../shared/quit";
import { QUIT_HINT_LINGER_MS, QuitHint } from "./quit-hint";

let push: (hint: QuitShortcutHint) => void;
const off = vi.fn();

beforeEach(() => {
  vi.useFakeTimers();
  off.mockClear();
  vi.spyOn(bridge.quit, "onShortcut").mockImplementation((listener) => {
    push = listener;
    return off;
  });
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

test("the hint shows while the quit shortcut is down", () => {
  render(<QuitHint />);
  expect(screen.queryByRole("status")).toBeNull();

  act(() => push({ state: "down" }));

  expect(screen.getByRole("status")).toHaveTextContent(/press it again to quit/);
});

test("the hint stays a moment after the key goes up, then goes", () => {
  render(<QuitHint />);
  act(() => push({ state: "down" }));
  act(() => push({ state: "up" }));

  act(() => {
    vi.advanceTimersByTime(QUIT_HINT_LINGER_MS - 1);
  });
  expect(screen.getByRole("status")).toBeInTheDocument();
  act(() => {
    vi.advanceTimersByTime(1);
  });
  expect(screen.queryByRole("status")).toBeNull();
});

test("a new press keeps the hint that was about to go", () => {
  render(<QuitHint />);
  act(() => push({ state: "down" }));
  act(() => push({ state: "up" }));
  act(() => push({ state: "down" }));

  act(() => {
    vi.advanceTimersByTime(QUIT_HINT_LINGER_MS);
  });

  expect(screen.getByRole("status")).toBeInTheDocument();
});

test("the subscription ends with the component", () => {
  const { unmount } = render(<QuitHint />);

  unmount();

  expect(off).toHaveBeenCalledOnce();
});
