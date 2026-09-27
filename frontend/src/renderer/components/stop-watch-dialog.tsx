import { useState } from "react";
import { CircleAlertIcon, TriangleAlertIcon } from "lucide-react";
import { useProposal } from "@/hooks/useProposals";
import { useStopWatch, type Watch } from "@/hooks/useWatches";
import { count, joinAnd } from "@/components/proposal-dialogs";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { isSelfWatch, watchLabel } from "@/lib/watch-status";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: Watch;
  onStopped: (watch: Watch) => void;
};

function DeclinedAlert({ watch, number }: { watch: Watch; number: number }) {
  const detail = useProposal(watch.id, number);
  const commits = detail.data?.commits?.length ?? 0;
  const replies = detail.data?.replies?.length ?? 0;
  const parts = [
    commits > 0 ? count(commits, "commit") : "",
    replies > 0 ? count(replies, "reply", "replies") : "",
  ].filter(Boolean);
  return (
    <Alert>
      <TriangleAlertIcon />
      <AlertTitle>Proposal {number} is declined.</AlertTitle>
      <AlertDescription>
        {parts.length > 0 ? `Its ${joinAnd(parts)} never go out.` : "Its work never goes out."} With the worktree kept,
        the commits stay on its branch with nothing pointing at them.
      </AlertDescription>
    </Alert>
  );
}

function worktreeLine(dir: string, keep: boolean): string {
  if (keep) return `The worktree stays at ${dir}. Delete it yourself when you are done with it.`;
  return `The worktree at ${dir} and its babysitter branch are deleted. Your own checkout is untouched.`;
}

export function StopWatchDialog({ open, onOpenChange, watch, onStopped }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-(--size-dialog) sm:max-w-(--size-dialog-max)">
        <StopWatchForm
          watch={watch}
          onStopped={(stopped) => {
            onOpenChange(false);
            onStopped(stopped);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

function StopWatchForm({ watch, onStopped }: { watch: Watch; onStopped: (watch: Watch) => void }) {
  const [keepWorktree, setKeepWorktree] = useState<boolean | undefined>();
  const stop = useStopWatch();
  const chosen = keepWorktree ?? watch.keepWorktree;

  const submit = () => {
    stop.mutate({ id: watch.id, keepWorktree }, { onSuccess: onStopped });
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>Stop watching {watchLabel(watch)}</DialogTitle>
        <DialogDescription>
          The daemon stops checking the pull request and reports a summary. Nothing on GitHub changes.
        </DialogDescription>
      </DialogHeader>

      {watch.pendingProposal ? <DeclinedAlert watch={watch} number={watch.pendingProposal} /> : null}

      {isSelfWatch(watch) ? null : (
        <>
          <Field orientation="horizontal">
            <Checkbox id="keep-worktree" checked={chosen} onCheckedChange={(v) => setKeepWorktree(v === true)} />
            <FieldLabel htmlFor="keep-worktree" className="font-normal">
              Keep the worktree on disk
            </FieldLabel>
          </Field>
          <FieldDescription>{worktreeLine(watch.worktreeDir || "its directory", chosen)}</FieldDescription>
        </>
      )}

      {stop.error ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{stop.error.message}</AlertTitle>
        </Alert>
      ) : null}

      <DialogFooter className="sm:justify-start">
        <Button type="button" onClick={submit} disabled={stop.isPending}>
          {stop.isPending ? <Spinner data-icon="inline-start" /> : null}
          Stop watching
        </Button>
        <DialogClose asChild>
          <Button type="button" variant="ghost">
            Cancel
          </Button>
        </DialogClose>
      </DialogFooter>
    </>
  );
}
