import { contextBridge, ipcRenderer, type IpcRendererEvent } from "electron";
import type { DaemonStatus } from "./shared/daemon-status";
import type { LogRecord, OpenLogFolderResult } from "./shared/logs";
import type { DesktopNotification, NotificationClick } from "./shared/notifications";
import { isQuitShortcutHint, type QuitShortcutHint } from "./shared/quit";
import type { ThemePreference } from "./shared/theme";
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
import type { UpdateSettings, UpdateStatus } from "./shared/updates";

export type BabysitterBridge = {
  daemon: {
    getStatus(): Promise<DaemonStatus>;
    onStatus(listener: (status: DaemonStatus) => void): () => void;
    restart(): Promise<void>;
  };
  app: {
    getVersion(): Promise<string>;
  };
  dialog: {
    pickDirectory(defaultPath?: string): Promise<string | null>;
  };
  theme: {
    follow(preference: ThemePreference): void;
  };
  notifications: {
    supported(): Promise<boolean>;
    show(notification: DesktopNotification): Promise<void>;
    setBadge(unread: number): Promise<void>;
    onClick(listener: (click: NotificationClick) => void): () => void;
    onOpen(listener: () => void): () => void;
  };
  updates: {
    getStatus(): Promise<UpdateStatus>;
    onStatus(listener: (status: UpdateStatus) => void): () => void;
    check(): Promise<void>;
    download(): Promise<void>;
    install(): Promise<void>;
    getSettings(): Promise<UpdateSettings>;
    setSettings(patch: Partial<UpdateSettings>): Promise<UpdateSettings>;
  };
  logs: {
    desktop: boolean;
    appRecords(): Promise<LogRecord[]>;
    onAppRecord(listener: (record: LogRecord) => void): () => void;
    openFolder(): Promise<OpenLogFolderResult>;
  };
  quit: {
    onShortcut(listener: (hint: QuitShortcutHint) => void): () => void;
  };
};

const bridge: BabysitterBridge = {
  daemon: {
    getStatus: () => ipcRenderer.invoke(DAEMON_GET_STATUS_CHANNEL),
    onStatus: (listener) => {
      const handler = (_event: IpcRendererEvent, status: DaemonStatus) => listener(status);
      ipcRenderer.on(DAEMON_STATUS_CHANNEL, handler);
      return () => ipcRenderer.off(DAEMON_STATUS_CHANNEL, handler);
    },
    restart: () => ipcRenderer.invoke(DAEMON_RESTART_CHANNEL),
  },
  app: {
    getVersion: () => ipcRenderer.invoke(APP_GET_VERSION_CHANNEL),
  },
  dialog: {
    pickDirectory: (defaultPath) => ipcRenderer.invoke(DIALOG_PICK_DIRECTORY_CHANNEL, defaultPath),
  },
  theme: {
    follow: (preference) => ipcRenderer.send(THEME_FOLLOW_CHANNEL, preference),
  },
  notifications: {
    supported: () => ipcRenderer.invoke(NOTIFICATIONS_SUPPORTED_CHANNEL),
    show: (notification) => ipcRenderer.invoke(NOTIFICATIONS_SHOW_CHANNEL, notification),
    setBadge: (unread) => ipcRenderer.invoke(NOTIFICATIONS_BADGE_CHANNEL, unread),
    onClick: (listener) => {
      const handler = (_event: IpcRendererEvent, click: NotificationClick) => listener(click);
      ipcRenderer.on(NOTIFICATIONS_CLICK_CHANNEL, handler);
      return () => ipcRenderer.off(NOTIFICATIONS_CLICK_CHANNEL, handler);
    },
    onOpen: (listener) => {
      const handler = () => listener();
      ipcRenderer.on(NOTIFICATIONS_OPEN_CHANNEL, handler);
      ipcRenderer.send(NOTIFICATIONS_OPEN_READY_CHANNEL);
      return () => ipcRenderer.off(NOTIFICATIONS_OPEN_CHANNEL, handler);
    },
  },
  updates: {
    getStatus: () => ipcRenderer.invoke(UPDATES_GET_STATUS_CHANNEL),
    onStatus: (listener) => {
      const handler = (_event: IpcRendererEvent, status: UpdateStatus) => listener(status);
      ipcRenderer.on(UPDATES_STATUS_CHANNEL, handler);
      return () => ipcRenderer.off(UPDATES_STATUS_CHANNEL, handler);
    },
    check: () => ipcRenderer.invoke(UPDATES_CHECK_CHANNEL),
    download: () => ipcRenderer.invoke(UPDATES_DOWNLOAD_CHANNEL),
    install: () => ipcRenderer.invoke(UPDATES_INSTALL_CHANNEL),
    getSettings: () => ipcRenderer.invoke(UPDATES_GET_SETTINGS_CHANNEL),
    setSettings: (patch) => ipcRenderer.invoke(UPDATES_SET_SETTINGS_CHANNEL, patch),
  },
  logs: {
    desktop: true,
    appRecords: () => ipcRenderer.invoke(LOGS_APP_RECORDS_CHANNEL),
    onAppRecord: (listener) => {
      const handler = (_event: IpcRendererEvent, record: LogRecord) => listener(record);
      ipcRenderer.on(LOGS_APP_RECORD_CHANNEL, handler);
      return () => ipcRenderer.off(LOGS_APP_RECORD_CHANNEL, handler);
    },
    openFolder: () => ipcRenderer.invoke(LOGS_OPEN_FOLDER_CHANNEL),
  },
  quit: {
    onShortcut: (listener) => {
      const handler = (_event: IpcRendererEvent, hint: unknown) => {
        if (isQuitShortcutHint(hint)) listener(hint);
      };
      ipcRenderer.on(QUIT_SHORTCUT_CHANNEL, handler);
      return () => ipcRenderer.off(QUIT_SHORTCUT_CHANNEL, handler);
    },
  },
};

contextBridge.exposeInMainWorld("babysitter", bridge);
