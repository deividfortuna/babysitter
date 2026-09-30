import { useState } from "react";
import { CheckIcon, CircleAlertIcon, CopyIcon, Undo2Icon } from "lucide-react";
import type { Watch } from "@/hooks/useWatches";
import { useCopy } from "@/hooks/useCopy";
import { HandbackError, useHandback } from "@/hooks/useHandback";
import { HandbackDialog, type AuthorWork } from "@/components/handback-dialog";
import { Meta } from "@/components/status-badges";
import { Tip } from "@/components/tip";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { watchLabel } from "@/lib/watch-status";

function CopyRow({ label, value }: { label: string; value: string }) {
  const { copied, copy } = useCopy();
  return (
    <div className="grid grid-cols-[72px_1fr_24px] items-center gap-2">
      <Meta>{label}</Meta>
      <code className="truncate font-mono text-xs" title={value}>
        {value}
      </code>
      <Tip label={copied ? "Copied" : `Copy ${label}`}>
        <Button
          type="button"
          variant="ghost"
          size="icon-xs"
          aria-label={`Copy ${label}`}
          className="text-muted-foreground"
          onClick={() => void copy(value)}
        >
          {copied ? <CheckIcon /> : <CopyIcon />}
        </Button>
      </Tip>
    </div>
  );
}

function refusalOf(error: Error | null): HandbackError | null {
  return error instanceof HandbackError ? error : null;
}

export function TakenOverPanel({ watch }: { watch: Watch }) {
  const handback = useHandback();
  const [work, setWork] = useState<AuthorWork | null>(null);
  const refusal = refusalOf(handback.error);
  const authorPid = refusal?.authorPid ?? null;
  const otherError = handback.error && !refusal?.asksToConfirm && authorPid === null ? handback.error : null;

  function handBack(confirm?: boolean) {
    handback.mutate(
      { id: watch.id, confirm },
      {
        onSuccess: () => setWork(null),
        onError: (error) => {
          const asked = refusalOf(error);
          if (asked?.asksToConfirm) setWork({ commits: asked.commits, files: asked.files });
        },
      },
    );
  }

  return (
    <div className="flex flex-col gap-3 rounded-lg border bg-muted/50 px-4 py-3.5">
      <p className="text-sm text-foreground/80">
        The session is with you in your terminal. The daemon keeps polling, and holds what it finds for when you hand
        back. It types nothing, pushes nothing and posts nothing.
      </p>
      <div className="flex flex-col gap-1.5">
        <CopyRow label="worktree" value={watch.worktreeDir} />
        <CopyRow label="branch" value={watch.workBranch} />
        <CopyRow label="push" value={`git push origin HEAD:${watch.headRef}`} />
      </div>
      {authorPid !== null ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>Your agent is still running{authorPid ? `, pid ${authorPid}` : ""}.</AlertTitle>
          <AlertDescription>
            Quit it in your terminal, then hand back. Two processes on one conversation corrupt it.
          </AlertDescription>
        </Alert>
      ) : null}
      {otherError && work === null ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{otherError.message}</AlertTitle>
        </Alert>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" size="sm" disabled={handback.isPending} onClick={() => handBack()}>
          {handback.isPending && work === null ? (
            <Spinner data-icon="inline-start" />
          ) : (
            <Undo2Icon data-icon="inline-start" />
          )}
          Hand back
        </Button>
        <Meta className="ml-1">or babysitter watch handback {watchLabel(watch)}</Meta>
      </div>
      {work ? (
        <HandbackDialog
          open
          onOpenChange={(open) => {
            if (!open) setWork(null);
          }}
          watch={watch}
          work={work}
          pending={handback.isPending}
          error={refusal?.asksToConfirm ? null : handback.error}
          onConfirm={() => handBack(true)}
        />
      ) : null}
    </div>
  );
}
