import { isBusy, resolveSettings, type UpdateSettings, type UpdateStatus } from "../shared/updates";

const FIRST_CHECK_DELAY_MS = 10_000;
const CHECK_INTERVAL_MS = 60 * 60 * 1000;
const INSTALL_TIMEOUT_MS = 30_000;
const INSTALL_TIMED_OUT = "The app did not restart in 30 seconds. Try again.";

type Mode = "auto" | "manual";

export type CheckResult = { isUpdateAvailable: boolean; updateInfo: { version: string } } | null;

export type UpdaterLogger = {
  info(message?: unknown): void;
  warn(message?: unknown): void;
  error(message?: unknown): void;
  debug?(message: string): void;
};

export type UpdaterEngine = {
  autoDownload: boolean;
  autoInstallOnAppQuit: boolean;
  channel: string | null;
  allowPrerelease: boolean;
  allowDowngrade: boolean;
  disableDifferentialDownload: boolean;
  logger: UpdaterLogger | null;
  checkForUpdates(): Promise<CheckResult>;
  downloadUpdate(): Promise<unknown>;
  quitAndInstall(isSilent?: boolean, isForceRunAfter?: boolean): void;
  setFeedURL(options: { provider: "generic"; url: string }): void;
  on(event: "download-progress", listener: (progress: { percent: number }) => void): unknown;
  on(event: "error", listener: (error: Error) => void): unknown;
};

export type NativeUpdater = {
  on(event: "update-downloaded", listener: () => void): unknown;
};

export type UpdateControllerDeps = {
  supported: boolean;
  currentVersion: string;
  engine: () => UpdaterEngine;
  native: NativeUpdater;
  settings: {
    read(): Partial<UpdateSettings>;
    write(settings: Partial<UpdateSettings>): void;
  };
  feedUrl?: string;
  beforeInstall: () => Promise<void>;
  onInstallFailed: () => void;
  log: (message: string) => void;
};

export type UpdateController = {
  start(): void;
  dispose(): void;
  getStatus(): UpdateStatus;
  onStatus(listener: (status: UpdateStatus) => void): () => void;
  check(): Promise<UpdateStatus>;
  download(): Promise<UpdateStatus>;
  install(): Promise<void>;
  getSettings(): UpdateSettings;
  setSettings(patch: Partial<UpdateSettings>): UpdateSettings;
};

const DESCRIPTIONS: [RegExp, string][] = [
  [/ERR_UPDATER_CHANNEL_FILE_NOT_FOUND/, "The latest release has no update files yet. Try again later."],
  [/ERR_UPDATER_NO_PUBLISHED_VERSIONS|\b404\b|Unable to find latest version/, "No release to update from yet."],
  [
    /ENOTFOUND|ECONNREFUSED|ECONNRESET|ETIMEDOUT|EAI_AGAIN|net::ERR_/,
    "Could not reach GitHub. Check the connection and try again.",
  ],
  [/read-only volume/, "Move Babysitter to the Applications folder, then try again."],
];

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function codeOf(error: unknown): string {
  const code = (error as { code?: unknown } | null)?.code;
  return typeof code === "string" ? code : "";
}

export function describeUpdateError(error: unknown): string {
  const message = messageOf(error);
  const text = `${codeOf(error)} ${message}`;
  return DESCRIPTIONS.find(([pattern]) => pattern.test(text))?.[1] ?? message;
}

