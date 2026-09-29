import {
  ExternalLinkIcon,
  FileDiffIcon,
  GitPullRequestDraftIcon,
  GitPullRequestIcon,
  MessageSquareIcon,
  type LucideIcon,
} from "lucide-react";
import type { PullRequest } from "@/hooks/usePulls";
import type { Watch } from "@/hooks/useWatches";
import { AuthorName, DiffStat, InboxRow, LabelBadges } from "@/components/inbox-row";
import { AttentionBadge, ChecksIcon, SessionBadge, TagBadges, ToneBadge } from "@/components/status-badges";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { duration, relativeTime } from "@/lib/time";
import {
  autoTags,
  isTakenOver,
  mergeTroubleTags,
  needsAttention,
  sessionWord,
  watchAuthor,
  watchLabel,
} from "@/lib/watch-status";

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

function nextStep(w: Watch): { label: string; icon: LucideIcon } {
  if (w.pendingProposal) return { label: `Review proposal ${w.pendingProposal}`, icon: FileDiffIcon };
  if (w.session.state === "exited") return { label: "Message the agent", icon: MessageSquareIcon };
  return { label: "Answer the agent", icon: MessageSquareIcon };
}

function NextStepActions({ watch: w, onOpen }: { watch: Watch; onOpen: () => void }) {
  const step = nextStep(w);
  return (
    <>
      <Button type="button" size="xs" onClick={onOpen}>
        <step.icon data-icon="inline-start" />
        {step.label}
      </Button>
      <Button asChild variant="ghost" size="xs">
        <a href={w.url} target="_blank" rel="noreferrer">
          <ExternalLinkIcon data-icon="inline-start" />
          Open on GitHub
        </a>
      </Button>
    </>
  );
}

type Props = {
  watch: Watch;
  pull?: PullRequest;
  onOpen: () => void;
};

export function WatchRow({ watch: w, pull, onOpen }: Props) {
  const author = watchAuthor(w, pull);
  const labels = pull?.labels ?? [];
  const tags = [...mergeTroubleTags(w.mergeableState), ...autoTags(w)];
  const attention = needsAttention(w);
  return (
    <InboxRow
      icon={<PullIcon draft={pull?.draft ?? false} />}
      title={w.title || watchLabel(w)}
      attention={attention}
      actions={attention ? <NextStepActions watch={w} onOpen={onOpen} /> : null}
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
