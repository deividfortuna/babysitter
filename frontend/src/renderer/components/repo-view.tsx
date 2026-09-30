import { useMemo, useState } from "react";
import {
  CircleAlertIcon,
  EyeIcon,
  ExternalLinkIcon,
  FolderGitIcon,
  GitPullRequestDraftIcon,
  GitPullRequestIcon,
  PanelRightIcon,
  RefreshCwIcon,
} from "lucide-react";
import { usePulls, type PullRequest } from "@/hooks/usePulls";
import { useRepoQueue, useRepos, useRequestSync, type QueuedPullRequest } from "@/hooks/useRepos";
import { useWatches, type Watch } from "@/hooks/useWatches";
import { RepoSettingsPanel } from "@/components/repo-settings-panel";
import { AuthorName, DiffStat, InboxGroup, InboxItem, LabelBadges } from "@/components/inbox-row";
import { CheckIcon, Meta, QueuedBadge, ToneBadge } from "@/components/status-badges";
import { ViewHeader } from "@/components/view-header";
import { WatchRow } from "@/components/watch-row";
import { Tip } from "@/components/tip";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import type { Navigate } from "@/lib/navigation";
import { relativeTime } from "@/lib/time";
import { needsYouFirst, queuePlace, type Tone } from "@/lib/watch-status";

function ciWord(status: PullRequest["ciStatus"]): { label: string; tone: Tone } | null {
  switch (status) {
    case "success":
      return { label: "ci green", tone: "good" };
    case "failure":
      return { label: "ci failing", tone: "bad" };
    case "pending":
      return { label: "ci pending", tone: "wait" };
    default:
      return null;
  }
}

function reviewWord(decision: PullRequest["reviewDecision"]): { label: string; tone: Tone } | null {
  switch (decision) {
    case "approved":
      return { label: "approved", tone: "good" };
    case "changes_requested":
      return { label: "changes requested", tone: "bad" };
    case "review_required":
      return { label: "review required", tone: "wait" };
    default:
      return null;
  }
}

type PullRowProps = { pr: PullRequest; queued?: QueuedPullRequest; onWatch: () => void };

function PullRow({ pr, queued, onWatch }: PullRowProps) {
  const ci = ciWord(pr.ciStatus);
  const review = reviewWord(pr.reviewDecision);
  const labels = pr.labels ?? [];
  const hasTags = pr.draft || Boolean(queued);
  return (
    <InboxItem
      icon={
        pr.draft ? (
          <GitPullRequestDraftIcon aria-hidden="true" className="text-muted-foreground" />
        ) : (
          <GitPullRequestIcon aria-hidden="true" className="text-success" />
        )
      }
      title={pr.title || `${pr.repo}#${pr.number}`}
      status={
        <>
          {review ? <ToneBadge tone={review.tone}>{review.label}</ToneBadge> : null}
          {ci ? <CheckIcon tone={ci.tone} text={ci.label} /> : null}
          <DiffStat pull={pr} />
        </>
      }
      details={[
        <span key="number">#{pr.number}</span>,
        <AuthorName key="author" login={pr.author} avatarUrl={pr.authorAvatarUrl} />,
        labels.length > 0 ? <LabelBadges key="labels" labels={labels} /> : null,
        hasTags ? (
          <span key="tags" className="inline-flex flex-wrap gap-1.5">
            {pr.draft ? <ToneBadge tone="neutral">draft</ToneBadge> : null}
            {queued ? <QueuedBadge /> : null}
          </span>
        ) : null,
        queued ? (
          <span key="queue">
            {queuePlace(queued.position)} · {queued.updateType}
          </span>
        ) : null,
        <Tip key="github" label="Open on GitHub">
          <a
            href={pr.htmlUrl}
            target="_blank"
            rel="noreferrer"
            className="inline-flex transition-colors hover:text-foreground"
          >
            <ExternalLinkIcon aria-hidden="true" className="size-3.5" />
            <span className="sr-only">Open on GitHub</span>
          </a>
        </Tip>,
      ]}
      time={`updated ${relativeTime(pr.updatedAt)}`}
      actions={
        <Button type="button" variant="outline" size="xs" onClick={onWatch}>
          <EyeIcon data-icon="inline-start" />
          Watch
        </Button>
      }
    />
  );
}

function unwatchedEmptyText(open: number, unwatched: number, everSynced: boolean): string | null {
  if (unwatched > 0) return null;
  if (open > 0) return "Every open pull request is watched.";
  if (everSynced) return "No open pull request. The daemon syncs every few minutes.";
  return "The daemon has not synced this repository yet.";
}

