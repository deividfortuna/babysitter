import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

type Props = {
  label: string;
  htmlFor?: string;
  description: ReactNode;
  className?: string;
  children: ReactNode;
};

export function SettingRow({ label, htmlFor, description, className, children }: Props) {
  const Label = htmlFor ? "label" : "span";
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <Label htmlFor={htmlFor} className="text-sm font-medium">
          {label}
        </Label>
        <p className="text-body/4.5 text-muted-foreground">{description}</p>
      </div>
      {children}
    </div>
  );
}
