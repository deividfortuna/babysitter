import type { DaemonStatus } from "../../shared/daemon-status";
import { AppIcon } from "@/components/app-icon";
import { Spinner } from "@/components/ui/spinner";

function stepText(status: DaemonStatus): string {
  return status.step === "environment" ? "Reading your shell environment" : "Starting the daemon";
}

export function LoadingScreen({ status }: { status: DaemonStatus }) {
  return (
    <main className="flex h-svh flex-col items-center justify-center gap-5 bg-background p-10 app-drag">
      <AppIcon className="size-20" />
      <h1 className="text-lg font-semibold tracking-tight">Babysitter</h1>
      <div role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner aria-hidden="true" />
        {stepText(status)}…
      </div>
    </main>
  );
}
