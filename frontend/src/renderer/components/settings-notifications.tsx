import { useState } from "react";
import { BellIcon, CircleAlertIcon } from "lucide-react";
import { KIND_ICON } from "@/lib/notification-icons";
import { useSaveSettings, useSettings, type Settings } from "@/hooks/useSettings";
import { Alert, AlertTitle } from "@/components/ui/alert";
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { NOTIFICATION_KINDS, withKindMuted, type NotificationKind } from "../../shared/notifications";

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

function unknownKinds(muted: readonly string[]): NotificationKind[] {
  return muted.filter((kind) => !NOTIFICATION_KINDS.includes(kind as NotificationKind)) as NotificationKind[];
}

function useSeenUnknownKinds(muted: readonly string[]): NotificationKind[] {
  const [seen, setSeen] = useState<NotificationKind[]>([]);
  const fresh = unknownKinds(muted).filter((kind) => !seen.includes(kind));
  if (fresh.length === 0) return seen;
  const all = [...seen, ...fresh];
  setSeen(all);
  return all;
}

function unknownCopy(kind: string): KindCopy {
  return { label: kind, description: "A kind a newer babysitter knows and this app does not." };
}

export function NotificationsPanel() {
  const settings = useSettings();
  const save = useSaveSettings();
  const seenUnknown = useSeenUnknownKinds(settings.data?.mutedNotificationKinds ?? []);

  if (settings.isError) {
    return (
      <Alert variant="destructive">
        <CircleAlertIcon />
        <AlertTitle>{settings.error.message}</AlertTitle>
      </Alert>
    );
  }
  if (!settings.data) return <Spinner />;

  const current = settings.data;
  const muted = current.mutedNotificationKinds ?? [];
  const write = (patch: Partial<Settings>) => save.mutate({ ...current, ...patch });
  const writeKind = (kind: NotificationKind, on: boolean) =>
    write({ mutedNotificationKinds: withKindMuted(muted, kind, !on) });

  return (
    <div className="flex flex-col gap-6">
      <FieldGroup>
        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="notifications-enabled">Show notifications</FieldLabel>
            <FieldDescription>
              A review comment nobody takes, a check that failed, and a pull request ready to merge reach you as a
              notification of the system. The history in this app keeps them either way.
            </FieldDescription>
          </FieldContent>
          <Switch
            id="notifications-enabled"
            checked={current.notificationsEnabled}
            disabled={save.isPending}
            onCheckedChange={(on) => write({ notificationsEnabled: on })}
          />
        </Field>

        <Field orientation="horizontal">
          <FieldContent>
            <FieldLabel htmlFor="notification-sound">Play a sound</FieldLabel>
            <FieldDescription>Off makes every notification silent.</FieldDescription>
          </FieldContent>
          <Switch
            id="notification-sound"
            checked={current.notificationSound}
            disabled={save.isPending || !current.notificationsEnabled}
            onCheckedChange={(on) => write({ notificationSound: on })}
          />
        </Field>
      </FieldGroup>

      <FieldSet>
        <FieldLegend variant="label">What to tell you about</FieldLegend>
        <FieldDescription>A kind you turn off stays in the history and only leaves the screen.</FieldDescription>
        <FieldGroup>
          {[...NOTIFICATION_KINDS, ...seenUnknown].map((kind) => {
            const { label, description } = KIND_COPY[kind] ?? unknownCopy(kind);
            const Icon = KIND_ICON[kind] ?? BellIcon;
            return (
              <Field key={kind} orientation="horizontal">
                <FieldContent>
                  <FieldLabel htmlFor={`notification-kind-${kind}`}>
                    <Icon className="size-4 text-muted-foreground" />
                    {label}
                  </FieldLabel>
                  <FieldDescription>{description}</FieldDescription>
                </FieldContent>
                <Switch
                  id={`notification-kind-${kind}`}
                  checked={!muted.includes(kind)}
                  disabled={save.isPending || !current.notificationsEnabled}
                  onCheckedChange={(on) => writeKind(kind, on)}
                />
              </Field>
            );
          })}
        </FieldGroup>
      </FieldSet>

      {save.error ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{save.error.message}</AlertTitle>
        </Alert>
      ) : null}
    </div>
  );
}
