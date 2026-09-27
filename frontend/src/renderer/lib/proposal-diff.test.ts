import { expect, test } from "vite-plus/test";
import { buildActivity } from "@test/fixtures";
import type { ProposalReply } from "@/hooks/useProposals";
import { anchorReplies, isNoisy, placeOnHead, readProposalDiff } from "./proposal-diff";

const HEAD = "9f3c2a1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";

const patch = [
  "diff --git a/app.go b/app.go",
  "index 1111111..2222222 100644",
  "--- a/app.go",
  "+++ b/app.go",
  "@@ -10,5 +10,6 @@ func run() {",
  " one",
  "-two",
  "+deux",
  "+trois",
  " three",
  " four",
  " five",
  "diff --git a/gone.go b/gone.go",
  "deleted file mode 100644",
  "index 3333333..0000000",
  "--- a/gone.go",
  "+++ /dev/null",
  "@@ -1,1 +0,0 @@",
  "-package gone",
  "",
].join("\n");

const files = [
  { path: "app.go", status: "M", added: 2, deleted: 1 },
  { path: "gone.go", status: "D", added: 0, deleted: 1 },
  { path: "later.go", status: "A", added: 3, deleted: 0 },
];

function reply(id: number, payload: Record<string, unknown>): ProposalReply {
  return {
    id,
    body: "ok",
    dropped: false,
    inReplyTo: id,
    answers: buildActivity({ kind: "review_comment", payload }),
  };
}

test("the patch splits into files, with the stats of the API", () => {
  const diff = readProposalDiff(patch, false, files);

  expect(diff.files.map((f) => [f.path, f.added, f.deleted, f.cut])).toEqual([
    ["app.go", 2, 1, false],
    ["gone.go", 0, 1, false],
  ]);
  expect(diff.missing.map((f) => f.path)).toEqual(["later.go"]);
});

test("a cut patch marks its last file", () => {
  const diff = readProposalDiff(patch, true, files);

  expect(diff.files.map((f) => f.cut)).toEqual([false, true]);
});

test("a line of the head lands on the side that shows it", () => {
  const [app] = readProposalDiff(patch, false, files).files;

  expect(placeOnHead(app.diff, 10)).toEqual({ side: "additions", lineNumber: 10 });
  expect(placeOnHead(app.diff, 11)).toEqual({ side: "deletions", lineNumber: 11 });
  expect(placeOnHead(app.diff, 12)).toEqual({ side: "additions", lineNumber: 13 });
  expect(placeOnHead(app.diff, 14)).toEqual({ side: "additions", lineNumber: 15 });
  expect(placeOnHead(app.diff, 9)).toBeNull();
  expect(placeOnHead(app.diff, 15)).toBeNull();
});

test("lockfiles, generated code and big changes are noisy", () => {
  expect(isNoisy("frontend/package-lock.json", 10)).toBe(true);
  expect(isNoisy("backend/go.sum", 2)).toBe(true);
  expect(isNoisy("api/service.pb.go", 2)).toBe(true);
  expect(isNoisy("web/__generated__/types.ts", 2)).toBe(true);
  expect(isNoisy("src/app.go", 501)).toBe(true);
  expect(isNoisy("src/app.go", 500)).toBe(false);
  expect(isNoisy("src/lockfile.go", 2)).toBe(false);
});

test("replies anchor to their line, the top of their file, or stay out", () => {
  const diff = readProposalDiff(patch, false, files);
  const replies = [
    reply(1, { path: "app.go", line: 11, side: "RIGHT", commit_id: HEAD }),
    reply(2, { path: "app.go", line: 12 }),
    reply(3, { path: "app.go", line: 40, side: "RIGHT" }),
    reply(4, { path: "app.go", line: 11, side: "LEFT" }),
    reply(5, { path: "app.go", line: 11, commit_id: "older" }),
    reply(6, { path: "gone.go", line: 7 }),
    reply(7, { path: "later.go", line: 1 }),
    reply(8, { body: "on the conversation" }),
    { id: 9, body: "a comment of the agent", dropped: false },
  ];

  const { annotations, unanchored } = anchorReplies(replies, diff.files, HEAD);

  expect(annotations.get("app.go")).toEqual([
    { side: "deletions", lineNumber: 11, metadata: { replyId: 1, line: 11, placed: true } },
    { side: "additions", lineNumber: 13, metadata: { replyId: 2, line: 12, placed: true } },
    { side: "additions", lineNumber: 0, metadata: { replyId: 3, line: 40, placed: false } },
    { side: "additions", lineNumber: 0, metadata: { replyId: 4, line: 11, placed: false } },
    { side: "additions", lineNumber: 0, metadata: { replyId: 5, line: 11, placed: false } },
  ]);
  expect(annotations.get("gone.go")).toEqual([
    { side: "deletions", lineNumber: 0, metadata: { replyId: 6, line: 7, placed: false } },
  ]);
  expect(unanchored.map((r) => r.id)).toEqual([7, 8, 9]);
});
