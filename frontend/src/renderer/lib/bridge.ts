import type { BabysitterBridge } from "../../preload";
import { LOCAL_CONNECTION_ID, type ConnectionList } from "../../shared/connections";
import type { DaemonStatus } from "../../shared/daemon-status";
import type { UpdateSettings, UpdateStatus } from "../../shared/updates";

const BROWSER_VERSION = "0.0.0-browser";
const BROWSER_UPDATE_STATUS: UpdateStatus = { state: "unsupported", currentVersion: BROWSER_VERSION };
const BROWSER_UPDATE_SETTINGS: UpdateSettings = { autoDownload: true, channel: "stable" };

const BROWSER_CONNECTIONS: ConnectionList = { activeId: LOCAL_CONNECTION_ID, localName: "This browser", remotes: [] };

function browserStatus(): DaemonStatus {
  const params = new URLSearchParams(window.location.search);
  const base = params.get("daemon");
  if (!base) return { state: "error", message: "Not running inside the desktop app." };
  try {
    const url = new URL(base);
    return {
      state: "ready",
      source: "attached",
      port: Number(url.port),
      baseUrl: url.toString(),
      token: params.get("token") ?? undefined,
    };
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
  connections: {
    list: async () => BROWSER_CONNECTIONS,
    onChange: () => () => undefined,
    use: async () => undefined,
    pair: async () => ({ ok: false, error: "Only the desktop app can pair with a remote daemon." }),
    remove: async () => undefined,
    discover: async () => [],
  },
  app: {
    getVersion: async () => BROWSER_VERSION,
    popupMenu: () => undefined,
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
  openIn: {
    targets: async () => [],
    launch: async () => ({ ok: false, error: "Only the desktop app can open folders." }),
  },
  quit: {
    onShortcut: () => () => undefined,
  },
};
