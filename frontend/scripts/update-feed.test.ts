import { expect, test } from "vitest";
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

test("alpha and beta prereleases make a feed", () => {
  const alpha = { ...arm, name: "Babysitter-0.2.0-alpha.1-darwin-arm64.zip" };
  expect(renderFeed({ version: "0.2.0-alpha.1", files: [alpha], releaseDate })).toContain("version: 0.2.0-alpha.1");
  const beta = { ...arm, name: "Babysitter-0.2.0-beta.3-darwin-arm64.zip" };
  expect(renderFeed({ version: "0.2.0-beta.3", files: [beta], releaseDate })).toContain("version: 0.2.0-beta.3");
});

test("a version with a leading v is refused", () => {
  expect(() => renderFeed({ version: "v0.2.0", files: [arm], releaseDate })).toThrow(/leading v/);
});

test("a prerelease that is not alpha or beta is refused, because the updater treats it as a channel of its own", () => {
  const rc = { ...arm, name: "Babysitter-0.2.0-rc.1-darwin-arm64.zip" };
  expect(() => renderFeed({ version: "0.2.0-rc.1", files: [rc], releaseDate })).toThrow(/alpha or beta/);
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
