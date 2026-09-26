import { MonitorIcon, MoonIcon, SunIcon, type LucideIcon } from "lucide-react";
import { useTheme } from "@/hooks/use-theme";
import { Button } from "@/components/ui/button";
import type { ThemePreference } from "@/lib/theme";
import { cn } from "@/lib/utils";

const choices: Array<{ preference: ThemePreference; label: string; Icon: LucideIcon }> = [
  { preference: "system", label: "Match the system", Icon: MonitorIcon },
  { preference: "light", label: "Light theme", Icon: SunIcon },
  { preference: "dark", label: "Dark theme", Icon: MoonIcon },
];

export function ThemeToggle() {
  const { preference, setPreference } = useTheme();

  return (
    <div
      role="group"
      aria-label="Theme"
      className="flex w-fit shrink-0 items-center gap-0.5 rounded-lg border bg-muted/40 p-0.5"
    >
      {choices.map(({ preference: choice, label, Icon }) => (
        <Button
          key={choice}
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          aria-pressed={preference === choice}
          title={label}
          className={cn("text-muted-foreground", preference === choice && "bg-background text-foreground shadow-xs")}
          onClick={() => setPreference(choice)}
        >
          <Icon />
        </Button>
      ))}
    </div>
  );
}
