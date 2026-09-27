import { useState, type ComponentType } from "react";
import { BellIcon, CodeIcon, DownloadIcon, EyeIcon, SettingsIcon, type LucideIcon } from "lucide-react";
import { ThemeToggle } from "@/components/theme-toggle";
import { DeveloperPanel } from "@/components/settings-developer";
import { NotificationsPanel } from "@/components/settings-notifications";
import { UpdatesPanel } from "@/components/settings-updates";
import { WatchingPanel } from "@/components/settings-watching";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldTitle } from "@/components/ui/field";
import { cn } from "@/lib/utils";

type PanelProps = {
  onSaved: () => void;
};

function GeneralPanel() {
  return (
    <FieldGroup>
      <Field orientation="horizontal">
        <FieldContent>
          <FieldTitle>Theme</FieldTitle>
          <FieldDescription>Light, dark, or whatever the system is set to.</FieldDescription>
        </FieldContent>
        <ThemeToggle />
      </Field>
    </FieldGroup>
  );
}

export type SettingsCategory = "general" | "watching" | "notifications" | "updates" | "developer";

type Category = {
  id: SettingsCategory;
  label: string;
  Icon: LucideIcon;
  Panel: ComponentType<PanelProps>;
};

const CATEGORIES: Category[] = [
  { id: "general", label: "General", Icon: SettingsIcon, Panel: GeneralPanel },
  { id: "watching", label: "Watching", Icon: EyeIcon, Panel: WatchingPanel },
  { id: "notifications", label: "Notifications", Icon: BellIcon, Panel: NotificationsPanel },
  { id: "updates", label: "Updates", Icon: DownloadIcon, Panel: UpdatesPanel },
  { id: "developer", label: "Developer", Icon: CodeIcon, Panel: DeveloperPanel },
];

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  category?: SettingsCategory;
};

function categoryOf(id: SettingsCategory) {
  return CATEGORIES.find((c) => c.id === id) ?? CATEGORIES[0];
}

export function SettingsDialog({ open, onOpenChange, category = "general" }: Props) {
  const [active, setActive] = useState(() => categoryOf(category));
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setActive(categoryOf(category));
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-(--size-dialog-height) w-(--size-dialog-wide) flex-col gap-0 overflow-hidden p-0 sm:max-w-(--size-dialog-max)">
        <DialogDescription className="sr-only">Preferences for the app.</DialogDescription>

        <div className="flex min-h-0 flex-1">
          <nav aria-label="Settings categories" className="flex w-48 shrink-0 flex-col gap-1 border-r bg-sidebar p-2">
            <DialogTitle className="px-2 pt-1 pb-2 eyebrow text-3xs/normal font-normal">Settings</DialogTitle>
            {CATEGORIES.map((category) => (
              <Button
                key={category.id}
                variant="ghost"
                size="sm"
                aria-current={category.id === active.id ? "page" : undefined}
                className={cn(
                  "w-full justify-start gap-2 font-normal",
                  category.id === active.id && "bg-sidebar-accent font-medium",
                )}
                onClick={() => setActive(category)}
              >
                <category.Icon />
                {category.label}
              </Button>
            ))}
          </nav>

          <section className="flex min-w-0 flex-1 flex-col gap-4 overflow-y-auto p-6">
            <h2 className="text-base font-semibold">{active.label}</h2>
            <active.Panel onSaved={() => onOpenChange(false)} />
          </section>
        </div>
      </DialogContent>
    </Dialog>
  );
}
