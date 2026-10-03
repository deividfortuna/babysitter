import type { ReactNode } from "react";
import { ChevronDownIcon } from "lucide-react";
import { Tip } from "@/components/tip";
import { Button } from "@/components/ui/button";
import { ButtonGroup, ButtonGroupSeparator } from "@/components/ui/button-group";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";

export function SplitButton({
  variant,
  label,
  more,
  size = "sm",
  children,
}: {
  variant: "default" | "outline";
  label: string;
  more: ReactNode;
  size?: "sm" | "xs";
  children: ReactNode;
}) {
  const compact = size === "xs";
  return (
    <ButtonGroup>
      {children}
      {more ? (
        <>
          {variant === "default" ? <ButtonGroupSeparator className="bg-primary-foreground/30" /> : null}
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Tip label={label}>
                <Button
                  size={compact ? "icon-xs" : "icon-sm"}
                  variant={variant}
                  className={compact ? undefined : "w-7"}
                  aria-label={label}
                >
                  <ChevronDownIcon />
                </Button>
              </Tip>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">{more}</DropdownMenuContent>
          </DropdownMenu>
        </>
      ) : null}
    </ButtonGroup>
  );
}
