import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CopyIcon, PlayIcon } from "lucide-react";
import type { DaemonStatus } from "../../shared/daemon-status";
import type { Watch } from "@/hooks/useWatches";
import { useCopy } from "@/hooks/useCopy";
import { Meta } from "@/components/status-badges";
import { ViewHeader } from "@/components/view-header";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { bridge } from "@/lib/bridge";
import { watchListQueryKey } from "@/lib/query-keys";
import { relativeTime } from "@/lib/time";
import { checksWord } from "@/lib/watch-status";

export function DaemonDown({ status }: { status: DaemonStatus }) {
  const queryClient = useQueryClient();
  const [restarting, setRestarting] = useState(false);
  const { copied, copy } = useCopy();

  if (status.state === "starting") {
    return (
      <>
        <ViewHeader />
        <div className="flex flex-1 items-center justify-center gap-2 p-10 text-sm text-muted-foreground">
          <Spinner />
          Starting the daemon…
        </div>
      </>
    );
  }

  const lastKnown =
    queryClient.getQueryData<Watch[]>(watchListQueryKey("active")) ??
    queryClient.getQueryData<Watch[]>(watchListQueryKey("all"))?.filter((w) => w.status === "active") ??
    [];

  async function restart() {
    setRestarting(true);
    try {
      await bridge.daemon.restart();
    } finally {
      setRestarting(false);
    }
  }

  async function copyLog() {
    if (status.details) await copy(status.details);
  }

  return (
    <>
      <ViewHeader />
      <div className="flex flex-col gap-3.5 p-4">
        <div className="flex flex-col gap-2.5 rounded-lg border border-attention bg-attention/5 p-3.5">
          <p className="text-base font-medium">The daemon is not running, so nothing is being watched.</p>
          <p className="text-sm text-foreground/80">
            Watches are stored and resume where they left off. No activity is replayed, and nothing that arrived is
            lost.
          </p>
          {status.message ? <p className="text-sm text-foreground/80">{status.message}</p> : null}
          {status.details ? (
            <pre className="max-h-48 overflow-auto rounded-md border border-dashed border-muted-foreground/60 bg-background p-2 font-mono text-2xs/relaxed whitespace-pre-wrap text-foreground/80">
              {status.details}
            </pre>
          ) : null}
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" disabled={restarting} onClick={() => void restart()}>
              {restarting ? <Spinner data-icon="inline-start" /> : <PlayIcon data-icon="inline-start" />}
              Start the daemon
            </Button>
            {status.details ? (
              <Button size="sm" variant="outline" onClick={() => void copyLog()}>
                <CopyIcon data-icon="inline-start" />
                {copied ? "Copied" : "Copy the log"}
              </Button>
            ) : null}
            <Meta className="ml-auto">or, in a terminal: babysitter daemon start</Meta>
          </div>
        </div>

        {lastKnown.length > 0 ? (
          <div className="flex flex-col gap-1.5">
            <span className="eyebrow">
              While it is down · {lastKnown.length} {lastKnown.length === 1 ? "watch" : "watches"}, last known state
            </span>
            {lastKnown.map((w) => (
              <div key={w.id} className="flex items-baseline gap-2 text-sm text-muted-foreground">
                <span className="truncate">
                  #{w.number} {w.title || w.headRef}
                </span>
                <Meta className="ml-auto shrink-0">
                  {w.lastPollAt ? `checked ${relativeTime(w.lastPollAt)}` : "not checked"} · {checksWord(w).label}
                </Meta>
              </div>
            ))}
          </div>
        ) : null}
      </div>
    </>
  );
}
