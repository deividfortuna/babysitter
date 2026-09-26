import iconDark from "../../../assets/icon-dark.svg";
import iconLight from "../../../assets/icon.svg";
import type { DaemonStatus } from "../../shared/daemon-status";
import { Spinner } from "@/components/ui/spinner";
import { isMac } from "@/lib/platform";
import { cn } from "@/lib/utils";

function stepText(status: DaemonStatus): string {
  return status.step === "environment" ? "Reading your shell environment" : "Starting the daemon";
}

export function LoadingScreen({ status }: { status: DaemonStatus }) {
  return (
    <main
      className={cn("flex h-svh flex-col items-center justify-center gap-5 bg-background p-10", isMac && "app-drag")}
    >
      <img src={iconLight} alt="" className="size-20 dark:hidden" />
      <img src={iconDark} alt="" className="hidden size-20 dark:block" />
      <h1 className="text-lg font-semibold tracking-tight">Babysitter</h1>
      <div role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner aria-hidden="true" />
        {stepText(status)}…
      </div>
    </main>
  );
}
