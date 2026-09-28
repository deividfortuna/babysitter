import { expect, test } from "vite-plus/test";
import { renderFeed } from "./update-feed";

const sha = (letter: string) => `${letter.repeat(86)}==`;

const arm = { name: "Babysitter-0.2.0-darwin-arm64.zip", sha512: sha("a"), size: 120 };
const intel = { name: "Babysitter-0.2.0-darwin-x64.zip", sha512: sha("b"), size: 130 };
const releaseDate = "2026-09-26T10:00:00.000Z";

test("the feed names the version, each zip with its checksum and size, and the release date", () => {
  expect(renderFeed({ version: "0.2.0", files: [arm, intel], releaseDate })).toBe(
    [
      "version: 0.2.0",
      "files:",
      "  - url: Babysitter-0.2.0-darwin-arm64.zip",
      `    sha512: ${sha("a")}`,
      "    size: 120",
      "  - url: Babysitter-0.2.0-darwin-x64.zip",
      `    sha512: ${sha("b")}`,
      "    size: 130",
      "path: Babysitter-0.2.0-darwin-arm64.zip",
      `sha512: ${sha("a")}`,
      `releaseDate: '${releaseDate}'`,
      "",
    ].join("\n"),
  );
});

test("the arm64 zip comes first whatever the order of the arguments", () => {
  const feed = renderFeed({ version: "0.2.0", files: [intel, arm], releaseDate });
  expect(feed.indexOf("arm64")).toBeLessThan(feed.indexOf("x64"));
  expect(feed).toContain("path: Babysitter-0.2.0-darwin-arm64.zip");
});

test("one arch alone makes a feed, for a local test build", () => {
  expect(renderFeed({ version: "0.2.0", files: [intel], releaseDate })).toContain(
    "path: Babysitter-0.2.0-darwin-x64.zip",
  );
});

test("a nightly makes a feed", () => {
  const nightly = { ...arm, name: "Babysitter-0.2.1-nightly.20260928.41-darwin-arm64.zip" };
  expect(renderFeed({ version: "0.2.1-nightly.20260928.41", files: [nightly], releaseDate })).toContain(
    "version: 0.2.1-nightly.20260928.41",
  );
});

test("a version with a leading v is refused", () => {
  expect(() => renderFeed({ version: "v0.2.0", files: [arm], releaseDate })).toThrow(/leading v/);
});

test("a preview is refused, because no app may update to it", () => {
  const preview = { ...arm, name: "Babysitter-0.2.1-preview.20260928.42-darwin-arm64.zip" };
  expect(() => renderFeed({ version: "0.2.1-preview.20260928.42", files: [preview], releaseDate })).toThrow(
    /only those have a feed/,
  );
});

test("any other prerelease is refused", () => {
  const beta = { ...arm, name: "Babysitter-0.2.0-beta.1-darwin-arm64.zip" };
  expect(() => renderFeed({ version: "0.2.0-beta.1", files: [beta], releaseDate })).toThrow(/only those have a feed/);
});

test("a zip that is not the macOS zip of this version is refused", () => {
  const dmg = { ...arm, name: "Babysitter-0.2.0-darwin-arm64.dmg" };
  expect(() => renderFeed({ version: "0.2.0", files: [dmg], releaseDate })).toThrow(
    /darwin-arm64.zip or darwin-x64.zip/,
  );
  const older = { ...arm, name: "Babysitter-0.1.0-darwin-arm64.zip" };
  expect(() => renderFeed({ version: "0.2.0", files: [older], releaseDate })).toThrow(
    /darwin-arm64.zip or darwin-x64.zip/,
  );
});

test("two zips of one arch are refused", () => {
  expect(() => renderFeed({ version: "0.2.0", files: [arm, arm], releaseDate })).toThrow(/arm64 twice/);
});

test("a feed with no zip is refused", () => {
  expect(() => renderFeed({ version: "0.2.0", files: [], releaseDate })).toThrow(/no zip/);
});

test("a checksum that is not a base64 sha512 is refused", () => {
  expect(() => renderFeed({ version: "0.2.0", files: [{ ...arm, sha512: "abc" }], releaseDate })).toThrow(/sha512/);
});

test("a size that is not a positive whole number is refused", () => {
  expect(() => renderFeed({ version: "0.2.0", files: [{ ...arm, size: 0 }], releaseDate })).toThrow(/size/);
  expect(() => renderFeed({ version: "0.2.0", files: [{ ...arm, size: 1.5 }], releaseDate })).toThrow(/size/);
});
