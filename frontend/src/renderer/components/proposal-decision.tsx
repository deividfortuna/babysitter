import { createContext, useContext, useState, type ReactNode } from "react";
import {
  useApproveProposal,
  useProposal,
  useProposals,
  useRejectProposal,
  useRetryProposal,
  type Approval,
  type Proposal,
  type ProposalDetail,
  type ProposalReply,
} from "@/hooks/useProposals";
import type { Watch } from "@/hooks/useWatches";
import { commitsWord, count, joinAnd, type Outgoing } from "@/components/proposal-dialogs";
import { isSelfWatch } from "@/lib/watch-status";

function waiting(p: Proposal): boolean {
  return p.status === "pending" || p.status === "failed";
}

export function useCurrentProposal(watch: Pick<Watch, "id" | "status" | "provider"> | undefined) {
  const gated = watch?.status === "active" && !isSelfWatch(watch);
  const list = useProposals(gated ? watch.id : null);
  return list.data?.find(waiting);
}

export type Draft = { edits: Record<number, string>; dropped: number[]; pushRejected: boolean };

const blank: Draft = { edits: {}, dropped: [], pushRejected: false };

export type Outcome = { number: number; text: string; icon: "released" | "rejected" };

function rewritten(r: ProposalReply, draft: Draft): boolean {
  const text = draft.edits[r.id];
  return text === undefined ? Boolean(r.edited) : text !== r.body;
}

export function codeRead(detail: ProposalDetail | undefined): detail is ProposalDetail {
  return detail !== undefined && !detail.codeError;
}

function commitCount(detail: ProposalDetail | undefined): number | null {
  return codeRead(detail) ? (detail.commits ?? []).length : null;
}

export function outgoing(
  watch: Pick<Watch, "headRef">,
  p: Proposal,
  detail: ProposalDetail | undefined,
  draft: Draft = blank,
): Outgoing {
  const kept = (p.replies ?? []).filter((r) => !r.dropped && !draft.dropped.includes(r.id));
  return {
    commits: commitCount(detail),
    replies: kept.length,
    edited: kept.filter((r) => rewritten(r, draft)).length,
    headRef: watch.headRef,
    head: p.headSha,
    pushes: p.hasPush && !draft.pushRejected && !p.pushRejected,
  };
}

function changed(r: ProposalReply, draft: Draft): boolean {
  const text = draft.edits[r.id];
  return text !== undefined && text !== (r.edited || r.body);
}

function approval(id: number, number: number, replies: ProposalReply[], draft: Draft, stopAsking: boolean): Approval {
  const edits = replies
    .filter((r) => !draft.dropped.includes(r.id) && changed(r, draft))
    .map((r) => ({ replyId: r.id, body: draft.edits[r.id] }));
  return {
    id,
    number,
    ...(edits.length > 0 ? { edits } : {}),
    ...(draft.dropped.length > 0 ? { drop: draft.dropped } : {}),
    ...(draft.pushRejected ? { rejectPush: true } : {}),
    ...(stopAsking ? { stopAsking: true } : {}),
  };
}

function releasedText(number: number, p: Proposal | undefined, out: Outgoing | undefined, stopAsking: boolean): string {
  let text = `Proposal ${number} released`;
  if (p && out) {
    const parts: string[] = [];
    if (out.pushes && out.commits !== 0) parts.push(`${commitsWord(out.commits)} pushed to ${out.headRef}`);
    if (out.replies > 0) parts.push(`${count(out.replies, "reply", "replies")} posted`);
    text += `: ${joinAnd(parts) || "nothing to send"}`;
    if (!out.pushes && p.hasPush) text += ", nothing pushed";
  }
  text += ".";
  if (stopAsking) text += " The watch runs in auto from here.";
  return text;
}

function rejectedText(number: number, reason: string): string {
  const after = reason
    ? "and the agent gets your reason with everything it was not told yet."
    : "and the agent hears that nothing went out.";
  return `Proposal ${number} rejected. Nothing was pushed or posted, ${after}`;
}