export function createUpdateController(deps: UpdateControllerDeps): UpdateController {
  const { currentVersion, log } = deps;
  const listeners = new Set<(status: UpdateStatus) => void>();
  let status: UpdateStatus = { state: deps.supported ? "idle" : "unsupported", currentVersion };
  let saved = deps.settings.read();
  let engine: UpdaterEngine | null = null;
  let downloadMode: Mode = "auto";
  let checkedAt: string | undefined;
  let runningCheck: Promise<CheckResult> | null = null;
  let firstCheck: ReturnType<typeof setTimeout> | undefined;
  let interval: ReturnType<typeof setInterval> | undefined;
  let installDeadline: ReturnType<typeof setTimeout> | undefined;

  function settings(): UpdateSettings {
    return resolveSettings(saved, currentVersion);
  }

  function publish(next: Omit<UpdateStatus, "currentVersion" | "checkedAt">): UpdateStatus {
    status = { ...next, currentVersion, ...(checkedAt ? { checkedAt } : {}) };
    for (const listener of listeners) listener(status);
    return status;
  }

  function engineIfSupported(): UpdaterEngine | null {
    if (!deps.supported) return null;
    engine ??= configure(deps.engine());
    return engine;
  }

  function followChannel(updater: UpdaterEngine, current: UpdateSettings) {
    const nightly = current.channel === "nightly";
    updater.channel = nightly ? "nightly" : "latest";
    updater.allowPrerelease = nightly;
    updater.allowDowngrade = false;
  }

  function configure(created: UpdaterEngine): UpdaterEngine {
    const write = (message: unknown) => log(`updater: ${String(message)}`);
    created.logger = { info: write, warn: write, error: write };
    created.autoDownload = false;
    created.autoInstallOnAppQuit = true;
    created.disableDifferentialDownload = true;
    followChannel(created, settings());
    if (deps.feedUrl) created.setFeedURL({ provider: "generic", url: deps.feedUrl });
    created.on("download-progress", (progress) => {
      if (status.state !== "downloading") return;
      publish({ state: "downloading", version: status.version, percent: progress.percent });
    });
    created.on("error", (error) => {
      if (status.state === "installing") return failInstall(error);
      if (status.state === "downloading") failDownload(downloadMode, error);
    });
    deps.native.on("update-downloaded", () => {
      publish({ state: "downloaded", version: status.version });
    });
    return created;
  }

  function engineWhen(allowed: boolean): UpdaterEngine | null {
    return allowed ? engineIfSupported() : null;
  }

  function downloadStarted(): boolean {
    return status.state !== "checking" && isBusy(status.state);
  }

  function ask(updater: UpdaterEngine): Promise<CheckResult> {
    runningCheck ??= updater.checkForUpdates().finally(() => {
      runningCheck = null;
    });
    return runningCheck;
  }

  async function check(mode: Mode): Promise<UpdateStatus> {
    const updater = engineWhen(!isBusy(status.state));
    if (!updater) return status;
    if (mode === "manual") publish({ state: "checking" });
    try {
      return found(await ask(updater));
    } catch (error) {
      log(`update: the check failed: ${messageOf(error)}`);
      return mode === "manual" ? publish({ state: "error", message: describeUpdateError(error) }) : status;
    }
  }

  function found(result: CheckResult): UpdateStatus {
    if (downloadStarted()) return status;
    checkedAt = new Date().toISOString();
    if (!result?.isUpdateAvailable) return publish({ state: "not-available" });
    publish({ state: "available", version: result.updateInfo.version });
    if (settings().autoDownload) void download("auto");
    return status;
  }

  async function download(mode: Mode): Promise<UpdateStatus> {
    const updater = engineWhen(status.state === "available");
    if (!updater) return status;
    downloadMode = mode;
    publish({ state: "downloading", version: status.version, percent: 0 });
    try {
      await updater.downloadUpdate();
      if (status.state === "downloading") publish({ state: "downloading", version: status.version, percent: 100 });
    } catch (error) {
      failDownload(mode, error);
    }
    return status;
  }

  function failDownload(mode: Mode, error: unknown) {
    if (status.state !== "downloading") return;
    log(`update: the download failed: ${messageOf(error)}`);
    const offer = { state: "available" as const, version: status.version };
    publish(mode === "manual" ? { ...offer, message: describeUpdateError(error) } : offer);
  }

  function failInstall(error: unknown) {
    clearTimeout(installDeadline);
    log(`update: the install failed: ${messageOf(error)}`);
    publish({ state: "downloaded", version: status.version, message: describeUpdateError(error) });
    deps.onInstallFailed();
  }

  function giveUpInstall() {
    if (status.state === "installing") failInstall(new Error(INSTALL_TIMED_OUT));
  }

  async function install(): Promise<void> {
    const updater = engineWhen(status.state === "downloaded");
    if (!updater) return;
    publish({ state: "installing", version: status.version });
    installDeadline = setTimeout(giveUpInstall, INSTALL_TIMEOUT_MS);
    await deps.beforeInstall();
    if (status.state !== "installing") return;
    try {
      updater.quitAndInstall(false, true);
    } catch (error) {
      failInstall(error);
    }
  }

  function setSettings(patch: Partial<UpdateSettings>): UpdateSettings {
    const previous = settings();
    saved = { ...saved, ...patch };
    deps.settings.write(saved);
    const next = settings();
    const updater = engineIfSupported();
    if (!updater) return next;
    followChannel(updater, next);
    const channelChanged = next.channel !== previous.channel;
    const downloadsTurnedOn = next.autoDownload && !previous.autoDownload;
    if (channelChanged) void check("auto");
    if (downloadsTurnedOn) void download("auto");
    return next;
  }

  return {
    start() {
      if (!engineIfSupported()) return;
      firstCheck = setTimeout(() => {
        void check("auto");
        interval = setInterval(() => void check("auto"), CHECK_INTERVAL_MS);
      }, FIRST_CHECK_DELAY_MS);
    },
    dispose() {
      clearTimeout(firstCheck);
      clearInterval(interval);
      clearTimeout(installDeadline);
    },
    getStatus: () => status,
    onStatus(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    check: () => check("manual"),
    download: () => download("manual"),
    install,
    getSettings: settings,
    setSettings,
  };
}
