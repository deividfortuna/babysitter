import {
  app,
  autoUpdater as nativeUpdater,
  BrowserWindow,
  dialog,
  ipcMain,
  Menu,
  nativeTheme,
  Notification,
  screen,
  shell,
  Tray,
  type BrowserWindowConstructorOptions,
  type IpcMainEvent,
  type IpcMainInvokeEvent,
} from "electron";
import { existsSync, mkdirSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { autoUpdater } from "electron-updater";
import { createUpdateController } from "./main/app-updater";
import { AppLog } from "./main/app-log";
import { ConnectionManager, localName } from "./main/connection-manager";
import { readConnections, writeConnections } from "./main/connection-store";
import { DaemonSupervisor } from "./main/daemon-supervisor";
import { EditorLauncher } from "./main/open-in";
import { localWatchFolder } from "./main/watch-folders";
import { discoverDaemons } from "./main/discovery";
import { checkRemote } from "./main/remote-check";
import { authorization, parsePairRequest } from "./shared/connections";
import { readUpdateSettings, writeUpdateSettings } from "./main/update-settings";
import { killLoginShells, shellRunner } from "./main/login-shell";
import { openQueue } from "./main/pending-open";
import { concealWindow, quitShortcut } from "./main/quit-shortcut";
import { centeredBounds } from "./main/window-bounds";
import { isMenuAnchor } from "./shared/app-menu";
import { defaultDataDir } from "./shared/daemon-discovery";
import { resolveDaemonLaunch } from "./shared/daemon-launch";
import { daemonEnvOnce } from "./shared/shell-env";
import {
  APP_GET_VERSION_CHANNEL,
  APP_MENU_POPUP_CHANNEL,
  CONNECTIONS_CHANGED_CHANNEL,
  CONNECTIONS_DISCOVER_CHANNEL,
  CONNECTIONS_LIST_CHANNEL,
  CONNECTIONS_PAIR_CHANNEL,
  CONNECTIONS_REMOVE_CHANNEL,
  CONNECTIONS_USE_CHANNEL,
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
  OPEN_IN_LAUNCH_CHANNEL,
  OPEN_IN_TARGETS_CHANNEL,
  QUIT_SHORTCUT_CHANNEL,
  THEME_FOLLOW_CHANNEL,
  UPDATES_CHECK_CHANNEL,
  UPDATES_DOWNLOAD_CHANNEL,
  UPDATES_GET_SETTINGS_CHANNEL,
  UPDATES_GET_STATUS_CHANNEL,
  UPDATES_INSTALL_CHANNEL,
  UPDATES_SET_SETTINGS_CHANNEL,
  UPDATES_STATUS_CHANNEL,
} from "./shared/ipc";
import { isDaemonLogRecord } from "./shared/logs";
import { openPathResult, type OpenFolderResult } from "./shared/open-in";
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
import { canvasColor, isThemePreference, windowControlsColors, type ThemePreference } from "./shared/theme";
import { MAC_WINDOW_BUTTON_POSITION, TITLEBAR_HEIGHT } from "./shared/titlebar";
import { readStoredTheme, writeStoredTheme } from "./main/theme-preference";

if (process.platform === "win32") app.setAppUserModelId("com.deividfortuna.babysitter");

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

const loginEnv = daemonEnvOnce({
  platform: process.platform,
  env: process.env,
  home: os.homedir(),
  run: shellRunner(LOGIN_SHELL_TIMEOUT_MS),
  log: (msg) => appLog.info(msg),
});

const daemon = new DaemonSupervisor({
  launch: resolveDaemonLaunch(process.env, app.isPackaged, process.resourcesPath, app.getAppPath(), process.platform),
  dataDir,
  env: loginEnv,
  log: (msg) => appLog.info(msg),
  output: routeDaemonOutput,
});

function broadcast(channel: string, payload: unknown) {
  for (const win of BrowserWindow.getAllWindows()) {
    win.webContents.send(channel, payload);
  }
}

const connections = new ConnectionManager({
  local: daemon,
  localName: localName(process.platform),
  read: () => readConnections(dataDir),
  write: (next) => writeConnections(dataDir, next),
  check: (url, token) => checkRemote(url, token),
  discover: () => discoverDaemons({ log: (msg) => appLog.info(msg) }),
  log: (msg) => appLog.info(msg),
});

connections.onStatus((status) => {
  if (status.state === "error") appLog.error(`daemon: ${status.message}`);
  broadcast(DAEMON_STATUS_CHANNEL, status);
});

connections.onList((list) => broadcast(CONNECTIONS_CHANGED_CHANNEL, list));

ipcMain.handle(CONNECTIONS_LIST_CHANNEL, () => connections.list());
ipcMain.handle(CONNECTIONS_USE_CHANNEL, (_event, id: unknown) => connections.use(String(id)));
ipcMain.handle(CONNECTIONS_PAIR_CHANNEL, (_event, request: unknown) => connections.pair(parsePairRequest(request)));
ipcMain.handle(CONNECTIONS_REMOVE_CHANNEL, (_event, id: unknown) => connections.remove(String(id)));
ipcMain.handle(CONNECTIONS_DISCOVER_CHANNEL, () => connections.discover());

appLog.onRecord((record) => broadcast(LOGS_APP_RECORD_CHANNEL, record));

ipcMain.handle(LOGS_APP_RECORDS_CHANNEL, () => appLog.records());
ipcMain.handle(LOGS_OPEN_FOLDER_CHANNEL, async (): Promise<OpenFolderResult> => {
  mkdirSync(appLog.folder, { recursive: true, mode: 0o750 });
  return openPathResult(await shell.openPath(appLog.folder));
});

const openIn = new EditorLauncher({
  platform: process.platform,
  env: loginEnv,
  openPath: (dir) => shell.openPath(dir),
  watchFolder: (watchId) => localWatchFolder(connections.getStatus(), watchId),
});

ipcMain.handle(OPEN_IN_TARGETS_CHANNEL, () => openIn.targets());
ipcMain.handle(OPEN_IN_LAUNCH_CHANNEL, async (_event, watchId: unknown, target: unknown): Promise<OpenFolderResult> => {
  const result = await openIn.launch(watchId, target);
  if (!result.ok) appLog.warn(`open in ${String(target)}: ${result.error}`);
  return result;
});

ipcMain.handle(DAEMON_GET_STATUS_CHANNEL, () => connections.getStatus());
ipcMain.handle(DAEMON_RESTART_CHANNEL, () => connections.retry());
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
  onInstallFailed: () => void connections.start(),
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
  paintWindows();
});

