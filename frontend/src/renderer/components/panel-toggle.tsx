import { PanelRightDashedIcon, PanelRightIcon } from "lucide-react";
import { Tip } from "@/components/tip";
import { Button } from "@/components/ui/button";
import { isMac } from "@/lib/platform";
import { cn } from "@/lib/utils";

export const clearsPanelToggle = "pr-8.5";

type Props = { label: string; open: boolean; onOpenChange: (open: boolean) => void };

export function PanelToggle({ label, open, onOpenChange }: Props) {
  return (
    <Tip side="bottom" label={label}>
      <Button
        variant="ghost"
        size="icon"
        aria-label={label}
        aria-expanded={open}
        className={cn("fixed top-titlebar-button-top z-20 size-7", isMac ? "right-5" : "right-window-controls")}
        onClick={() => onOpenChange(!open)}
      >
        {open ? <PanelRightDashedIcon /> : <PanelRightIcon />}
      </Button>
    </Tip>
  );
}

export function PanelToggleSpace() {
  return (
    <span
      aria-hidden
      data-slot="panel-toggle-space"
      className={cn(
        "pointer-events-none fixed top-titlebar-button-top size-7 app-no-drag",
        isMac ? "right-5" : "right-window-controls",
      )}
    />
  );
}
