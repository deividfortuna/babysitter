import type { BabysitterBridge } from "../../preload";
import type { DaemonStatus } from "../../shared/daemon-status";
import type { UpdateSettings, UpdateStatus } from "../../shared/updates";

const BROWSER_VERSION = "0.0.0-browser";
const BROWSER_UPDATE_STATUS: UpdateStatus = { state: "unsupported", currentVersion: BROWSER_VERSION };
const BROWSER_UPDATE_SETTINGS: UpdateSettings = { autoDownload: true, channel: "stable" };

function browserStatus(): DaemonStatus {
  const base = new URLSearchParams(window.location.search).get("daemon");
  if (!base) return { state: "error", message: "Not running inside the desktop app." };
  try {
    const url = new URL(base);
    return { state: "ready", source: "attached", port: Number(url.port), baseUrl: url.toString() };
  } catch {
    return { state: "error", message: `Bad daemon URL: ${base}` };
  }
}

export const bridge: BabysitterBridge = window.babysitter ?? {
  daemon: {
    getStatus: async () => browserStatus(),
    onStatus: () => () => undefined,
    restart: async () => undefined,
  },
  app: {
    getVersion: async () => BROWSER_VERSION,
  },
  dialog: {
    pickDirectory: async () => null,
  },
  theme: {
    follow: () => undefined,
  },
  notifications: {
    supported: async () => false,
    show: async () => undefined,
    setBadge: async () => undefined,
    onClick: () => () => undefined,
    onOpen: () => () => undefined,
  },
  updates: {
    getStatus: async () => BROWSER_UPDATE_STATUS,
    onStatus: () => () => undefined,
    check: async () => undefined,
    download: async () => undefined,
    install: async () => undefined,
    getSettings: async () => BROWSER_UPDATE_SETTINGS,
    setSettings: async (patch) => ({ ...BROWSER_UPDATE_SETTINGS, ...patch }),
  },
  logs: {
    desktop: false,
    appRecords: async () => [],
    onAppRecord: () => () => undefined,
    openFolder: async () => ({ ok: false, error: "Only the desktop app can open the log folder." }),
  },
  quit: {
    onShortcut: () => () => undefined,
  },
};
