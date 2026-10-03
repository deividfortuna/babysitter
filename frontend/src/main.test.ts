import { beforeEach, expect, test, vi } from "vite-plus/test";
import {
  APP_MENU_POPUP_CHANNEL,
  NOTIFICATIONS_CLICK_CHANNEL,
  NOTIFICATIONS_OPEN_READY_CHANNEL,
  NOTIFICATIONS_SHOW_CHANNEL,
  OPEN_IN_LAUNCH_CHANNEL,
  OPEN_IN_TARGETS_CHANNEL,
  QUIT_SHORTCUT_CHANNEL,
  THEME_FOLLOW_CHANNEL,
  UPDATES_GET_STATUS_CHANNEL,
  UPDATES_SET_SETTINGS_CHANNEL,
} from "./shared/ipc";
import { CANVAS, INK } from "./shared/theme";
import { MAC_WINDOW_BUTTON_HEIGHT, TITLEBAR_HEIGHT } from "./shared/titlebar";

type Handler = (...args: never[]) => unknown;

type Launcher = {
  openPath: (dir: string) => Promise<string>;
  watchFolder: (watchId: number) => Promise<string | null>;
};

const appDir = vi.hoisted(() => process.cwd());

const electron = vi.hoisted(() => ({
  windows: [] as { webContents: { inputListeners: Handler[] }; shown: boolean; focused: boolean }[],
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
  attention: 0,
  daemonStarts: 0,
  overlays: [] as Record<string, unknown>[],
  zoom: 1,
  popups: [] as Record<string, unknown>[],
  systemDark: false,
  themeUpdated: null as (() => void) | null,
  launches: [] as unknown[][],
  openedPaths: [] as string[],
  launcher: null as Launcher | null,
}));

