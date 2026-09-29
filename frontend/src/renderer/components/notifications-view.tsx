import { useMemo } from "react";
import { BellIcon, CheckIcon, CircleAlertIcon } from "lucide-react";
import { useNotifications, useReadNotifications, type Notification } from "@/hooks/useNotifications";
import { useMergeWatch, useWatches } from "@/hooks/useWatches";
import { KIND_ICON } from "@/lib/notification-icons";
import { InboxGroup, InboxRow } from "@/components/inbox-row";
import { Meta, ToneBadge } from "@/components/status-badges";
import { ViewHeader } from "@/components/view-header";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import type { Navigate } from "@/lib/navigation";
import { groupByDay, relativeTime } from "@/lib/time";

type Props = {
  enabled: boolean;
  onNavigate: Navigate;
};

export function NotificationsView({ enabled, onNavigate }: Props) {
  const notifications = useNotifications(enabled);
  const read = useReadNotifications();
  const watches = useWatches(enabled);
  const activeWatchIds = useMemo(() => new Set((watches.data ?? []).map((w) => w.id)), [watches.data]);
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
      <div className="flex flex-col gap-3 p-3">
        {groupByDay(rows, (item) => item.createdAt).map(([day, list]) => (
          <InboxGroup key={day} heading={day}>
            {list.map((item) => (
              <NotificationRow
                key={item.id}
                item={item}
                approveMergeId={approveMergeId(item, activeWatchIds)}
                onRead={() => read.mutate([item.id])}
                onNavigate={onNavigate}
              />
            ))}
          </InboxGroup>
        ))}
      </div>
    </div>
  );
}

type RowProps = {
  item: Notification;
  approveMergeId?: number;
  onRead: () => void;
  onNavigate: Navigate;
};

function NotificationRow({ item, approveMergeId, onRead, onNavigate }: RowProps) {
  const Icon = KIND_ICON[item.kind] ?? BellIcon;
  const unread = !item.readAt;
  const waitsOnYou = item.kind === "agent" || approveMergeId !== undefined;
  const attention = unread && waitsOnYou;
  const open = () => {
    onRead();
    if (item.watchId) onNavigate({ kind: "watch", id: item.watchId });
  };
  return (
    <InboxRow
      icon={<Icon aria-hidden="true" className={attention ? "text-attention" : "text-muted-foreground"} />}
      title={item.title}
      attention={attention}
      emphasized={unread}
      status={
        <>
          {unread ? <UnreadDot /> : null}
          <ToneBadge tone="neutral">{item.kind}</ToneBadge>
        </>
      }
      details={[
        item.repo ? (
          <span key="pull">
            {item.repo}#{item.number}
          </span>
        ) : null,
        <span key="body">{item.body}</span>,
      ]}
      time={relativeTime(item.createdAt)}
      onOpen={open}
      actions={
        approveMergeId === undefined ? null : (
          <ApproveMergeActions watchId={approveMergeId} onDone={onRead} onOpen={open} />
        )
      }
    />
  );
}

function UnreadDot() {
  return (
    <span className="inline-flex">
      <span aria-hidden="true" className="size-2 rounded-full bg-attention" />
      <span className="sr-only">unread</span>
    </span>
  );
}

function approveMergeId(item: Notification, activeWatchIds: ReadonlySet<number>): number | undefined {
  const watchId = item.watchId;
  const offered = item.action === "approve_merge" && watchId !== undefined && activeWatchIds.has(watchId);
  return offered ? watchId : undefined;
}

type ActionProps = {
  watchId: number;
  onDone: () => void;
  onOpen: () => void;
};

function ApproveMergeActions({ watchId, onDone, onOpen }: ActionProps) {
  const merge = useMergeWatch();
  return (
    <>
      <Button
        size="xs"
        disabled={merge.isPending}
        onClick={() => merge.mutate({ id: watchId, approve: true }, { onSuccess: onDone })}
      >
        {merge.isPending ? <Spinner data-icon="inline-start" /> : <CheckIcon data-icon="inline-start" />}
        Approve and merge
      </Button>
      <Button variant="ghost" size="xs" onClick={onOpen}>
        Open the watch
      </Button>
      {merge.error ? <span className="text-xs text-destructive">{merge.error.message}</span> : null}
    </>
  );
}
