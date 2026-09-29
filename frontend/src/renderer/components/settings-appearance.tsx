import { ThemeToggle } from "@/components/theme-toggle";
import { SettingsCard, SettingsRow, useTrackSave } from "@/components/settings-page";
import { Switch } from "@/components/ui/switch";
import { useAlwaysShowRateLimit } from "@/hooks/use-always-show-rate-limit";

export function AppearancePanel() {
  const { alwaysShow, setAlwaysShow } = useAlwaysShowRateLimit();
  const track = useTrackSave();
  const markSaved = () => track(Promise.resolve());

  return (
    <SettingsCard>
      <SettingsRow label="Theme" description="Light, dark, or whatever the system is set to.">
        <ThemeToggle onChange={markSaved} />
      </SettingsRow>
      <SettingsRow
        label="Always show the GitHub rate limit"
        htmlFor="always-show-rate-limit"
        description="Without it, the sidebar shows the rate limit only when more than half of it is used, or when the daemon slows down or pauses its polls."
      >
        <Switch
          id="always-show-rate-limit"
          checked={alwaysShow}
          onCheckedChange={(on) => {
            setAlwaysShow(on);
            markSaved();
          }}
        />
      </SettingsRow>
    </SettingsCard>
  );
}
