import { TriangleAlertIcon } from "lucide-react";
import { versionsDiffer, versionWarning } from "../../shared/connections";
import { useAppVersion } from "@/hooks/useAppVersion";
import { Tip } from "@/components/tip";
import { cn } from "@/lib/utils";

type Props = {
  name: string;
  version?: string;
  focusable?: boolean;
  className?: string;
};

export function VersionWarning({ name, version, focusable = false, className }: Props) {
  const app = useAppVersion();
  if (!app || !version || !versionsDiffer(app, version)) return null;
  const label = versionWarning(name, version, app);
  const icon = <TriangleAlertIcon className="size-4 text-attention" />;
  return (
    <Tip label={<span className="block max-w-64">{label}</span>}>
      {focusable ? (
        <button
          type="button"
          aria-label={label}
          className={cn(
            "inline-flex shrink-0 rounded-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50",
            className,
          )}
        >
          {icon}
        </button>
      ) : (
        <span role="img" aria-label={label} className={cn("inline-flex shrink-0", className)}>
          {icon}
        </span>
      )}
    </Tip>
  );
}
