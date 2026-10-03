import type { ComponentProps, ReactNode } from "react";
import { Kbd } from "@/components/ui/kbd";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function TipLabel({ label, shortcut }: { label: string; shortcut: string }) {
  return (
    <>
      {label} <Kbd>{shortcut}</Kbd>
    </>
  );
}

type Side = ComponentProps<typeof TooltipContent>["side"];

type TipProps = ComponentProps<typeof TooltipTrigger> & { label: ReactNode; side?: Side };

export function Tip({ label, side, children, ...triggerProps }: TipProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild {...triggerProps}>
        {children}
      </TooltipTrigger>
      <TooltipContent side={side}>{label}</TooltipContent>
    </Tooltip>
  );
}
