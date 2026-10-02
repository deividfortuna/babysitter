import { ArchiveIcon, CircleAlertIcon } from "lucide-react";
import { usePullsByLabel } from "@/hooks/usePulls";
import { useWatches } from "@/hooks/useWatches";
import { InboxGroup, PullsErrorAlert } from "@/components/inbox-row";
import { Meta } from "@/components/status-badges";
import { StoppedRow } from "@/components/stopped-row";
import { ViewHeader } from "@/components/view-header";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import type { Navigate } from "@/lib/navigation";
import { groupByDay } from "@/lib/time";
import { watchLabel } from "@/lib/watch-status";

type Props = {
  enabled: boolean;
  onNavigate: Navigate;
};

export function StoppedView({ enabled, onNavigate }: Props) {
  const watches = useWatches(enabled, "all");
  const pulls = usePullsByLabel(enabled, "all");
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
      <PullsErrorAlert error={pulls.error} />
      <div className="flex flex-col gap-3 p-5">
        {groupByDay(stopped, (w) => w.stoppedAt).map(([day, list]) => (
          <InboxGroup key={day} heading={day}>
            {list.map((w) => (
              <StoppedRow
                key={w.id}
                watch={w}
                pull={pulls.byLabel.get(watchLabel(w))}
                onOpen={() => onNavigate({ kind: "watch", id: w.id })}
              />
            ))}
          </InboxGroup>
        ))}
      </div>
    </div>
  );
}
