import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, onTestFinished, test, vi } from "vitest";
import { killLoginShells, shellRunner } from "./login-shell";

function isAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

const isWindows = process.platform === "win32";

test.skipIf(isWindows)("gives the standard output of a shell that exits with 0", async () => {
  expect(await shellRunner(5_000)("/bin/sh", ["-c", "printf 'a\\0b'; echo noise >&2"])).toBe("a\0b");
});

test.skipIf(isWindows)("gives nothing for a shell that exits with an error", async () => {
  expect(await shellRunner(5_000)("/bin/sh", ["-c", "printf out; exit 3"])).toBeNull();
});

test("gives nothing for a shell that is not there", async () => {
  expect(await shellRunner(5_000)("/no/such/shell", [])).toBeNull();
});

test.skipIf(isWindows)("kills a shell that waits past the timeout", async () => {
  const started = Date.now();

  expect(await shellRunner(100)("/bin/sh", ["-c", "sleep 10"])).toBeNull();
  expect(Date.now() - started).toBeLessThan(5_000);
});

test.skipIf(isWindows)("kills the processes a shell started when it waits past the timeout", async () => {
  const dir = mkdtempSync(path.join(os.tmpdir(), "babysitter-login-shell-"));
  onTestFinished(() => rmSync(dir, { recursive: true, force: true }));
  const pidFile = path.join(dir, "pid");

  expect(await shellRunner(300)("/bin/sh", ["-c", `sleep 30 & echo $! > '${pidFile}'; wait`])).toBeNull();

  const pid = Number(readFileSync(pidFile, "utf8"));
  await vi.waitFor(() => expect(isAlive(pid)).toBe(false));
});

test.skipIf(isWindows)("kills a running shell and the processes it started when the app quits", async () => {
  const dir = mkdtempSync(path.join(os.tmpdir(), "babysitter-login-shell-"));
  onTestFinished(() => rmSync(dir, { recursive: true, force: true }));
  const pidFile = path.join(dir, "pid");
  const running = shellRunner(10_000)("/bin/sh", ["-c", `sleep 30 & echo $! > '${pidFile}'; wait`]);
  const pid = await vi.waitFor(() => {
    const read = Number(readFileSync(pidFile, "utf8"));
    expect(read).toBeGreaterThan(0);
    return read;
  });

  killLoginShells();

  expect(await running).toBeNull();
  await vi.waitFor(() => expect(isAlive(pid)).toBe(false));
});

test.skipIf(isWindows)(
  "gives the output when the shell exits while a process it started still holds the output",
  async () => {
    const started = Date.now();

    expect(await shellRunner(2_000)("/bin/sh", ["-c", "sleep 5 & printf ok"])).toBe("ok");
    expect(Date.now() - started).toBeLessThan(1_000);
  },
);

test.skipIf(isWindows)("gives the shell no input, so a startup file that reads one does not wait", async () => {
  expect(await shellRunner(5_000)("/bin/sh", ["-c", "read line; printf 'read %s' \"$?\""])).toBe("read 1");
});
