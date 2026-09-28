import { GitPullRequestDraftIcon, GitPullRequestIcon } from "lucide-react";
import type { PullRequest } from "@/hooks/usePulls";
import type { Watch } from "@/hooks/useWatches";
import { AuthorName, DiffStat, InboxRow, LabelBadges } from "@/components/inbox-row";
import { AttentionBadge, ChecksIcon, SessionBadge, TagBadges, ToneBadge } from "@/components/status-badges";
import { Badge } from "@/components/ui/badge";
import { duration, relativeTime } from "@/lib/time";
import { autoTags, isTakenOver, mergeTroubleTags, needsAttention, sessionWord, watchLabel } from "@/lib/watch-status";

function StateBadge({ watch: w }: { watch: Watch }) {
  if (needsAttention(w)) {
    return (
      <AttentionBadge>{w.pendingProposal ? "approval needed" : sessionWord(w.session.state).label}</AttentionBadge>
    );
  }
  if (isTakenOver(w)) {
    return (
      <ToneBadge tone="neutral" title={`With you for ${duration(w.takenOverAt ?? "")}`}>
        with you
      </ToneBadge>
    );
  }
  if (w.lastError) {
    return (
      <Badge variant="destructive" title={w.lastError}>
        error
      </Badge>
    );
  }
  if (w.readySince) return <ToneBadge tone="neutral">ready to merge</ToneBadge>;
  if (w.session.state === "none") return null;
  return <SessionBadge state={w.session.state} />;
}

function PullIcon({ draft }: { draft: boolean }) {
  if (draft) return <GitPullRequestDraftIcon aria-hidden="true" className="text-muted-foreground" />;
  return <GitPullRequestIcon aria-hidden="true" className="text-success" />;
}

type Props = {
  watch: Watch;
  pull?: PullRequest;
  onOpen: () => void;
};

export function WatchRow({ watch: w, pull, onOpen }: Props) {
  const author = w.author || pull?.author;
  const labels = pull?.labels ?? [];
  const tags = [...mergeTroubleTags(w.mergeableState), ...autoTags(w)];
  return (
    <InboxRow
      icon={<PullIcon draft={pull?.draft ?? false} />}
      title={w.title || watchLabel(w)}
      attention={needsAttention(w)}
      status={
        <>
          <StateBadge watch={w} />
          <ChecksIcon watch={w} />
          <DiffStat pull={pull} />
        </>
      }
      details={[
        <span key="number">#{w.number}</span>,
        author ? <AuthorName key="author" login={author} /> : null,
        labels.length > 0 ? <LabelBadges key="labels" labels={labels} /> : null,
        tags.length > 0 ? (
          <span key="tags" className="inline-flex flex-wrap gap-1.5">
            <TagBadges tags={tags} />
          </span>
        ) : null,
      ]}
      time={w.lastPollAt ? `checked ${relativeTime(w.lastPollAt)}` : "not checked yet"}
      onOpen={onOpen}
    />
  );
}
