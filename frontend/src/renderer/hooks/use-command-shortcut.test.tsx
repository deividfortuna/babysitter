import { fireEvent, renderHook } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vite-plus/test";
import { useCommandShortcut } from "./use-command-shortcut";

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

function listenForCtrlBackquote() {
  const onPress = vi.fn();
  renderHook(() => useCommandShortcut("`", onPress, true, { ctrl: true }));
  return onPress;
}

test.each([
  { mac: true, keys: { ctrlKey: true, key: "`", code: "Backquote" }, runs: true },
  { mac: true, keys: { metaKey: true, key: "`", code: "Backquote" }, runs: false },
  { mac: false, keys: { ctrlKey: true, key: "`", code: "Backquote" }, runs: true },
  { mac: false, keys: { ctrlKey: true, shiftKey: true, key: "~", code: "Backquote" }, runs: false },
  { mac: false, keys: { ctrlKey: true, key: "Dead", code: "Backquote" }, runs: true },
  { mac: false, keys: { ctrlKey: true, key: "'", code: "Quote" }, runs: false },
])("with ctrl, on macOS $mac, $keys runs: $runs", ({ mac, keys, runs }) => {
  platform.isMac = mac;
  const onPress = listenForCtrlBackquote();

  fireEvent.keyDown(window, keys);

  expect(onPress).toHaveBeenCalledTimes(runs ? 1 : 0);
});

test.each([
  { name: "a US layout", mac: false, keys: { ctrlKey: true, key: ".", code: "Period" }, runs: true },
  {
    name: "a French AZERTY layout, where Shift types the period",
    mac: false,
    keys: { ctrlKey: true, shiftKey: true, key: ".", code: "Comma" },
    runs: true,
  },
  {
    name: "a French AZERTY layout on macOS",
    mac: true,
    keys: { metaKey: true, shiftKey: true, key: ".", code: "Comma" },
    runs: true,
  },
  {
    name: "Shift on a US layout",
    mac: false,
    keys: { ctrlKey: true, shiftKey: true, key: ">", code: "Period" },
    runs: false,
  },
])("the period on $name runs: $runs", ({ mac, keys, runs }) => {
  platform.isMac = mac;
  const onPress = vi.fn();
  renderHook(() => useCommandShortcut(".", onPress, true));

  fireEvent.keyDown(window, keys);

  expect(onPress).toHaveBeenCalledTimes(runs ? 1 : 0);
});

test("Shift does not stand in for a letter", () => {
  const onPress = vi.fn();
  renderHook(() => useCommandShortcut("p", onPress, true));

  fireEvent.keyDown(window, { ctrlKey: true, shiftKey: true, key: "P", code: "KeyP" });

  expect(onPress).not.toHaveBeenCalled();
});
