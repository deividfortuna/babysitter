import { ArchiveIcon, CircleAlertIcon } from "lucide-react";
import { useWatches } from "@/hooks/useWatches";
import { Meta, StopBadge } from "@/components/status-badges";
import { ViewHeader } from "@/components/view-header";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import type { Navigate } from "@/lib/navigation";
import { duration, relativeTime } from "@/lib/time";

type Props = {
  enabled: boolean;
  onNavigate: Navigate;
};

export function StoppedView({ enabled, onNavigate }: Props) {
  const watches = useWatches(enabled, "all");
  const stopped = (watches.data ?? [])
    .filter((w) => w.status === "stopped")
    .sort((a, b) => (b.stoppedAt ?? "").localeCompare(a.stoppedAt ?? ""));

  const title = <h1 className="text-lg font-medium tracking-tight">Stopped</h1>;

  if (watches.error) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <div className="p-5">
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>{watches.error.message}</AlertTitle>
          </Alert>
        </div>
      </>
    );
  }

  if (watches.isPending) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <div className="flex flex-col gap-3 p-5">
          <Skeleton className="h-14 w-full" />
        </div>
      </>
    );
  }

  if (stopped.length === 0) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <Empty className="flex-1 py-16">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <ArchiveIcon />
            </EmptyMedia>
            <EmptyTitle>Nothing stopped yet</EmptyTitle>
            <EmptyDescription>
              A watch lands here when its pull request merges or closes, or when you stop it.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </>
    );
  }

  return (
    <div className="flex flex-col">
      <ViewHeader>
        {title}
        <Meta>{stopped.length} archived</Meta>
      </ViewHeader>
      {stopped.map((w) => {
        const sum = w.summary;
        return (
          <button
            key={w.id}
            type="button"
            onClick={() => onNavigate({ kind: "watch", id: w.id })}
            className="flex w-full flex-col gap-1.5 border-b px-5 py-3 text-left transition-colors hover:bg-muted/60"
          >
            <div className="flex items-baseline gap-2">
              <StopBadge watch={w} />
              <span className="truncate text-title font-medium text-foreground/75">
                {w.title || `${w.repo}#${w.number}`}
              </span>
              <Meta className="ml-auto shrink-0">
                {w.repo}#{w.number}
              </Meta>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Meta>watched {duration(w.startedAt, w.stoppedAt)}</Meta>
              {w.stoppedAt ? <Meta>· stopped {relativeTime(w.stoppedAt)}</Meta> : null}
              {sum ? (
                <Meta>
                  · {sum.messages} {sum.messages === 1 ? "message" : "messages"} to the agent
                </Meta>
              ) : null}
            </div>
          </button>
        );
      })}
    </div>
  );
}
