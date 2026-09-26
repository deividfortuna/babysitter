import { spawn, type ChildProcess } from "node:child_process";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { apiBaseUrl, parseRunFile, type RunFileInfo } from "../shared/daemon-discovery";
import type { DaemonLaunchSpec } from "../shared/daemon-launch";
import type { DaemonStatus } from "../shared/daemon-status";
import type { Env } from "../shared/shell-env";
import { connectSupervisor, type SupervisorLinkHandle } from "./supervisor-link";

const START_TIMEOUT_MS = 15_000;
const RUN_FILE_POLL_MS = 200;
const LOG_TAIL_LINES = 40;
const RESTART_KILL_TIMEOUT_MS = 3_000;
const SHUTDOWN_REQUEST_TIMEOUT_MS = 2_000;

export type DaemonSupervisorOptions = {
  launch: DaemonLaunchSpec;
  dataDir: string;
  env: () => Promise<Env>;
  log?: (msg: string) => void;
};

export class DaemonSupervisor {
  private status: DaemonStatus = { state: "stopped" };
  private readonly listeners = new Set<(status: DaemonStatus) => void>();
  private child: ChildProcess | null = null;
  private owner: string | undefined;
  private link: SupervisorLinkHandle | null = null;
  private logTail: string[] = [];
  private attempt = 0;
  private readonly log: (msg: string) => void;

  constructor(private readonly opts: DaemonSupervisorOptions) {
    this.log = opts.log ?? (() => undefined);
  }

  getStatus(): DaemonStatus {
    return this.status;
  }

