import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type Props = {
  label: string;
  htmlFor?: string;
  description?: ReactNode;
  disabled?: boolean;
  className?: string;
  children?: ReactNode;
};

export function SettingRow({ label, htmlFor, description, disabled, className, children }: Props) {
  const Label = htmlFor ? "label" : "span";
  return (
    <div data-disabled={disabled || undefined} className={cn("group/row flex items-center gap-3", className)}>
      <div className="flex min-w-0 flex-1 flex-col gap-1 group-data-disabled/row:opacity-60">
        <Label htmlFor={htmlFor} className="text-sm font-medium">
          {label}
        </Label>
        {description ? <p className="text-body/4.5 text-muted-foreground">{description}</p> : null}
      </div>
      {children}
    </div>
  );
}
