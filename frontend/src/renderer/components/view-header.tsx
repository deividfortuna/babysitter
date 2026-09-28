import type { ComponentProps } from "react";
import { isMac } from "@/lib/platform";
import { cn } from "@/lib/utils";

export const clearsWindowButtonsWhenSidebarCollapses =
  "[[data-slot=sidebar][data-state=collapsed]~[data-slot=sidebar-inset]_&]:pl-titlebar-nav-clearance";

export function ViewHeader({ className, ...props }: ComponentProps<"header">) {
  return (
    <header
      data-slot="view-header"
      className={cn(
        "sticky top-0 z-10 box-content flex min-h-titlebar shrink-0 items-center gap-2.5 border-b bg-background px-5 transition-padding duration-200 ease-linear",
        isMac && ["app-drag", clearsWindowButtonsWhenSidebarCollapses],
        className,
      )}
      {...props}
    />
  );
}
