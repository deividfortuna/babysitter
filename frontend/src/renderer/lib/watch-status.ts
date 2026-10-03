import type { Activity } from "@/hooks/useWatchActivity";
import type { PullRequest } from "@/hooks/usePulls";
import type { Watch } from "@/hooks/useWatches";

export type Tone = "neutral" | "good" | "done" | "bad" | "wait";

export type CheckRow = { name: string; state: string };

const stateOrder: Record<string, number> = { failed: 0, pending: 1, waiting: 2, passed: 3, success: 3 };

export function checkRows(w: Pick<Watch, "checkStates">): CheckRow[] {
  return Object.entries(w.checkStates ?? {})
    .map(([name, state]) => ({ name, state }))
    .sort((a, b) => (stateOrder[a.state] ?? 9) - (stateOrder[b.state] ?? 9) || a.name.localeCompare(b.name));
}

export function failedCheckNames(w: Pick<Watch, "checkStates">): string[] {
  return checkRows(w)
    .filter((r) => r.state === "failed")
    .map((r) => r.name);
}

export function checksWord(w: Pick<Watch, "checkStates" | "greenSha" | "headSha">): { label: string; tone: Tone } {
  const rows = checkRows(w);
  const failed = failedCheckNames(w).length;
  const pending = rows.filter((r) => r.state === "pending").length;
  if (failed > 0) return { label: `${failed} failing ${failed === 1 ? "check" : "checks"}`, tone: "bad" };
  if (pending > 0) return { label: `${pending} pending`, tone: "wait" };
  if (rows.length === 0) return { label: "no checks", tone: "neutral" };
  if (w.greenSha && w.greenSha === w.headSha)
    return { label: `${rows.length} ${rows.length === 1 ? "check" : "checks"} green`, tone: "good" };
  return { label: "passed", tone: "good" };
}

export function mergeableWord(state: string): { label: string; tone: Tone } | null {
  if (!state || state === "unknown") return null;
  switch (state) {
    case "clean":
      return { label: "clean", tone: "good" };
    case "dirty":
      return { label: "conflicts", tone: "bad" };
    case "behind":
      return { label: "behind", tone: "wait" };
    case "blocked":
      return { label: "blocked", tone: "wait" };
    case "unstable":
      return { label: "unstable", tone: "wait" };
    default:
      return { label: state, tone: "neutral" };
  }
}

export function mergeWord(
  w: Pick<Watch, "status" | "readySince" | "readyBlockers">,
): { label: string; tone: Tone } | null {
  if (w.status !== "active") return null;
  if (w.readySince) return { label: "ready to merge", tone: "good" };
  if ((w.readyBlockers ?? []).length === 0) return null;
  return { label: "not ready to merge", tone: "wait" };
}

export function mergeMethodText(method: Watch["mergeMethod"] | undefined): string {
  switch (method) {
    case "squash":
      return "squash";
    case "merge":
      return "merge commit";
    case "rebase":
      return "rebase";
    default:
      return "the first method the repository allows";
  }
}

export function autoReasonText(reason: Watch["autoReason"]): string {
  switch (reason) {
    case "mine":
      return "Auto start began it because you opened the pull request.";
    case "assigned":
      return "Auto start began it because the pull request is assigned to you.";
    case "dependabot":
      return "Auto start began it because Dependabot opened the pull request.";
    default:
      return "";
  }
}

export type Tag = { label: string; tone: Tone; title?: string };

export function autoTags(w: Pick<Watch, "autoReason" | "updateType" | "mergeWhenReady" | "dependabot">): Tag[] {
  const tags: Tag[] = [];
  if (w.autoReason) tags.push({ label: "auto", tone: "neutral", title: autoReasonText(w.autoReason) });
  if (w.dependabot && w.updateType)
    tags.push({ label: w.updateType, tone: "neutral", title: `A ${w.updateType} update` });
  if (w.mergeWhenReady) {
    tags.push({
      label: "merge when ready",
      tone: "neutral",
      title: "The daemon merges as soon as the watch is ready to merge",
    });
  }
  return tags;
}

export function mergeTroubleTags(state: string): Tag[] {
  const word = mergeableWord(state);
  if (!word || word.tone === "good") return [];
  return [word];
}

const ordinals = new Intl.PluralRules("en-US", { type: "ordinal" });
const ordinalSuffix: Record<string, string> = { one: "st", two: "nd", few: "rd", other: "th" };

export function queuePlace(position: number): string {
  if (position <= 1) return "next in queue";
  return `${position}${ordinalSuffix[ordinals.select(position)] ?? "th"} in queue`;
}

export function stopReasonText(reason: Watch["stopReason"]): string {
  switch (reason) {
    case "merged":
      return "It is merged, so the watch stopped on its own.";
    case "closed":
      return "The pull request was closed, so the watch stopped on its own.";
    case "lost_access":
      return "The daemon lost access to the pull request, so the watch stopped.";
    case "error":
      return "The watch stopped after repeated errors.";
    case "user":
      return "You stopped this watch.";
    default:
      return "This watch is stopped.";
  }
}

