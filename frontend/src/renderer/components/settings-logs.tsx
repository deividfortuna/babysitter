import { LogsViewer } from "@/components/logs-viewer";
import { SettingsCard, SettingsRow, useTrackSave } from "@/components/settings-page";
import { Switch } from "@/components/ui/switch";
import { useLogLevel, useSetLogLevel } from "@/hooks/useLogs";

export function LogsPanel() {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-4">
      <SettingsCard>
        <DebugLogsRow />
      </SettingsCard>
      <p className="text-body/snug text-muted-foreground">
        Both write to the logs folder of the data directory. In a terminal, <code>babysitter daemon logs</code> prints
        the same records.
      </p>
      <LogsViewer className="min-h-0 flex-1" />
    </div>
  );
}

function DebugLogsRow() {
  const level = useLogLevel();
  const setLevel = useSetLogLevel();
  const track = useTrackSave();
  const unavailable = !level.data || setLevel.isPending;

  return (
    <SettingsRow
      label="Debug logs"
      htmlFor="debug-logs"
      description="The daemon also records what only a developer needs, such as each HTTP request. It goes back to info when the daemon stops."
    >
      <Switch
        id="debug-logs"
        checked={level.data === "debug"}
        disabled={unavailable}
        onCheckedChange={(on) => track(setLevel.mutateAsync(on ? "debug" : "info"))}
      />
    </SettingsRow>
  );
}
