import { fireEvent, render, renderHook } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { useHistoryShortcuts } from "./use-history-shortcuts";

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

function listen() {
  const back = vi.fn();
  const forward = vi.fn();
  renderHook(() => useHistoryShortcuts(back, forward));
  return { back, forward };
}

test.each([
  { mac: true, keys: { metaKey: true, code: "BracketLeft" }, move: "back" },
  { mac: true, keys: { metaKey: true, code: "BracketRight" }, move: "forward" },
  { mac: true, keys: { ctrlKey: true, code: "BracketLeft" }, move: null },
  { mac: true, keys: { metaKey: true, shiftKey: true, code: "BracketLeft" }, move: null },
  { mac: true, keys: { altKey: true, code: "ArrowLeft" }, move: null },
  { mac: false, keys: { altKey: true, code: "ArrowLeft" }, move: "back" },
  { mac: false, keys: { altKey: true, code: "ArrowRight" }, move: "forward" },
  { mac: false, keys: { altKey: true, ctrlKey: true, code: "ArrowLeft" }, move: null },
  { mac: false, keys: { metaKey: true, code: "BracketLeft" }, move: null },
])("on macOS $mac, $keys goes $move", ({ mac, keys, move }) => {
  platform.isMac = mac;
  const moves = listen();

  fireEvent.keyDown(window, keys);

  expect(moves.back).toHaveBeenCalledTimes(move === "back" ? 1 : 0);
  expect(moves.forward).toHaveBeenCalledTimes(move === "forward" ? 1 : 0);
});

test("the side buttons of the mouse go back and forward", () => {
  const moves = listen();

  fireEvent.mouseUp(window, { button: 3 });
  fireEvent.mouseUp(window, { button: 4 });
  fireEvent.mouseUp(window, { button: 0 });

  expect(moves.back).toHaveBeenCalledOnce();
  expect(moves.forward).toHaveBeenCalledOnce();
});

test("ignores the shortcuts while the user types text", () => {
  platform.isMac = true;
  const { getByRole } = render(<textarea aria-label="Message" />);
  const moves = listen();

  fireEvent.keyDown(getByRole("textbox"), { metaKey: true, code: "BracketLeft" });

  expect(moves.back).not.toHaveBeenCalled();
});
