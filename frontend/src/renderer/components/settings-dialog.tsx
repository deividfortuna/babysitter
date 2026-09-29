import { useState, type ComponentType } from "react";
import {
  ActivityIcon,
  BellIcon,
  DownloadIcon,
  GitPullRequestIcon,
  PaletteIcon,
  ScrollTextIcon,
  SparkleIcon,
  type LucideIcon,
} from "lucide-react";
import { AppearancePanel } from "@/components/settings-appearance";
import { AgentPanel } from "@/components/settings-agent";
import { LogsPanel } from "@/components/settings-logs";
import { NotificationsPanel } from "@/components/settings-notifications";
import { SaveMark, SaveTracker, useSaveState } from "@/components/settings-page";
import { PollingPanel } from "@/components/settings-polling";
import { ReviewPanel } from "@/components/settings-review";
import { UpdatesPanel } from "@/components/settings-updates";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { useAppUpdate } from "@/hooks/useAppUpdate";
import { cn } from "@/lib/utils";

export type SettingsCategory = "appearance" | "notifications" | "updates" | "agent" | "review" | "polling" | "logs";

type Page = {
  id: SettingsCategory;
  label: string;
  description?: string;
  Icon: LucideIcon;
  Panel: ComponentType;
  fill?: boolean;
};

type Group = {
  label: string;
  pages: Page[];
};

const GROUPS: Group[] = [
  {
    label: "App",
    pages: [
      {
        id: "appearance",
        label: "Appearance",
        description: "How the app looks.",
        Icon: PaletteIcon,
        Panel: AppearancePanel,
      },
      { id: "notifications", label: "Notifications", Icon: BellIcon, Panel: NotificationsPanel },
      { id: "updates", label: "Updates", Icon: DownloadIcon, Panel: UpdatesPanel },
    ],
  },
  {
    label: "New watches",
    pages: [
      { id: "agent", label: "Agent", description: "Defaults of a new watch.", Icon: SparkleIcon, Panel: AgentPanel },
      {
        id: "review",
        label: "Review and merge",
        description: "Defaults every new watch inherits.",
        Icon: GitPullRequestIcon,
        Panel: ReviewPanel,
      },
    ],
  },
  {
    label: "Daemon",
    pages: [
      {
        id: "polling",
        label: "Polling",
        description: "How often the daemon asks GitHub. Faster finds changes sooner and spends more of the rate limit.",
        Icon: ActivityIcon,
        Panel: PollingPanel,
      },
      {
        id: "logs",
        label: "Logs",
        description: "What the daemon and the app recorded, newest at the end.",
        Icon: ScrollTextIcon,
        Panel: LogsPanel,
        fill: true,
      },
    ],
  },
];

const PAGES = GROUPS.flatMap((group) => group.pages);

function pageOf(id: SettingsCategory): Page {
  return PAGES.find((page) => page.id === id) ?? PAGES[0];
}

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  category?: SettingsCategory;
};

export function SettingsDialog({ open, onOpenChange, category = "appearance" }: Props) {
  const [active, setActive] = useState(() => pageOf(category));
  const [wasOpen, setWasOpen] = useState(open);
  const save = useSaveState();

  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setActive(pageOf(category));
  }

  function show(page: Page) {
    setActive(page);
    save.reset();
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) save.reset();
        onOpenChange(next);
      }}
    >
      <DialogContent className="flex h-(--size-dialog-height) w-(--size-dialog-wide) gap-0 overflow-hidden p-0 sm:max-w-(--size-dialog-max)">
        <DialogTitle className="sr-only">Settings</DialogTitle>
        <DialogDescription className="sr-only">Preferences for the app.</DialogDescription>

        <nav
          aria-label="Settings categories"
          className="flex w-52 shrink-0 flex-col gap-0.5 overflow-y-auto border-r bg-sidebar p-2"
        >
          {GROUPS.map((group) => (
            <div key={group.label} role="group" aria-label={group.label} className="flex flex-col gap-0.5">
              <h3 className="px-2.5 pt-3 pb-1 eyebrow text-3xs/normal font-normal">{group.label}</h3>
              {group.pages.map((page) => (
                <Button
                  key={page.id}
                  variant="ghost"
                  size="sm"
                  aria-current={page.id === active.id ? "page" : undefined}
                  className={cn(
                    "w-full justify-start gap-2 px-2.5 font-normal",
                    page.id === active.id && "bg-sidebar-accent font-medium shadow-xs hover:bg-sidebar-accent",
                  )}
                  onClick={() => show(page)}
                >
                  <page.Icon />
                  {page.label}
                </Button>
              ))}
            </div>
          ))}
          <AppVersion />
        </nav>

        <section
          aria-labelledby="settings-page-title"
          className={cn("flex min-w-0 flex-1 flex-col gap-4 p-6", active.fill ? "overflow-hidden" : "overflow-y-auto")}
        >
          <header className="flex items-start gap-3 pr-7">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <h2 id="settings-page-title" className="text-base font-semibold">
                {active.label}
              </h2>
              {active.description ? <p className="text-sm/snug text-muted-foreground">{active.description}</p> : null}
            </div>
            <SaveMark state={save.state} />
          </header>
          <SaveTracker track={save.track}>
            <div className={cn("flex flex-col", active.fill && "min-h-0 flex-1")}>
              <active.Panel key={active.id} />
            </div>
          </SaveTracker>
        </section>
      </DialogContent>
    </Dialog>
  );
}

function AppVersion() {
  const { status } = useAppUpdate();
  if (!status) return null;
  return (
    <p className="mt-auto px-2.5 pt-2 pb-1 font-mono text-2xs text-muted-foreground">
      babysitter {status.currentVersion}
    </p>
  );
}
