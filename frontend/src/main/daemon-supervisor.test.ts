import { EventEmitter } from "node:events";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { PassThrough } from "node:stream";
import { http, HttpResponse } from "msw";
import { afterEach, expect, test, vi } from "vitest";
import { server } from "@test/msw";
import type { DaemonStatus } from "../shared/daemon-status";
import type { Env } from "../shared/shell-env";

const spawn = vi.hoisted(() => vi.fn());
vi.mock("node:child_process", () => ({ spawn, default: { spawn } }));

const { DaemonSupervisor } = await import("./daemon-supervisor");

class FakeChild extends EventEmitter {
  pid = 4242;
  exitCode: number | null = null;
  signalCode: NodeJS.Signals | null = null;
  stdout = new PassThrough();
  stderr = new PassThrough();
  kill = vi.fn(() => this.exit());
  exit() {
    this.exitCode = 0;
    this.emit("exit", 0, null);
  }
}

const children: FakeChild[] = [];

spawn.mockImplementation(() => {
  const child = new FakeChild();
  children.push(child);
  return child;
});

const dataDirs: string[] = [];

afterEach(() => {
  vi.useRealTimers();
  for (const child of children.splice(0)) child.exit();
  for (const dir of dataDirs.splice(0)) rmSync(dir, { recursive: true, force: true });
  spawn.mockClear();
});

function supervisor(env: () => Promise<Env>) {
  const dataDir = mkdtempSync(path.join(os.tmpdir(), "babysitter-supervisor-"));
  dataDirs.push(dataDir);
  const daemon = new DaemonSupervisor({
    launch: { command: "/app/daemon/babysitter", source: "bundled" },
    dataDir,
    env,
  });
  return { daemon, dataDir };
}

test("starts the daemon with the environment the app resolves", async () => {
  const env = { PATH: "/opt/homebrew/bin:/usr/bin", GITHUB_TOKEN: "ghp_shell" };

  await supervisor(async () => env).daemon.start();

  expect(spawn).toHaveBeenCalledTimes(1);
  expect(spawn.mock.calls[0][2]).toMatchObject({ env });
});

test("says it reads the environment, then that it starts the daemon", async () => {
  const { daemon } = supervisor(async () => ({ PATH: "/usr/bin" }));
  const steps: DaemonStatus[] = [];
  daemon.onStatus((status) => steps.push(status));

  await daemon.start();

  expect(steps).toEqual([
    { state: "starting" },
    { state: "starting", step: "environment" },
    { state: "starting", step: "daemon" },
  ]);
});

test("attaches to a daemon that runs without reading the environment", async () => {
  const env = vi.fn(async () => ({ PATH: "/usr/bin" }));
  const { daemon, dataDir } = supervisor(env);
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 99, port: 50123, owner: "cli" }));
  server.use(http.get("http://127.0.0.1:50123/api/v1/healthz", () => HttpResponse.json({ ok: true })));

  await daemon.start();

  expect(env).not.toHaveBeenCalled();
  expect(spawn).not.toHaveBeenCalled();
  expect(daemon.getStatus()).toMatchObject({ state: "ready", source: "attached", port: 50123 });
});

test("starts no daemon when the app stops it while the environment is pending", async () => {
  let resolveEnv: (env: Env) => void = () => undefined;
  const pending = new Promise<Env>((resolve) => {
    resolveEnv = resolve;
  });
  const { daemon } = supervisor(() => pending);

  const started = daemon.start();
  await daemon.stop();
  resolveEnv({ PATH: "/usr/bin" });
  await started;

  expect(spawn).not.toHaveBeenCalled();
  expect(daemon.getStatus()).toEqual({ state: "stopped" });
});

test("kills the daemon it spawned when the app stops it before the daemon reports a port", async () => {
  const { daemon } = supervisor(async () => ({ PATH: "/usr/bin" }));
  await daemon.start();
  expect(daemon.getStatus()).toEqual({ state: "starting", step: "daemon" });

  await daemon.stop();

  expect(children[0].kill).toHaveBeenCalledOnce();
  expect(daemon.getStatus()).toEqual({ state: "stopped" });
});

