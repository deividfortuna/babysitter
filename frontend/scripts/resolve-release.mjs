import { execFileSync } from "node:child_process";
import { appendFileSync, readFileSync } from "node:fs";
import {
  assertDispatchRef,
  assertStableVersion,
  baseVersion,
  channelOf,
  latestRelease,
  nightlyTooSoon,
  planRelease,
  stableVersionOf,
  trainVersion,
} from "./release-channel.ts";

const { GITHUB_EVENT_NAME, GITHUB_REF, GITHUB_REF_NAME, GITHUB_SHA, GITHUB_RUN_NUMBER, GITHUB_REPOSITORY } =
  process.env;
const { DISPATCH_CHANNEL, DISPATCH_VERSION, GITHUB_OUTPUT } = process.env;

function api(path, ...flags) {
  return JSON.parse(execFileSync("gh", ["api", ...flags, `repos/${GITHUB_REPOSITORY}${path}`], { encoding: "utf8" }));
}

function publishedReleases() {
  return api("/releases?per_page=100", "--paginate", "--slurp")
    .flat()
    .filter((release) => !release.draft && release.published_at)
    .map((release) => ({ tag: release.tag_name, publishedAt: release.published_at }));
}

function comparison(base, head) {
  return api(`/compare/${base}...${head}?per_page=1`).status;
}

function assertOnBranch(sha, branch) {
  const status = comparison(sha, branch);
  if (status === "ahead" || status === "identical") return;
  throw new Error(`commit ${sha} is not in ${branch} (${status})`);
}

function packageVersion() {
  return JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8")).version;
}

function write(outputs) {
  const lines = Object.entries(outputs).map(([key, value]) => `${key}=${value}\n`);
  appendFileSync(GITHUB_OUTPUT, lines.join(""));
  process.stdout.write(lines.join(""));
}

function skipNightly(reason) {
  console.log(reason);
  write({ release: false });
}

function trainRelease(channel, releases) {
  const base = baseVersion(packageVersion(), latestRelease(releases, "stable")?.tag);
  const version = trainVersion(base, channel, new Date(), Number(GITHUB_RUN_NUMBER));
  return planRelease(channel, version, GITHUB_SHA, latestRelease(releases, channel));
}

function stableSource(releases, defaultBranch) {
  if (GITHUB_EVENT_NAME !== "workflow_dispatch") return { version: GITHUB_REF_NAME.replace(/^v/, ""), ref: GITHUB_SHA };
  const nightly = latestRelease(releases, "nightly");
  if (!nightly) throw new Error("no nightly is published, and a stable release ships the commit of the latest one");
  const ref = api(`/commits/${nightly.tag}`).sha;
  assertOnBranch(ref, defaultBranch);
  console.log(`the stable release ships ${ref}, the commit of ${nightly.tag}`);
  return { version: DISPATCH_VERSION?.trim().replace(/^v/, "") || stableVersionOf(nightly.tag), ref };
}

function stableRelease(releases, defaultBranch) {
  const { version, ref } = stableSource(releases, defaultBranch);
  const latestStable = latestRelease(releases, "stable");
  assertStableVersion(version, latestStable?.tag);
  return planRelease("stable", version, ref, latestStable);
}

function nightlyWaitReason(releases) {
  const last = latestRelease(releases, "nightly");
  if (!last) return undefined;
  if (nightlyTooSoon(last, new Date())) return `${last.tag} is less than six hours old`;
  const status = comparison(last.tag, GITHUB_SHA);
  return status === "ahead" ? undefined : `${GITHUB_SHA} has no new commit after ${last.tag} (${status})`;
}

const channel = channelOf(GITHUB_EVENT_NAME, DISPATCH_CHANNEL);
const defaultBranch = api("").default_branch;
if (GITHUB_EVENT_NAME === "workflow_dispatch") assertDispatchRef(channel, GITHUB_REF, defaultBranch);
if (channel !== "preview") assertOnBranch(GITHUB_SHA, defaultBranch);

const releases = publishedReleases();
const waitReason = GITHUB_EVENT_NAME === "schedule" ? nightlyWaitReason(releases) : undefined;

if (waitReason) {
  skipNightly(waitReason);
} else {
  const plan = channel === "stable" ? stableRelease(releases, defaultBranch) : trainRelease(channel, releases);
  write({
    release: true,
    channel: plan.channel,
    version: plan.version,
    tag: plan.tag,
    name: plan.name,
    ref: plan.ref,
    latest: plan.latest,
    feed: plan.feed,
    previous_tag: plan.previousTag,
  });
}
