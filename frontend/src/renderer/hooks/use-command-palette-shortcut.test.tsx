import { fireEvent, render, renderHook } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { useCommandPaletteShortcut } from "./use-command-palette-shortcut";

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

function listen(allowed = true) {
  const onOpen = vi.fn();
  renderHook(() => useCommandPaletteShortcut(onOpen, allowed));
  return onOpen;
}

test.each([
  { mac: true, keys: { metaKey: true, key: "p" }, opens: true },
  { mac: true, keys: { metaKey: true, shiftKey: true, key: "P" }, opens: false },
  { mac: true, keys: { ctrlKey: true, key: "p" }, opens: false },
  { mac: true, keys: { metaKey: true, altKey: true, key: "p" }, opens: false },
  { mac: true, keys: { metaKey: true, key: "o" }, opens: false },
  { mac: false, keys: { ctrlKey: true, key: "p" }, opens: true },
  { mac: false, keys: { ctrlKey: true, shiftKey: true, key: "P" }, opens: false },
  { mac: false, keys: { metaKey: true, key: "p" }, opens: false },
  { mac: false, keys: { ctrlKey: true, metaKey: true, key: "p" }, opens: false },
])("on macOS $mac, $keys opens the palette: $opens", ({ mac, keys, opens }) => {
  platform.isMac = mac;
  const onOpen = listen();

  fireEvent.keyDown(window, keys);

  expect(onOpen).toHaveBeenCalledTimes(opens ? 1 : 0);
});

test("follows the character p, not the position of the key", () => {
  const onOpen = listen();

  fireEvent.keyDown(window, { ctrlKey: true, key: "p", code: "KeyR" });
  fireEvent.keyDown(window, { ctrlKey: true, key: "l", code: "KeyP" });

  expect(onOpen).toHaveBeenCalledOnce();
});

test("falls back to the position of the key when the layout types no Latin letter", () => {
  const onOpen = listen();

  fireEvent.keyDown(window, { ctrlKey: true, key: "з", code: "KeyP" });

  expect(onOpen).toHaveBeenCalledOnce();
});

test("opens while the user types text", () => {
  platform.isMac = true;
  const { getByRole } = render(<textarea aria-label="Message" />);
  const onOpen = listen();

  fireEvent.keyDown(getByRole("textbox"), { metaKey: true, key: "p" });

  expect(onOpen).toHaveBeenCalledOnce();
});

test("does not open over another dialog", () => {
  render(<div role="dialog" aria-label="Settings" />);
  const onOpen = listen();

  fireEvent.keyDown(window, { ctrlKey: true, key: "p" });

  expect(onOpen).not.toHaveBeenCalled();
});

test("does not open while it is not allowed", () => {
  const onOpen = listen(false);

  fireEvent.keyDown(window, { ctrlKey: true, key: "p" });

  expect(onOpen).not.toHaveBeenCalled();
});
