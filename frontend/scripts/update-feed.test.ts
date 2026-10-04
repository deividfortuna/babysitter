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

const preview = "0.2.1-preview.20260928.42";
const beta = "0.2.0-beta.1";

test.each([
  {
    name: "a version with a leading v is refused",
    refused: feed([armZip], { version: "v0.2.0" }),
    message: /leading v/,
  },
  {
    name: "a preview is refused, because no app may update to it",
    refused: feed([file("arm64", "zip", "a", preview)], { version: preview }),
    message: /only those have a feed/,
  },
  {
    name: "any other prerelease is refused",
    refused: feed([file("arm64", "zip", "a", beta)], { version: beta }),
    message: /only those have a feed/,
  },
  {
    name: "a Windows installer is refused, because it is not the macOS zip or DMG of this version",
    refused: feed([{ ...armZip, url: "Babysitter-0.2.0-win32-x64-Setup.exe" }]),
    message: /not the macOS zip or DMG of 0.2.0/,
  },
  {
    name: "a zip of another version is refused, because it is not the macOS zip or DMG of this version",
    refused: feed([file("arm64", "zip", "a", "0.1.0")]),
    message: /not the macOS zip or DMG of 0.2.0/,
  },
  { name: "one file twice is refused", refused: feed([armZip, armZip]), message: /arm64.zip twice/ },
  {
    name: "a feed with no zip is refused, because the updater installs the zip",
    refused: feed([armDmg]),
    message: /no zip/,
  },
  {
    name: "a checksum that is not a base64 sha512 is refused",
    refused: feed([{ ...armZip, sha512: "abc" }]),
    message: /sha512/,
  },
  { name: "a size of zero is refused", refused: feed([{ ...armZip, size: 0 }]), message: /size/ },
  { name: "a size that is not a whole number is refused", refused: feed([{ ...armZip, size: 1.5 }]), message: /size/ },
])("$name", ({ refused, message }) => {
  expect(() => renderFeed(refused)).toThrow(message);
});
