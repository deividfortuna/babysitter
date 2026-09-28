export type Channel = "stable" | "nightly" | "preview";

export type PublishedRelease = {
  tag: string;
  publishedAt: string;
};

export type ReleasePlan = {
  channel: Channel;
  version: string;
  tag: string;
  name: string;
  ref: string;
  latest: boolean;
  feed: string;
  previousTag: string;
};

export const NIGHTLY_GAP_MS = 6 * 60 * 60 * 1000;

const CHANNELS: Channel[] = ["stable", "nightly", "preview"];
const STABLE_VERSION = /^(\d+)\.(\d+)\.(\d+)$/;
const TRAIN_VERSION = /^(\d+\.\d+\.\d+)-(nightly|preview)\.\d{8}\.\d+$/;
const FEEDS: Record<Channel, string> = { stable: "latest-mac.yml", nightly: "nightly-mac.yml", preview: "" };
const LABELS: Record<Channel, string> = {
  stable: "",
  nightly: "Nightly ",
  preview: "Preview (maintainer test build, do not install) ",
};

function isChannel(value: string): value is Channel {
  return CHANNELS.includes(value as Channel);
}

export function channelOf(event: string, dispatched: string | undefined): Channel {
  if (event === "schedule") return "nightly";
  if (event === "push") return "stable";
  if (event !== "workflow_dispatch") throw new Error(`the release does not run on ${event}`);
  const channel = dispatched || "preview";
  if (!isChannel(channel)) throw new Error(`channel ${JSON.stringify(channel)} is not stable, nightly or preview`);
  return channel;
}

export function assertDispatchRef(channel: Channel, ref: string, defaultBranch: string): void {
  if (channel === "preview") return;
  if (ref === `refs/heads/${defaultBranch}`) return;
  throw new Error(
    `a ${channel} release starts from ${defaultBranch}, not ${ref}. Use the preview channel for a branch`,
  );
}

export function channelOfVersion(version: string): Channel | undefined {
  if (STABLE_VERSION.test(version)) return "stable";
  return TRAIN_VERSION.exec(version)?.[2] as Channel | undefined;
}

function channelOfTag(tag: string): Channel | undefined {
  return tag.startsWith("v") ? channelOfVersion(tag.slice(1)) : undefined;
}

export function latestRelease(releases: PublishedRelease[], channel: Channel): PublishedRelease | undefined {
  return releases
    .filter((release) => channelOfTag(release.tag) === channel)
    .sort((a, b) => Date.parse(b.publishedAt) - Date.parse(a.publishedAt))[0];
}

export function nightlyTooSoon(last: PublishedRelease, now: Date): boolean {
  return now.getTime() - Date.parse(last.publishedAt) < NIGHTLY_GAP_MS;
}

function parts(version: string): number[] {
  const match = STABLE_VERSION.exec(version);
  if (!match) throw new Error(`version ${JSON.stringify(version)} is not X.Y.Z`);
  return match.slice(1).map(Number);
}

function newer(a: string, b: string): string {
  const [left, right] = [parts(a), parts(b)];
  const difference = left.map((part, index) => part - right[index]).find((part) => part !== 0) ?? 0;
  return difference >= 0 ? a : b;
}

function nextPatch(version: string): string {
  const [major, minor, patch] = parts(version);
  return `${major}.${minor}.${patch + 1}`;
}

export function baseVersion(packageVersion: string, latestStableTag: string | undefined): string {
  const declared = packageVersion.replace(/[-+].*$/, "");
  const afterStable = latestStableTag ? nextPatch(latestStableTag.slice(1)) : declared;
  return newer(declared, afterStable);
}

function day(date: Date): string {
  return date.toISOString().slice(0, 10).replaceAll("-", "");
}

export function trainVersion(base: string, channel: Channel, date: Date, run: number): string {
  if (channel === "stable") throw new Error("a stable version has no train");
  return `${base}-${channel}.${day(date)}.${run}`;
}

export function stableVersionOf(nightlyTag: string): string {
  const match = TRAIN_VERSION.exec(nightlyTag.slice(1));
  if (!nightlyTag.startsWith("v") || match?.[2] !== "nightly") {
    throw new Error(`${nightlyTag} is not a nightly tag`);
  }
  return match[1];
}

export function assertStableVersion(version: string): void {
  if (!STABLE_VERSION.test(version)) throw new Error(`a stable version is X.Y.Z, not ${JSON.stringify(version)}`);
}

export function planRelease(channel: Channel, version: string, ref: string, previous?: PublishedRelease): ReleasePlan {
  const stable = channel === "stable";
  const suffix = stable ? "" : ` (${ref.slice(0, 12)})`;
  return {
    channel,
    version,
    tag: `v${version}`,
    name: `Babysitter ${LABELS[channel]}${stable ? "v" : ""}${version}${suffix}`,
    ref,
    latest: stable,
    feed: FEEDS[channel],
    previousTag: channel === "preview" ? "" : (previous?.tag ?? ""),
  };
}
