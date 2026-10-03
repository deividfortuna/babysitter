import type { MouseEvent, ReactNode } from "react";
import { ArrowLeftIcon, ArrowRightIcon, MenuIcon } from "lucide-react";
import { Tip } from "@/components/tip";
import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { historyShortcuts } from "@/hooks/use-history-shortcuts";
import type { HistoryControls } from "@/hooks/use-view-history";
import { bridge } from "@/lib/bridge";
import { isMac, isWindows } from "@/lib/platform";

function sidebarShortcut(): string {
  return isMac ? "⌘B" : "Ctrl+B";
}

function TipLabel({ label, shortcut }: { label: string; shortcut: string }) {
  return (
    <>
      {label} <Kbd>{shortcut}</Kbd>
    </>
  );
}

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
    <Tip side="bottom" label={<TipLabel label={label} shortcut={shortcut} />}>
      <span className="inline-flex">
        <Button variant="ghost" size="icon" className="size-7" aria-label={label} disabled={disabled} onClick={onClick}>
          {children}
        </Button>
      </span>
    </Tip>
  );
}

function popupMenuUnder(event: MouseEvent<HTMLButtonElement>) {
  const { left, bottom } = event.currentTarget.getBoundingClientRect();
  bridge.app.popupMenu({ x: left, y: bottom });
}

function MenuButton() {
  if (!isWindows) return null;
  return (
    <Tip side="bottom" label="Menu">
      <Button variant="ghost" size="icon" className="size-7" aria-label="Menu" onClick={popupMenuUnder}>
        <MenuIcon />
      </Button>
    </Tip>
  );
}

function NavigationButtons({ canGoBack, canGoForward, onBack, onForward }: HistoryControls) {
  const shortcuts = historyShortcuts();
  return (
    <>
      <MenuButton />
      <Tip side="bottom" label={<TipLabel label="Toggle sidebar" shortcut={sidebarShortcut()} />}>
        <SidebarTrigger />
      </Tip>
      <HistoryButton label="Go back" shortcut={shortcuts.back.label} disabled={!canGoBack} onClick={onBack}>
        <ArrowLeftIcon />
      </HistoryButton>
      <HistoryButton label="Go forward" shortcut={shortcuts.forward.label} disabled={!canGoForward} onClick={onForward}>
        <ArrowRightIcon />
      </HistoryButton>
    </>
  );
}

export function TitlebarNav(history: HistoryControls) {
  return (
    <div
      data-slot="titlebar-nav"
      className="fixed top-titlebar-button-top left-titlebar-nav-left z-20 flex w-titlebar-nav-width items-center justify-between"
    >
      <NavigationButtons {...history} />
    </div>
  );
}
