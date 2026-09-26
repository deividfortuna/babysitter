import { CopyIcon } from "lucide-react";
import type { Watch } from "@/hooks/useWatches";
import { useCopy } from "@/hooks/useCopy";
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
import { Input } from "@/components/ui/input";
import { watchLabel } from "@/lib/watch-status";

export function takeoverCommand(watch: Watch): string {
  return `babysitter watch takeover ${watchLabel(watch)}`;
}

function sessionEnds(watch: Watch): string {
  if (!watch.pendingProposal) return "The session of the daemon ends.";
  return `The session of the daemon ends, and proposal ${watch.pendingProposal} is declined. Nothing of it goes out.`;
}

type Props = { open: boolean; onOpenChange: (open: boolean) => void; watch: Watch };

export function TakeoverDialog({ open, onOpenChange, watch }: Props) {
  const { copied, copy } = useCopy();
  const command = takeoverCommand(watch);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-(--size-dialog) sm:max-w-(--size-dialog-max)">
        <DialogHeader>
          <DialogTitle>Continue in your terminal</DialogTitle>
          <DialogDescription>
            Run this in a terminal to take the session of {watchLabel(watch)} and keep the conversation. Hand it back
            from here when you are done.
          </DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2">
          <Input readOnly aria-label="Takeover command" value={command} className="bg-muted font-mono text-xs" />
          <Button type="button" variant="outline" onClick={() => void copy(command)}>
            <CopyIcon data-icon="inline-start" />
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="eyebrow">When you run it</span>
          <ul className="flex list-disc flex-col gap-1 pl-5 text-sm text-foreground/80">
            <li>{sessionEnds(watch)}</li>
            <li>
              Your terminal opens the same conversation in the worktree, with no rules: it can push, and it runs any
              tool.
            </li>
            <li>The daemon keeps polling, but types, pushes and posts nothing until you hand back.</li>
            <li>
              Add <code className="font-mono text-xs">--shell</code> for a shell in the worktree instead of the agent.
            </li>
          </ul>
        </div>
        <DialogFooter className="sm:justify-start">
          <DialogClose asChild>
            <Button type="button">Done</Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
