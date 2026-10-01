import { expect, test } from "vite-plus/test";
import { mergeFeeds, parseFeed, renderFeed, type Feed } from "./update-feed";

const sha = (letter: string) => `${letter.repeat(86)}==`;

const file = (arch: string, ext: string, letter: string, version = "0.2.0") => ({
  url: `Babysitter-${version}-darwin-${arch}.${ext}`,
  sha512: sha(letter),
  size: 100,
});

const armZip = file("arm64", "zip", "a");
const armDmg = file("arm64", "dmg", "b");
const intelZip = file("x64", "zip", "c");
const intelDmg = file("x64", "dmg", "d");
const releaseDate = "2026-09-26T10:00:00.000Z";

const feed = (files: Feed["files"], overrides: Partial<Feed> = {}): Feed => ({
  version: "0.2.0",
  files,
  releaseDate,
  ...overrides,
});

const electronBuilderFeed = [
  "version: 0.2.0",
  "files:",
  "  - url: Babysitter-0.2.0-darwin-x64.zip",
  `    sha512: ${sha("c")}`,
  "    size: 100",
  "  - url: Babysitter-0.2.0-darwin-x64.dmg",
  `    sha512: ${sha("d")}`,
  "    size: 100",
  "path: Babysitter-0.2.0-darwin-x64.zip",
  `sha512: ${sha("c")}`,
  "releaseDate: '2026-09-26T09:00:00.000Z'",
  "",
].join("\n");

test("a feed of electron-builder reads into its version, its files and its release date", () => {
  expect(parseFeed(electronBuilderFeed, "x64.yml")).toEqual({
    version: "0.2.0",
    files: [intelZip, intelDmg],
    releaseDate: "2026-09-26T09:00:00.000Z",
  });
});

test("a feed with no version is refused", () => {
  expect(() => parseFeed("files:\n", "empty.yml")).toThrow(/empty.yml has no version/);
});

test("the merged feed lists the files of both arches, arm64 first, and its path is the arm64 zip", () => {
  const merged = mergeFeeds([feed([intelZip, intelDmg]), feed([armZip, armDmg])]);
  expect(renderFeed(merged)).toBe(
    [
      "version: 0.2.0",
      "files:",
      "  - url: Babysitter-0.2.0-darwin-arm64.zip",
      `    sha512: ${sha("a")}`,
      "    size: 100",
      "  - url: Babysitter-0.2.0-darwin-arm64.dmg",
      `    sha512: ${sha("b")}`,
      "    size: 100",
      "  - url: Babysitter-0.2.0-darwin-x64.zip",
      `    sha512: ${sha("c")}`,
      "    size: 100",
      "  - url: Babysitter-0.2.0-darwin-x64.dmg",
      `    sha512: ${sha("d")}`,
      "    size: 100",
      "path: Babysitter-0.2.0-darwin-arm64.zip",
      `sha512: ${sha("a")}`,
      `releaseDate: '${releaseDate}'`,
      "",
    ].join("\n"),
  );
});

test("the merged feed takes the latest release date", () => {
  const later = "2026-09-26T11:00:00.000Z";
  expect(mergeFeeds([feed([armZip]), feed([intelZip], { releaseDate: later })]).releaseDate).toBe(later);
});

test("feeds of two versions are refused", () => {
  expect(() => mergeFeeds([feed([armZip]), feed([intelZip], { version: "0.2.1" })])).toThrow(/not of one version/);
});

test("one arch alone makes a feed, for a local test build", () => {
  expect(renderFeed(feed([intelZip]))).toContain("path: Babysitter-0.2.0-darwin-x64.zip");
});

test("a nightly makes a feed", () => {
  const version = "0.2.1-nightly.20260928.41";
  expect(renderFeed(feed([file("arm64", "zip", "a", version)], { version }))).toContain(`version: ${version}`);
});

test("a version with a leading v is refused", () => {
  expect(() => renderFeed(feed([armZip], { version: "v0.2.0" }))).toThrow(/leading v/);
});

test("a preview is refused, because no app may update to it", () => {
  const version = "0.2.1-preview.20260928.42";
  expect(() => renderFeed(feed([file("arm64", "zip", "a", version)], { version }))).toThrow(/only those have a feed/);
});

test("any other prerelease is refused", () => {
  const version = "0.2.0-beta.1";
  expect(() => renderFeed(feed([file("arm64", "zip", "a", version)], { version }))).toThrow(/only those have a feed/);
});

test("a file that is not the macOS zip or DMG of this version is refused", () => {
  expect(() => renderFeed(feed([{ ...armZip, url: "Babysitter-0.2.0-win32-x64-Setup.exe" }]))).toThrow(
    /not the macOS zip or DMG of 0.2.0/,
  );
  expect(() => renderFeed(feed([file("arm64", "zip", "a", "0.1.0")]))).toThrow(/not the macOS zip or DMG of 0.2.0/);
});

test("one file twice is refused", () => {
  expect(() => renderFeed(feed([armZip, armZip]))).toThrow(/arm64.zip twice/);
});

test("a feed with no zip is refused, because the updater installs the zip", () => {
  expect(() => renderFeed(feed([armDmg]))).toThrow(/no zip/);
});

test("a checksum that is not a base64 sha512 is refused", () => {
  expect(() => renderFeed(feed([{ ...armZip, sha512: "abc" }]))).toThrow(/sha512/);
});

test("a size that is not a positive whole number is refused", () => {
  expect(() => renderFeed(feed([{ ...armZip, size: 0 }]))).toThrow(/size/);
  expect(() => renderFeed(feed([{ ...armZip, size: 1.5 }]))).toThrow(/size/);
});
