import { parsePatchFiles, type AnnotationSide, type DiffLineAnnotation, type FileDiffMetadata } from "@pierre/diffs";
import type { ProposalDetail, ProposalReply } from "@/hooks/useProposals";
import { isGenerated, readingOrder } from "@/lib/reading-order";

export type ProposalFile = NonNullable<ProposalDetail["files"]>[number];

export type DiffFile = {
  path: string;
  diff: FileDiffMetadata;
  added: number;
  deleted: number;
  binary: boolean;
  noisy: boolean;
};

export type ProposalDiff = {
  files: DiffFile[];
  missing: ProposalFile[];
};

export type ReplyAnchor = {
  replyId: number;
  line: number;
  placed: boolean;
};

export type AnchoredReplies = {
  annotations: Map<string, DiffLineAnnotation<ReplyAnchor>[]>;
  unanchored: ProposalReply[];
};

const NOISY_LINES = 500;

export function isNoisy(path: string, changedLines: number): boolean {
  return changedLines > NOISY_LINES || isGenerated(path);
}

export function readPatch(patch: string, files: ProposalFile[]): DiffFile[] {
  const stats = new Map(files.map((f) => [f.path, f]));
  return parsePatchFiles(patch)
    .flatMap((p) => p.files)
    .map((diff): DiffFile => {
      const stat = stats.get(diff.name);
      const added = stat?.added ?? 0;
      const deleted = stat?.deleted ?? 0;
      return {
        path: diff.name,
        diff,
        added,
        deleted,
        binary: stat?.binary ?? false,
        noisy: isNoisy(diff.name, added + deleted),
      };
    });
}

export function readProposalDiff(patch: string, files: ProposalFile[], loaded: DiffFile[] = []): ProposalDiff {
  const read = [...readPatch(patch, files), ...loaded];
  const shown = new Set(read.map((f) => f.path));
  return { files: readingOrder(read), missing: files.filter((f) => !shown.has(f.path)) };
}

const contentKeys = new WeakMap<FileDiffMetadata, string>();

function fnv1a(text: string, hash: number): number {
  let next = hash;
  for (let i = 0; i < text.length; i++) {
    next ^= text.charCodeAt(i);
    next = Math.imul(next, 0x01000193);
  }
  return next >>> 0;
}

export function contentKey(diff: FileDiffMetadata): string {
  const known = contentKeys.get(diff);
  if (known) return known;
  const parts = [
    diff.type,
    diff.prevName ?? "",
    ...diff.hunks.map((h) => h.hunkSpecs ?? ""),
    ...diff.deletionLines,
    "\0",
    ...diff.additionLines,
  ];
  let hash = 0x811c9dc5;
  for (const part of parts) hash = fnv1a(`${part}\n`, hash);
  const key = `${hash.toString(36)}:${parts.length}`;
  contentKeys.set(diff, key);
  return key;
}

export function viewKey(file: DiffFile): string {
  return file.diff.newObjectId ?? contentKey(file.diff);
}

type Place = { side: AnnotationSide; lineNumber: number };

export function placeOnHead(diff: FileDiffMetadata, line: number): Place | null {
  for (const hunk of diff.hunks) {
    let oldLine = hunk.deletionStart;
    let newLine = hunk.additionStart;
    for (const part of hunk.hunkContent) {
      if (part.type === "context") {
        if (line >= oldLine && line < oldLine + part.lines) {
          return { side: "additions", lineNumber: newLine + (line - oldLine) };
        }
        oldLine += part.lines;
        newLine += part.lines;
        continue;
      }
      if (line >= oldLine && line < oldLine + part.deletions) return { side: "deletions", lineNumber: line };
      oldLine += part.deletions;
      newLine += part.additions;
    }
  }
  return null;
}

type InlineComment = { path: string; line: number; side?: string; commit?: string };

function inlineComment(reply: ProposalReply): InlineComment | null {
  const payload = reply.answers?.payload;
  const path = payload?.path;
  const line = payload?.line;
  if (typeof path !== "string" || typeof line !== "number") return null;
  const side = typeof payload?.side === "string" ? payload.side : undefined;
  const commit = typeof payload?.commit_id === "string" ? payload.commit_id : undefined;
  return { path, line, side, commit };
}

function onHead(comment: InlineComment, headSha: string): boolean {
  const onRightSide = comment.side !== "LEFT";
  const onThisHead = !comment.commit || comment.commit === headSha;
  return onRightSide && onThisHead;
}

function fileLevelSide(diff: FileDiffMetadata): AnnotationSide {
  return diff.type === "deleted" ? "deletions" : "additions";
}

export function anchorReplies(replies: ProposalReply[], files: DiffFile[], headSha: string): AnchoredReplies {
  const byPath = new Map(files.map((f) => [f.path, f]));
  const annotations = new Map<string, DiffLineAnnotation<ReplyAnchor>[]>();
  const unanchored: ProposalReply[] = [];
  for (const reply of replies) {
    const comment = inlineComment(reply);
    const file = comment ? byPath.get(comment.path) : undefined;
    if (!comment || !file) {
      unanchored.push(reply);
      continue;
    }
    const place = onHead(comment, headSha) ? placeOnHead(file.diff, comment.line) : null;
    const annotation: DiffLineAnnotation<ReplyAnchor> = place
      ? { ...place, metadata: { replyId: reply.id, line: comment.line, placed: true } }
      : {
          side: fileLevelSide(file.diff),
          lineNumber: 0,
          metadata: { replyId: reply.id, line: comment.line, placed: false },
        };
    annotations.set(file.path, [...(annotations.get(file.path) ?? []), annotation]);
  }
  return { annotations, unanchored };
}
