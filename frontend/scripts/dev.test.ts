import { EventEmitter } from "node:events";
import { afterEach, expect, test, vi } from "vite-plus/test";

type FakeChild = EventEmitter & { kill: () => void };

const dev = vi.hoisted(() => ({
  apps: [] as FakeChild[],
  bundleChanged: [] as (() => void)[],
}));

vi.mock("node:child_process", async () => {
  const { EventEmitter } = await import("node:events");
  const childProcess = {
    spawn: (command: string) => {
      const child = Object.assign(new EventEmitter(), { kill: () => {} });
      if (command !== "vp") dev.apps.push(child);
      return child;
    },
  };
  return { ...childProcess, default: childProcess };
});

vi.mock("node:fs", () => {
  const fs = {
    existsSync: () => true,
    watchFile: (_file: string, _options: unknown, listener: () => void) => dev.bundleChanged.push(listener),
    unwatchFile: () => {},
  };
  return { ...fs, default: fs };
});

vi.mock("electron", () => ({ default: "/electron" }));

vi.mock("vite", () => ({
  createServer: async () => ({
    listen: async () => {},
    resolvedUrls: { local: ["http://localhost:5173/"] },
    printUrls: () => {},
    close: async () => {},
  }),
}));

const RESTART_DEBOUNCE_MS = 300;

const signalHandlers = new Map<string, () => void>();

async function startDev() {
  dev.apps = [];
  dev.bundleChanged = [];
  signalHandlers.clear();
  vi.resetModules();
  vi.useFakeTimers();
  vi.spyOn(process, "once").mockImplementation((event, listener) => {
    signalHandlers.set(String(event), listener as () => void);
    return process;
  });
  vi.spyOn(process, "exit").mockImplementation(() => undefined as never);
  await import("./dev.mjs");
  await vi.advanceTimersByTimeAsync(RESTART_DEBOUNCE_MS);
}

async function changeBundle() {
  for (const listener of dev.bundleChanged) listener();
  await vi.advanceTimersByTimeAsync(RESTART_DEBOUNCE_MS);
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

test("an app that exits after the runner begins to stop is not replaced by a new one", async () => {
  await startDev();
  const [first] = dev.apps;

  await changeBundle();
  signalHandlers.get("SIGINT")?.();
  first.emit("exit", 0);

  expect(dev.apps).toHaveLength(1);
});

test("bundle changes while the old app still stops start one new app, not one for each change", async () => {
  await startDev();
  const [first] = dev.apps;

  await changeBundle();
  await changeBundle();
  first.emit("exit", 0);

  expect(dev.apps).toHaveLength(2);
});
