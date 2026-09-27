import { EventEmitter } from "node:events";
import { afterEach, beforeEach, expect, test, vi } from "vite-plus/test";
import type { UpdateSettings, UpdateStatus } from "../shared/updates";
import {
  createUpdateController,
  describeUpdateError,
  type CheckResult,
  type UpdateControllerDeps,
  type UpdaterLogger,
} from "./app-updater";

class FakeEngine extends EventEmitter {
  autoDownload = true;
  autoInstallOnAppQuit = false;
  allowPrerelease = false;
  allowDowngrade = true;
  disableDifferentialDownload = false;
  logger: UpdaterLogger | null = null;
  checkForUpdates = vi.fn(async (): Promise<CheckResult> => ({
    isUpdateAvailable: false,
    updateInfo: { version: "0.2.0" },
  }));
  downloadUpdate = vi.fn(async (): Promise<unknown> => []);
  quitAndInstall = vi.fn();
  setFeedURL = vi.fn();
  set channel(_value: string) {
    throw new Error("the channel setter turns on downgrades and breaks the prerelease lookup");
  }

  offers(version: string) {
    this.checkForUpdates.mockResolvedValue({ isUpdateAvailable: true, updateInfo: { version } });
  }

  fails(error: Error) {
    this.checkForUpdates.mockRejectedValue(error);
  }
}

const HOUR = 60 * 60 * 1000;

let engine: FakeEngine;
let native: EventEmitter;
let saved: Partial<UpdateSettings>;
let log: ReturnType<typeof vi.fn<(message: string) => void>>;

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-09-26T10:00:00.000Z"));
  engine = new FakeEngine();
  native = new EventEmitter();
  saved = {};
  log = vi.fn<(message: string) => void>();
});

afterEach(() => {
  vi.useRealTimers();
});

function controller(overrides: Partial<UpdateControllerDeps> = {}) {
  return createUpdateController({
    supported: true,
    currentVersion: "0.1.0",
    engine: () => engine,
    native,
    settings: {
      read: () => saved,
      write: (next) => {
        saved = next;
      },
    },
    beforeInstall: vi.fn(async () => undefined),
    onInstallFailed: vi.fn(),
    log,
    ...overrides,
  });
}

function statuses(updates: ReturnType<typeof controller>): UpdateStatus[] {
  const seen: UpdateStatus[] = [];
  updates.onStatus((status) => seen.push(status));
  return seen;
}

test("a build that cannot update says so and never builds the engine", async () => {
  const engineFactory = vi.fn(() => engine);
  const updates = controller({ supported: false, engine: engineFactory });

  updates.start();
  await vi.advanceTimersByTimeAsync(2 * HOUR);

  expect(updates.getStatus()).toEqual({ state: "unsupported", currentVersion: "0.1.0" });
  expect(await updates.check()).toEqual({ state: "unsupported", currentVersion: "0.1.0" });
  expect(engineFactory).not.toHaveBeenCalled();
});

test("a build that cannot update still keeps the choices of the user", () => {
  const updates = controller({ supported: false });

  expect(updates.setSettings({ autoDownload: false })).toEqual({ autoDownload: false, channel: "stable" });
  expect(saved).toEqual({ autoDownload: false });
});

test("the engine downloads only when told, installs on quit, never downgrades and never takes a diff", () => {
  controller().start();

  expect(engine.autoDownload).toBe(false);
  expect(engine.autoInstallOnAppQuit).toBe(true);
  expect(engine.allowDowngrade).toBe(false);
  expect(engine.disableDifferentialDownload).toBe(true);
  expect(engine.logger).not.toBeNull();
});

test("a stable build asks only for stable releases", () => {
  controller().start();
  expect(engine.allowPrerelease).toBe(false);
});

test("a prerelease build asks for prereleases too", () => {
  controller({ currentVersion: "0.1.0-alpha.1" }).start();
  expect(engine.allowPrerelease).toBe(true);
});

test("a saved channel wins over the channel of the version", () => {
  saved = { channel: "stable" };
  controller({ currentVersion: "0.1.0-alpha.1" }).start();
  expect(engine.allowPrerelease).toBe(false);
});

test("a feed address replaces GitHub, for a local test of an update", () => {
  controller({ feedUrl: "http://127.0.0.1:8765" }).start();
  expect(engine.setFeedURL).toHaveBeenCalledWith({ provider: "generic", url: "http://127.0.0.1:8765" });
});

