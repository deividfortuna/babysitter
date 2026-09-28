import { readFileSync } from "node:fs";
import path from "node:path";
import { expect, test } from "vite-plus/test";
import { macSigning, updateResources } from "./mac-signing";

const notaryKey = {
  APPLE_API_KEY: "/tmp/AuthKey.p8",
  APPLE_API_KEY_ID: "KEYID",
  APPLE_API_ISSUER: "issuer-uuid",
};

const namedSign = (identity: string) => ({ identity, continueOnError: false });
const adHocSign = { identity: "-", identityValidation: false, continueOnError: false };

test("a machine with no setting signs with the identity the keychain holds", () => {
  expect(macSigning({})).toEqual({ osxSign: {} });
});

test("skip leaves the app without a signature, whatever else is set", () => {
  expect(
    macSigning({ BABYSITTER_SKIP_SIGN: "1", BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X", ...notaryKey }),
  ).toEqual({});
});

test("a named identity signs with that identity", () => {
  expect(macSigning({ BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X (TEAM)" })).toEqual({
    osxSign: namedSign("Developer ID Application: X (TEAM)"),
  });
});

test("a named identity that fails to sign fails the build", () => {
  expect(macSigning({ BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X" }).osxSign?.continueOnError).toBe(false);
});

test("ad hoc signs with no identity, so an arm64 build still launches", () => {
  expect(macSigning({ BABYSITTER_ADHOC_SIGN: "1" })).toMatchObject({ osxSign: adHocSign });
});

test("ad hoc hands the dash to codesign instead of looking for it in the keychain", () => {
  expect(macSigning({ BABYSITTER_ADHOC_SIGN: "1" }).osxSign?.identityValidation).toBe(false);
});

test("an ad hoc signature that fails fails the build", () => {
  expect(macSigning({ BABYSITTER_ADHOC_SIGN: "1" }).osxSign?.continueOnError).toBe(false);
});

test("ad hoc leaves out the hardened runtime, whose library check refuses a framework with no team", () => {
  const optionsForFile = macSigning({ BABYSITTER_ADHOC_SIGN: "1" }).osxSign?.optionsForFile;
  expect(optionsForFile?.("Babysitter.app/Contents/Frameworks/Electron Framework.framework")).toEqual({
    hardenedRuntime: false,
  });
});

test("a named identity keeps the hardened runtime that notarization asks for", () => {
  expect(
    macSigning({ BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X" }).osxSign?.optionsForFile,
  ).toBeUndefined();
});

test("a named identity wins over ad hoc", () => {
  expect(macSigning({ BABYSITTER_ADHOC_SIGN: "1", BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X" })).toEqual({
    osxSign: namedSign("Developer ID Application: X"),
  });
});

test("a full App Store Connect key notarizes the signed app", () => {
  expect(macSigning({ BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X", ...notaryKey })).toEqual({
    osxSign: namedSign("Developer ID Application: X"),
    osxNotarize: { appleApiKey: "/tmp/AuthKey.p8", appleApiKeyId: "KEYID", appleApiIssuer: "issuer-uuid" },
  });
});

test("a partial App Store Connect key does not notarize", () => {
  expect(
    macSigning({ BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X", APPLE_API_KEY: "/tmp/AuthKey.p8" }),
  ).toEqual({
    osxSign: namedSign("Developer ID Application: X"),
  });
});

test("an ad hoc signature is never sent to notarization", () => {
  const signing = macSigning({ BABYSITTER_ADHOC_SIGN: "1", ...notaryKey });
  expect(signing).toMatchObject({ osxSign: adHocSign });
  expect(signing.osxNotarize).toBeUndefined();
});

test("blank values count as not set", () => {
  expect(macSigning({ BABYSITTER_SIGN_IDENTITY: "  ", BABYSITTER_ADHOC_SIGN: "" })).toEqual({ osxSign: {} });
});

test("a build signed with a named identity ships the update feed settings", () => {
  expect(updateResources({ BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X" })).toEqual([
    "assets/app-update.yml",
  ]);
});

test("an ad hoc build ships no update feed settings, because Squirrel refuses to install over an ad hoc signature", () => {
  expect(updateResources({ BABYSITTER_ADHOC_SIGN: "1" })).toEqual([]);
});

test("a build with no signature setting ships no update feed settings", () => {
  expect(updateResources({})).toEqual([]);
});

test("skip ships no update feed settings, even with a named identity", () => {
  expect(
    updateResources({ BABYSITTER_SKIP_SIGN: "1", BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X" }),
  ).toEqual([]);
});

test("a preview ships no update feed settings, even with a named identity, so it never updates", () => {
  expect(
    updateResources({ BABYSITTER_RELEASE_CHANNEL: "preview", BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X" }),
  ).toEqual([]);
});

test("a nightly with a named identity ships the update feed settings", () => {
  expect(
    updateResources({ BABYSITTER_RELEASE_CHANNEL: "nightly", BABYSITTER_SIGN_IDENTITY: "Developer ID Application: X" }),
  ).toEqual(["assets/app-update.yml"]);
});

test("the update feed settings point at the GitHub releases of babysitter", () => {
  const yml = readFileSync(path.join(__dirname, "..", "assets", "app-update.yml"), "utf8");
  expect(yml).toBe(
    [
      "provider: github",
      "owner: deividfortuna",
      "repo: babysitter",
      "updaterCacheDirName: babysitter-updater",
      "",
    ].join("\n"),
  );
});
