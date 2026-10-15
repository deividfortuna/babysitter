import path from "node:path";
import type { Configuration } from "electron-builder";
import { notarizeApp, type NotaryKey } from "./notarize.ts";
import { channelOfVersion, type Channel } from "./release-channel.ts";

type Env = Record<string, string | undefined>;

export type DesktopPlatform = "darwin" | "win32" | "linux";

export type DesktopBuild = {
  platform: DesktopPlatform;
  version: string;
  frontendDir: string;
  outputDir: string;
  env: Env;
};

export const APP_ID = "com.deividfortuna.babysitter";
const PRODUCT_NAME = "Babysitter";
const EXECUTABLE_NAME = "babysitter";

type MacSignature = Pick<NonNullable<Configuration["mac"]>, "identity" | "hardenedRuntime">;

const NO_SIGNATURE: MacSignature = { identity: null };
const AD_HOC: MacSignature = { identity: "-", hardenedRuntime: false };
const KEYCHAIN: MacSignature = {};

function setting(env: Env, name: string): string | undefined {
  const value = env[name]?.trim();
  return value ? value : undefined;
}

function notaryKey(env: Env): NotaryKey | undefined {
  const key = setting(env, "APPLE_API_KEY");
  const keyId = setting(env, "APPLE_API_KEY_ID");
  const issuer = setting(env, "APPLE_API_ISSUER");
  return key && keyId && issuer ? { key, keyId, issuer } : undefined;
}

function skipsSignature(env: Env): boolean {
  return setting(env, "BABYSITTER_SKIP_SIGN") === "1";
}

function namedIdentity(env: Env): string | undefined {
  return skipsSignature(env) ? undefined : setting(env, "BABYSITTER_SIGN_IDENTITY");
}

const DEVELOPER_ID_PREFIX = "Developer ID Application:";

function certificateName(identity: string): string {
  return identity.startsWith(DEVELOPER_ID_PREFIX) ? identity.slice(DEVELOPER_ID_PREFIX.length).trim() : identity;
}

export function macSignature(env: Env): MacSignature {
  if (skipsSignature(env)) return NO_SIGNATURE;
  const identity = namedIdentity(env);
  if (identity) return { identity: certificateName(identity) };
  if (setting(env, "BABYSITTER_ADHOC_SIGN") === "1") return AD_HOC;
  return KEYCHAIN;
}

export function notaryHook(env: Env): Configuration["afterSign"] {
  const key = notaryKey(env);
  if (!key || !namedIdentity(env)) return undefined;
  return async (context) => {
    if (context.electronPlatformName !== "darwin") return;
    await notarizeApp(path.join(context.appOutDir, `${PRODUCT_NAME}.app`), key);
  };
}

const FEED_CHANNELS: Partial<Record<Channel, "latest" | "nightly">> = { stable: "latest", nightly: "nightly" };

type FeedBuild = Pick<DesktopBuild, "platform" | "version" | "env">;

function isSignedMac(build: FeedBuild): boolean {
  return build.platform === "darwin" && namedIdentity(build.env) !== undefined;
}

function feedChannel(build: FeedBuild): "latest" | "nightly" | undefined {
  if (!isSignedMac(build)) return undefined;
  const channel = channelOfVersion(build.version);
  return channel ? FEED_CHANNELS[channel] : undefined;
}

export function updateFeed(build: FeedBuild): Configuration["publish"] {
  const channel = feedChannel(build);
  if (!channel) return null;
  const nightly = channel === "nightly";
  return {
    provider: "github",
    owner: "deividfortuna",
    repo: "babysitter",
    releaseType: nightly ? "prerelease" : "release",
    ...(nightly ? { channel } : {}),
  };
}

export function builderConfig(build: DesktopBuild): Configuration {
  const asset = (name: string) => path.join(build.frontendDir, "assets", name);
  return {
    appId: APP_ID,
    productName: PRODUCT_NAME,
    directories: { output: build.outputDir, buildResources: path.join(build.frontendDir, "assets") },
    files: ["package.json", "dist-electron/**", "dist/renderer/**"],
    extraResources: [
      { from: path.join(build.frontendDir, "daemon"), to: "daemon" },
      { from: asset("icon.png"), to: "icon.png" },
      { from: asset("trayTemplate.png"), to: "trayTemplate.png" },
      { from: asset("trayTemplate@2x.png"), to: "trayTemplate@2x.png" },
    ],
    asar: true,
    npmRebuild: false,
    nodeGypRebuild: false,
    publish: updateFeed(build),
    afterSign: notaryHook(build.env),
    electronFuses: {
      runAsNode: false,
      enableCookieEncryption: true,
      enableNodeOptionsEnvironmentVariable: false,
      enableNodeCliInspectArguments: false,
      enableEmbeddedAsarIntegrityValidation: true,
      onlyLoadAppFromAsar: true,
    },
    mac: {
      target: ["dmg", "zip"],
      icon: asset("icon.icns"),
      entitlements: asset("entitlements.mac.plist"),
      entitlementsInherit: asset("entitlements.mac.plist"),
      artifactName: "Babysitter-${version}-darwin-${arch}.${ext}",
      notarize: false,
      ...macSignature(build.env),
    },
    dmg: { title: PRODUCT_NAME, icon: asset("icon.icns"), format: "ULFO" },
    win: {
      target: ["nsis"],
      icon: asset("icon.ico"),
      executableName: EXECUTABLE_NAME,
      artifactName: "Babysitter-${version}-win32-${arch}-Setup.${ext}",
      signAndEditExecutable: true,
    },
    nsis: { oneClick: true, perMachine: false, differentialPackage: true },
    linux: {
      target: ["deb", "rpm"],
      icon: asset("icon.png"),
      executableName: EXECUTABLE_NAME,
      category: "Development",
      maintainer: "Deivid Fortuna <deividfortuna@gmail.com>",
    },
  };
}
