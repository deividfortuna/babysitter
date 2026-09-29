import {
  app,
  autoUpdater as nativeUpdater,
  BrowserWindow,
  dialog,
  ipcMain,
  Menu,
  nativeTheme,
  Notification,
  shell,
  Tray,
  type IpcMainInvokeEvent,
} from "electron";
import { existsSync, mkdirSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import started from "electron-squirrel-startup";
import { autoUpdater } from "electron-updater";
import { createUpdateController } from "./main/app-updater";
import { AppLog } from "./main/app-log";
import { DaemonSupervisor } from "./main/daemon-supervisor";
import { readUpdateSettings, writeUpdateSettings } from "./main/update-settings";
import { killLoginShells, shellRunner } from "./main/login-shell";
import { openQueue } from "./main/pending-open";
import { defaultDataDir } from "./shared/daemon-discovery";
import { resolveDaemonLaunch } from "./shared/daemon-launch";
import { daemonEnvOnce } from "./shared/shell-env";
import {
  APP_GET_VERSION_CHANNEL,
  DAEMON_GET_STATUS_CHANNEL,
  DAEMON_RESTART_CHANNEL,
  DAEMON_STATUS_CHANNEL,
  DIALOG_PICK_DIRECTORY_CHANNEL,
  LOGS_APP_RECORD_CHANNEL,
  LOGS_APP_RECORDS_CHANNEL,
  LOGS_OPEN_FOLDER_CHANNEL,
  NOTIFICATIONS_BADGE_CHANNEL,
  NOTIFICATIONS_CLICK_CHANNEL,
  NOTIFICATIONS_OPEN_CHANNEL,
  NOTIFICATIONS_OPEN_READY_CHANNEL,
  NOTIFICATIONS_SHOW_CHANNEL,
  NOTIFICATIONS_SUPPORTED_CHANNEL,
  THEME_FOLLOW_CHANNEL,
  UPDATES_CHECK_CHANNEL,
  UPDATES_DOWNLOAD_CHANNEL,
  UPDATES_GET_SETTINGS_CHANNEL,
  UPDATES_GET_STATUS_CHANNEL,
  UPDATES_INSTALL_CHANNEL,
  UPDATES_SET_SETTINGS_CHANNEL,
  UPDATES_STATUS_CHANNEL,
} from "./shared/ipc";
import { isDaemonLogRecord, type OpenLogFolderResult } from "./shared/logs";
import { parseSettingsPatch } from "./shared/updates";
import {
  badgeText,
  clickPlan,
  presentation,
  shouldReplaceBounce,
  trayTooltip,
  type DesktopNotification,
  type Presentation,
} from "./shared/notifications";
import { canvasColor, isThemePreference, type ThemePreference } from "./shared/theme";
import { MAC_WINDOW_BUTTON_POSITION } from "./shared/titlebar";
import { readStoredTheme, writeStoredTheme } from "./main/theme-preference";

if (started) {
  app.quit();
}

const dataDir = defaultDataDir(process.platform, process.env, os.homedir());

let themePreference: ThemePreference = readStoredTheme(dataDir);

const devTerminal = app.isPackaged ? () => undefined : (line: string) => console.log(line);

const appLog = new AppLog({ file: path.join(dataDir, "logs", "app.log"), echo: devTerminal });

function routeDaemonOutput(line: string) {
  if (isDaemonLogRecord(line)) {
    devTerminal(`daemon: ${line}`);
    return;
  }
  appLog.warn(`daemon: ${line}`);
}

const LOGIN_SHELL_TIMEOUT_MS = 10_000;

const daemon = new DaemonSupervisor({
  launch: resolveDaemonLaunch(process.env, app.isPackaged, process.resourcesPath, app.getAppPath(), process.platform),
  dataDir,
  env: daemonEnvOnce({
    platform: process.platform,
    env: process.env,
    home: os.homedir(),
    run: shellRunner(LOGIN_SHELL_TIMEOUT_MS),
    log: (msg) => appLog.info(msg),
  }),
  log: (msg) => appLog.info(msg),
  output: routeDaemonOutput,
});

function broadcast(channel: string, payload: unknown) {
  for (const win of BrowserWindow.getAllWindows()) {
    win.webContents.send(channel, payload);
  }
}

daemon.onStatus((status) => {
  if (status.state === "error") appLog.error(`daemon: ${status.message}`);
  broadcast(DAEMON_STATUS_CHANNEL, status);
});

appLog.onRecord((record) => broadcast(LOGS_APP_RECORD_CHANNEL, record));

ipcMain.handle(LOGS_APP_RECORDS_CHANNEL, () => appLog.records());
ipcMain.handle(LOGS_OPEN_FOLDER_CHANNEL, async (): Promise<OpenLogFolderResult> => {
  mkdirSync(appLog.folder, { recursive: true, mode: 0o750 });
  const error = await shell.openPath(appLog.folder);
  return error ? { ok: false, error } : { ok: true };
});

ipcMain.handle(DAEMON_GET_STATUS_CHANNEL, () => daemon.getStatus());
ipcMain.handle(DAEMON_RESTART_CHANNEL, () => daemon.restart());
ipcMain.handle(APP_GET_VERSION_CHANNEL, () => app.getVersion());

const DAEMON_STOP_BEFORE_INSTALL_MS = 10_000;
const QUIT_CAP_MS = 15_000;

function updatesSupported(): boolean {
  const feedSettings = path.join(process.resourcesPath ?? "", "app-update.yml");
  return process.platform === "darwin" && app.isPackaged && existsSync(feedSettings);
}

const updates = createUpdateController({
  supported: updatesSupported(),
  currentVersion: app.getVersion(),
  engine: () => autoUpdater,
  native: nativeUpdater,
  settings: {
    read: () => readUpdateSettings(dataDir),
    write: (settings) => writeUpdateSettings(dataDir, settings),
  },
  feedUrl: process.env.BABYSITTER_UPDATE_FEED_URL?.trim() || undefined,
  beforeInstall: () => {
    killLoginShells();
    return daemon.stopAndWait(DAEMON_STOP_BEFORE_INSTALL_MS);
  },
  onInstallFailed: () => void daemon.start(),
  log: (msg) => appLog.info(msg),
});

updates.onStatus((status) => broadcast(UPDATES_STATUS_CHANNEL, status));

ipcMain.handle(UPDATES_GET_STATUS_CHANNEL, () => updates.getStatus());
ipcMain.handle(UPDATES_CHECK_CHANNEL, async () => {
  await updates.check();
});
ipcMain.handle(UPDATES_DOWNLOAD_CHANNEL, async () => {
  await updates.download();
});
ipcMain.handle(UPDATES_INSTALL_CHANNEL, () => updates.install());
ipcMain.handle(UPDATES_GET_SETTINGS_CHANNEL, () => updates.getSettings());
ipcMain.handle(UPDATES_SET_SETTINGS_CHANNEL, (_event, patch: unknown) =>
  updates.setSettings(parseSettingsPatch(patch)),
);

ipcMain.on(THEME_FOLLOW_CHANNEL, (_event, preference: unknown) => {
  if (!isThemePreference(preference)) return;
  nativeTheme.themeSource = preference;
  themePreference = preference;
  writeStoredTheme(dataDir, preference);
  const canvas = canvasColor(preference, nativeTheme.shouldUseDarkColors);
  for (const win of BrowserWindow.getAllWindows()) {
    win.setBackgroundColor(canvas);
  }
});

ipcMain.handle(NOTIFICATIONS_SUPPORTED_CHANNEL, () => Notification.isSupported());

function appIconPath(): string | undefined {
  if (process.platform === "darwin") return undefined;
  const file = app.isPackaged
    ? path.join(process.resourcesPath, "icon.png")
    : path.join(app.getAppPath(), "assets", "icon.png");
  return existsSync(file) ? file : undefined;
}

let pendingBounce: { id: number; critical: boolean } | null = null;
let flashing = false;

function cancelBounce() {
  if (pendingBounce === null) return;
  const { id } = pendingBounce;
  pendingBounce = null;
  app.dock?.cancelBounce(id);
}

app.on("browser-window-focus", cancelBounce);

ipcMain.handle(NOTIFICATIONS_SHOW_CHANNEL, (_event, notification: DesktopNotification) => {
  const win = BrowserWindow.getAllWindows()[0];
  const shows = presentation(notification, Notification.isSupported(), process.platform);
  if (shows.toast) {
    const toast = new Notification({
      title: notification.title,
      body: notification.body,
      icon: appIconPath(),
      silent: notification.silent ?? false,
    });
    toast.on("click", () => openFromToast(notification));
    toast.show();
  }
  const inFront = win?.isFocused() ?? false;
  if (!inFront) signalAttention(win, shows);
});

function openFromToast(notification: DesktopNotification) {
  const plan = clickPlan(notification);
  if (plan.browser) void shell.openExternal(plan.browser);
  if (plan.raise) showWindow();
  const reader = plan.raise || BrowserWindow.getAllWindows().length > 0;
  if (reader) opens.request(NOTIFICATIONS_CLICK_CHANNEL, plan.click);
}

function signalAttention(win: BrowserWindow | undefined, shows: Presentation) {
  if (shows.bounce) {
    if (!app.dock || !shouldReplaceBounce(pendingBounce)) return;
    cancelBounce();
    const id = app.dock.bounce(shows.bounce);
    if (typeof id === "number" && id >= 0) pendingBounce = { id, critical: shows.bounce === "critical" };
    return;
  }
  if (!shows.flash || !win || flashing) return;
  flashing = true;
  win.flashFrame(true);
  win.once("focus", () => {
    flashing = false;
    win.flashFrame(false);
  });
}

ipcMain.handle(NOTIFICATIONS_BADGE_CHANNEL, (_event, unread: number) => {
  setUnread(typeof unread === "number" && unread > 0 ? unread : 0);
});

let unreadCount = 0;
let tray: Tray | null = null;

function setUnread(count: number) {
  unreadCount = count;
  if (process.platform === "darwin") {
    app.dock?.setBadge(badgeText(count));
  } else {
    app.setBadgeCount(count);
  }
  drawTray();
}

function drawTray() {
  if (!tray) return;
  tray.setToolTip(trayTooltip(unreadCount));
  if (process.platform === "darwin") tray.setTitle(badgeText(unreadCount));
  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: trayTooltip(unreadCount), enabled: false },
      { type: "separator" },
      { label: "Open babysitter", click: () => showWindow() },
      { label: "Notifications", click: () => showWindow(NOTIFICATIONS_OPEN_CHANNEL) },
      { label: "Mark all as read", enabled: unreadCount > 0, click: () => void markAllRead() },
      { type: "separator" },
      { label: "Quit", role: "quit" },
    ]),
  );
}

