import { beforeEach, expect, test, vi } from "vitest";
import type { BabysitterBridge } from "./preload";
import {
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
    send() {},
    on: (channel: string, handler: (...args: unknown[]) => void) => electron.listeners.set(channel, handler),
    off: (channel: string) => electron.listeners.delete(channel),
  },
}));

let bridge: BabysitterBridge;

beforeEach(async () => {
  electron.invoked.length = 0;
  electron.listeners.clear();
  vi.resetModules();
  await import("./preload");
  bridge = electron.exposed as BabysitterBridge;
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
