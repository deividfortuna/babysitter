import { BellIcon, CircleAlertIcon, GitPullRequestIcon } from "lucide-react";
import { useNotifications, useReadNotifications } from "@/hooks/useNotifications";
import { KIND_ICON } from "@/lib/notification-icons";
import { Meta } from "@/components/status-badges";
import { ViewHeader } from "@/components/view-header";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import type { Navigate } from "@/lib/navigation";
import { relativeTime } from "@/lib/time";
import { cn } from "@/lib/utils";

type Props = {
  enabled: boolean;
  onNavigate: Navigate;
};

export function NotificationsView({ enabled, onNavigate }: Props) {
  const notifications = useNotifications(enabled);
  const read = useReadNotifications();
  const rows = notifications.data?.notifications ?? [];
  const unread = notifications.data?.unreadCount ?? 0;

  const title = <h1 className="text-lg font-medium tracking-tight">Notifications</h1>;

  if (notifications.error) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <div className="p-5">
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>{notifications.error.message}</AlertTitle>
          </Alert>
        </div>
      </>
    );
  }

  if (notifications.isPending) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <div className="flex flex-col gap-3 p-5">
          <Skeleton className="h-14 w-full" />
        </div>
      </>
    );
  }

  if (rows.length === 0) {
    return (
      <>
        <ViewHeader>{title}</ViewHeader>
        <Empty className="flex-1 py-16">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <BellIcon />
            </EmptyMedia>
            <EmptyTitle>Nothing to tell you</EmptyTitle>
            <EmptyDescription>
              A review comment nobody takes, a check that failed, a pull request ready to merge, and whatever an agent
              asks you for land here.
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
        <Meta>{unread} unread</Meta>
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto"
          disabled={unread === 0 || read.isPending}
          onClick={() => read.mutate(undefined)}
        >
          Mark all as read
        </Button>
      </ViewHeader>
      {read.error ? (
        <Alert variant="destructive" className="m-5 w-auto">
          <CircleAlertIcon />
          <AlertTitle>{read.error.message}</AlertTitle>
        </Alert>
      ) : null}
      {rows.map((item) => {
        const Icon = KIND_ICON[item.kind] ?? BellIcon;
        return (
          <button
            key={item.id}
            type="button"
            onClick={() => {
              read.mutate([item.id]);
              if (item.watchId) onNavigate({ kind: "watch", id: item.watchId });
            }}
            className={cn(
              "flex w-full flex-col gap-1.5 border-b px-5 py-3 text-left transition-colors hover:bg-muted/60",
              !item.readAt && "bg-muted/30",
            )}
          >
            <div className="flex items-baseline gap-2">
              <Badge variant="outline" className="font-mono">
                <Icon />
                {item.kind}
              </Badge>
              <span className="truncate text-title font-medium text-foreground/75">{item.title}</span>
              {item.repo ? (
                <Meta className="ml-auto shrink-0">
                  <GitPullRequestIcon className="inline size-3 align-[-2px]" /> {item.repo}#{item.number}
                </Meta>
              ) : null}
            </div>
            <div className="flex flex-wrap items-baseline gap-2">
              <span className="text-sm text-muted-foreground">{item.body}</span>
              <Meta className="ml-auto shrink-0">{relativeTime(item.createdAt)}</Meta>
            </div>
          </button>
        );
      })}
    </div>
  );
}
