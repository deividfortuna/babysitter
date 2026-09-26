import { useMemo, type ReactNode } from "react";
import {
  CircleAlertIcon,
  EyeIcon,
  ExternalLinkIcon,
  FolderGitIcon,
  GitPullRequestIcon,
  RefreshCwIcon,
} from "lucide-react";
import { usePulls, type PullRequest } from "@/hooks/usePulls";
import { useRepos, useRequestSync } from "@/hooks/useRepos";
import { useWatches, type Watch } from "@/hooks/useWatches";
import { Meta, ToneBadge } from "@/components/status-badges";
import { ViewHeader } from "@/components/view-header";
import { WatchRow } from "@/components/watch-row";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import type { Navigate } from "@/lib/navigation";
import { relativeTime } from "@/lib/time";
import type { Tone } from "@/lib/watch-status";

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

function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <div className="sticky top-0 z-10 border-b bg-muted px-5 py-2 font-mono text-2xs tracking-label text-muted-foreground">
      {children}
    </div>
  );
}

function PullRow({ pr, onWatch }: { pr: PullRequest; onWatch: () => void }) {
  const ci = ciWord(pr.ciStatus);
  const review = reviewWord(pr.reviewDecision);
  return (
    <div className="flex items-start gap-3 border-b px-5 py-3 transition-colors hover:bg-muted/60">
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex items-baseline gap-2">
          <span className="truncate text-title font-medium text-foreground/75">
            {pr.title || `${pr.repo}#${pr.number}`}
          </span>
          <Meta>#{pr.number}</Meta>
          {pr.draft ? (
            <Badge variant="outline" className="font-mono">
              draft
            </Badge>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Meta>@{pr.author}</Meta>
          <Meta>
            {pr.headRef} → {pr.baseRef}
          </Meta>
          {ci ? <ToneBadge tone={ci.tone}>{ci.label}</ToneBadge> : null}
          {review ? <ToneBadge tone={review.tone}>{review.label}</ToneBadge> : null}
          <Meta>updated {relativeTime(pr.updatedAt)}</Meta>
          <a
            href={pr.htmlUrl}
            target="_blank"
            rel="noreferrer"
            title="Open on GitHub"
            className="text-muted-foreground transition-colors hover:text-foreground"
          >
            <ExternalLinkIcon className="size-3" />
            <span className="sr-only">Open on GitHub</span>
          </a>
        </div>
      </div>
      <Button type="button" variant="outline" size="xs" className="shrink-0 self-center" onClick={onWatch}>
        <EyeIcon data-icon="inline-start" />
        Watch
      </Button>
    </div>
  );
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

  const repo = repos.data?.find((r) => r.fullName === name);
  const watched = useMemo<Watch[]>(
    () => (watches.data ?? []).filter((w) => w.repo === name).sort((a, b) => b.number - a.number),
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
    <div className="flex flex-col">
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
        </div>
      </ViewHeader>

      {repo.lastError ? (
        <Alert variant="destructive" className="rounded-none border-x-0 border-t-0">
          <CircleAlertIcon />
          <AlertTitle>The last sync failed</AlertTitle>
          <AlertDescription>{repo.lastError}</AlertDescription>
        </Alert>
      ) : null}

      <SectionLabel>watching · {watched.length}</SectionLabel>
      {watched.length === 0 ? (
        <p className="border-b px-5 py-4 text-sm text-muted-foreground">
          No pull request of this repository is watched. Pick one from the open ones below.
        </p>
      ) : (
        watched.map((w) => <WatchRow key={w.id} watch={w} onOpen={() => onNavigate({ kind: "watch", id: w.id })} />)
      )}

      <SectionLabel>open, not watched · {unwatched.length}</SectionLabel>
      {unwatched.length === 0 ? (
        <p className="border-b px-5 py-4 text-sm text-muted-foreground">
          {open.length === 0
            ? repo.lastSyncedAt
              ? "No open pull request. The daemon syncs every few minutes."
              : "The daemon has not synced this repository yet."
            : "Every open pull request is watched."}
        </p>
      ) : (
        unwatched.map((pr) => <PullRow key={pr.number} pr={pr} onWatch={() => onWatchPull(pr)} />)
      )}
    </div>
  );
}
