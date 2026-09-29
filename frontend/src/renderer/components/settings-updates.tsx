import { useState } from "react";
import { useAppUpdate, useUpdateSettings } from "@/hooks/useAppUpdate";
import { SettingsCard, SettingsError, SettingsRow, SettingsSection, useTrackSave } from "@/components/settings-page";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { relativeTime } from "@/lib/time";
import {
  isBusy,
  isUpdateChannel,
  type UpdateChannel,
  type UpdateSettings,
  type UpdateStatus,
} from "../../shared/updates";

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
  const track = useTrackSave();
  const [saveError, setSaveError] = useState<string | null>(null);

  if (!status) return <Spinner />;

  if (status.state === "unsupported") {
    return (
      <SettingsCard>
        <SettingsRow
          label={`Babysitter ${status.currentVersion}`}
          description="This build does not update itself. Install a new version with Homebrew or from the release page."
        />
      </SettingsCard>
    );
  }

  const checking = status.state === "checking";
  const write = (patch: Partial<UpdateSettings>) => {
    const work = save(patch);
    track(work);
    work.then(
      () => setSaveError(null),
      (error: unknown) => setSaveError(error instanceof Error ? error.message : "Could not save the update settings."),
    );
  };

  return (
    <div className="flex flex-col gap-4.5">
      <SettingsSection label="This version">
        <SettingsCard>
          <SettingsRow label={`Babysitter ${status.currentVersion}`} description={describe(status)}>
            <Button variant="outline" size="sm" disabled={isBusy(status.state)} onClick={() => void check()}>
              {checking ? <Spinner data-icon="inline-start" /> : null}
              {checking ? "Checking…" : "Check for updates"}
            </Button>
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      {settings ? (
        <SettingsSection label="New versions">
          <SettingsCard>
            <SettingsRow
              label="Download updates automatically"
              htmlFor="updates-auto-download"
              description="When off, the app still checks every hour and asks before it downloads."
            >
              <Switch
                id="updates-auto-download"
                checked={settings.autoDownload}
                onCheckedChange={(on) => write({ autoDownload: on })}
              />
            </SettingsRow>
            <SettingsRow
              label="Channel"
              htmlFor="updates-channel"
              description="Nightly installs a build of main at most every six hours. A change back to Stable does not install an older version: the app stays on this one until a newer stable version is out."
            >
              <Select
                value={settings.channel}
                onValueChange={(next) => {
                  if (isUpdateChannel(next)) write({ channel: next });
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
            </SettingsRow>
          </SettingsCard>
        </SettingsSection>
      ) : null}

      <SettingsError message={saveError ?? status.message} />
    </div>
  );
}