function trayIconPath(): string {
  const name = process.platform === "darwin" ? "trayTemplate.png" : "icon.png";
  return app.isPackaged ? path.join(process.resourcesPath, name) : path.join(app.getAppPath(), "assets", name);
}

function createTray() {
  const file = trayIconPath();
  if (tray || !existsSync(file)) return;
  tray = new Tray(file);
  tray.on("click", () => showWindow(NOTIFICATIONS_OPEN_CHANNEL));
  drawTray();
}

const opens = openQueue();

ipcMain.on(NOTIFICATIONS_OPEN_READY_CHANNEL, (event) => {
  opens.listening((channel, payload) => event.sender.send(channel, payload));
});

function showWindow(channel?: string) {
  let win = BrowserWindow.getAllWindows()[0];
  if (!win) {
    createWindow();
    win = BrowserWindow.getAllWindows()[0];
  }
  if (!win) return;
  if (win.isMinimized()) win.restore();
  win.show();
  win.focus();
  if (channel) opens.request(channel);
}

async function markAllRead() {
  const status = daemon.getStatus();
  if (status.state !== "ready") return;
  try {
    const response = await fetch(`${status.baseUrl}/notifications/read`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    if (response.ok) setUnread(0);
  } catch (err) {
    appLog.warn(`menu bar: could not mark the notifications read: ${String(err)}`);
  }
}

ipcMain.handle(DIALOG_PICK_DIRECTORY_CHANNEL, async (event: IpcMainInvokeEvent, defaultPath?: unknown) => {
  const win = BrowserWindow.fromWebContents(event.sender);
  const options = {
    properties: ["openDirectory" as const],
    ...(typeof defaultPath === "string" && defaultPath ? { defaultPath } : {}),
  };
  const result = win ? await dialog.showOpenDialog(win, options) : await dialog.showOpenDialog(options);
  return result.canceled ? null : (result.filePaths[0] ?? null);
});

function createWindow() {
  const mainWindow = new BrowserWindow({
    width: 1320,
    height: 860,
    minWidth: 960,
    minHeight: 640,
    backgroundColor: canvasColor(themePreference, nativeTheme.shouldUseDarkColors),
    icon: appIconPath(),
    ...(process.platform === "darwin"
      ? {
          titleBarStyle: "hiddenInset" as const,
          trafficLightPosition: MAC_WINDOW_BUTTON_POSITION,
        }
      : {}),
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
    },
  });

  mainWindow.on("closed", () => opens.gone());

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith("https://")) void shell.openExternal(url);
    return { action: "deny" };
  });

  if (MAIN_WINDOW_VITE_DEV_SERVER_URL) {
    void mainWindow.loadURL(MAIN_WINDOW_VITE_DEV_SERVER_URL);
    mainWindow.webContents.openDevTools({ mode: "detach" });
  } else {
    void mainWindow.loadFile(path.join(__dirname, `../renderer/${MAIN_WINDOW_VITE_NAME}/index.html`));
  }
}

app.on("ready", () => {
  nativeTheme.themeSource = themePreference;
  void daemon.start();
  createWindow();
  createTray();
  updates.start();
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin" && !tray) {
    app.quit();
  }
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow();
  }
});

let quitting = false;
app.on("before-quit", (event) => {
  if (quitting) return;
  quitting = true;
  event.preventDefault();
  updates.dispose();
  killLoginShells();
  setTimeout(() => app.exit(0), QUIT_CAP_MS).unref();
  void daemon.stop().finally(() => app.quit());
});