export function stopWord(w: Pick<Watch, "stopReason">): { label: string; tone: Tone } {
  const label = w.stopReason ? `stopped · ${w.stopReason.replaceAll("_", " ")}` : "stopped";
  return { label, tone: w.stopReason === "merged" ? "done" : "neutral" };
}

export function watchLabel(w: Pick<Watch, "repo" | "number">): string {
  return `${w.repo}#${w.number}`;
}

export function watchedLabels(watches: Pick<Watch, "repo" | "number">[]): Set<string> {
  return new Set(watches.map(watchLabel));
}

export function isTakenOver(w: Pick<Watch, "status" | "takenOverAt">): boolean {
  return w.status === "active" && Boolean(w.takenOverAt);
}

const actionableKinds: ReadonlySet<Activity["kind"]> = new Set([
  "comment",
  "review_comment",
  "review",
  "check_failed",
  "behind",
  "conflict",
]);

export function heldForHandback(w: Pick<Watch, "status" | "takenOverAt">, a: Pick<Activity, "kind" | "nudgedAt">) {
  return isTakenOver(w) && actionableKinds.has(a.kind) && !a.nudgedAt;
}

export function isSelfWatch(w: Pick<Watch, "provider">): boolean {
  return w.provider === "self";
}

export function worktreeText(w: Pick<Watch, "worktreeDir" | "sourceDir" | "summary" | "provider">): string {
  if (isSelfWatch(w)) return `not there: the agent worked in ${w.sourceDir || "your checkout"}`;
  const dir = w.worktreeDir || "its directory";
  if (!w.summary?.worktreeRemoved) return `left in place at ${dir}`;
  const branch = w.summary.workBranchLeft;
  if (!branch) return `deleted from ${dir}`;
  const checkout = w.sourceDir || "your checkout";
  return `deleted from ${dir}, its branch ${branch} stays in ${checkout}`;
}

export type SessionState = Watch["session"]["state"];

export function sessionWord(state: SessionState): { label: string; tone: Tone; needsYou: boolean } {
  switch (state) {
    case "starting":
      return { label: "agent starting", tone: "wait", needsYou: false };
    case "idle":
      return { label: "agent idle", tone: "neutral", needsYou: false };
    case "active":
      return { label: "agent working", tone: "good", needsYou: false };
    case "waiting":
      return { label: "agent waits on background work", tone: "wait", needsYou: false };
    case "waiting_input":
      return { label: "agent asks you", tone: "bad", needsYou: true };
    case "blocked":
      return { label: "agent waits on a permission", tone: "bad", needsYou: true };
    case "exited":
      return { label: "agent exited", tone: "bad", needsYou: true };
    default:
      return { label: "no agent", tone: "neutral", needsYou: false };
  }
}

export function isAgentLive(state: SessionState): boolean {
  return state === "active";
}

export function needsAttention(w: Pick<Watch, "status" | "session" | "pendingProposal">): boolean {
  return w.status === "active" && (sessionWord(w.session.state).needsYou || Boolean(w.pendingProposal));
}

export function needsYouFirst(
  a: Pick<Watch, "status" | "session" | "pendingProposal" | "number">,
  b: Pick<Watch, "status" | "session" | "pendingProposal" | "number">,
): number {
  return Number(needsAttention(b)) - Number(needsAttention(a)) || b.number - a.number;
}

export function watchAuthor(w: Pick<Watch, "author">, pull?: Pick<PullRequest, "author">): string {
  return w.author || pull?.author || "";
}

export function watchAuthorAvatar(
  w: Pick<Watch, "authorAvatarUrl">,
  pull?: Pick<PullRequest, "authorAvatarUrl">,
): string | undefined {
  return w.authorAvatarUrl || pull?.authorAvatarUrl;
}

const ESC = String.fromCharCode(27);
const BEL = String.fromCharCode(7);
const osc = new RegExp(ESC + "\\][^" + BEL + "]*(" + BEL + "|" + ESC + "\\\\)", "g");
const csi = new RegExp(ESC + "\\[[0-9;?]*[ -/]*[@-~]", "g");
const simple = new RegExp(ESC + "[@-Z\\\\-_]", "g");
const control = new RegExp(
  "[" +
    String.fromCharCode(0) +
    "-" +
    String.fromCharCode(8) +
    String.fromCharCode(11) +
    "-" +
    String.fromCharCode(31) +
    "]",
  "g",
);

export function plainOutput(output: string): string {
  return output.replace(osc, "").replace(csi, "").replace(simple, "").replace(/\r/g, "").replace(control, "");
}

export function activityUrl(a: Pick<Activity, "kind" | "url" | "payload">): string {
  if (a.kind !== "commit") return a.url;
  const sha = a.payload?.sha;
  const [repo, pull] = a.url.split("/pull/");
  if (typeof sha !== "string" || !sha || !pull) return a.url;
  return `${repo}/commit/${sha}`;
}