nativeTheme.on("updated", paintWindows);

function paintWindows() {
  const canvas = canvasColor(themePreference, nativeTheme.shouldUseDarkColors);
  for (const win of BrowserWindow.getAllWindows()) {
    win.setBackgroundColor(canvas);
    if (process.platform !== "darwin") win.setTitleBarOverlay(windowControls());
  }
}

function windowControls() {
  return { ...windowControlsColors(themePreference, nativeTheme.shouldUseDarkColors), height: TITLEBAR_HEIGHT };
}

function titleBar(): BrowserWindowConstructorOptions {
  if (process.platform === "darwin") {
    return { titleBarStyle: "hiddenInset", trafficLightPosition: MAC_WINDOW_BUTTON_POSITION };
  }
  return { titleBarStyle: "hidden", titleBarOverlay: windowControls() };
}

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
  const shows = presentation(notification, Notification.isSupported(), process.platform, win?.isFocused());
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
  signalAttention(win, shows);
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
  const status = connections.getStatus();
  if (status.state !== "ready") return;
  try {
    const response = await fetch(`${status.baseUrl}/notifications/read`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authorization(status.token) },
      body: "{}",
    });
    if (response.ok) setUnread(0);
  } catch (err) {
    appLog.warn(`menu bar: could not mark the notifications read: ${String(err)}`);
  }
}

ipcMain.on(APP_MENU_POPUP_CHANNEL, (event: IpcMainEvent, anchor: unknown) => {
  if (!isMenuAnchor(anchor)) return;
  const zoom = event.sender.getZoomFactor();
  Menu.getApplicationMenu()?.popup({
    window: BrowserWindow.fromWebContents(event.sender) ?? undefined,
    x: Math.round(anchor.x * zoom),
    y: Math.round(anchor.y * zoom),
  });
});

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
  const { workArea } = screen.getDisplayNearestPoint(screen.getCursorScreenPoint());
  const mainWindow = new BrowserWindow({
    ...centeredBounds(workArea, { width: 1320, height: 860 }),
    minWidth: Math.min(960, workArea.width),
    minHeight: Math.min(640, workArea.height),
    backgroundColor: canvasColor(themePreference, nativeTheme.shouldUseDarkColors),
    icon: appIconPath(),
    ...titleBar(),
    webPreferences: {
      preload: path.join(__dirname, "preload.cjs"),
    },
  });

  mainWindow.on("closed", () => opens.gone());

  mainWindow.webContents.on(
    "before-input-event",
    quitShortcut({
      platform: process.platform,
      notify: (hint) => {
        if (!mainWindow.isDestroyed()) mainWindow.webContents.send(QUIT_SHORTCUT_CHANNEL, hint);
      },
      conceal: () => concealWindow(mainWindow),
      quit: () => app.quit(),
    }),
  );

  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith("https://")) void shell.openExternal(url);
    return { action: "deny" };
  });

  const devServerUrl = app.isPackaged ? undefined : process.env.VITE_DEV_SERVER_URL;
  if (devServerUrl) {
    void mainWindow.loadURL(devServerUrl);
    mainWindow.webContents.openDevTools({ mode: "detach" });
  } else {
    void mainWindow.loadFile(path.join(__dirname, "..", "dist", "renderer", "index.html"));
  }
}

app.on("ready", () => {
  nativeTheme.themeSource = themePreference;
  void connections.start();
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
  connections.dispose();
  killLoginShells();
  setTimeout(() => app.exit(0), QUIT_CAP_MS).unref();
  void daemon.stop().finally(() => app.quit());
});
