import { useEffect, useRef } from "react";
import { useNotifications, useReadNotifications, type Notification } from "@/hooks/useNotifications";
import { useSettings, type Settings } from "@/hooks/useSettings";
import { bridge } from "@/lib/bridge";
import { hasKind } from "../../shared/notifications";
import type { Navigate } from "@/lib/navigation";

export function useNativeNotifications(enabled: boolean, onNavigate: Navigate, presentedFrom: number | null) {
  const { data } = useNotifications(enabled);
  const settings = useSettings(enabled);
  const known = settings.data !== undefined;
  const read = useReadNotifications();
  const markRead = useRef(read.mutate);
  const shownUpTo = useRef(0);
  const badge = useRef<number | null>(null);
  const rules = useRef<Settings | undefined>(settings.data);

  useEffect(() => {
    markRead.current = read.mutate;
    rules.current = settings.data;
  });

  useEffect(() => {
    if (!data) return;
    if (badge.current !== data.unreadCount) {
      badge.current = data.unreadCount;
      void bridge.notifications.setBadge(data.unreadCount);
    }
    const show = rules.current;
    if (!show || presentedFrom === null) return;
    const mark = Math.max(shownUpTo.current, presentedFrom);
    const newest = data.notifications[0]?.id ?? 0;
    shownUpTo.current = Math.max(newest, mark);
    if (!enabled || !show.notificationsEnabled) return;
    if (quietInFront(show)) return;
    for (const item of fresh(data.notifications, mark)) {
      if (hasKind(item.kind, show.mutedNotificationKinds)) continue;
      void bridge.notifications.show({
        id: item.id,
        title: item.title,
        body: item.body,
        kind: item.kind,
        watchId: item.watchId,
        url: item.url,
        silent: item.silent === true || hasKind(item.kind, show.silentNotificationKinds),
      });
    }
  }, [data, known, enabled, presentedFrom]);

  useEffect(
    () =>
      bridge.notifications.onClick((click) => {
        if (click.id !== undefined) markRead.current([click.id]);
        if (click.watchId) onNavigate({ kind: "watch", id: click.watchId });
      }),
    [onNavigate],
  );

  useEffect(() => bridge.notifications.onOpen(() => onNavigate({ kind: "notifications" })), [onNavigate]);
}

function quietInFront(settings: Settings): boolean {
  return settings.notificationsBackgroundOnly && document.hasFocus();
}

function fresh(notifications: Notification[], mark: number): Notification[] {
  return notifications.filter((item) => item.id > mark && !item.readAt).reverse();
}
