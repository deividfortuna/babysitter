import { fireEvent, render, renderHook } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { historyMouseButton, historyShortcut, useHistoryShortcuts } from "./use-history-shortcuts";

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

const noModifiers = { metaKey: false, ctrlKey: false, altKey: false, shiftKey: false };

test("on macOS, Command with the brackets goes back and forward", () => {
  expect(historyShortcut({ ...noModifiers, metaKey: true, code: "BracketLeft", key: "[" }, true)).toBe("back");
  expect(historyShortcut({ ...noModifiers, metaKey: true, code: "BracketRight", key: "]" }, true)).toBe("forward");
  expect(historyShortcut({ ...noModifiers, ctrlKey: true, code: "BracketLeft", key: "[" }, true)).toBeNull();
  expect(historyShortcut({ ...noModifiers, altKey: true, code: "ArrowLeft", key: "ArrowLeft" }, true)).toBeNull();
  expect(
    historyShortcut({ ...noModifiers, metaKey: true, shiftKey: true, code: "BracketLeft", key: "{" }, true),
  ).toBeNull();
});

test("on Windows and Linux, Alt with the arrows goes back and forward", () => {
  expect(historyShortcut({ ...noModifiers, altKey: true, code: "ArrowLeft", key: "ArrowLeft" }, false)).toBe("back");
  expect(historyShortcut({ ...noModifiers, altKey: true, code: "ArrowRight", key: "ArrowRight" }, false)).toBe(
    "forward",
  );
  expect(historyShortcut({ ...noModifiers, metaKey: true, code: "BracketLeft", key: "[" }, false)).toBeNull();
  expect(
    historyShortcut({ ...noModifiers, altKey: true, ctrlKey: true, code: "ArrowLeft", key: "ArrowLeft" }, false),
  ).toBeNull();
});

test("the side buttons of the mouse go back and forward", () => {
  expect(historyMouseButton(3)).toBe("back");
  expect(historyMouseButton(4)).toBe("forward");
  expect(historyMouseButton(0)).toBeNull();
});

test("moves on the shortcuts and the mouse side buttons", () => {
  platform.isMac = true;
  const back = vi.fn();
  const forward = vi.fn();
  renderHook(() => useHistoryShortcuts(back, forward));

  fireEvent.keyDown(window, { metaKey: true, code: "BracketLeft", key: "[" });
  fireEvent.mouseUp(window, { button: 4 });

  expect(back).toHaveBeenCalledOnce();
  expect(forward).toHaveBeenCalledOnce();
});

test("ignores the shortcuts while the user types text", () => {
  platform.isMac = true;
  const back = vi.fn();
  const { getByRole } = render(<textarea aria-label="Message" />);
  renderHook(() => useHistoryShortcuts(back, vi.fn()));

  fireEvent.keyDown(getByRole("textbox"), { metaKey: true, code: "BracketLeft", key: "[" });

  expect(back).not.toHaveBeenCalled();
});

test("stops listening when it unmounts", () => {
  platform.isMac = true;
  const back = vi.fn();
  const { unmount } = renderHook(() => useHistoryShortcuts(back, vi.fn()));

  unmount();
  fireEvent.keyDown(window, { metaKey: true, code: "BracketLeft", key: "[" });

  expect(back).not.toHaveBeenCalled();
});
