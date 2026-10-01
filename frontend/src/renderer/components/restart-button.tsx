import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import type { UpdateStatus } from "../../shared/updates";

type Props = {
  status: UpdateStatus;
  install: () => Promise<unknown>;
  label: string;
  className?: string;
};

export function RestartButton({ status, install, label, className }: Props) {
  const installing = status.state === "installing";
  return (
    <Button size="sm" className={className} disabled={installing} onClick={() => void install()}>
      {installing ? <Spinner data-icon="inline-start" /> : null}
      {installing ? "Restarting…" : label}
    </Button>
  );
}