type Props = {
  enabled: boolean;
  name: string;
  onNavigate: Navigate;
  onWatchPR: () => void;
  onWatchPull: (pr: PullRequest) => void;
};

export function RepoView({ enabled, name, onNavigate, onWatchPR, onWatchPull }: Props) {
  const repos = useRepos(enabled);
  const watches = useWatches(enabled);
  const pulls = usePulls(enabled);
  const requestSync = useRequestSync();
  const [settingsOpen, setSettingsOpen] = useState(false);

  const repo = repos.data?.find((r) => r.fullName === name);
  const queue = useRepoQueue(enabled && repo ? repo.id : null);
  const queuedByNumber = useMemo(() => new Map((queue.data ?? []).map((item) => [item.number, item])), [queue.data]);
  const watched = useMemo<Watch[]>(
    () => (watches.data ?? []).filter((w) => w.repo === name).sort(needsYouFirst),
    [watches.data, name],
  );
  const watchedNumbers = useMemo(() => new Set(watched.map((w) => w.number)), [watched]);
  const open = useMemo(
    () =>
      (pulls.data ?? [])
        .filter((pr) => pr.repo === name && pr.state === "open")
        .sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)),
    [pulls.data, name],
  );
  const unwatched = open.filter((pr) => !watchedNumbers.has(pr.number));
  const pullsByNumber = useMemo(() => new Map(open.map((pr) => [pr.number, pr])), [open]);

  const title = <h1 className="truncate text-lg font-medium tracking-tight">{name}</h1>;

  const error = repos.error ?? watches.error ?? pulls.error;
  if (error) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <div className="p-5">
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>{error.message}</AlertTitle>
          </Alert>
        </div>
      </>
    );
  }

  if (repos.isPending || watches.isPending || pulls.isPending) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <div role="status" aria-label="Loading repository details" className="flex flex-col gap-3 p-5">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      </>
    );
  }

  if (!repo) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <Empty className="flex-1 py-16">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <FolderGitIcon />
            </EmptyMedia>
            <EmptyTitle>{name} is not registered</EmptyTitle>
            <EmptyDescription>The repository was removed, or it is not added yet.</EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button variant="outline" onClick={() => onNavigate({ kind: "watching" })}>
              Show every watch
            </Button>
          </EmptyContent>
        </Empty>
      </>
    );
  }

  const synced = repo.lastSyncedAt ? `synced ${relativeTime(repo.lastSyncedAt)}` : "not synced yet";

  return (
    <div className="flex min-h-0 flex-1">
      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto">
        <ViewHeader>
          {title}
          <Meta className="shrink-0">
            {watched.length} watched · {open.length} open · {synced}
          </Meta>
          <div className="ml-auto flex shrink-0 items-center gap-1.5">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              title="Sync now"
              disabled={requestSync.isPending}
              onClick={() => requestSync.mutate()}
            >
              <RefreshCwIcon data-icon="inline-start" />
              Sync
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={onWatchPR}>
              <GitPullRequestIcon data-icon="inline-start" />
              Watch by URL
            </Button>
            {settingsOpen ? null : (
              <Tip label="Repository settings">
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Repository settings"
                  className="size-7"
                  onClick={() => setSettingsOpen(true)}
                >
                  <PanelRightIcon />
                </Button>
              </Tip>
            )}
          </div>
        </ViewHeader>

        {repo.lastError ? (
          <Alert variant="destructive" className="rounded-none border-x-0 border-t-0">
            <CircleAlertIcon />
            <AlertTitle>The last sync failed</AlertTitle>
            <AlertDescription>{repo.lastError}</AlertDescription>
          </Alert>
        ) : null}

        <div className="flex flex-col gap-3 p-3">
          <InboxGroup
            heading={`Watching · ${watched.length}`}
            empty={
              watched.length === 0
                ? "No pull request of this repository is watched. Pick one from the open ones below."
                : null
            }
          >
            {watched.map((w) => (
              <WatchRow
                key={w.id}
                watch={w}
                pull={pullsByNumber.get(w.number)}
                onOpen={() => onNavigate({ kind: "watch", id: w.id })}
              />
            ))}
          </InboxGroup>

          <InboxGroup
            heading={`Open, not watched · ${unwatched.length}`}
            empty={unwatchedEmptyText(open.length, unwatched.length, Boolean(repo.lastSyncedAt))}
          >
            {unwatched.map((pr) => (
              <PullRow key={pr.number} pr={pr} queued={queuedByNumber.get(pr.number)} onWatch={() => onWatchPull(pr)} />
            ))}
          </InboxGroup>
        </div>
      </div>
      {settingsOpen ? <RepoSettingsPanel repo={repo} onClose={() => setSettingsOpen(false)} /> : null}
    </div>
  );
}