test("the first check runs soon after the start, then once an hour", async () => {
  controller().start();

  await vi.advanceTimersByTimeAsync(9_000);
  expect(engine.checkForUpdates).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(1_000);
  expect(engine.checkForUpdates).toHaveBeenCalledTimes(1);
  await vi.advanceTimersByTimeAsync(HOUR);
  expect(engine.checkForUpdates).toHaveBeenCalledTimes(2);
});

test("dispose stops the checks", async () => {
  const updates = controller();
  updates.start();
  updates.dispose();

  await vi.advanceTimersByTimeAsync(2 * HOUR);

  expect(engine.checkForUpdates).not.toHaveBeenCalled();
});

test("a check with nothing new says so and when it checked", async () => {
  const updates = controller();
  updates.start();

  expect(await updates.check()).toEqual({
    state: "not-available",
    currentVersion: "0.1.0",
    checkedAt: "2026-09-26T10:00:00.000Z",
  });
});

test("a check that finds a version downloads it, and it is ready only when Squirrel has it", async () => {
  engine.offers("0.2.0");
  let finish: () => void = () => undefined;
  engine.downloadUpdate.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = () => resolve([]);
      }),
  );
  const updates = controller();
  updates.start();
  const seen = statuses(updates);

  await updates.check();
  engine.emit("download-progress", { percent: 42.5 });
  expect(updates.getStatus()).toMatchObject({ state: "downloading", version: "0.2.0", percent: 42.5 });
  finish();
  await vi.advanceTimersByTimeAsync(0);
  expect(updates.getStatus()).toMatchObject({ state: "downloading", version: "0.2.0", percent: 100 });

  native.emit("update-downloaded");

  expect(updates.getStatus()).toMatchObject({ state: "downloaded", version: "0.2.0" });
  expect(seen.map((status) => status.state)).toEqual([
    "checking",
    "available",
    "downloading",
    "downloading",
    "downloading",
    "downloaded",
  ]);
});

test("with downloads off, a found version waits for the user", async () => {
  saved = { autoDownload: false };
  engine.offers("0.2.0");
  const updates = controller();
  updates.start();

  await updates.check();

  expect(updates.getStatus()).toMatchObject({ state: "available", version: "0.2.0" });
  expect(engine.downloadUpdate).not.toHaveBeenCalled();

  await updates.download();

  expect(engine.downloadUpdate).toHaveBeenCalledOnce();
  expect(updates.getStatus()).toMatchObject({ state: "downloading", version: "0.2.0" });
});

test("a check that fails on its own is logged and leaves the status as it was", async () => {
  engine.fails(new Error("HttpError: 404 Not Found"));
  const updates = controller();
  updates.start();
  const seen = statuses(updates);

  await vi.advanceTimersByTimeAsync(10_000);

  expect(seen).toEqual([]);
  expect(updates.getStatus()).toEqual({ state: "idle", currentVersion: "0.1.0" });
  expect(log).toHaveBeenCalledWith(expect.stringContaining("404"));
});

test("a check the user asks for shows why it failed", async () => {
  engine.fails(new Error("Unable to find latest version on GitHub: HttpError: 404"));
  const updates = controller();
  updates.start();

  expect(await updates.check()).toEqual({
    state: "error",
    currentVersion: "0.1.0",
    message: "No release to update from yet.",
  });
});

test("a download that fails on its own goes back to offer the version", async () => {
  engine.offers("0.2.0");
  engine.downloadUpdate.mockRejectedValue(new Error("net::ERR_CONNECTION_RESET"));
  const updates = controller();
  updates.start();

  await vi.advanceTimersByTimeAsync(10_000);

  expect(updates.getStatus()).toMatchObject({ state: "available", version: "0.2.0" });
});

test("a download the user asks for shows why it failed", async () => {
  saved = { autoDownload: false };
  engine.offers("0.2.0");
  engine.downloadUpdate.mockRejectedValue(new Error("getaddrinfo ENOTFOUND github.com"));
  const updates = controller();
  updates.start();
  await updates.check();

  expect(await updates.download()).toEqual({
    state: "available",
    currentVersion: "0.1.0",
    version: "0.2.0",
    message: "Could not reach GitHub. Check the connection and try again.",
    checkedAt: "2026-09-26T10:00:00.000Z",
  });
});

