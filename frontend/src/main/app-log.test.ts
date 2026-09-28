import { existsSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, expect, test } from "vite-plus/test";
import type { LogRecord } from "../shared/logs";
import { AppLog } from "./app-log";

const dirs: string[] = [];

afterEach(() => {
  for (const dir of dirs.splice(0)) rmSync(dir, { recursive: true, force: true });
});

function logFile(): string {
  const dir = mkdtempSync(path.join(os.tmpdir(), "babysitter-app-log-"));
  dirs.push(dir);
  return path.join(dir, "logs", "app.log");
}

function fileRecords(file: string): LogRecord[] {
  return readFileSync(file, "utf8")
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line) as LogRecord);
}

test("writes each record to the file as one JSON line with its attributes", () => {
  const file = logFile();
  const log = new AppLog({ file, now: () => new Date("2026-09-28T10:00:00Z") });

  log.info("daemon: attached to pid 42", { port: 50123 });
  log.warn("update: the check failed");

  expect(fileRecords(file)).toEqual([
    {
      seq: 1,
      time: "2026-09-28T10:00:00.000Z",
      level: "info",
      msg: "daemon: attached to pid 42",
      attrs: [{ key: "port", value: "50123" }],
    },
    { seq: 2, time: "2026-09-28T10:00:00.000Z", level: "warn", msg: "update: the check failed", attrs: [] },
  ]);
});

test("keeps the last records in memory and tells the listeners of each new one", () => {
  const log = new AppLog({ file: logFile(), keep: 2 });
  const seen: string[] = [];
  const stop = log.onRecord((record) => seen.push(record.msg));

  log.info("one");
  log.info("two");
  stop();
  log.info("three");

  expect(log.records().map((r) => r.msg)).toEqual(["two", "three"]);
  expect(seen).toEqual(["one", "two"]);
});

test("moves the file aside when the next line would pass the size", () => {
  const file = logFile();
  const log = new AppLog({ file, maxBytes: 200 });

  log.info("first", { padding: "x".repeat(60) });
  log.info("second", { padding: "x".repeat(60) });

  expect(existsSync(`${file}.1`)).toBe(true);
  expect(fileRecords(`${file}.1`).map((r) => r.msg)).toEqual(["first"]);
  expect(fileRecords(file).map((r) => r.msg)).toEqual(["second"]);
});

test("keeps the records in memory when the file cannot be written", () => {
  const log = new AppLog({ file: "/dev/null/app.log" });

  log.error("the daemon stopped");

  expect(log.records().map((r) => r.msg)).toEqual(["the daemon stopped"]);
});