vi.mock("./main/open-in", () => ({
  EditorLauncher: class {
    constructor(options: Launcher) {
      electron.launcher = options;
    }
    async targets() {
      return ["vscode", "file-manager"];
    }
    async launch(watchId: unknown, target: unknown) {
      electron.launches.push([watchId, target]);
      return target === "vscode" ? { ok: true } : { ok: false, error: "Zed is not installed." };
    }
  },
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

vi.mock("./main/daemon-supervisor", () => ({
  DaemonSupervisor: class {
    onStatus() {}
    getStatus() {
      return { state: "starting" };
    }
    async start() {
      electron.daemonStarts++;
    }
    async restart() {}
    stop() {
      return electron.daemonStops === "never" ? new Promise(() => undefined) : Promise.resolve();
    }
    async stopAndWait() {}
  },
}));

vi.mock("electron", () => {
  class FakeWebContents {
    inputListeners: Handler[] = [];
    send(channel: string, payload: unknown) {
      electron.sent.push({ channel, payload });
    }
    on(name: string, fn: Handler) {
      if (name === "before-input-event") this.inputListeners.push(fn);
    }
    setWindowOpenHandler() {}
    openDevTools() {}
    getZoomFactor() {
      return electron.zoom;
    }
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
    isDestroyed() {
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
    flashFrame(on: boolean) {
      if (on) electron.attention++;
    }
    loadURL() {}
    loadFile() {}
    setBackgroundColor() {}
    setTitleBarOverlay(options: Record<string, unknown>) {
      electron.overlays.push(options);
    }
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
      dock: {
        bounce: () => {
          electron.attention++;
          return 1;
        },
        cancelBounce() {},
        setBadge() {},
      },
      setBadgeCount() {},
      setAppUserModelId() {},
    },
    BrowserWindow: FakeBrowserWindow,
    dialog: { showOpenDialog: () => ({ canceled: true, filePaths: [] }) },
    ipcMain: {
      handle: (channel: string, fn: Handler) => electron.handlers.set(channel, fn),
      on: (channel: string, fn: Handler) => electron.listeners.set(channel, fn),
    },
    Menu: {
      buildFromTemplate: (template: unknown) => template,
      getApplicationMenu: () => ({ popup: (options: Record<string, unknown>) => electron.popups.push(options) }),
    },
    nativeTheme: {
      get shouldUseDarkColors() {
        return electron.systemDark;
      },
      themeSource: "system",
      on: (event: string, fn: () => void) => {
        if (event === "updated") electron.themeUpdated = fn;
      },
    },
    Notification: FakeNotification,
    shell: {
      openExternal: (url: string) => electron.opened.push(url),
      openPath: async (dir: string) => {
        electron.openedPaths.push(dir);
        return "";
      },
    },
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
  electron.attention = 0;
  electron.daemonStarts = 0;
  electron.overlays.length = 0;
  electron.zoom = 1;
  electron.popups.length = 0;
  electron.systemDark = false;
  electron.themeUpdated = null;
  electron.launches.length = 0;
  electron.openedPaths.length = 0;
  electron.launcher = null;
  vi.resetModules();
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

function followTheme(preference: string) {
  const listener = electron.listeners.get(THEME_FOLLOW_CHANNEL);
  if (!listener) throw new Error("the main process follows no theme");
  (listener as (event: unknown, preference: unknown) => unknown)(null, preference);
}

function popupMenu(anchor: unknown) {
  const listener = electron.listeners.get(APP_MENU_POPUP_CHANNEL);
  if (!listener) throw new Error("the main process opens no menu");
  (listener as (event: unknown, anchor: unknown) => unknown)({ sender: electron.windows[0].webContents }, anchor);
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

test("a notification while the window has the focus shows the banner and calls nobody back", () => {
  electron.appEvents.get("activate")?.();
  electron.windows[0].focused = true;

  show({ id: 4, title: "PR #7", body: "alice needs an answer", kind: "agent" });

  expect(electron.toasts).toHaveLength(1);
  expect(electron.attention).toBe(0);
});

test("a notification while the window is in the background shows the banner and calls you back", () => {
  electron.appEvents.get("activate")?.();

  show({ id: 4, title: "PR #7", body: "alice needs an answer", kind: "agent" });

  expect(electron.toasts).toHaveLength(1);
  expect(electron.attention).toBe(1);
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

test("on Windows the window draws no title bar or menu and puts its buttons over the app in the colors of the theme", async () => {
  vi.stubGlobal("process", { ...process, platform: "win32" });
  electron.theme = "dark";
  await loadMain();
  electron.appEvents.get("ready")?.();

  expect(electron.windowOptions[0]).toMatchObject({
    titleBarStyle: "hidden",
    titleBarOverlay: { color: CANVAS.dark, symbolColor: INK.dark, height: TITLEBAR_HEIGHT },
  });
  vi.unstubAllGlobals();
});

test("on Windows a change of theme paints the window buttons again", async () => {
  vi.stubGlobal("process", { ...process, platform: "win32" });
  electron.theme = "dark";
  await loadMain();
  electron.appEvents.get("ready")?.();

  followTheme("light");

  expect(electron.overlays.at(-1)).toEqual({ color: CANVAS.light, symbolColor: INK.light, height: TITLEBAR_HEIGHT });
  vi.unstubAllGlobals();
});

test("on Windows a change of the system theme paints the window buttons again", async () => {
  vi.stubGlobal("process", { ...process, platform: "win32" });
  electron.theme = "system";
  await loadMain();
  electron.appEvents.get("ready")?.();

  electron.systemDark = true;
  electron.themeUpdated?.();

  expect(electron.overlays.at(-1)).toEqual({ color: CANVAS.dark, symbolColor: INK.dark, height: TITLEBAR_HEIGHT });
  vi.unstubAllGlobals();
});

test("the menu of the app opens under the point the renderer gives, in the zoom of the page", () => {
  electron.appEvents.get("ready")?.();
  electron.zoom = 1.5;

  popupMenu({ x: 12, y: 38 });

  expect(electron.popups).toEqual([{ window: electron.windows[0], x: 18, y: 57 }]);
});

test("the menu of the app stays closed for a point it cannot read", () => {
  electron.appEvents.get("ready")?.();

  popupMenu({ x: "12", y: 38 });
  popupMenu(null);

  expect(electron.popups).toEqual([]);
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

test("the renderer lists the editors and opens the folder of a watch through the launcher", async () => {
  expect(await invoke(OPEN_IN_TARGETS_CHANNEL)).toEqual(["vscode", "file-manager"]);
  expect(await invoke(OPEN_IN_LAUNCH_CHANNEL, 42, "vscode")).toEqual({ ok: true });
  expect(await invoke(OPEN_IN_LAUNCH_CHANNEL, 42, "zed")).toEqual({
    ok: false,
    error: "Zed is not installed.",
  });

  expect(electron.launches).toEqual([
    [42, "vscode"],
    [42, "zed"],
  ]);
});

test("the file manager opens a folder through the shell of Electron", async () => {
  expect(await electron.launcher?.openPath("/work/pr-12")).toBe("");
  expect(electron.openedPaths).toEqual(["/work/pr-12"]);
});

test("a watch has no folder to open while no local daemon is ready", async () => {
  expect(await electron.launcher?.watchFolder(42)).toBeNull();
});

test("a build that is not packaged says it does not update itself", () => {
  expect(invoke(UPDATES_GET_STATUS_CHANNEL)).toEqual({ state: "unsupported", currentVersion: "0.0.0-test" });
});

test("the update choices keep only the keys with the right type", () => {
  expect(invoke(UPDATES_SET_SETTINGS_CHANNEL, { autoDownload: false, channel: "beta" })).toEqual({
    autoDownload: false,
    channel: "stable",
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

test("a double press of the quit shortcut in the window shows the hint and quits", () => {
  electron.appEvents.get("activate")?.();
  const [listener] = electron.windows[0].webContents.inputListeners;
  const onMac = process.platform === "darwin";
  const press = {
    type: "keyDown",
    key: "q",
    meta: onMac,
    control: !onMac,
    alt: false,
    shift: false,
    isAutoRepeat: false,
  };
  const event = { preventDefault: vi.fn() };

  (listener as (event: unknown, input: unknown) => void)(event, press);
  (listener as (event: unknown, input: unknown) => void)(event, press);

  expect(event.preventDefault).toHaveBeenCalledTimes(2);
  expect(electron.sent.filter((sent) => sent.channel === QUIT_SHORTCUT_CHANNEL)).toEqual([
    { channel: QUIT_SHORTCUT_CHANNEL, payload: { state: "down" } },
    { channel: QUIT_SHORTCUT_CHANNEL, payload: { state: "up" } },
  ]);
  expect(electron.quits).toBe(1);
});