test("a download the user asks for can run again after it failed", async () => {
  saved = { autoDownload: false };
  engine.offers("0.2.0");
  engine.downloadUpdate.mockRejectedValueOnce(new Error("getaddrinfo ENOTFOUND github.com"));
  const updates = controller();
  updates.start();
  await updates.check();
  await updates.download();

  expect(await updates.download()).toMatchObject({ state: "downloading", version: "0.2.0", percent: 100 });
  expect(updates.getStatus().message).toBeUndefined();
  expect(engine.downloadUpdate).toHaveBeenCalledTimes(2);
});

test("a check the user asks for answers with the download it started", async () => {
  engine.offers("0.2.0");
  engine.downloadUpdate.mockImplementation(() => new Promise(() => undefined));
  const updates = controller();
  updates.start();

  expect(await updates.check()).toMatchObject({ state: "downloading", version: "0.2.0" });
});

test("a check on its own that ends after the user started the download leaves the download alone", async () => {
  saved = { autoDownload: false };
  engine.offers("0.2.0");
  const updates = controller();
  updates.start();
  await updates.check();
  let finishCheck: () => void = () => undefined;
  engine.checkForUpdates.mockImplementation(
    () =>
      new Promise((resolve) => {
        finishCheck = () => resolve({ isUpdateAvailable: true, updateInfo: { version: "0.2.0" } });
      }),
  );
  engine.downloadUpdate.mockImplementation(() => new Promise(() => undefined));
  await vi.advanceTimersByTimeAsync(10_000);

  void updates.download();
  finishCheck();
  await vi.advanceTimersByTimeAsync(0);
  engine.emit("download-progress", { percent: 30 });

  expect(updates.getStatus()).toMatchObject({ state: "downloading", version: "0.2.0", percent: 30 });
  expect(engine.downloadUpdate).toHaveBeenCalledOnce();
});

test("a check the user asks for while a check on its own runs waits for that one", async () => {
  saved = { autoDownload: false };
  let finishCheck: () => void = () => undefined;
  engine.checkForUpdates.mockImplementation(
    () =>
      new Promise((resolve) => {
        finishCheck = () => resolve({ isUpdateAvailable: true, updateInfo: { version: "0.2.0" } });
      }),
  );
  const updates = controller();
  updates.start();
  await vi.advanceTimersByTimeAsync(10_000);

  const asked = updates.check();
  expect(updates.getStatus().state).toBe("checking");
  finishCheck();

  expect(await asked).toMatchObject({ state: "available", version: "0.2.0" });
  expect(engine.checkForUpdates).toHaveBeenCalledOnce();
});

test("Squirrel refusing the download after it arrived goes back to offer the version", async () => {
  engine.offers("0.2.0");
  const updates = controller();
  updates.start();
  await updates.check();
  await vi.advanceTimersByTimeAsync(0);

  engine.emit("error", new Error("Code signature at URL did not pass validation"));

  expect(updates.getStatus()).toMatchObject({ state: "available", version: "0.2.0" });
});

test("a check while a download runs or waits to install does nothing", async () => {
  engine.offers("0.2.0");
  const updates = controller();
  updates.start();
  await updates.check();
  expect(engine.checkForUpdates).toHaveBeenCalledTimes(1);

  await updates.check();
  native.emit("update-downloaded");
  await updates.check();

  expect(engine.checkForUpdates).toHaveBeenCalledTimes(1);
  expect(updates.getStatus().state).toBe("downloaded");
});

test("install does nothing before Squirrel has the update", async () => {
  const updates = controller();
  updates.start();

  await updates.install();

  expect(engine.quitAndInstall).not.toHaveBeenCalled();
});

async function downloaded(deps: Partial<UpdateControllerDeps> = {}) {
  engine.offers("0.2.0");
  const updates = controller(deps);
  updates.start();
  await updates.check();
  native.emit("update-downloaded");
  return updates;
}

test("install stops the daemon first, then quits into the new version", async () => {
  const order: string[] = [];
  const beforeInstall = vi.fn(async () => {
    order.push("daemon down");
  });
  engine.quitAndInstall.mockImplementation(() => order.push("quit and install"));
  const updates = await downloaded({ beforeInstall });

  await updates.install();

  expect(order).toEqual(["daemon down", "quit and install"]);
  expect(engine.quitAndInstall).toHaveBeenCalledWith(false, true);
  expect(updates.getStatus()).toMatchObject({ state: "installing", version: "0.2.0" });
});

