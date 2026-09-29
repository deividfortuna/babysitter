import { useState } from "react";
import { BellIcon } from "lucide-react";
import { DaemonSettings, SettingsCard, SettingsError, SettingsRow, useTrackedWrite } from "@/components/settings-page";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { useNotificationsPresent } from "@/hooks/useNotificationsPresent";
import type { Settings } from "@/hooks/useSettings";
import { bridge } from "@/lib/bridge";
import { KIND_ICON } from "@/lib/notification-icons";
import { NOTIFICATION_KINDS, testNotification, withKind, type NotificationKind } from "../../shared/notifications";

type KindCopy = {
  label: string;
  description: string;
};

const KIND_COPY: Record<NotificationKind, KindCopy> = {
  agent: {
    label: "Agent requests",
    description: "The agent asks you for something it cannot do on its own, or its work waits on your approval.",
  },
  review: {
    label: "Review comments",
    description: "A review comment or a review that nobody has taken yet.",
  },
  checks: {
    label: "Checks",
    description: "A check of the pull request that failed, and the one that goes green again.",
  },
  watch: {
    label: "Watches",
    description: "A watch and its agent session that start and stop, and a push of the daemon that failed.",
  },
  merge: {
    label: "Merges",
    description: "A pull request that can merge, and one that failed to.",
  },
  auto: {
    label: "Auto start",
    description: "A watch started on its own, or a Dependabot update waits on your approval.",
  },
};

function unknownKinds(kinds: readonly string[]): NotificationKind[] {
  return kinds.filter((kind) => !NOTIFICATION_KINDS.includes(kind as NotificationKind)) as NotificationKind[];
}

function useSeenUnknownKinds(kinds: readonly string[]): NotificationKind[] {
  const [seen, setSeen] = useState<NotificationKind[]>([]);
  const fresh = unknownKinds(kinds).filter((kind) => !seen.includes(kind));
  if (fresh.length === 0) return seen;
  const all = [...seen, ...fresh];
  setSeen(all);
  return all;
}

function unknownCopy(kind: string): KindCopy {
  return { label: kind, description: "A kind a newer babysitter knows and this app does not." };
}

export function NotificationsPanel() {
  return <DaemonSettings>{(settings) => <NotificationsForm settings={settings} />}</DaemonSettings>;
}

function NotificationsForm({ settings }: { settings: Settings }) {
  const { save, error } = useTrackedWrite();
  const supported = useNotificationsPresent() === true;
  const muted = settings.mutedNotificationKinds ?? [];
  const silent = settings.silentNotificationKinds ?? [];
  const seenUnknown = useSeenUnknownKinds([...muted, ...silent]);
  const enabled = settings.notificationsEnabled;
  const testable = supported && enabled;

  return (
    <div className="flex flex-col gap-4">
      <SettingsCard className="bg-transparent">
        <SettingsRow
          label="Show notifications of the system"
          htmlFor="notifications-enabled"
          description="The history in this app keeps every kind either way."
        >
          <Button
            variant="outline"
            size="sm"
            disabled={!testable}
            title={supported ? undefined : "This window cannot show notifications of the system."}
            onClick={() => void bridge.notifications.show(testNotification())}
          >
            <BellIcon />
            Send a test
          </Button>
          <Switch
            id="notifications-enabled"
            checked={enabled}
            onCheckedChange={(on) => save({ notificationsEnabled: on })}
          />
        </SettingsRow>
      </SettingsCard>

      <div role="table" aria-label="Notification kinds" className="flex flex-col">
        <div role="row" className="grid grid-cols-[1fr_4rem_4rem] items-center px-3.5 pb-1.5 eyebrow text-3xs/normal">
          <span role="columnheader">Kind</span>
          <span role="columnheader" className="text-center">
            Notify
          </span>
          <span role="columnheader" className="text-center">
            Sound
          </span>
        </div>
        <SettingsCard>
          {[...NOTIFICATION_KINDS, ...seenUnknown].map((kind) => {
            const { label, description } = KIND_COPY[kind] ?? unknownCopy(kind);
            const Icon = KIND_ICON[kind] ?? BellIcon;
            const notifies = enabled && !muted.includes(kind);
            return (
              <div
                key={kind}
                role="row"
                data-disabled={!enabled || undefined}
                className="group/kind grid grid-cols-[1fr_4rem_4rem] items-center px-3.5 py-2.25"
              >
                <div role="rowheader" className="flex min-w-0 items-start gap-2.5 group-data-disabled/kind:opacity-60">
                  <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                  <div className="flex min-w-0 flex-col">
                    <span className="text-sm font-medium">{label}</span>
                    <span title={description} className="truncate text-xs/snug text-muted-foreground">
                      {description}
                    </span>
                  </div>
                </div>
                <span role="cell" className="flex justify-center">
                  <Switch
                    aria-label={`Notify: ${label}`}
                    checked={notifies}
                    disabled={!enabled}
                    onCheckedChange={(on) => save({ mutedNotificationKinds: withKind(muted, kind, !on) })}
                  />
                </span>
                <span role="cell" className="flex justify-center">
                  <Switch
                    aria-label={`Sound: ${label}`}
                    checked={notifies && !silent.includes(kind)}
                    disabled={!notifies}
                    onCheckedChange={(on) => save({ silentNotificationKinds: withKind(silent, kind, !on) })}
                  />
                </span>
              </div>
            );
          })}
        </SettingsCard>
      </div>

      <SettingsCard>
        <SettingsRow
          label="Only while the app is in the background"
          htmlFor="notifications-background-only"
          description="The app stays quiet while you look at it. The daemon on its own shows every notification."
          disabled={!enabled}
        >
          <Switch
            id="notifications-background-only"
            checked={settings.notificationsBackgroundOnly}
            disabled={!enabled}
            onCheckedChange={(on) => save({ notificationsBackgroundOnly: on })}
          />
        </SettingsRow>
      </SettingsCard>

      <SettingsError message={error} />
    </div>
  );
}
