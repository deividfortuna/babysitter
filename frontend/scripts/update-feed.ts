import { channelOfVersion, type Channel } from "./release-channel.ts";

export type FeedFile = {
  url: string;
  sha512: string;
  size: number;
};

export type Feed = {
  version: string;
  files: FeedFile[];
  releaseDate: string;
};

const SHA512 = /^[A-Za-z0-9+/]{86}==$/;
const FEED_CHANNELS: (Channel | undefined)[] = ["stable", "nightly"];
const ARCH_ORDER = ["arm64", "x64"];
const EXT_ORDER = ["zip", "dmg"];

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function unquote(value: string): string {
  const quoted = value.length >= 2 && value.startsWith("'") && value.endsWith("'");
  return quoted ? value.slice(1, -1).replaceAll("''", "'") : value;
}

function assertVersion(version: string): void {
  if (version.startsWith("v")) throw new Error(`version ${JSON.stringify(version)} has a leading v`);
  if (!FEED_CHANNELS.includes(channelOfVersion(version))) {
    throw new Error(
      `version ${JSON.stringify(version)} is not a stable version or a nightly, and only those have a feed`,
    );
  }
}

function isPositiveWhole(size: number): boolean {
  return Number.isInteger(size) && size > 0;
}

type Asset = { arch: string; ext: string };

function assetOf(version: string, file: FeedFile): Asset {
  const name = new RegExp(`^Babysitter-${escapeRegExp(version)}-darwin-(arm64|x64)\\.(zip|dmg)$`).exec(file.url);
  if (!name) throw new Error(`${JSON.stringify(file.url)} is not the macOS zip or DMG of ${version}`);
  return { arch: name[1], ext: name[2] };
}

function assertFile(file: FeedFile): void {
  if (!SHA512.test(file.sha512)) throw new Error(`sha512 of ${file.url} is not a base64 sha512`);
  if (!isPositiveWhole(file.size)) throw new Error(`size of ${file.url} is not a positive whole number`);
}

function rank(version: string, file: FeedFile): number {
  const { arch, ext } = assetOf(version, file);
  return ARCH_ORDER.indexOf(arch) * EXT_ORDER.length + EXT_ORDER.indexOf(ext);
}

function sortedFiles(version: string, files: FeedFile[]): FeedFile[] {
  const byUrl = new Map<string, FeedFile>();
  for (const file of files) {
    assertFile(file);
    if (byUrl.has(file.url)) throw new Error(`the feed has ${file.url} twice`);
    byUrl.set(file.url, file);
  }
  const sorted = [...byUrl.values()].sort((a, b) => rank(version, a) - rank(version, b));
  if (!sorted.some((file) => assetOf(version, file).ext === "zip")) throw new Error("the feed has no zip");
  return sorted;
}

type FileField = keyof FeedFile;

function setFileField(file: Partial<FeedFile>, key: string, value: string): void {
  const field = key as FileField;
  if (field === "size") file.size = Number(value);
  if (field === "url" || field === "sha512") file[field] = unquote(value);
}

function completeFile(file: Partial<FeedFile>, source: string): FeedFile {
  const { url = "", sha512 = "", size = 0 } = file;
  if (!url) throw new Error(`a file of ${source} has no url`);
  return { url, sha512, size };
}

export function parseFeed(text: string, source: string): Feed {
  const top = new Map<string, string>();
  const files: Partial<FeedFile>[] = [];
  for (const line of text.split(/\r?\n/)) {
    const entry = /^(\s*)(- )?([A-Za-z0-9]+):\s*(.*)$/.exec(line);
    if (!entry) continue;
    const [, indent, item, key, value] = entry;
    if (item) files.push({});
    const partOfFile = Boolean(indent || item);
    if (partOfFile) setFileField(files.at(-1) ?? {}, key, value.trim());
    else top.set(key, unquote(value.trim()));
  }
  const version = top.get("version");
  const releaseDate = top.get("releaseDate");
  if (!version || !releaseDate) throw new Error(`${source} has no version or no releaseDate`);
  return { version, releaseDate, files: files.map((file) => completeFile(file, source)) };
}

export function mergeFeeds(feeds: Feed[]): Feed {
  const [first, ...others] = feeds;
  if (!first) throw new Error("there is no feed to merge");
  const other = others.find((feed) => feed.version !== first.version);
  if (other) throw new Error(`the feeds are of ${first.version} and ${other.version}, not of one version`);
  return {
    version: first.version,
    files: feeds.flatMap((feed) => feed.files),
    releaseDate:
      feeds
        .map((feed) => feed.releaseDate)
        .sort()
        .at(-1) ?? first.releaseDate,
  };
}

export function renderFeed({ version, files, releaseDate }: Feed): string {
  assertVersion(version);
  const sorted = sortedFiles(version, files);
  const zip = sorted.find((file) => assetOf(version, file).ext === "zip") ?? sorted[0];
  const lines = [`version: ${version}`, "files:"];
  for (const file of sorted) {
    lines.push(`  - url: ${file.url}`, `    sha512: ${file.sha512}`, `    size: ${file.size}`);
  }
  lines.push(`path: ${zip.url}`, `sha512: ${zip.sha512}`, `releaseDate: '${releaseDate}'`, "");
  return lines.join("\n");
}
