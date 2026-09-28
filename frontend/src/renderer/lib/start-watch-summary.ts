import type { ApprovalMode } from "@/hooks/useProposals";
import type { MergeMethod } from "@/hooks/useWatches";
import { mergeMethodLabel } from "@/components/merge-method-select";
import { approvalsInvalid, approvalsRequired } from "@/lib/approvals";

type SummaryChoices = {
  agent: string;
  model: string;
  effort?: string;
  approvalMode: ApprovalMode;
  approvals: string;
  mergeMethod: MergeMethod;
};

function approvalsSummary(field: string): string {
  if (approvalsInvalid(field)) return "approvals need a whole number";
  const count = approvalsRequired(field);
  if (count === undefined) return "rule of the base branch";
  if (count === 0) return "no approval";
  return count === 1 ? "1 approval" : `${count} approvals`;
}

export function settingsSummary({
  agent,
  model,
  effort,
  approvalMode,
  approvals,
  mergeMethod,
}: SummaryChoices): string {
  return [
    model ? `${agent} ${model}` : agent,
    effort ? `${effort.toLowerCase()} effort` : "",
    approvalMode,
    approvalsSummary(approvals),
    mergeMethodLabel(mergeMethod).toLowerCase(),
  ]
    .filter(Boolean)
    .join(" · ");
}