test("stays stopped when the app stops the daemon while it probes the health of the spawned one", async () => {
  let probed: () => void = () => undefined;
  const probing = new Promise<void>((resolve) => {
    probed = resolve;
  });
  let answerProbe: () => void = () => undefined;
  const answer = new Promise<void>((resolve) => {
    answerProbe = resolve;
  });
  server.use(
    http.get("http://127.0.0.1:50124/api/v1/healthz", async () => {
      probed();
      await answer;
      return HttpResponse.json({ ok: true });
    }),
  );
  const { daemon, dataDir } = supervisor(async () => ({ PATH: "/usr/bin" }));
  await daemon.start();
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 4242, port: 50124, owner: "cli" }));
  await probing;

  await daemon.stop();
  answerProbe();

  await vi.waitFor(() => expect(daemon.getStatus()).toEqual({ state: "stopped" }), { timeout: 500 });
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(daemon.getStatus()).toEqual({ state: "stopped" });
});

test("starts one daemon when a restart comes while the environment is pending", async () => {
  let resolveEnv: (env: Env) => void = () => undefined;
  const pending = new Promise<Env>((resolve) => {
    resolveEnv = resolve;
  });
  const { daemon } = supervisor(() => pending);

  const started = daemon.start();
  const restarted = daemon.restart();
  resolveEnv({ PATH: "/usr/bin" });
  await Promise.all([started, restarted]);

  expect(spawn).toHaveBeenCalledTimes(1);
});

test("keeps the daemon of a restart when the old one exits after the kill wait", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  const { daemon } = supervisor(async () => ({ PATH: "/usr/bin" }));
  await daemon.start();
  const old = children[0];
  old.kill.mockImplementation(() => undefined);

  const restarted = daemon.restart();
  await vi.advanceTimersByTimeAsync(3_000);
  await restarted;
  expect(spawn).toHaveBeenCalledTimes(2);

  old.exit();

  expect(daemon.getStatus()).toEqual({ state: "starting", step: "daemon" });
});

test("starts no daemon when the app stops it while a restart waits for the old one to exit", async () => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  const { daemon } = supervisor(async () => ({ PATH: "/usr/bin" }));
  await daemon.start();
  children[0].kill.mockImplementation(() => undefined);

  const restarted = daemon.restart();
  await daemon.stop();
  await vi.advanceTimersByTimeAsync(3_000);
  await restarted;

  expect(spawn).toHaveBeenCalledTimes(1);
  expect(daemon.getStatus()).toEqual({ state: "stopped" });
});

function daemonOnPort(port: number, options: { downAfterShutdown: boolean }) {
  const daemon = { up: true, shutdowns: 0 };
  server.use(
    http.get(`http://127.0.0.1:${port}/api/v1/healthz`, () =>
      daemon.up ? HttpResponse.json({ ok: true }) : HttpResponse.error(),
    ),
    http.post(`http://127.0.0.1:${port}/api/v1/control/shutdown`, () => {
      daemon.shutdowns++;
      if (options.downAfterShutdown) setTimeout(() => (daemon.up = false), 50);
      return HttpResponse.json({ ok: true });
    }),
  );
  return daemon;
}

test("an install waits until the daemon the app spawned stops answering", async () => {
  const running = daemonOnPort(50125, { downAfterShutdown: true });
  const { daemon, dataDir } = supervisor(async () => ({ PATH: "/usr/bin" }));
  await daemon.start();
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 4242, port: 50125, owner: "app" }));
  await vi.waitFor(() => expect(daemon.getStatus()).toMatchObject({ state: "ready", source: "spawned" }), {
    timeout: 2_000,
  });

  await daemon.stopAndWait(5_000);

  expect(running.shutdowns).toBe(1);
  expect(running.up).toBe(false);
  expect(children[0].kill).not.toHaveBeenCalled();
  expect(daemon.getStatus()).toEqual({ state: "stopped" });
});

