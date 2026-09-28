import type { ReactNode } from "react";
import { ArrowLeftIcon, ArrowRightIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { isMac } from "@/lib/platform";

export type HistoryControls = {
  canGoBack: boolean;
  canGoForward: boolean;
  onBack: () => void;
  onForward: () => void;
};

const shortcuts = isMac ? { back: "⌘[", forward: "⌘]" } : { back: "Alt+←", forward: "Alt+→" };

function HistoryButton({
  label,
  shortcut,
  disabled,
  onClick,
  children,
}: {
  label: string;
  shortcut: string;
  disabled: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">
          <Button
            variant="ghost"
            size="icon"
            className="size-7"
            aria-label={label}
            disabled={disabled}
            onClick={onClick}
          >
            {children}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        {label} <Kbd>{shortcut}</Kbd>
      </TooltipContent>
    </Tooltip>
  );
}

function NavigationButtons({ canGoBack, canGoForward, onBack, onForward }: HistoryControls) {
  return (
    <>
      <SidebarTrigger />
      <HistoryButton label="Go back" shortcut={shortcuts.back} disabled={!canGoBack} onClick={onBack}>
        <ArrowLeftIcon />
      </HistoryButton>
      <HistoryButton label="Go forward" shortcut={shortcuts.forward} disabled={!canGoForward} onClick={onForward}>
        <ArrowRightIcon />
      </HistoryButton>
    </>
  );
}

export function AppHeader(history: HistoryControls) {
  if (isMac) return null;
  return (
    <header className="flex h-10 shrink-0 items-center gap-0.5 px-3">
      <NavigationButtons {...history} />
    </header>
  );
}

export function TitlebarNav(history: HistoryControls) {
  if (!isMac) return null;
  return (
    <div
      data-slot="titlebar-nav"
      className="fixed top-[calc((var(--titlebar-height)-(--spacing(7)))/2)] left-20 z-20 flex items-center gap-0.5"
    >
      <NavigationButtons {...history} />
    </div>
  );
}
