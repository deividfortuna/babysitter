import { beforeEach, expect, test, vi } from "vitest";
import {
  NOTIFICATIONS_CLICK_CHANNEL,
  NOTIFICATIONS_OPEN_READY_CHANNEL,
  NOTIFICATIONS_SHOW_CHANNEL,
  UPDATES_GET_STATUS_CHANNEL,
  UPDATES_SET_SETTINGS_CHANNEL,
} from "./shared/ipc";
import { MAC_WINDOW_BUTTON_HEIGHT, TITLEBAR_HEIGHT } from "./shared/titlebar";

type Handler = (...args: never[]) => unknown;

const appDir = vi.hoisted(() => process.cwd());

const electron = vi.hoisted(() => ({
  windows: [] as { webContents: unknown; shown: boolean }[],
  toasts: [] as { click: () => void }[],
  sent: [] as { channel: string; payload: unknown }[],
  opened: [] as string[],
  handlers: new Map<string, (...args: never[]) => unknown>(),
  listeners: new Map<string, (...args: never[]) => unknown>(),
  appEvents: new Map<string, (...args: never[]) => unknown>(),
  quits: 0,
  windowOptions: [] as Record<string, unknown>[],
  theme: "system" as string,
  trays: 0,
  exits: [] as number[],
  daemonStops: "at once" as "at once" | "never",
  updateSettings: {} as Record<string, unknown>,
}));

vi.mock("./main/update-settings", () => ({
  readUpdateSettings: () => electron.updateSettings,
  writeUpdateSettings: (_dir: string, settings: Record<string, unknown>) => {
    electron.updateSettings = settings;
  },
}));

vi.mock("electron-updater", () => ({ autoUpdater: {} }));

vi.mock("./main/theme-preference", () => ({
  readStoredTheme: () => electron.theme,
  writeStoredTheme: (_dir: string, preference: string) => {
    electron.theme = preference;
  },
}));

vi.mock("electron-squirrel-startup", () => ({ default: false }));

vi.mock("./main/daemon-supervisor", () => ({
  DaemonSupervisor: class {
    onStatus() {}
    getStatus() {
      return { state: "starting" };
    }
    async start() {}
    async restart() {}
    stop() {
      return electron.daemonStops === "never" ? new Promise(() => undefined) : Promise.resolve();
    }
    async stopAndWait() {}
  },
}));

vi.mock("electron", () => {
  class FakeWebContents {
    send(channel: string, payload: unknown) {
      electron.sent.push({ channel, payload });
    }
    setWindowOpenHandler() {}
    openDevTools() {}
  }
  class FakeBrowserWindow {
    webContents = new FakeWebContents();
    shown = false;
    focused = false;
    constructor(options: Record<string, unknown>) {
      electron.windowOptions.push(options);
      electron.windows.push(this);
    }
    static getAllWindows() {
      return electron.windows;
    }
    static fromWebContents() {
      return electron.windows[0];
    }
    isFocused() {
      return this.focused;
    }
    isMinimized() {
      return false;
    }
    restore() {}
    show() {
      this.shown = true;
    }
    focus() {
      this.focused = true;
    }
    on() {}
    once() {}
    flashFrame() {}
    loadURL() {}
    loadFile() {}
  }
  class FakeNotification {
    private clicked: (() => void) | null = null;
    constructor() {
      electron.toasts.push({ click: () => this.clicked?.() });
    }
    static isSupported() {
      return true;
    }
    on(name: string, fn: () => void) {
      if (name === "click") this.clicked = fn;
    }
    show() {}
  }
  return {
    autoUpdater: { on() {} },
    app: {
      isPackaged: false,
      getAppPath: () => appDir,
      getVersion: () => "0.0.0-test",
      quit() {
        electron.quits++;
      },
      exit(code: number) {
        electron.exits.push(code);
      },
      on: (name: string, fn: Handler) => electron.appEvents.set(name, fn),
      dock: { bounce: () => 1, cancelBounce() {}, setBadge() {} },
      setBadgeCount() {},
    },
    BrowserWindow: FakeBrowserWindow,
    dialog: { showOpenDialog: () => ({ canceled: true, filePaths: [] }) },
    ipcMain: {
      handle: (channel: string, fn: Handler) => electron.handlers.set(channel, fn),
      on: (channel: string, fn: Handler) => electron.listeners.set(channel, fn),
    },
    Menu: { buildFromTemplate: (template: unknown) => template },
    nativeTheme: { shouldUseDarkColors: false, themeSource: "system" },
    Notification: FakeNotification,
    shell: { openExternal: (url: string) => electron.opened.push(url) },
    Tray: class {
      constructor() {
        electron.trays++;
      }
      on() {}
      setToolTip() {}
      setTitle() {}
      setContextMenu() {}
    },
  };
});

async function loadMain() {
  electron.windows.length = 0;
  electron.toasts.length = 0;
  electron.sent.length = 0;
  electron.opened.length = 0;
  electron.handlers.clear();
  electron.listeners.clear();
  electron.appEvents.clear();
  electron.windowOptions.length = 0;
  electron.quits = 0;
  electron.trays = 0;
  electron.exits.length = 0;
  electron.daemonStops = "at once";
  electron.updateSettings = {};
  vi.resetModules();
  vi.stubGlobal("MAIN_WINDOW_VITE_DEV_SERVER_URL", undefined);
  vi.stubGlobal("MAIN_WINDOW_VITE_NAME", "main_window");
  await import("./main");
}

