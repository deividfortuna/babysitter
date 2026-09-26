export type FeedFile = {
  name: string;
  sha512: string;
  size: number;
};

export type FeedRelease = {
  version: string;
  files: FeedFile[];
  releaseDate: string;
};

const VERSION = /^\d+\.\d+\.\d+(?:-([0-9A-Za-z-]+)(?:\.[0-9A-Za-z-]+)*)?$/;
const SHA512 = /^[A-Za-z0-9+/]{86}==$/;
const UPDATER_PRERELEASES = ["alpha", "beta"];
const ARCH_ORDER = ["arm64", "x64"];

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function assertVersion(version: string): void {
  if (version.startsWith("v")) throw new Error(`version ${JSON.stringify(version)} has a leading v`);
  const match = VERSION.exec(version);
  if (!match) throw new Error(`version ${JSON.stringify(version)} is not semver`);
  const prerelease = match[1];
  if (!readableByUpdater(prerelease)) {
    throw new Error(
      `prerelease ${JSON.stringify(prerelease)} is not alpha or beta, and the updater reads any other name as a channel of its own`,
    );
  }
}

function readableByUpdater(prerelease: string | undefined): boolean {
  return prerelease === undefined || UPDATER_PRERELEASES.includes(prerelease);
}

function isPositiveWhole(size: number): boolean {
  return Number.isInteger(size) && size > 0;
}

function archOf(version: string, file: FeedFile): string {
  const zip = new RegExp(`^Babysitter-${escapeRegExp(version)}-darwin-(arm64|x64)\\.zip$`).exec(file.name);
  if (!zip)
    throw new Error(`${JSON.stringify(file.name)} is not Babysitter-${version}-darwin-arm64.zip or darwin-x64.zip`);
  return zip[1];
}

function assertFile(file: FeedFile): void {
  if (!SHA512.test(file.sha512)) throw new Error(`sha512 of ${file.name} is not a base64 sha512`);
  if (!isPositiveWhole(file.size)) throw new Error(`size of ${file.name} is not a positive whole number`);
}

function sortedByArch(version: string, files: FeedFile[]): FeedFile[] {
  if (files.length === 0) throw new Error("the feed has no zip");
  const byArch = new Map<string, FeedFile>();
  for (const file of files) {
    const arch = archOf(version, file);
    assertFile(file);
    if (byArch.has(arch)) throw new Error(`the feed has ${arch} twice`);
    byArch.set(arch, file);
  }
  return ARCH_ORDER.flatMap((arch) => byArch.get(arch) ?? []);
}

export function renderFeed({ version, files, releaseDate }: FeedRelease): string {
  assertVersion(version);
  const sorted = sortedByArch(version, files);
  const first = sorted[0];
  const lines = [`version: ${version}`, "files:"];
  for (const file of sorted) {
    lines.push(`  - url: ${file.name}`, `    sha512: ${file.sha512}`, `    size: ${file.size}`);
  }
  lines.push(`path: ${first.name}`, `sha512: ${first.sha512}`, `releaseDate: '${releaseDate}'`, "");
  return lines.join("\n");
}
