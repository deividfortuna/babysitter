import { expect, test } from "vite-plus/test";
import { builderConfig, macSignature, updateFeed, type DesktopBuild } from "./desktop-builder";

const notaryKey = {
  APPLE_API_KEY: "/tmp/AuthKey.p8",
  APPLE_API_KEY_ID: "KEYID",
  APPLE_API_ISSUER: "issuer-uuid",
};

const identity = "Developer ID Application: X (TEAM)";
const certificate = "X (TEAM)";

const build = (overrides: Partial<DesktopBuild> = {}): DesktopBuild => ({
  platform: "darwin",
  version: "0.2.0",
  frontendDir: "/repo/frontend",
  outputDir: "/repo/frontend/out/release",
  env: {},
  ...overrides,
});

test("a machine with no setting signs with the identity the keychain holds and does not notarize", () => {
  expect(macSignature({})).toEqual({ notarize: false });
});

test("skip leaves the app without a signature, whatever else is set", () => {
  expect(
    macSignature({ BABYSITTER_SKIP_SIGN: "1", BABYSITTER_SIGN_IDENTITY: identity, BABYSITTER_ADHOC_SIGN: "1" }),
  ).toEqual({ identity: null, notarize: false });
});

test("a named identity signs with that identity and keeps the hardened runtime that notarization asks for", () => {
  expect(macSignature({ BABYSITTER_SIGN_IDENTITY: identity })).toEqual({ identity: certificate, notarize: false });
});

test("a full App Store Connect key notarizes the app a named identity signs", () => {
  expect(macSignature({ BABYSITTER_SIGN_IDENTITY: identity, ...notaryKey })).toEqual({
    identity: certificate,
    notarize: true,
  });
});

test("a partial App Store Connect key does not notarize", () => {
  expect(macSignature({ BABYSITTER_SIGN_IDENTITY: identity, APPLE_API_KEY: "/tmp/AuthKey.p8" }).notarize).toBe(false);
});

test("ad hoc signs with the dash and leaves out the hardened runtime, whose library check refuses the framework", () => {
  expect(macSignature({ BABYSITTER_ADHOC_SIGN: "1" })).toEqual({
    identity: "-",
    hardenedRuntime: false,
    notarize: false,
  });
});

test("an ad hoc signature is never sent to notarization", () => {
  expect(macSignature({ BABYSITTER_ADHOC_SIGN: "1", ...notaryKey }).notarize).toBe(false);
});

test("a named identity wins over ad hoc", () => {
  expect(macSignature({ BABYSITTER_ADHOC_SIGN: "1", BABYSITTER_SIGN_IDENTITY: identity }).identity).toBe(certificate);
});

test("the identity loses its Developer ID prefix, which electron-builder refuses, and keeps a name without it", () => {
  expect(macSignature({ BABYSITTER_SIGN_IDENTITY: identity }).identity).toBe(certificate);
  expect(macSignature({ BABYSITTER_SIGN_IDENTITY: certificate }).identity).toBe(certificate);
});

test("blank values count as not set", () => {
  expect(macSignature({ BABYSITTER_SIGN_IDENTITY: "  ", BABYSITTER_ADHOC_SIGN: "" })).toEqual({ notarize: false });
});

test("a stable build signed with a named identity reads the latest feed of the GitHub releases", () => {
  expect(updateFeed(build({ env: { BABYSITTER_SIGN_IDENTITY: identity } }))).toEqual({
    provider: "github",
    owner: "deividfortuna",
    repo: "babysitter",
    releaseType: "release",
  });
});

test("a nightly signed with a named identity reads the nightly feed of the prereleases", () => {
  expect(
    updateFeed(build({ version: "0.2.1-nightly.20260928.41", env: { BABYSITTER_SIGN_IDENTITY: identity } })),
  ).toEqual({
    provider: "github",
    owner: "deividfortuna",
    repo: "babysitter",
    releaseType: "prerelease",
    channel: "nightly",
  });
});

test("an ad hoc build has no feed, because Squirrel refuses to install over an ad hoc signature", () => {
  expect(updateFeed(build({ env: { BABYSITTER_ADHOC_SIGN: "1" } }))).toBeNull();
});

test("a build with no signature setting has no feed", () => {
  expect(updateFeed(build())).toBeNull();
});

test("skip has no feed, even with a named identity", () => {
  expect(updateFeed(build({ env: { BABYSITTER_SKIP_SIGN: "1", BABYSITTER_SIGN_IDENTITY: identity } }))).toBeNull();
});

test("a preview has no feed, even with a named identity, so it never updates", () => {
  expect(
    updateFeed(build({ version: "0.2.1-preview.20260928.42", env: { BABYSITTER_SIGN_IDENTITY: identity } })),
  ).toBeNull();
});

test("the Windows installer has no feed, because the app updates itself on macOS only", () => {
  expect(updateFeed(build({ platform: "win32", env: { BABYSITTER_SIGN_IDENTITY: identity } }))).toBeNull();
});

test("the app keeps its bundle id and its name, so a new build updates the old one in place", () => {
  const config = builderConfig(build());
  expect({ appId: config.appId, productName: config.productName }).toEqual({
    appId: "com.deividfortuna.babysitter",
    productName: "Babysitter",
  });
});

test("the assets keep the names of the releases, which the cask and the feed point at", () => {
  const config = builderConfig(build());
  expect(config.mac?.artifactName).toBe("Babysitter-${version}-darwin-${arch}.${ext}");
  expect(config.win?.artifactName).toBe("Babysitter-${version}-win32-${arch}-Setup.${ext}");
});

test("the daemon and the icons of the tray go into the resources of the app", () => {
  expect(builderConfig(build()).extraResources).toEqual([
    { from: "/repo/frontend/daemon", to: "daemon" },
    { from: "/repo/frontend/assets/icon.png", to: "icon.png" },
    { from: "/repo/frontend/assets/trayTemplate.png", to: "trayTemplate.png" },
    { from: "/repo/frontend/assets/trayTemplate@2x.png", to: "trayTemplate@2x.png" },
  ]);
});

test("the fuses refuse node mode and load only the checked asar", () => {
  expect(builderConfig(build()).electronFuses).toMatchObject({
    runAsNode: false,
    enableEmbeddedAsarIntegrityValidation: true,
    onlyLoadAppFromAsar: true,
  });
});

test("the hardened runtime allows only the JIT of V8, not unsigned memory or other libraries", () => {
  const { mac } = builderConfig(build());
  expect({ entitlements: mac?.entitlements, entitlementsInherit: mac?.entitlementsInherit }).toEqual({
    entitlements: "/repo/frontend/assets/entitlements.mac.plist",
    entitlementsInherit: "/repo/frontend/assets/entitlements.mac.plist",
  });
});

test("Windows installs per user with no question, as the installer of Squirrel did", () => {
  expect(builderConfig(build({ platform: "win32" })).nsis).toMatchObject({ oneClick: true, perMachine: false });
});