test("an install stops a daemon of the app that it attached to, so the new app cannot attach to the old daemon", async () => {
  const running = daemonOnPort(50126, { downAfterShutdown: true });
  const { daemon, dataDir } = supervisor(async () => ({ PATH: "/usr/bin" }));
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 99, port: 50126, owner: "app" }));
  await daemon.start();
  expect(daemon.getStatus()).toMatchObject({ state: "ready", source: "attached" });

  await daemon.stopAndWait(5_000);

  expect(running.shutdowns).toBe(1);
  expect(running.up).toBe(false);
});

test("an install leaves a daemon started from the terminal running", async () => {
  const running = daemonOnPort(50127, { downAfterShutdown: true });
  const { daemon, dataDir } = supervisor(async () => ({ PATH: "/usr/bin" }));
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 99, port: 50127, owner: "cli" }));
  await daemon.start();

  await daemon.stopAndWait(5_000);

  expect(running.shutdowns).toBe(0);
  expect(running.up).toBe(true);
  expect(daemon.getStatus()).toEqual({ state: "stopped" });
});

function daemonThatNeverAnswersShutdown(port: number) {
  server.use(
    http.get(`http://127.0.0.1:${port}/api/v1/healthz`, () => HttpResponse.json({ ok: true })),
    http.post(`http://127.0.0.1:${port}/api/v1/control/shutdown`, () => new Promise<Response>(() => undefined)),
  );
}

test("an install stops waiting at the deadline when an attached daemon never answers the shutdown", async () => {
  daemonThatNeverAnswersShutdown(50129);
  const { daemon, dataDir } = supervisor(async () => ({ PATH: "/usr/bin" }));
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 99, port: 50129, owner: "app" }));
  await daemon.start();
  const started = Date.now();

  await daemon.stopAndWait(600);

  expect(Date.now() - started).toBeLessThan(1_500);
});

test("an install stops waiting at the deadline when a spawned daemon never answers the shutdown", async () => {
  daemonThatNeverAnswersShutdown(50130);
  const { daemon, dataDir } = supervisor(async () => ({ PATH: "/usr/bin" }));
  await daemon.start();
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 4242, port: 50130, owner: "app" }));
  await vi.waitFor(() => expect(daemon.getStatus()).toMatchObject({ state: "ready", source: "spawned" }), {
    timeout: 2_000,
  });
  const started = Date.now();

  await daemon.stopAndWait(600);

  expect(Date.now() - started).toBeLessThan(1_500);
});

test("an install kills the daemon it spawned when it still answers at the deadline, so the new app cannot attach to it", async () => {
  daemonThatNeverAnswersShutdown(50132);
  const { daemon, dataDir } = supervisor(async () => ({ PATH: "/usr/bin" }));
  await daemon.start();
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 4242, port: 50132, owner: "app" }));
  await vi.waitFor(() => expect(daemon.getStatus()).toMatchObject({ state: "ready", source: "spawned" }), {
    timeout: 2_000,
  });

  await daemon.stopAndWait(600);

  expect(children[0].kill).toHaveBeenCalledExactlyOnceWith("SIGKILL");
});

test("a quit does not wait for ever on a daemon that never answers the shutdown", async () => {
  daemonThatNeverAnswersShutdown(50131);
  const { daemon, dataDir } = supervisor(async () => ({ PATH: "/usr/bin" }));
  await daemon.start();
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 4242, port: 50131, owner: "app" }));
  await vi.waitFor(() => expect(daemon.getStatus()).toMatchObject({ state: "ready", source: "spawned" }), {
    timeout: 2_000,
  });
  const started = Date.now();

  await daemon.stop();

  expect(Date.now() - started).toBeLessThan(3_000);
});

test("an install stops waiting at the deadline when the daemon keeps answering", async () => {
  const running = daemonOnPort(50128, { downAfterShutdown: false });
  const { daemon, dataDir } = supervisor(async () => ({ PATH: "/usr/bin" }));
  writeFileSync(path.join(dataDir, "running.json"), JSON.stringify({ pid: 99, port: 50128, owner: "app" }));
  await daemon.start();
  const started = Date.now();

  await daemon.stopAndWait(600);

  expect(running.shutdowns).toBe(1);
  expect(Date.now() - started).toBeGreaterThanOrEqual(500);
  expect(Date.now() - started).toBeLessThan(2_500);
});
