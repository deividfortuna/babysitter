import { ArchiveIcon, CircleAlertIcon } from "lucide-react";
import { usePullsByLabel } from "@/hooks/usePulls";
import { useWatches, type Watch } from "@/hooks/useWatches";
import { InboxGroup } from "@/components/inbox-row";
import { Meta } from "@/components/status-badges";
import { StoppedRow } from "@/components/stopped-row";
import { ViewHeader } from "@/components/view-header";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import type { Navigate } from "@/lib/navigation";
import { watchLabel } from "@/lib/watch-status";

function stoppedToday(w: Watch): boolean {
  if (!w.stoppedAt) return false;
  return new Date(w.stoppedAt).toDateString() === new Date().toDateString();
}

function byDay(stopped: Watch[]): [string, Watch[]][] {
  const today = stopped.filter(stoppedToday);
  const earlier = stopped.filter((w) => !stoppedToday(w));
  const days: [string, Watch[]][] = [
    ["Today", today],
    ["Earlier", earlier],
  ];
  return days.filter(([, list]) => list.length > 0);
}

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
      <div className="flex flex-col gap-3 p-3">
        {byDay(stopped).map(([day, list]) => (
          <InboxGroup key={day} heading={day}>
            {list.map((w) => (
              <StoppedRow
                key={w.id}
                watch={w}
                pull={pulls.get(watchLabel(w))}
                onOpen={() => onNavigate({ kind: "watch", id: w.id })}
              />
            ))}
          </InboxGroup>
        ))}
      </div>
    </div>
  );
}
