import type { Watch } from "@/hooks/useWatches";
import {
  AttentionBadge,
  AutoBadges,
  ChecksBadge,
  MergeableBadge,
  MergeBadge,
  Meta,
  SessionBadge,
  ToneBadge,
} from "@/components/status-badges";
import { Badge } from "@/components/ui/badge";
import { duration, relativeTime, shortSha } from "@/lib/time";
import { checksWord, isTakenOver, needsAttention, sessionWord } from "@/lib/watch-status";
import { cn } from "@/lib/utils";

function quietText(w: Watch): string {
  if (isTakenOver(w)) return `with you · ${duration(w.takenOverAt ?? "")}`;
  const checks = checksWord(w);
  if (checks.tone === "bad") return `CI ${checks.label}`;
  return `quiet · ${checks.label}`;
}

type Props = {
  watch: Watch;
  onOpen: () => void;
};

export function WatchRow({ watch: w, onOpen }: Props) {
  const attention = needsAttention(w);
  const session = sessionWord(w.session.state);
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        "flex w-full flex-col gap-1.5 border-b px-5 py-3 text-left transition-colors hover:bg-muted/60",
        attention && "bg-attention/5",
      )}
    >
      <div className="flex items-baseline gap-2">
        <span className={cn("truncate text-title font-medium", !attention && "text-foreground/75")}>
          {w.title || `${w.repo}#${w.number}`}
        </span>
        <Meta>#{w.number}</Meta>
        {attention ? (
          <AttentionBadge className="ml-auto shrink-0">
            {w.pendingProposal ? "approval needed" : session.label}
          </AttentionBadge>
        ) : (
          <Meta className="ml-auto shrink-0 text-muted-foreground/80">{quietText(w)}</Meta>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Meta>{w.headRef}</Meta>
        <Meta>{shortSha(w.headSha)}</Meta>
        <ChecksBadge watch={w} />
        <MergeableBadge state={w.mergeableState} />
        <MergeBadge watch={w} />
        {isTakenOver(w) ? <ToneBadge tone="neutral">with you</ToneBadge> : null}
        {w.session.state !== "none" && !attention && !isTakenOver(w) ? <SessionBadge state={w.session.state} /> : null}
        <AutoBadges watch={w} />
        <Meta>{w.lastPollAt ? `checked ${relativeTime(w.lastPollAt)}` : "not checked yet"}</Meta>
        {w.lastError ? (
          <Badge variant="destructive" title={w.lastError}>
            error
          </Badge>
        ) : null}
      </div>
    </button>
  );
}
