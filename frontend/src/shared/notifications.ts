import type { components } from "../api/schema";

export type NotificationKind = components["schemas"]["HttpdNotification"]["kind"];

export const NOTIFICATION_KINDS: readonly NotificationKind[] = ["agent", "review", "checks", "watch", "merge", "auto"];

export type DesktopNotification = {
  id?: number;
  title: string;
  body: string;
  kind?: string;
  watchId?: number;
  url?: string;
  silent?: boolean;
};

export type NotificationClick = {
  id?: number;
  watchId?: number;
};

export function badgeText(unread: number): string {
  if (unread < 1) return "";
  return unread > 99 ? "99+" : String(unread);
}

export function trayTooltip(unread: number): string {
  if (unread < 1) return "babysitter: nothing unread";
  return `babysitter: ${unread} unread notification${unread === 1 ? "" : "s"}`;
}

const ATTENTION_KINDS: ReadonlySet<string> = new Set<NotificationKind>(["agent", "merge"]);

export function shouldSignalAttention(kind: string | undefined): boolean {
  return kind !== undefined && ATTENTION_KINDS.has(kind);
}

export function bounceType(kind: string | undefined): "critical" | "informational" {
  return kind === "agent" ? "critical" : "informational";
}

export function shouldReplaceBounce(pending: { critical: boolean } | null): boolean {
  return pending === null || !pending.critical;
}

export function hasKind(kind: string | undefined, kinds: readonly string[] | null | undefined): boolean {
  return kind !== undefined && (kinds ?? []).includes(kind);
}

export function withKind(kinds: readonly string[], kind: NotificationKind, listed: boolean): NotificationKind[] {
  const rest = kinds.filter((k) => k !== kind) as NotificationKind[];
  return listed ? [...rest, kind] : rest;
}

export function testNotification(): DesktopNotification {
  return {
    title: "babysitter",
    body: "A notification of the system looks like this.",
    silent: false,
  };
}

export function shouldToast(notification: { title?: string; kind?: string }, supported: boolean): boolean {
  return Boolean(notification.title) && supported;
}

export type Presentation = {
  toast: boolean;
  bounce: "critical" | "informational" | null;
  flash: boolean;
};

export function presentation(
  notification: { title?: string; kind?: string },
  supported: boolean,
  platform: string,
  focused = false,
): Presentation {
  if (!shouldToast(notification, supported)) return { toast: false, bounce: null, flash: false };
  if (focused) return { toast: true, bounce: null, flash: false };
  if (platform === "darwin") return { toast: true, bounce: bounceType(notification.kind), flash: false };
  return { toast: true, bounce: null, flash: shouldSignalAttention(notification.kind) };
}

export type ClickTarget = { kind: "watch"; watchId: number } | { kind: "url"; url: string } | { kind: "app" };

export function clickTarget(notification: DesktopNotification): ClickTarget {
  if (notification.watchId) return { kind: "watch", watchId: notification.watchId };
  if (notification.url?.startsWith("https://")) return { kind: "url", url: notification.url };
  return { kind: "app" };
}

export function clickPlan(notification: DesktopNotification): {
  browser: string | null;
  raise: boolean;
  click: NotificationClick;
} {
  const target = clickTarget(notification);
  return {
    browser: target.kind === "url" ? target.url : null,
    raise: target.kind !== "url",
    click: { id: notification.id, watchId: notification.watchId },
  };
}
