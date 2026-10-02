import { TriangleAlertIcon } from "lucide-react";
import { versionsDiffer, versionWarning } from "../../shared/connections";
import { useAppVersion } from "@/hooks/useAppVersion";
import { Tip } from "@/components/tip";
import { cn } from "@/lib/utils";

type Props = {
  name: string;
  version?: string;
  className?: string;
};

export function VersionWarning({ name, version, className }: Props) {
  const app = useAppVersion();
  if (!app || !version || !versionsDiffer(app, version)) return null;
  const label = versionWarning(name, version, app);
  return (
    <Tip label={<span className="block max-w-64">{label}</span>}>
      <span role="img" aria-label={label} className={cn("inline-flex shrink-0", className)}>
        <TriangleAlertIcon className="size-4 text-attention" />
      </span>
    </Tip>
  );
}