  onStatus(listener: (status: DaemonStatus) => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private setStatus(next: DaemonStatus) {
    this.status = next;
    for (const listener of this.listeners) listener(next);
  }

  private get runFilePath(): string {
    return path.join(this.opts.dataDir, "running.json");
  }

  async start(): Promise<void> {
    const attempt = ++this.attempt;
    this.setStatus({ state: "starting" });

    const running = await this.runningDaemon();
    if (this.superseded(attempt)) return;
    if (running) {
      this.log(`daemon: attached to pid ${running.pid} on port ${running.port}`);
      this.linkTo(running);
      this.becomeReady(running, "attached");
      return;
    }

    await this.spawnDaemon(attempt);
  }

  private superseded(attempt: number): boolean {
    return attempt !== this.attempt;
  }

  private async runningDaemon(): Promise<RunFileInfo | null> {
    const existing = await this.readRunFile();
    return existing && (await probeHealth(existing.port)) ? existing : null;
  }

  private async spawnDaemon(attempt: number) {
    this.setStatus({ state: "starting", step: "environment" });
    const env = await this.opts.env();
    if (this.superseded(attempt)) return;
    this.setStatus({ state: "starting", step: "daemon" });

    const { command } = this.opts.launch;
    const args = ["daemon", "start", "--owner", "app", "--data-dir", this.opts.dataDir];
    this.log(`daemon: spawning ${command} ${args.join(" ")}`);

    let child: ChildProcess;
    try {
      child = spawn(command, args, { stdio: ["ignore", "pipe", "pipe"], windowsHide: true, env });
    } catch (err) {
      this.setStatus({ state: "error", message: `Could not start the daemon: ${(err as Error).message}` });
      return;
    }
    this.child = child;
    this.logTail = [];
    child.stdout?.setEncoding("utf8");
    child.stderr?.setEncoding("utf8");
    child.stdout?.on("data", (chunk: string) => this.collect(chunk));
    child.stderr?.on("data", (chunk: string) => this.collect(chunk));

    child.on("error", (err) => {
      this.setStatus({
        state: "error",
        message: `Could not start the daemon: ${err.message}`,
        details: this.details(),
      });
    });
    child.on("exit", (code, signal) => {
      if (this.child !== child) return;
      this.child = null;
      this.link?.dispose();
      this.link = null;
      if (this.status.state === "stopped") return;
      const reason = signal ? `signal ${signal}` : `exit code ${code}`;
      this.setStatus({ state: "error", message: `The daemon stopped (${reason}).`, details: this.details() });
    });

    void this.waitForRunFile(child);
  }

  private async waitForRunFile(child: ChildProcess) {
    const deadline = Date.now() + START_TIMEOUT_MS;
    while (Date.now() < deadline) {
      if (this.child !== child) return;
      const info = await this.readRunFile();
      if (info && info.pid === child.pid && (await probeHealth(info.port))) {
        if (this.child !== child) return;
        this.log(`daemon: ready on port ${info.port}`);
        this.linkTo(info);
        this.becomeReady(info, "spawned");
        return;
      }
      await sleep(RUN_FILE_POLL_MS);
    }
    if (this.child === child) {
      this.setStatus({ state: "error", message: "The daemon did not report a port in time.", details: this.details() });
    }
  }

  private becomeReady(info: RunFileInfo, source: "spawned" | "attached") {
    this.owner = info.owner;
    this.setStatus(ready(info, source));
  }

  private linkTo(info: RunFileInfo) {
    this.link?.dispose();
    this.link = null;
    if (info.owner !== "app" || !info.supervisor) return;
    this.link = connectSupervisor(info.supervisor, this.log);
  }

  async restart(): Promise<void> {
    const attempt = ++this.attempt;
    this.link?.dispose();
    this.link = null;
    const child = this.child;
    this.child = null;
    this.setStatus({ state: "stopped" });
    if (child && child.exitCode === null) {
      await new Promise<void>((resolve) => {
        const timer = setTimeout(resolve, RESTART_KILL_TIMEOUT_MS);
        child.once("exit", () => {
          clearTimeout(timer);
          resolve();
        });
        child.kill();
      });
    }
    if (this.superseded(attempt)) return;
    await this.start();
  }

  async stop(requestTimeoutMs = SHUTDOWN_REQUEST_TIMEOUT_MS): Promise<void> {
    this.attempt++;
    const current = this.status;
    this.setStatus({ state: "stopped" });
    this.link?.dispose();
    this.link = null;
    const child = this.child;
    if (!child) return;
    if (!current.port) {
      this.child = null;
      child.kill();
      return;
    }
    await requestShutdown(current.port, requestTimeoutMs);
  }

  async stopAndWait(timeoutMs: number): Promise<void> {
    const deadline = Date.now() + timeoutMs;
    const port = this.ownedPort();
    const spawned = this.child;
    await this.stop(timeoutMs);
    if (port === undefined) return;
    if (!spawned) await requestShutdown(port, timeLeft(deadline));
    const down = await waitUntilDown(port, deadline);
    if (!down && spawned) await killAndWait(spawned);
  }

  private ownedPort(): number | undefined {
    const ownsReadyDaemon = this.status.state === "ready" && this.owner === "app";
    return ownsReadyDaemon ? this.status.port : undefined;
  }

  private async readRunFile(): Promise<RunFileInfo | null> {
    try {
      return parseRunFile(await readFile(this.runFilePath, "utf8"));
    } catch {
      return null;
    }
  }

  private collect(chunk: string) {
    for (const line of chunk.split("\n")) {
      if (!line.trim()) continue;
      this.log(`daemon: ${line}`);
      this.logTail.push(line);
      if (this.logTail.length > LOG_TAIL_LINES) this.logTail.shift();
    }
  }

  private details(): string {
    return this.logTail.join("\n");
  }
}

function ready(info: RunFileInfo, source: "spawned" | "attached"): DaemonStatus {
  return { state: "ready", source, pid: info.pid, port: info.port, baseUrl: apiBaseUrl(info.port) };
}

function timeLeft(deadline: number): number {
  return Math.max(0, deadline - Date.now());
}

async function requestShutdown(port: number, timeoutMs: number): Promise<void> {
  try {
    await fetch(`${apiBaseUrl(port)}/control/shutdown`, { method: "POST", signal: AbortSignal.timeout(timeoutMs) });
  } catch {}
}

async function waitUntilDown(port: number, deadline: number): Promise<boolean> {
  while (Date.now() < deadline) {
    if (!(await probeHealth(port))) return true;
    await sleep(RUN_FILE_POLL_MS);
  }
  return false;
}

function hasExited(child: ChildProcess): boolean {
  return child.exitCode !== null || child.signalCode !== null;
}

function killAndWait(child: ChildProcess): Promise<void> {
  if (hasExited(child)) return Promise.resolve();
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, RESTART_KILL_TIMEOUT_MS);
    child.once("exit", () => {
      clearTimeout(timer);
      resolve();
    });
    child.kill("SIGKILL");
  });
}

async function probeHealth(port: number): Promise<boolean> {
  try {
    const res = await fetch(`${apiBaseUrl(port)}/healthz`, { signal: AbortSignal.timeout(1_000) });
    return res.ok;
  } catch {
    return false;
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
