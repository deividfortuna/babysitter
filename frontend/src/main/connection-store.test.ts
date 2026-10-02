import { mkdtempSync, rmSync, statSync, writeFileSync as realWriteFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, expect, test, vi } from "vite-plus/test";
import type { Connections } from "../shared/connections";

const crash = vi.hoisted(() => ({ midWrite: false }));

vi.mock("node:fs", async (importOriginal) => {
  const fs = await importOriginal<typeof import("node:fs")>();
  const writeFileSync: typeof fs.writeFileSync = (file, data, options) => {
    if (!crash.midWrite) return fs.writeFileSync(file, data, options);
    const half = typeof data === "string" ? data.slice(0, data.length / 2) : data;
    fs.writeFileSync(file, half, options);
    throw new Error("the app stopped during the write");
  };
  return { ...fs, writeFileSync, default: { ...fs, writeFileSync } };
});

const { connectionsPath, readConnections, writeConnections } = await import("./connection-store");

const STUDIO: Connections = {
  activeId: "r1",
  remotes: [{ id: "r1", name: "studio", url: "http://studio.local:7420", token: "secret" }],
};

let dir: string;

beforeEach(() => {
  dir = mkdtempSync(path.join(os.tmpdir(), "connections-"));
});

afterEach(() => {
  crash.midWrite = false;
  rmSync(dir, { recursive: true, force: true });
});

test("a write that stops halfway keeps the connections saved before", () => {
  writeConnections(dir, STUDIO);

  crash.midWrite = true;
  expect(() => writeConnections(dir, { activeId: "local", remotes: [] })).toThrow("the app stopped during the write");
  crash.midWrite = false;

  expect(readConnections(dir)).toEqual(STUDIO);
});

test.skipIf(process.platform === "win32")("only the user can read the saved connections", () => {
  realWriteFileSync(connectionsPath(dir), "{}", { mode: 0o644 });

  writeConnections(dir, STUDIO);

  expect(statSync(connectionsPath(dir)).mode & 0o777).toBe(0o600);
});
