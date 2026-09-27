import { useAlwaysShowRateLimit } from "@/hooks/use-always-show-rate-limit";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
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
    </FieldGroup>
  );
}