function show(notification: Record<string, unknown>) {
  const handler = electron.handlers.get(NOTIFICATIONS_SHOW_CHANNEL);
  if (!handler) throw new Error("the main process took no notification handler");
  (handler as (event: unknown, notification: unknown) => unknown)(null, notification);
}

function rendererReady(index = 0) {
  const listener = electron.listeners.get(NOTIFICATIONS_OPEN_READY_CHANNEL);
  if (!listener) throw new Error("the main process listens for no renderer");
  (listener as (event: unknown) => unknown)({ sender: electron.windows[index].webContents });
}

beforeEach(async () => {
  await loadMain();
});

test("a click on a banner with no window open makes one and takes the click to it", () => {
  show({ id: 4, title: "PR #7", body: "build failed", watchId: 42 });
  electron.toasts[0].click();

  expect(electron.windows).toHaveLength(1);
  rendererReady();
  expect(electron.sent).toEqual([{ channel: NOTIFICATIONS_CLICK_CHANNEL, payload: { id: 4, watchId: 42 } }]);
});

test("a click on a banner takes the click to the window that stands", () => {
  electron.appEvents.get("activate")?.();
  rendererReady();
  show({ id: 4, title: "PR #7", body: "build failed", watchId: 42 });
  electron.toasts[0].click();

  expect(electron.windows).toHaveLength(1);
  expect(electron.sent).toEqual([{ channel: NOTIFICATIONS_CLICK_CHANNEL, payload: { id: 4, watchId: 42 } }]);
});

test("a click on a banner of a pull request opens it and marks the row read", () => {
  electron.appEvents.get("activate")?.();
  rendererReady();
  show({ id: 7, title: "PR #7", body: "alice needs an answer", url: "https://github.com/octo/hello/pull/7" });
  electron.toasts[0].click();

  expect(electron.opened).toEqual(["https://github.com/octo/hello/pull/7"]);
  expect(electron.sent).toEqual([{ channel: NOTIFICATIONS_CLICK_CHANNEL, payload: { id: 7, watchId: undefined } }]);
});

test("a click that only opens the browser holds nothing back for a later window", () => {
  show({ id: 7, title: "PR #7", body: "alice needs an answer", url: "https://github.com/octo/hello/pull/7" });
  electron.toasts[0].click();
  expect(electron.windows).toHaveLength(0);

  electron.appEvents.get("activate")?.();
  rendererReady();

  expect(electron.sent).toEqual([]);
});

test("the window paints in the theme the user chose, not the one of the system", async () => {
  electron.theme = "dark";
  await loadMain();
  electron.appEvents.get("ready")?.();

  expect(electron.windowOptions[0].backgroundColor).toBe("#0d1117");
});

test("on macOS the window buttons sit in the middle of the title bar", async () => {
  vi.stubGlobal("process", { ...process, platform: "darwin" });
  await loadMain();
  electron.appEvents.get("ready")?.();

  const position = electron.windowOptions[0].trafficLightPosition as { y: number };
  expect(position.y * 2 + MAC_WINDOW_BUTTON_HEIGHT).toBe(TITLEBAR_HEIGHT);
  vi.unstubAllGlobals();
});

test("closing the window where a menu bar item runs leaves the app alive", async () => {
  vi.stubGlobal("process", { ...process, platform: "linux" });
  await loadMain();
  electron.appEvents.get("ready")?.();

  electron.appEvents.get("window-all-closed")?.();

  expect({ quits: electron.quits, trays: electron.trays }).toEqual({ quits: 0, trays: 1 });
  vi.unstubAllGlobals();
});

function invoke(channel: string, ...args: unknown[]): unknown {
  const handler = electron.handlers.get(channel);
  if (!handler) throw new Error(`the main process has no handler for ${channel}`);
  return (handler as (event: unknown, ...args: unknown[]) => unknown)(null, ...args);
}

test("a build that is not packaged says it does not update itself", () => {
  expect(invoke(UPDATES_GET_STATUS_CHANNEL)).toEqual({ state: "unsupported", currentVersion: "0.0.0-test" });
});

test("the update choices keep only the keys with the right type", () => {
  expect(invoke(UPDATES_SET_SETTINGS_CHANNEL, { autoDownload: false, channel: "nightly" })).toEqual({
    autoDownload: false,
    channel: "prerelease",
  });
  expect(electron.updateSettings).toEqual({ autoDownload: false });
});

test("a quit that the daemon holds up ends anyway after 15 seconds, so a waiting update can install", async () => {
  vi.useFakeTimers();
  electron.daemonStops = "never";
  const event = { preventDefault: vi.fn() };

  electron.appEvents.get("before-quit")?.(event as never);
  await vi.advanceTimersByTimeAsync(14_999);
  expect(electron.exits).toEqual([]);
  await vi.advanceTimersByTimeAsync(1);

  expect(event.preventDefault).toHaveBeenCalledOnce();
  expect(electron.exits).toEqual([0]);
  vi.useRealTimers();
});

test("a quit that the daemon lets go of quits at once", async () => {
  const event = { preventDefault: vi.fn() };

  electron.appEvents.get("before-quit")?.(event as never);
  await vi.waitFor(() => expect(electron.quits).toBe(1));

  expect(electron.exits).toEqual([]);
});
