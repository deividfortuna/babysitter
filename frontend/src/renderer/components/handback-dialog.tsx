import { CircleAlertIcon } from "lucide-react";
import type { Watch } from "@/hooks/useWatches";
import type { WorkCommit } from "@/hooks/useHandback";
import { count } from "@/components/proposal-dialogs";
import { Alert, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";
import { shortSha } from "@/lib/time";
import { watchLabel } from "@/lib/watch-status";

export type AuthorWork = { commits: WorkCommit[]; files: string[] };

const listClass = "rounded-md border bg-muted/40 p-3 font-mono text-2xs/relaxed whitespace-pre-wrap";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: Watch;
  work: AuthorWork;
  pending: boolean;
  error: Error | null;
  onConfirm: () => void;
};

export function HandbackDialog({ open, onOpenChange, watch, work, pending, error, onConfirm }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-(--size-dialog) sm:max-w-(--size-dialog-max)">
        <DialogHeader className="pr-6">
          <DialogTitle>Hand back {watchLabel(watch)} with work that is not pushed?</DialogTitle>
          <DialogDescription>
            The agent continues from the worktree as it is. It sees this work, but nothing of it is on the pull request
            until a push: <code className="font-mono text-xs">git push origin HEAD:{watch.headRef}</code>.
          </DialogDescription>
        </DialogHeader>
        {work.commits.length > 0 ? (
          <div className="flex flex-col gap-1.5">
            <span className="eyebrow">{count(work.commits.length, "commit")} the pull request does not have</span>
            <pre className={listClass}>{work.commits.map((c) => `${shortSha(c.sha)} ${c.subject}`).join("\n")}</pre>
          </div>
        ) : null}
        {work.files.length > 0 ? (
          <div className="flex flex-col gap-1.5">
            <span className="eyebrow">{count(work.files.length, "file")} changed and not committed</span>
            <pre className={listClass}>{work.files.join("\n")}</pre>
          </div>
        ) : null}
        {error ? (
          <Alert variant="destructive">
            <CircleAlertIcon />
            <AlertTitle>{error.message}</AlertTitle>
          </Alert>
        ) : null}
        <DialogFooter className="sm:justify-start">
          <Button type="button" disabled={pending} onClick={onConfirm}>
            {pending ? <Spinner data-icon="inline-start" /> : null}
            Hand back
          </Button>
          <DialogClose asChild>
            <Button type="button" variant="ghost">
              Cancel
            </Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
