import type { ComponentProps, ReactNode } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

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
