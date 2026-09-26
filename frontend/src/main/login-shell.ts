import { spawn, type ChildProcess } from "node:child_process";
import type { ShellRunner } from "../shared/shell-env";

const PIPE_DRAIN_MS = 200;

const runningShells = new Set<ChildProcess>();

function killProcessGroup(child: ChildProcess) {
  if (child.pid === undefined) return;
  try {
    process.kill(-child.pid, "SIGKILL");
  } catch {
    child.kill("SIGKILL");
  }
}

export function killLoginShells() {
  for (const child of runningShells) killProcessGroup(child);
}

export function shellRunner(timeoutMs: number): ShellRunner {
  return (shell, args) =>
    new Promise((resolve) => {
      let child: ChildProcess;
      try {
        child = spawn(shell, args, { stdio: ["ignore", "pipe", "ignore"], windowsHide: true, detached: true });
      } catch {
        resolve(null);
        return;
      }
      runningShells.add(child);
      const timer = setTimeout(() => {
        runningShells.delete(child);
        killProcessGroup(child);
        resolve(null);
      }, timeoutMs);
      const finish = (result: string | null) => {
        runningShells.delete(child);
        clearTimeout(timer);
        child.stdout?.destroy();
        resolve(result);
      };
      let stdout = "";
      child.stdout?.setEncoding("utf8");
      child.stdout?.on("data", (chunk: string) => {
        stdout += chunk;
      });
      child.once("error", () => finish(null));
      child.once("exit", (code) => {
        if (code !== 0) {
          finish(null);
          return;
        }
        if (child.stdout?.readableEnded) {
          finish(stdout);
          return;
        }
        const drain = setTimeout(() => finish(stdout), PIPE_DRAIN_MS);
        child.stdout?.once("end", () => {
          clearTimeout(drain);
          finish(stdout);
        });
      });
    });
}
