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
        className={cn("absolute top-titlebar-button-top z-20 size-7", isMac ? "right-5" : "right-window-controls")}
        onClick={() => onOpenChange(!open)}
      >
        {open ? <PanelRightDashedIcon /> : <PanelRightIcon />}
      </Button>
    </Tip>
  );
}
