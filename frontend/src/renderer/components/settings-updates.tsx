import { CircleAlertIcon } from "lucide-react";
import { useAppUpdate, useUpdateSettings } from "@/hooks/useAppUpdate";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel, FieldTitle } from "@/components/ui/field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { relativeTime } from "@/lib/time";
import { isBusy, isUpdateChannel, type UpdateChannel, type UpdateStatus } from "../../shared/updates";

const CHANNELS: { value: UpdateChannel; label: string }[] = [
  { value: "stable", label: "Stable" },
  { value: "nightly", label: "Nightly" },
];

function describe(status: UpdateStatus): string {
  const version = status.version ?? "";
  switch (status.state) {
    case "checking":
      return "Checking for a new version…";
    case "not-available":
      return status.checkedAt
        ? `You have the latest version. Checked ${relativeTime(status.checkedAt)}.`
        : "You have the latest version.";
    case "available":
      return `Babysitter ${version} is out.`;
    case "downloading":
      return `Downloading ${version}: ${Math.floor(status.percent ?? 0)}%.`;
    case "downloaded":
      return `Babysitter ${version} is ready. Restart the app to update.`;
    case "installing":
      return "Restarting…";
    default:
      return "Babysitter checks for a new version every hour.";
  }
}

export function UpdatesPanel() {
  const { status, check } = useAppUpdate();
  const { settings, save } = useUpdateSettings();

  if (!status) return <Spinner />;

  if (status.state === "unsupported") {
    return (
      <FieldGroup>
        <Field>
          <FieldContent>
            <FieldTitle>Babysitter {status.currentVersion}</FieldTitle>
            <FieldDescription>
              This build does not update itself. Install a new version with Homebrew or from the release page.
            </FieldDescription>
          </FieldContent>
        </Field>
      </FieldGroup>
    );
  }

  const checking = status.state === "checking";

  return (
    <div className="flex flex-col gap-6">
      <FieldGroup>
        <Field orientation="horizontal">
          <FieldContent>
            <FieldTitle>Babysitter {status.currentVersion}</FieldTitle>
            <FieldDescription>{describe(status)}</FieldDescription>
          </FieldContent>
          <Button variant="outline" size="sm" disabled={isBusy(status.state)} onClick={() => void check()}>
            {checking ? <Spinner data-icon="inline-start" /> : null}
            {checking ? "Checking…" : "Check for updates"}
          </Button>
        </Field>

        {settings ? (
          <>
            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel htmlFor="updates-auto-download">Download updates automatically</FieldLabel>
                <FieldDescription>
                  When off, the app still checks every hour and asks before it downloads.
                </FieldDescription>
              </FieldContent>
              <Switch
                id="updates-auto-download"
                checked={settings.autoDownload}
                onCheckedChange={(on) => void save({ autoDownload: on })}
              />
            </Field>

            <Field orientation="horizontal">
              <FieldContent>
                <FieldLabel htmlFor="updates-channel">Channel</FieldLabel>
                <FieldDescription>
                  Nightly installs a build of main at most every six hours. A change back to Stable does not install an
                  older version: the app stays on this one until a newer stable version is out.
                </FieldDescription>
              </FieldContent>
              <Select
                value={settings.channel}
                onValueChange={(next) => {
                  if (isUpdateChannel(next)) void save({ channel: next });
                }}
              >
                <SelectTrigger id="updates-channel" className="w-36">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {CHANNELS.map((channel) => (
                    <SelectItem key={channel.value} value={channel.value}>
                      {channel.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </>
        ) : null}
      </FieldGroup>

      {status.message ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{status.message}</AlertTitle>
        </Alert>
      ) : null}
    </div>
  );
}
