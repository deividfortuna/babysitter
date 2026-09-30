import { GitMergeIcon, GitPullRequestClosedIcon, GitPullRequestIcon } from "lucide-react";
import type { PullRequest } from "@/hooks/usePulls";
import type { Watch } from "@/hooks/useWatches";
import { AuthorName, DiffStat, InboxRow, LabelBadges } from "@/components/inbox-row";
import { StopBadge, TagBadges } from "@/components/status-badges";
import { duration, relativeTime } from "@/lib/time";
import { autoTags, watchAuthor, watchAuthorAvatar, watchLabel } from "@/lib/watch-status";

function StopIcon({ reason }: { reason: Watch["stopReason"] }) {
  switch (reason) {
    case "merged":
      return <GitMergeIcon aria-hidden="true" className="text-chart-4" />;
    case "closed":
      return <GitPullRequestClosedIcon aria-hidden="true" className="text-destructive" />;
    default:
      return <GitPullRequestIcon aria-hidden="true" className="text-muted-foreground" />;
  }
}

function watchedText(w: Watch): string {
  const watched = `watched ${duration(w.startedAt, w.stoppedAt)}`;
  if (!w.summary) return watched;
  const messages = w.summary.messages;
  return `${watched} · ${messages} ${messages === 1 ? "message" : "messages"} to the agent`;
}

type Props = {
  watch: Watch;
  pull?: PullRequest;
  onOpen: () => void;
};

export function StoppedRow({ watch: w, pull, onOpen }: Props) {
  const author = watchAuthor(w, pull);
  const labels = pull?.labels ?? [];
  const tags = autoTags(w);
  return (
    <InboxRow
      icon={<StopIcon reason={w.stopReason} />}
      title={w.title || watchLabel(w)}
      status={
        <>
          <StopBadge watch={w} />
          <DiffStat pull={pull} />
        </>
      }
      details={[
        <span key="label">{watchLabel(w)}</span>,
        author ? <AuthorName key="author" login={author} avatarUrl={watchAuthorAvatar(w, pull)} /> : null,
        labels.length > 0 ? <LabelBadges key="labels" labels={labels} /> : null,
        tags.length > 0 ? (
          <span key="tags" className="inline-flex flex-wrap gap-1.5">
            <TagBadges tags={tags} />
          </span>
        ) : null,
        <span key="watched">{watchedText(w)}</span>,
      ]}
      time={w.stoppedAt ? `stopped ${relativeTime(w.stoppedAt)}` : ""}
      onOpen={onOpen}
    />
  );
}
