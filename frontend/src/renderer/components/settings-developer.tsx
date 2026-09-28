import { useAlwaysShowRateLimit } from "@/hooks/use-always-show-rate-limit";
import { useLogLevel, useSetLogLevel } from "@/hooks/useLogs";
import { LogsViewer } from "@/components/logs-viewer";
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldSeparator,
  FieldSet,
  FieldLegend,
} from "@/components/ui/field";
import { Switch } from "@/components/ui/switch";

export function DeveloperPanel() {
  const { alwaysShow, setAlwaysShow } = useAlwaysShowRateLimit();

  return (
    <FieldGroup>
      <Field orientation="horizontal">
        <FieldContent>
          <FieldLabel htmlFor="always-show-rate-limit">Always show the GitHub rate limit</FieldLabel>
          <FieldDescription>
            The sidebar shows the rate limit only when more than half of it is used, or when the daemon slows down or
            pauses its polls. On shows it all the time.
          </FieldDescription>
        </FieldContent>
        <Switch id="always-show-rate-limit" checked={alwaysShow} onCheckedChange={setAlwaysShow} />
      </Field>
      <DebugLogsField />
      <FieldSeparator />
      <FieldSet>
        <FieldLegend>Logs</FieldLegend>
        <FieldDescription>
          What the daemon and the app recorded, newest at the end. Both write to the logs folder of the data directory.
          In a terminal, <code>babysitter daemon logs</code> prints the same records.
        </FieldDescription>
        <LogsViewer />
      </FieldSet>
    </FieldGroup>
  );
}

function DebugLogsField() {
  const level = useLogLevel();
  const setLevel = useSetLogLevel();
  const unavailable = !level.data || setLevel.isPending;

  return (
    <Field orientation="horizontal" data-invalid={setLevel.isError || undefined}>
      <FieldContent>
        <FieldLabel htmlFor="debug-logs">Debug logs</FieldLabel>
        <FieldDescription>
          The daemon also records what only a developer needs, such as each HTTP request. It goes back to info when the
          daemon stops.
        </FieldDescription>
        {setLevel.error ? <FieldError>{setLevel.error.message}</FieldError> : null}
      </FieldContent>
      <Switch
        id="debug-logs"
        checked={level.data === "debug"}
        disabled={unavailable}
        onCheckedChange={(on) => setLevel.mutate(on ? "debug" : "info")}
      />
    </Field>
  );
}
