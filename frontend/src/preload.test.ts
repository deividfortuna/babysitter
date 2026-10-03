import { beforeEach, expect, test, vi } from "vite-plus/test";
import type { BabysitterBridge } from "./preload";
import {
  APP_MENU_POPUP_CHANNEL,
  OPEN_IN_LAUNCH_CHANNEL,
  OPEN_IN_TARGETS_CHANNEL,
  QUIT_SHORTCUT_CHANNEL,
  UPDATES_CHECK_CHANNEL,
  UPDATES_DOWNLOAD_CHANNEL,
  UPDATES_GET_SETTINGS_CHANNEL,
  UPDATES_GET_STATUS_CHANNEL,
  UPDATES_INSTALL_CHANNEL,
  UPDATES_SET_SETTINGS_CHANNEL,
  UPDATES_STATUS_CHANNEL,
} from "./shared/ipc";

const electron = vi.hoisted(() => ({
  exposed: null as unknown,
  invoked: [] as unknown[][],
  sent: [] as unknown[][],
  listeners: new Map<string, (...args: unknown[]) => void>(),
}));

vi.mock("electron", () => ({
  contextBridge: {
    exposeInMainWorld: (_name: string, api: unknown) => {
      electron.exposed = api;
    },
  },
  ipcRenderer: {
    invoke: async (...args: unknown[]) => {
      electron.invoked.push(args);
    },
    send: (...args: unknown[]) => {
      electron.sent.push(args);
    },
    on: (channel: string, handler: (...args: unknown[]) => void) => electron.listeners.set(channel, handler),
    off: (channel: string) => electron.listeners.delete(channel),
  },
}));

let bridge: BabysitterBridge;

beforeEach(async () => {
  electron.invoked.length = 0;
  electron.sent.length = 0;
  electron.listeners.clear();
  vi.resetModules();
  await import("./preload");
  bridge = electron.exposed as BabysitterBridge;
});

test("the menu goes to the main process with the point to open it at", () => {
  bridge.app.popupMenu({ x: 12, y: 38 });

  expect(electron.sent).toEqual([[APP_MENU_POPUP_CHANNEL, { x: 12, y: 38 }]]);
});

test("each update call goes to its channel", async () => {
  await bridge.updates.getStatus();
  await bridge.updates.check();
  await bridge.updates.download();
  await bridge.updates.install();
  await bridge.updates.getSettings();
  await bridge.updates.setSettings({ channel: "stable" });

  expect(electron.invoked).toEqual([
    [UPDATES_GET_STATUS_CHANNEL],
    [UPDATES_CHECK_CHANNEL],
    [UPDATES_DOWNLOAD_CHANNEL],
    [UPDATES_INSTALL_CHANNEL],
    [UPDATES_GET_SETTINGS_CHANNEL],
    [UPDATES_SET_SETTINGS_CHANNEL, { channel: "stable" }],
  ]);
});

test("the editor calls go to their channels with the watch and the editor", async () => {
  await bridge.openIn.targets();
  await bridge.openIn.launch(42, "vscode");

  expect(electron.invoked).toEqual([[OPEN_IN_TARGETS_CHANNEL], [OPEN_IN_LAUNCH_CHANNEL, 42, "vscode"]]);
});

test("the update status reaches the listener until it lets go", () => {
  const listener = vi.fn();
  const off = bridge.updates.onStatus(listener);

  electron.listeners.get(UPDATES_STATUS_CHANNEL)?.(
    {},
    { state: "available", currentVersion: "0.1.0", version: "0.2.0" },
  );
  off();

  expect(listener).toHaveBeenCalledWith({ state: "available", currentVersion: "0.1.0", version: "0.2.0" });
  expect(electron.listeners.has(UPDATES_STATUS_CHANNEL)).toBe(false);
});

test("the quit hint reaches the listener only when it has a known state", () => {
  const listener = vi.fn();
  const off = bridge.quit.onShortcut(listener);

  electron.listeners.get(QUIT_SHORTCUT_CHANNEL)?.({}, { state: "down" });
  electron.listeners.get(QUIT_SHORTCUT_CHANNEL)?.({}, { state: "sideways" });
  electron.listeners.get(QUIT_SHORTCUT_CHANNEL)?.({}, null);
  off();

  expect(listener.mock.calls).toEqual([[{ state: "down" }]]);
  expect(electron.listeners.has(QUIT_SHORTCUT_CHANNEL)).toBe(false);
});