type Request = { isPending: boolean; error: Error | null };

function errorOf(
  request: { error: Error | null; variables?: { number: number } },
  number: number | undefined,
): Error | null {
  return request.variables?.number === number ? request.error : null;
}

export type Preview = {
  detail: ProposalDetail | undefined;
  ready: boolean;
  loading: boolean;
  error: string | null;
  reload: () => void;
};

type ProposalDecision = {
  current: Proposal | undefined;
  detail: ProposalDetail | undefined;
  preview: Preview;
  codeUnread: boolean;
  draft: Draft;
  change: (edit: (draft: Draft) => Draft) => void;
  out: Outgoing | undefined;
  outcome: Outcome | null;
  approve: (number: number, stopAsking: boolean, onDone?: () => void) => void;
  reject: (number: number, reason: string, discard: boolean) => void;
  retry: (number: number) => void;
  approving: Request & { reset: () => void };
  rejecting: Request;
  retrying: Request;
};

const DecisionContext = createContext<ProposalDecision | null>(null);

export function ProposalDecisionProvider({ watch, children }: { watch: Watch; children: ReactNode }) {
  const current = useCurrentProposal(watch);
  const detailQuery = useProposal(current ? watch.id : null, current?.number ?? null, current);
  const detail = detailQuery.data;
  const preview: Preview = {
    detail,
    ready: codeRead(detail),
    loading: detailQuery.isFetching,
    error: detailQuery.error?.message ?? detail?.codeError ?? null,
    reload: () => void detailQuery.refetch(),
  };
  const approveRequest = useApproveProposal();
  const rejectRequest = useRejectProposal();
  const retryRequest = useRetryProposal();
  const [draft, setDraft] = useState<Draft>(blank);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [shown, setShown] = useState(current?.number);

  if (current?.number !== shown) {
    setShown(current?.number);
    setDraft(blank);
    if (current) setOutcome(null);
  }

  const out = current ? outgoing(watch, current, detail, draft) : undefined;
  const read = (number: number) => (current?.number === number ? current : undefined);

  function approve(number: number, stopAsking: boolean, onDone?: () => void) {
    const p = read(number);
    const text = releasedText(number, p, p ? out : undefined, stopAsking);
    approveRequest.mutate(approval(watch.id, number, p?.replies ?? [], p ? draft : blank, stopAsking), {
      onSuccess: (after) => {
        onDone?.();
        if (after.status === "released") setOutcome({ number, icon: "released", text });
      },
    });
  }

  function reject(number: number, reason: string, discard: boolean) {
    rejectRequest.mutate(
      { id: watch.id, number, ...(reason ? { reason } : {}), ...(discard ? { discard } : {}) },
      { onSuccess: () => setOutcome({ number, icon: "rejected", text: rejectedText(number, reason) }) },
    );
  }

  function retry(number: number) {
    const p = read(number);
    const text = releasedText(number, p, p ? outgoing(watch, p, detail) : undefined, false);
    retryRequest.mutate(
      { id: watch.id, number },
      {
        onSuccess: (after) => {
          if (after.status === "released") setOutcome({ number, icon: "released", text });
        },
      },
    );
  }

  const value: ProposalDecision = {
    current,
    detail,
    preview,
    codeUnread: Boolean(out?.pushes) && !preview.ready,
    draft,
    change: setDraft,
    out,
    outcome,
    approve,
    reject,
    retry,
    approving: {
      isPending: approveRequest.isPending,
      error: errorOf(approveRequest, current?.number),
      reset: approveRequest.reset,
    },
    rejecting: { isPending: rejectRequest.isPending, error: errorOf(rejectRequest, current?.number) },
    retrying: { isPending: retryRequest.isPending, error: errorOf(retryRequest, current?.number) },
  };
  return <DecisionContext.Provider value={value}>{children}</DecisionContext.Provider>;
}

export function useProposalDecision(): ProposalDecision {
  const decision = useContext(DecisionContext);
  if (!decision) throw new Error("useProposalDecision needs a ProposalDecisionProvider");
  return decision;
}