test("an install after which the app does not quit gives the restart back and starts the daemon again", async () => {
  const onInstallFailed = vi.fn();
  const updates = await downloaded({ onInstallFailed });
  await updates.install();
  expect(engine.quitAndInstall).toHaveBeenCalledOnce();

  await vi.advanceTimersByTimeAsync(29_999);
  expect(updates.getStatus().state).toBe("installing");
  await vi.advanceTimersByTimeAsync(1);

  expect(onInstallFailed).toHaveBeenCalledOnce();
  expect(updates.getStatus()).toMatchObject({
    state: "downloaded",
    version: "0.2.0",
    message: "The app did not restart in 30 seconds. Try again.",
  });
});

test("an install whose daemon never stops gives the restart back and never quits", async () => {
  let stopped: () => void = () => undefined;
  const onInstallFailed = vi.fn();
  const updates = await downloaded({
    onInstallFailed,
    beforeInstall: () =>
      new Promise((resolve) => {
        stopped = resolve;
      }),
  });

  void updates.install();
  await vi.advanceTimersByTimeAsync(30_000);
  stopped();
  await vi.advanceTimersByTimeAsync(0);

  expect(onInstallFailed).toHaveBeenCalledOnce();
  expect(engine.quitAndInstall).not.toHaveBeenCalled();
  expect(updates.getStatus().state).toBe("downloaded");
});

test("an install that failed does not fail again when its time runs out", async () => {
  const onInstallFailed = vi.fn();
  const updates = await downloaded({ onInstallFailed });
  await updates.install();
  engine.emit("error", new Error("ShipIt could not replace the app"));

  await vi.advanceTimersByTimeAsync(30_000);

  expect(onInstallFailed).toHaveBeenCalledOnce();
  expect(updates.getStatus().message).toBe("ShipIt could not replace the app");
});

test("an install that fails starts the daemon again and offers the restart again", async () => {
  const onInstallFailed = vi.fn();
  const updates = await downloaded({ onInstallFailed });
  await updates.install();

  engine.emit("error", new Error("ShipIt could not replace the app"));

  expect(onInstallFailed).toHaveBeenCalledOnce();
  expect(updates.getStatus()).toMatchObject({
    state: "downloaded",
    version: "0.2.0",
    message: "ShipIt could not replace the app",
  });
});

test("a new channel is saved, changes what the engine asks for and checks at once", async () => {
  const updates = controller();
  updates.start();

  expect(updates.setSettings({ channel: "prerelease" })).toEqual({ autoDownload: true, channel: "prerelease" });
  await vi.advanceTimersByTimeAsync(0);

  expect(saved).toEqual({ channel: "prerelease" });
  expect(engine.allowPrerelease).toBe(true);
  expect(engine.checkForUpdates).toHaveBeenCalledOnce();
});

test("the same channel again does not check", async () => {
  const updates = controller();
  updates.start();

  updates.setSettings({ channel: "stable" });
  await vi.advanceTimersByTimeAsync(0);

  expect(engine.checkForUpdates).not.toHaveBeenCalled();
});

test("turning downloads on while a version waits starts the download", async () => {
  saved = { autoDownload: false };
  engine.offers("0.2.0");
  const updates = controller();
  updates.start();
  await updates.check();

  updates.setSettings({ autoDownload: true });
  await vi.advanceTimersByTimeAsync(0);

  expect(engine.downloadUpdate).toHaveBeenCalledOnce();
});

test.each([
  [
    Object.assign(new Error("Cannot find latest-mac.yml"), { code: "ERR_UPDATER_CHANNEL_FILE_NOT_FOUND" }),
    "The latest release has no update files yet. Try again later.",
  ],
  [
    Object.assign(new Error("No published versions on GitHub"), { code: "ERR_UPDATER_NO_PUBLISHED_VERSIONS" }),
    "No release to update from yet.",
  ],
  [
    new Error('HttpError: 404 "method: GET url: https://github.com/deividfortuna/babysitter/releases.atom"'),
    "No release to update from yet.",
  ],
  [new Error("getaddrinfo ENOTFOUND github.com"), "Could not reach GitHub. Check the connection and try again."],
  [new Error("net::ERR_INTERNET_DISCONNECTED"), "Could not reach GitHub. Check the connection and try again."],
  [
    new Error("Cannot update while running on a read-only volume"),
    "Move Babysitter to the Applications folder, then try again.",
  ],
  [new Error("something else"), "something else"],
  ["a string", "a string"],
])("describes %s", (error, message) => {
  expect(describeUpdateError(error)).toBe(message);
});
