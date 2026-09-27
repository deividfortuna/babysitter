import { expect, test } from "vite-plus/test";
import { buildStoppedWatch, buildWatch } from "@test/fixtures";
import type { Watch } from "@/hooks/useWatches";
import {
  autoReasonText,
  checksWord,
  mergeMethodText,
  mergeWord,
  needsAttention,
  plainOutput,
  queuePlace,
  sessionWord,
  stopWord,
  worktreeText,
} from "./watch-status";

test("names what the agent does and whether it waits on the author", () => {
  expect(sessionWord("active")).toEqual({ label: "agent working", tone: "good", needsYou: false });
  expect(sessionWord("waiting_input").needsYou).toBe(true);
  expect(sessionWord("blocked").needsYou).toBe(true);
  expect(sessionWord("exited").needsYou).toBe(true);
  expect(sessionWord("none").label).toBe("no agent");
  expect(needsAttention(buildWatch({ session: { state: "blocked", pid: 1, logPath: "" } }))).toBe(true);
  expect(needsAttention(buildWatch({ status: "stopped", session: { state: "exited", pid: 1, logPath: "" } }))).toBe(
    false,
  );
  expect(needsAttention(buildWatch({ pendingProposal: 3 }))).toBe(true);
  expect(needsAttention(buildWatch({ status: "stopped", pendingProposal: 3 }))).toBe(false);
});

test("drops the escape sequences of a terminal", () => {
  const esc = String.fromCharCode(27);
  const bel = String.fromCharCode(7);
  expect(plainOutput(`${esc}[31mred${esc}[0m ${esc}]0;title${bel}plain\r\n${esc}[?25l`)).toBe("red plain\n");
});

test("says what became of the worktree of a stopped watch", () => {
  const stopped = (summary: Partial<Watch["summary"]>) =>
    buildStoppedWatch({ worktreeDir: "/data/worktrees/octo-babysitter-12", sourceDir: "/home/me/babysitter", summary });
  expect(worktreeText(stopped({ worktreeRemoved: false }))).toBe("left in place at /data/worktrees/octo-babysitter-12");
  expect(worktreeText(stopped({ worktreeRemoved: true }))).toBe("deleted from /data/worktrees/octo-babysitter-12");
  expect(worktreeText(stopped({ worktreeRemoved: true, workBranchLeft: "babysitter/fix" }))).toBe(
    "deleted from /data/worktrees/octo-babysitter-12, its branch babysitter/fix stays in /home/me/babysitter",
  );
});

test("says whether the pull request of a watch is ready to merge", () => {
  expect(mergeWord(buildWatch({ readySince: "2026-01-01T00:10:00Z" }))).toEqual({
    label: "ready to merge",
    tone: "good",
  });
  expect(mergeWord(buildWatch({ readyBlockers: ["no approval yet"] }))).toEqual({
    label: "not ready to merge",
    tone: "wait",
  });
  expect(mergeWord(buildWatch({ readyBlockers: [] }))).toBeNull();
  expect(mergeWord(buildStoppedWatch({ readySince: "2026-01-01T00:10:00Z" }))).toBeNull();
  expect(mergeMethodText("merge")).toBe("merge commit");
  expect(mergeMethodText("")).toBe("the first method the repository allows");
});

test("counts the green checks in the singular and the plural", () => {
  const green = (checkStates: Record<string, string>) =>
    checksWord({ checkStates, greenSha: "abc123", headSha: "abc123" });
  expect(green({ lint: "passed" })).toEqual({ label: "1 check green", tone: "good" });
  expect(green({ lint: "passed", test: "passed" })).toEqual({ label: "2 checks green", tone: "good" });
  expect(checksWord({ checkStates: { lint: "pending" }, greenSha: "", headSha: "abc123" })).toEqual({
    label: "1 pending",
    tone: "wait",
  });
});

test("names why a watch stopped and marks a merged one as done", () => {
  expect(stopWord({ stopReason: "merged" })).toEqual({ label: "stopped · merged", tone: "done" });
  expect(stopWord({ stopReason: "lost_access" })).toEqual({ label: "stopped · lost access", tone: "neutral" });
  expect(stopWord({ stopReason: "" })).toEqual({ label: "stopped", tone: "neutral" });
});

test("counts the failing checks in the singular and the plural, without their names", () => {
  const failing = (checkStates: Record<string, string>) => checksWord({ checkStates, greenSha: "", headSha: "abc123" });
  expect(failing({ lint: "failed", test: "passed" })).toEqual({ label: "1 failing check", tone: "bad" });
  expect(failing({ lint: "failed", test: "failed", build: "pending" })).toEqual({
    label: "2 failing checks",
    tone: "bad",
  });
});

test("names the place of a pull request in the queue", () => {
  expect(queuePlace(1)).toBe("next in queue");
  expect(queuePlace(2)).toBe("2nd in queue");
  expect(queuePlace(3)).toBe("3rd in queue");
  expect(queuePlace(4)).toBe("4th in queue");
  expect(queuePlace(11)).toBe("11th in queue");
  expect(queuePlace(21)).toBe("21st in queue");
});

test("says why auto start began a watch", () => {
  expect(autoReasonText("mine")).toBe("Auto start began it because you opened the pull request.");
  expect(autoReasonText("assigned")).toBe("Auto start began it because the pull request is assigned to you.");
  expect(autoReasonText("dependabot")).toBe("Auto start began it because Dependabot opened the pull request.");
  expect(autoReasonText("")).toBe("");
});
