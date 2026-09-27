import { mkdtempSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, beforeEach, expect, test } from "vite-plus/test";
import { readUpdateSettings, writeUpdateSettings } from "./update-settings";

let dir: string;

beforeEach(() => {
  dir = mkdtempSync(path.join(os.tmpdir(), "babysitter-update-settings-"));
});

afterEach(() => {
  rmSync(dir, { recursive: true, force: true });
});

test("no file means no saved choice", () => {
  expect(readUpdateSettings(dir)).toEqual({});
});

test("a file that is not JSON means no saved choice", () => {
  writeFileSync(path.join(dir, "update-settings.json"), "{not json");
  expect(readUpdateSettings(dir)).toEqual({});
});

test("keys with the wrong type are dropped when read", () => {
  writeFileSync(path.join(dir, "update-settings.json"), JSON.stringify({ autoDownload: "yes", channel: "prerelease" }));
  expect(readUpdateSettings(dir)).toEqual({ channel: "prerelease" });
});

test("a written choice reads back", () => {
  writeUpdateSettings(dir, { autoDownload: false, channel: "stable" });
  expect(readUpdateSettings(dir)).toEqual({ autoDownload: false, channel: "stable" });
});

test("a write makes the data directory when it is missing", () => {
  const nested = path.join(dir, "data");
  writeUpdateSettings(nested, { channel: "prerelease" });
  expect(JSON.parse(readFileSync(path.join(nested, "update-settings.json"), "utf8"))).toEqual({
    channel: "prerelease",
  });
});

test("a write replaces the file whole and leaves no temporary file", () => {
  writeUpdateSettings(dir, { autoDownload: false });
  writeUpdateSettings(dir, { channel: "stable" });
  expect(readdirSync(dir)).toEqual(["update-settings.json"]);
  expect(readUpdateSettings(dir)).toEqual({ channel: "stable" });
});

test("a write to a place that cannot hold the file does not throw", () => {
  const file = path.join(dir, "file");
  writeFileSync(file, "");
  expect(() => writeUpdateSettings(path.join(file, "data"), { channel: "stable" })).not.toThrow();
});
